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

// StrategyConfig configures a strategy instance from a template's Params + recipe
// (wired by main). Zero values fall back to strategy defaults, so a neutral config
// reproduces the classic weed Manhattan→Amsterdam run.
type StrategyConfig struct {
	BuyArea  uint8  // buy zone (default Manhattan)
	SellArea uint8  // sell zone (default Amsterdam)
	Drug     string // drug to trade ("" = weed)

	IsAlly  func(uint64) bool       // do-not-attack set (mission-driven PvP steering)
	PayBail func() bool             // live auto-bail setting (may be nil)
	Recipe  func() []string         // live ordered enabled step ids (nil → default)
	StepMax func(stepID string) int // live per-step daily action cap (nil → defaults)

	HeistDifficulty int8   // -1 = max affordable; 0..2 = fixed tier
	MissionPriority string // "" / "daily" = daily-first; "weekly" = weekly-first
}

// PvEArbitrage runs the drug run: stockpile the traded drug in the buy zone (up to
// ×3 the per-hustle stake cap), haul to the sell zone and sell in max-size lots,
// and when holdings run dry head back to restock. Out of energy it parks in the
// Black Market. It runs as the "core" step of a configurable pipeline (stepRunner).
type PvEArbitrage struct {
	Manhattan uint8  // buy area
	Amsterdam uint8  // sell area
	drug      string // drug name traded (default "weed")

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
	drug := cfg.Drug
	if drug == "" {
		drug = weedName
	}
	s := &PvEArbitrage{Manhattan: cfg.BuyArea, Amsterdam: cfg.SellArea, drug: drug, isAlly: cfg.IsAlly, payBail: cfg.PayBail, spMu: newStakeParamCache()}
	s.run = &stepRunner{
		recipe: cfg.Recipe, stepMax: cfg.StepMax, isAlly: cfg.IsAlly, payBail: cfg.PayBail, primary: classPVE,
		heistDifficulty: cfg.HeistDifficulty, missionPriority: cfg.MissionPriority,
		heistCk: newDailyLimiter(), stepCount: newDayCounter(), core: s.tradeCore,
	}
	return s
}

func (s *PvEArbitrage) Next(ctx context.Context, r StrategyReader, d Decision) (Action, bool) {
	return s.run.Next(ctx, r, d)
}

// tradeCore is the "core" step: the actual weed arbitrage (out-of-energy retreat +
// buy/sell/travel routing). It always finds something to do while the dealer has
// energy, so any recipe step placed AFTER it won't run.
func (s *PvEArbitrage) tradeCore(ctx context.Context, r StrategyReader, d Decision) (Action, bool) {
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
