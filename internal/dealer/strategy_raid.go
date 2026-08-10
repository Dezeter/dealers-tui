package dealer

import (
	"context"
	"strconv"

	"dealers/internal/chain/bindings"
)

// targetPageLimit bounds the potential-targets page we scan per tick.
const targetPageLimit = 50

// PvPRaider attacks every attackable non-ally dealer in the current area. When
// no target is present it falls back to the PvE weed run, keeping the dealer
// productively circulating (re-probing PvP with no idle gas burn); out of energy
// it retreats to the Black Market and sells its loot. It runs as the "core" step
// of a configurable pipeline (stepRunner).
type PvPRaider struct {
	IsAlly func(tokenID uint64) bool // do-not-attack predicate

	sold *oncePerDay   // one sell attempt per looted drug per dealer per day
	pve  *PvEArbitrage // supplies the trade core (no-target fallback) + stake cache
	run  *stepRunner
}

// NewPvPRaider builds the raider over the two free zones with default params —
// the compatibility constructor. New callers use NewPvPRaiderCfg.
func NewPvPRaider(manhattan, amsterdam uint8, isAlly func(uint64) bool, payBail func() bool, recipe func() []string) *PvPRaider {
	return NewPvPRaiderCfg(StrategyConfig{
		BuyArea: manhattan, SellArea: amsterdam,
		IsAlly: isAlly, PayBail: payBail, Recipe: recipe, HeistDifficulty: -1,
	})
}

// NewPvPRaiderCfg builds the raider from a template config. Its no-target fallback
// trade run inherits the same buy/sell/drug params.
func NewPvPRaiderCfg(cfg StrategyConfig) *PvPRaider {
	if cfg.IsAlly == nil {
		cfg.IsAlly = func(uint64) bool { return false }
	}
	pve := NewPvEArbitrageCfg(cfg)
	s := &PvPRaider{IsAlly: cfg.IsAlly, sold: newOncePerDay(), pve: pve}
	s.run = &stepRunner{
		recipe: cfg.Recipe, stepMax: cfg.StepMax, isAlly: cfg.IsAlly, payBail: cfg.PayBail, primary: classPVP,
		heistDifficulty: cfg.HeistDifficulty, missionPriority: cfg.MissionPriority,
		heistCk: newDailyLimiter(), stepCount: newDayCounter(), core: s.raidCore,
	}
	return s
}

func (s *PvPRaider) Next(ctx context.Context, r StrategyReader, d Decision) (Action, bool) {
	return s.run.Next(ctx, r, d)
}

// raidCore is the "core" step: attack a non-ally target in the current area; out
// of energy retreat to the Black Market and liquidate loot; with no target, fall
// back to the trade core so the dealer stays productive and re-probes PvP.
func (s *PvPRaider) raidCore(ctx context.Context, r StrategyReader, d Decision) (Action, bool) {
	st := d.Snap.State
	tokenID := d.Snap.TokenID

	// Out of energy → Black Market, then liquidate the loot.
	if st.DailyAttemptsRemaining == 0 {
		if st.CurrentArea != bindings.BlackMarketArea {
			return Action{Kind: ActionTravel, DestArea: bindings.BlackMarketArea}, true
		}
		return s.sellLoot(st, tokenID)
	}

	// Attack the first non-ally, attackable target in this area (needs REP≥200,
	// which the contract enforces; getPotentialTargets simply returns none below it).
	if st.Reputation != nil && st.Reputation.Int64() >= PVPUnlockRep {
		if targets, _, err := r.PotentialTargets(ctx, tokenID, 0, targetPageLimit); err == nil {
			for i := range targets {
				t := &targets[i]
				if t.TokenID == nil || s.IsAlly(t.TokenID.Uint64()) || !t.CanAttackNow {
					continue
				}
				return Action{Kind: ActionPVP, DefenderID: t.TokenID.Uint64()}, true
			}
		}
	}

	// No one to hit here → trade instead (moves the dealer, re-probing PvP next tick).
	return s.pve.tradeCore(ctx, r, d)
}

// sellLoot emits one black-market sale per looted drug (balance > 0), at most
// once per drug per day so a non-sellable balance can't spin. Returns idle once
// everything has had its shot.
func (s *PvPRaider) sellLoot(st *bindings.FullDealerState, tokenID uint64) (Action, bool) {
	day := utcDay()
	for i := range st.DrugBalances {
		b := &st.DrugBalances[i]
		if b.Balance == nil || b.Balance.Sign() <= 0 || b.DrugID == nil {
			continue
		}
		if s.sold.try(sellKey(tokenID, b.DrugID.Uint64()), day) {
			return Action{Kind: ActionSellDrop, DrugID: b.DrugID.Uint64(), Amount: b.Balance.Uint64()}, true
		}
	}
	return Action{}, false
}

func sellKey(tokenID, drugID uint64) string {
	return "sell:" + strconv.FormatUint(tokenID, 10) + ":" + strconv.FormatUint(drugID, 10)
}
