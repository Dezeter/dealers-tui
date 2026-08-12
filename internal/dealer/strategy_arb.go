package dealer

import (
	"context"
	"math/big"

	"dealers/internal/chain/bindings"
)

// weedName is the drug this strategy trades (matched case-insensitively).
const weedName = "weed"

// stockpileMultiple is how many per-hustle max-buys to stockpile on Manhattan
// before hauling to Amsterdam — "×3 the max buyable" (user spec). Batching the
// buys amortises the travel and keeps the cash-at-risk bounded (never the whole
// bank), since the per-hustle cap is itself the rep-based stake limit.
const stockpileMultiple = 3

// LiveParams are the per-tick tunable parameters a strategy reads fresh each tick,
// so an in-UI template edit takes effect on the next autopilot tick with no
// restart. Areas are resolved ids (main resolves the template's zone names).
type LiveParams struct {
	BuyArea         uint8  // buy zone id
	SellArea        uint8  // sell zone id
	Drug            string // drug to trade ("" = weed)
	HeistDifficulty int8   // -1 = max affordable; 0..2 = fixed tier
	MissionPriority string // "" / "daily" = daily-first; "weekly" = weekly-first
}

// StrategyConfig configures a strategy instance from a template's Params + recipe
// (wired by main). Zero values fall back to strategy defaults, so a neutral config
// reproduces the classic weed Manhattan→Amsterdam run. Provide Live to read the
// params fresh each tick (in-UI edits apply live); otherwise the static BuyArea/
// SellArea/Drug/HeistDifficulty/MissionPriority are snapshotted once.
type StrategyConfig struct {
	BuyArea  uint8  // buy zone (default Manhattan)
	SellArea uint8  // sell zone (default Amsterdam)
	Drug     string // drug to trade ("" = weed)

	IsAlly  func(uint64) bool       // do-not-attack set (mission-driven PvP steering)
	PayBail func() bool             // live auto-bail setting (may be nil)
	Recipe  func() []string         // live ordered enabled step ids (nil → default)
	StepMax func(stepID string) int // live per-step daily action cap (nil → defaults)
	Live    func() LiveParams       // live tunables (nil → snapshot the fields below)

	HeistDifficulty int8   // -1 = max affordable; 0..2 = fixed tier
	MissionPriority string // "" / "daily" = daily-first; "weekly" = weekly-first
}

// resolveLive returns the config's live-params source, synthesising a constant one
// from the static fields when none is supplied (so plain constructors/tests keep
// working unchanged).
func resolveLive(cfg StrategyConfig) func() LiveParams {
	if cfg.Live != nil {
		return cfg.Live
	}
	lp := LiveParams{
		BuyArea: cfg.BuyArea, SellArea: cfg.SellArea, Drug: cfg.Drug,
		HeistDifficulty: cfg.HeistDifficulty, MissionPriority: cfg.MissionPriority,
	}
	return func() LiveParams { return lp }
}

// PvEArbitrage runs the drug run: stockpile the traded drug in the buy zone (up to
// ×3 the per-hustle stake cap), haul to the sell zone and sell in max-size lots,
// and when holdings run dry head back to restock. Out of energy it parks in the
// Black Market. It runs as the "core" step of a configurable pipeline (stepRunner).
type PvEArbitrage struct {
	Manhattan uint8  // buy area (refreshed from live() each tick)
	Amsterdam uint8  // sell area (refreshed from live() each tick)
	drug      string // drug name traded (refreshed from live() each tick; default "weed")

	live    func() LiveParams // per-tick tunables (route/drug); never nil
	isAlly  func(uint64) bool // do-not-attack set, for mission-driven PvP steering
	payBail func() bool       // live "pay bail after a failed breakout" setting (may be nil)
	spMu    *stakeParamCache
	amount  uint64 // per-hustle buy override (0 = use the stake cap)
	run     *stepRunner
}

// NewPvEArbitrage builds the classic weed run with default params — the
// compatibility constructor. New callers use NewPvEArbitrageCfg.
func NewPvEArbitrage(manhattan, amsterdam uint8, isAlly func(uint64) bool, payBail func() bool, recipe func() []string) *PvEArbitrage {
	return NewPvEArbitrageCfg(StrategyConfig{
		BuyArea: manhattan, SellArea: amsterdam,
		IsAlly: isAlly, PayBail: payBail, Recipe: recipe, HeistDifficulty: -1,
	})
}

// NewPvEArbitrageCfg builds the trade strategy from a template config.
func NewPvEArbitrageCfg(cfg StrategyConfig) *PvEArbitrage {
	live := resolveLive(cfg)
	s := &PvEArbitrage{live: live, isAlly: cfg.IsAlly, payBail: cfg.PayBail, spMu: newStakeParamCache()}
	s.refreshParams()
	s.run = &stepRunner{
		recipe: cfg.Recipe, stepMax: cfg.StepMax, isAlly: cfg.IsAlly, payBail: cfg.PayBail, primary: classPVE,
		live: live, heistCk: newDailyLimiter(), stepCount: newDayCounter(), core: s.tradeCore,
	}
	return s
}

// refreshParams pulls the live route/drug into the struct fields. Called at the
// top of tradeCore each tick; safe because the autopilot ticks sequentially (one
// goroutine) and every dealer on a template shares its params.
func (s *PvEArbitrage) refreshParams() {
	lp := s.live()
	s.Manhattan, s.Amsterdam = lp.BuyArea, lp.SellArea
	if s.drug = lp.Drug; s.drug == "" {
		s.drug = weedName
	}
}

func (s *PvEArbitrage) Next(ctx context.Context, r StrategyReader, d Decision) (Action, bool) {
	return s.run.Next(ctx, r, d)
}

// tradeCore is the "core" step: the actual weed arbitrage (out-of-energy retreat +
// buy/sell/travel routing). It always finds something to do while the dealer has
// energy, so any recipe step placed AFTER it won't run.
func (s *PvEArbitrage) tradeCore(ctx context.Context, r StrategyReader, d Decision) (Action, bool) {
	s.refreshParams() // pick up any live route/drug change from the template
	st := d.Snap.State
	tokenID := d.Snap.TokenID
	// Out of energy → retreat to the Black Market and stop.
	if st.DailyAttemptsRemaining == 0 {
		if st.CurrentArea != bindings.BlackMarketArea {
			return Action{Kind: ActionTravel, DestArea: bindings.BlackMarketArea}, true
		}
		return Action{}, false
	}

	holdings := drugHoldings(st.DrugBalances, s.drug)

	switch st.CurrentArea {
	case s.Manhattan:
		return s.onManhattan(ctx, r, st, tokenID, d.Area, holdings)
	case s.Amsterdam:
		return s.onAmsterdam(ctx, r, st, tokenID, d.Area, holdings)
	default:
		// Anywhere else (fresh start, wandered): head to the buy area to begin.
		return Action{Kind: ActionTravel, DestArea: s.Manhattan}, true
	}
}

// onManhattan buys weed toward the stockpile target, else hauls to Amsterdam.
func (s *PvEArbitrage) onManhattan(ctx context.Context, r StrategyReader, st *bindings.FullDealerState, tokenID uint64, market []bindings.AreaDrug, holdings uint64) (Action, bool) {
	weed, ok := findDrug(market, s.drug)
	if !ok || weed.BuyPrice == nil || weed.BuyPrice.Sign() <= 0 {
		// No weed to buy here — if we're already holding, go sell; else idle.
		if holdings > 0 {
			return Action{Kind: ActionTravel, DestArea: s.Amsterdam}, true
		}
		return Action{}, false
	}
	perHustle := s.perHustleCap(ctx, r, tokenID, st, weed.BuyPrice)
	target := perHustle * stockpileMultiple
	if perHustle > 0 && holdings < target {
		amount := perHustle
		if room := target - holdings; room < amount {
			amount = room
		}
		if afford := affordable(st.CashBalance, weed.BuyPrice); afford < amount {
			amount = afford
		}
		// Cap the cash-at-risk per buy to ≤ ⅓ of the current balance (user rule:
		// buy the most the stake cap allows, but never sink more than a third of
		// cash into one hustle). If even one unit exceeds a third, this yields 0
		// and the buy is skipped — a broke dealer won't grind 1-unit weed.
		if st.CashBalance != nil {
			third := new(big.Int).Div(st.CashBalance, big.NewInt(3))
			if cap3 := affordable(third, weed.BuyPrice); cap3 < amount {
				amount = cap3
			}
		}
		if amount > 0 {
			return Action{Kind: ActionPVE, Hustle: bindings.HustleBuy, DrugID: weed.DrugID.Uint64(), Amount: amount}, true
		}
	}
	// Stockpile full (or can't afford more) → haul to Amsterdam if we have any.
	if holdings > 0 {
		return Action{Kind: ActionTravel, DestArea: s.Amsterdam}, true
	}
	return Action{}, false
}

// onAmsterdam sells weed in max-size lots; when holdings run out, returns to
// Manhattan to restock (the "не хватает на MAX продажу → возвращается" rule,
// generalised to "sell down the stockpile, then go refill").
func (s *PvEArbitrage) onAmsterdam(ctx context.Context, r StrategyReader, st *bindings.FullDealerState, tokenID uint64, market []bindings.AreaDrug, holdings uint64) (Action, bool) {
	if holdings == 0 {
		return Action{Kind: ActionTravel, DestArea: s.Manhattan}, true
	}
	weed, ok := findDrug(market, s.drug)
	if !ok || weed.SellPrice == nil || weed.SellPrice.Sign() <= 0 {
		// Can't sell here after all — go back rather than sit on inventory.
		return Action{Kind: ActionTravel, DestArea: s.Manhattan}, true
	}
	amount := holdings
	if limit := s.perHustleCap(ctx, r, tokenID, st, weed.SellPrice); limit > 0 && limit < amount {
		amount = limit
	}
	if amount == 0 {
		return Action{Kind: ActionTravel, DestArea: s.Manhattan}, true
	}
	return Action{Kind: ActionPVE, Hustle: bindings.HustleSell, DrugID: weed.DrugID.Uint64(), Amount: amount}, true
}

// perHustleCap is the largest number of units tradeable in one hustle under the
// rep-based stake cap at the given unit price. Returns 0 when the cap can't be
// read (skip this tick) — never guesses, so it can't trigger an over-stake revert.
func (s *PvEArbitrage) perHustleCap(ctx context.Context, r StrategyReader, tokenID uint64, _ *bindings.FullDealerState, price *big.Int) uint64 {
	if s.amount > 0 {
		return s.amount
	}
	gs, err := r.GameState(ctx, tokenID)
	if err != nil || gs == nil {
		return 0
	}
	sp := s.spMu.get(ctx, r)
	if sp == nil {
		return 0
	}
	limit := MaxStake(gs, sp)
	if limit == nil {
		// No determinable rep cap (degenerate low-rep state): idle rather than
		// risk an over-stake revert. Real dealers with reputation have a cap.
		return 0
	}
	return MaxUnitsAtPrice(limit, price)
}

// affordable returns how many whole units at price the dealer can pay for.
func affordable(cash, price *big.Int) uint64 {
	if cash == nil || price == nil || price.Sign() <= 0 {
		return 0
	}
	return new(big.Int).Div(cash, price).Uint64()
}

// pveBuyUnits sizes a PvE buy the way the trade core does: the most the rep stake
// cap allows at this price, bounded by cash on hand and the ⅓-of-balance rule.
// Returns 0 when the cap can't be read or ⅓ of cash won't cover a single unit —
// so a broke or degenerate-low-rep dealer buys nothing rather than 1-unit
// grinding. Shared by the trade core and PvE mission steering.
func pveBuyUnits(ctx context.Context, r StrategyReader, tokenID uint64, cash, price *big.Int) uint64 {
	gs, err := r.GameState(ctx, tokenID)
	if err != nil || gs == nil {
		return 0
	}
	sp, err := r.StakeParams(ctx)
	if err != nil || sp == nil {
		return 0
	}
	limit := MaxStake(gs, sp)
	if limit == nil {
		return 0
	}
	amount := MaxUnitsAtPrice(limit, price)
	if afford := affordable(cash, price); afford < amount {
		amount = afford
	}
	if cash != nil {
		third := new(big.Int).Div(cash, big.NewInt(3))
		if c3 := affordable(third, price); c3 < amount {
			amount = c3
		}
	}
	return amount
}
