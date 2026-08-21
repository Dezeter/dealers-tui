package dealer

import (
	"context"
	"strconv"

	"dealers/internal/chain/bindings"
	"dealers/internal/template"
)

// ProgStep is one compiled program step: an action plus the params it needs, with
// trade zones already resolved to area ids by the caller (main). Count, when > 0,
// is a per-UTC-day cap: the step may act at most Count times per day, then yields
// to the steps below it (0 = unlimited). Caps reset at midnight UTC.
type ProgStep struct {
	Action          string
	Drug            string
	BuyArea         uint8 // trade buy zone id
	SellArea        uint8 // trade sell zone id
	HeistDifficulty int8
	HeatAt          int8 // clear_stars: activate at this star level (0 = default 3)
	PayBail         bool // breakout: pay ETH bail when the free attempt is used up
	Count           int
}

// Program is the autopilot strategy that runs each dealer's template as a
// PRIORITY LIST: every tick it scans the steps top-to-bottom and acts on the
// first one that has something to do. Template order IS priority — a maintenance
// step (accept missions, clear heat, check in) placed above the greedy core step
// always takes its turn first, and the core (trade/pvp) runs with whatever is
// left. A step's Count, when > 0, is a per-UTC-day cap: once the step has acted
// Count times today it yields to the steps below it. There is no implicit safety
// head — escaping jail and clearing heat are ordinary steps the user orders in
// the program; a non-actionable step is simply skipped, so a jailed dealer's
// breakout step wins wherever it sits (its earlier steps go non-actionable).
type Program struct {
	steps   func(tokenID uint64) []ProgStep // resolve the dealer's compiled program (live)
	isAlly  func(uint64) bool
	payBail func() bool

	arb     *PvEArbitrage // reused trade behaviour; its live params are set per trade step
	cur     LiveParams    // per-step params fed to arb via its live() closure
	sold    *oncePerDay   // black-market loot sale tracker (pvp step, out of energy)
	heistCk *dailyLimiter // bounds heist-checkin retries so a dealer that can't afford the season entry advances
	caps    *dayCounter   // per-(dealer,step) daily action cap enforcing ProgStep.Count
	home    uint8         // fallback area to leave the black market for (Manhattan)
}

// NewProgram builds the priority-list program strategy. steps resolves a dealer's
// compiled program live each tick (nil/empty = idle). home is a normal area
// (Manhattan) a stranded raider travels to so it can leave the black market and
// act again.
func NewProgram(steps func(tokenID uint64) []ProgStep, isAlly func(uint64) bool, payBail func() bool, home uint8) *Program {
	if isAlly == nil {
		isAlly = func(uint64) bool { return false }
	}
	p := &Program{steps: steps, isAlly: isAlly, payBail: payBail, home: home, sold: newOncePerDay(), heistCk: newDailyLimiter(), caps: newDayCounter()}
	p.arb = NewPvEArbitrageCfg(StrategyConfig{
		IsAlly: isAlly, PayBail: payBail, HeistDifficulty: -1,
		Live: func() LiveParams { return p.cur },
	})
	return p
}

// Next runs the dealer's program for this tick as a priority list: it scans the
// steps top-to-bottom and acts on the first one that produces an action, skipping
// steps with nothing to do and steps that have hit today's Count cap.
func (p *Program) Next(ctx context.Context, r StrategyReader, d Decision) (Action, bool) {
	tokenID := d.Snap.TokenID
	steps := p.steps(tokenID)
	if len(steps) == 0 || d.Snap.State == nil {
		return Action{}, false
	}
	for i := range steps {
		s := steps[i]
		if s.Count > 0 && p.caps.reached(capKey(tokenID, i), s.Count) {
			continue // hit today's cap → let lower-priority steps take the tick
		}
		res := p.runStep(ctx, r, d, s)
		if !res.ok { // nothing to do → try the next step down
			continue
		}
		if res.rep && s.Count > 0 {
			p.caps.add(capKey(tokenID, i)) // count this action toward the daily cap
		}
		return res.action, true
	}
	return Action{}, false // whole program idle this tick
}

// capKey identifies one dealer's step for the daily Count cap. Keyed by step
// index: reordering a template mid-day can carry a count to the step now at that
// slot, which self-heals at the next UTC midnight.
func capKey(tokenID uint64, step int) string {
	return strconv.FormatUint(tokenID, 10) + ":" + strconv.Itoa(step)
}

// stepResult is one step's decision for the tick.
type stepResult struct {
	action Action
	ok     bool // produced an action to emit
	rep    bool // this action counts as one use toward the step's daily Count cap
}

// act builds an actionable result; rep marks it as counting toward Count.
func act(a Action, rep bool) stepResult { return stepResult{action: a, ok: true, rep: rep} }

// runStep executes one step's action, returning what to do this tick. Every step
// self-gates: a non-applicable step returns an idle result (ok=false), so the
// executor advances past it.
func (p *Program) runStep(ctx context.Context, r StrategyReader, d Decision, s ProgStep) stepResult {
	st := d.Snap.State
	tokenID := d.Snap.TokenID
	switch s.Action {
	case template.ActionBreakout:
		// Pay bail when the step opts in OR the global setting is on.
		bail := s.PayBail || (p.payBail != nil && p.payBail())
		if a, ok := jailbreakFirst(st, bail); ok {
			return act(a, true)
		}
	case template.ActionClearStars:
		if actionable(st) {
			if a, ok := posterAt(st, uint8(s.HeatAt)); ok {
				return act(a, true)
			}
		}
	case template.ActionHeistCheckIn:
		if actionable(st) {
			if a, ok := heistCheckInStep(ctx, r, tokenID, p.heistCk); ok {
				return act(a, true)
			}
		}
	case template.ActionMissions: // legacy combined (claim + accept)
		if actionable(st) {
			if a, ok := missionStep(ctx, r, tokenID); ok {
				return act(a, true)
			}
		}
	case template.ActionMissionsAccept:
		if actionable(st) {
			if a, ok := missionAcceptStep(ctx, r, tokenID); ok {
				return act(a, true)
			}
		}
	case template.ActionMissionsClaim:
		if actionable(st) {
			if a, ok := missionClaimStep(ctx, r, tokenID); ok {
				return act(a, true)
			}
		}
	case template.ActionMissionsFollow:
		if actionable(st) {
			if a, ok := missionFollowStep(ctx, r, d, p.isAlly, s.Drug); ok {
				return act(a, true)
			}
		}
	case template.ActionTrade:
		if actionable(st) {
			p.cur = LiveParams{BuyArea: s.BuyArea, SellArea: s.SellArea, Drug: s.Drug, HeistDifficulty: -1}
			if a, ok := p.arb.tradeCore(ctx, r, d); ok {
				return act(a, a.Kind == ActionPVE) // buys/sells count; travel doesn't
			}
		}
	case template.ActionPvP:
		if actionable(st) {
			return p.pvpStep(ctx, r, d, s)
		}
	case template.ActionHeist:
		if actionable(st) {
			return p.heistRun(ctx, r, d, s.HeistDifficulty)
		}
	}
	return stepResult{} // ok=false → advance
}

// pvpStep attacks a present non-ally target. Out of energy it liquidates loot in
// the black market ONLY if it has any (never a pointless trip), then idles where
// it is. With energy but NO target it deals drugs instead of idling — the same
// weed run the trade step uses (the step's own zones, or the Manhattan default) —
// so a raider whose program has no trade step still spends its energy, keeps
// circulating Manhattan↔Amsterdam to re-probe targets every tick, and leaves the
// black-market dead-end on its own (the trade route heads to the buy zone). These
// fallback trades DON'T count toward the raid Count (rep=false); the step advances
// only once energy is spent and any loot liquidated — exactly like the old raider
// that fell back to trading whenever no target was in range.
func (p *Program) pvpStep(ctx context.Context, r StrategyReader, d Decision, s ProgStep) stepResult {
	st := d.Snap.State
	tokenID := d.Snap.TokenID
	inBM := st.CurrentArea == bindings.BlackMarketArea
	if st.DailyAttemptsRemaining == 0 {
		if hasLoot(st) {
			if !inBM {
				return act(Action{Kind: ActionTravel, DestArea: bindings.BlackMarketArea}, false)
			}
			if a, ok := p.sellLootOnce(st, tokenID); ok {
				return act(a, false)
			}
		}
		return stepResult{} // nothing to sell / out of energy → idle in place
	}
	if a, ok := pvpAttackAction(ctx, r, st, tokenID, p.isAlly); ok {
		return act(a, true) // an attack counts toward Count
	}
	// Energy but no target here → deal drugs to stay productive. buildProgram already
	// resolves every step's zones to Manhattan/Amsterdam; fall back to home for a
	// hand-built step with no buy zone so the raider still heads somewhere real.
	buy := s.BuyArea
	if buy == 0 {
		buy = p.home
	}
	p.cur = LiveParams{BuyArea: buy, SellArea: s.SellArea, Drug: s.Drug, HeistDifficulty: -1}
	if a, ok := p.arb.tradeCore(ctx, r, d); ok {
		return act(a, false) // fallback trade/haul — doesn't count toward the raid Count
	}
	return stepResult{} // truly nothing to do → advance to the next step
}

// hasLoot reports whether the dealer holds any drugs (raid loot to liquidate).
func hasLoot(st *bindings.FullDealerState) bool {
	for i := range st.DrugBalances {
		if b := &st.DrugBalances[i]; b.Balance != nil && b.Balance.Sign() > 0 {
			return true
		}
	}
	return false
}

// heistRun drives one heist run to a clean bank: start at the difficulty, push to
// the first cashable stage (≥2) and cash out. rep is marked only on the cash-out
// (a completed run), so Count counts finished runs; the step stays active across
// the multi-stage run so advancing never abandons it mid-run.
func (p *Program) heistRun(ctx context.Context, r StrategyReader, d Decision, difficulty int8) stepResult {
	st := d.Snap.State
	tokenID := d.Snap.TokenID
	hid, err := r.ActiveHeist(ctx, tokenID)
	if err != nil {
		return stepResult{}
	}
	if hid != 0 {
		h, err := r.GetHeist(ctx, hid)
		if err != nil {
			return stepResult{}
		}
		switch bindings.HeistStatus(h.Status) {
		case bindings.HeistPreStage:
			return act(Action{Kind: ActionHeistStage, HeistID: hid}, false)
		case bindings.HeistRevealedWin, bindings.HeistSetback:
			if h.CurrentStage >= heistMinCashStage {
				return act(Action{Kind: ActionHeistCashOut, HeistID: hid}, true) // completed run
			}
			if bindings.HeistStatus(h.Status) == bindings.HeistRevealedWin {
				return act(Action{Kind: ActionHeistStage, HeistID: hid}, false)
			}
			return stepResult{} // setback below the cashable stage — leave it
		default:
			return stepResult{} // COMMITTED (skipped as pending) or terminal
		}
	}
	if st.DailyAttemptsRemaining > 0 {
		if diff, ok := heistDifficultyFor(st, difficulty); ok {
			return act(Action{Kind: ActionStartHeist, HeistFamily: bindings.FamilyCash, HeistDifficulty: diff}, false)
		}
	}
	return stepResult{}
}

// sellLootOnce emits one black-market sale per looted drug (balance > 0), at most
// once per drug per day so a non-sellable balance can't spin.
func (p *Program) sellLootOnce(st *bindings.FullDealerState, tokenID uint64) (Action, bool) {
	day := utcDay()
	for i := range st.DrugBalances {
		b := &st.DrugBalances[i]
		if b.Balance == nil || b.Balance.Sign() <= 0 || b.DrugID == nil {
			continue
		}
		if p.sold.try(sellKey(tokenID, b.DrugID.Uint64()), day) {
			return Action{Kind: ActionSellDrop, DrugID: b.DrugID.Uint64(), Amount: b.Balance.Uint64()}, true
		}
	}
	return Action{}, false
}
