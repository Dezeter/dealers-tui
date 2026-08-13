package dealer

import (
	"context"

	"dealers/internal/chain/bindings"
	"dealers/internal/template"
)

// ProgStep is one compiled program step: an action plus the params it needs, with
// trade zones already resolved to area ids by the caller (main). Count is how many
// times to repeat the action before advancing (0 = until the step has nothing left
// to do — "до успеха").
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

// ProgState persists a dealer's position in its program (implemented by
// internal/progstate). Step is the current step index, Reps how many reps of it
// have completed.
type ProgState interface {
	Get(tokenID uint64) (step, reps int)
	Set(tokenID uint64, step, reps int) error
}

// Program is the autopilot strategy that runs each dealer's template as a
// SEQUENTIAL step program: it executes the current step, advancing to the next
// only when the step is satisfied (a fixed Count of actions, or "until it has
// nothing left to do"), looping back at the end. The per-dealer position is
// persisted (ProgState) so it resumes across restarts. There is no implicit
// safety head — escaping jail and clearing heat are ordinary steps the user
// places in the program; a non-actionable step is skipped within the same tick,
// so a jailed dealer reaches its breakout step wherever it sits.
type Program struct {
	steps   func(tokenID uint64) []ProgStep // resolve the dealer's compiled program (live)
	state   ProgState
	isAlly  func(uint64) bool
	payBail func() bool

	arb  *PvEArbitrage // reused trade behaviour; its live params are set per trade step
	cur  LiveParams    // per-step params fed to arb via its live() closure
	sold *oncePerDay   // black-market loot sale tracker (pvp step, out of energy)
	home uint8         // fallback area to leave the black market for (Manhattan)
}

// NewProgram builds the sequential program strategy. steps resolves a dealer's
// compiled program live each tick (nil/empty = idle); state persists progress.
// home is a normal area (Manhattan) a stranded raider travels to so it can leave
// the black market and act again.
func NewProgram(steps func(tokenID uint64) []ProgStep, state ProgState, isAlly func(uint64) bool, payBail func() bool, home uint8) *Program {
	if isAlly == nil {
		isAlly = func(uint64) bool { return false }
	}
	p := &Program{steps: steps, state: state, isAlly: isAlly, payBail: payBail, home: home, sold: newOncePerDay()}
	p.arb = NewPvEArbitrageCfg(StrategyConfig{
		IsAlly: isAlly, PayBail: payBail, HeistDifficulty: -1,
		Live: func() LiveParams { return p.cur },
	})
	return p
}

// Next runs the dealer's program for this tick. It scans from the persisted
// position, skipping steps with nothing to do (advancing past them), and acts on
// the first step that produces an action — persisting the new position.
func (p *Program) Next(ctx context.Context, r StrategyReader, d Decision) (Action, bool) {
	tokenID := d.Snap.TokenID
	steps := p.steps(tokenID)
	n := len(steps)
	if n == 0 || d.Snap.State == nil {
		return Action{}, false
	}
	step, reps := p.state.Get(tokenID)
	if step < 0 || step >= n {
		step, reps = 0, 0
	}
	for scanned := 0; scanned < n; scanned++ {
		s := steps[step]
		if s.Count > 0 && reps >= s.Count { // fixed count already met → advance
			step, reps = (step+1)%n, 0
			continue
		}
		res := p.runStep(ctx, r, d, s)
		if !res.ok { // nothing to do (goal met / blocked) → advance to the next step
			step, reps = (step+1)%n, 0
			continue
		}
		next, nreps := step, reps
		if res.rep {
			nreps++
		}
		if res.done || (s.Count > 0 && nreps >= s.Count) {
			next, nreps = (step+1)%n, 0
		}
		_ = p.state.Set(tokenID, next, nreps)
		return res.action, true
	}
	// Whole program idle this tick — persist the looped-back position and idle.
	_ = p.state.Set(tokenID, step, reps)
	return Action{}, false
}

// stepResult is one step's decision for the tick.
type stepResult struct {
	action Action
	ok     bool // produced an action to emit
	rep    bool // this action counts as one repetition toward the step's Count
	done   bool // the step is finished regardless of Count → advance next tick
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
			if a, ok := heistCheckInStep(ctx, r, tokenID, nil); ok {
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
			return p.pvpStep(ctx, r, d)
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
// it is. With energy but no target it yields to the next step — but if it's
// stranded in the black market (which has no targets), it first travels home so a
// later tick can act, instead of sitting there idle with energy to spend.
func (p *Program) pvpStep(ctx context.Context, r StrategyReader, d Decision) stepResult {
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
	// Energy but no target here. Escape the black-market dead-end so we can act.
	if inBM && p.home != 0 && p.home != bindings.BlackMarketArea {
		return act(Action{Kind: ActionTravel, DestArea: p.home}, false)
	}
	return stepResult{} // no target → advance to the next step
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
