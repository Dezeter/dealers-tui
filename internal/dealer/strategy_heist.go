package dealer

import (
	"context"

	"dealers/internal/chain/bindings"
)

// Heist difficulty gates/stakes — the shipped defaults (CHAIN_REFERENCE §heist):
// difficulty 0/1/2, each needs reputation ≥ repGate and stakes cashEntry $CASH.
// Used to pick the highest difficulty a dealer can actually start ("на максималки")
// without spinning on a rep/cash revert. If the owner ever retunes these, a start
// may revert (logged, harmless) — adjust here if so.
var heistGates = [3]struct{ rep, cash int64 }{
	{600, 600},    // difficulty 0
	{1500, 4000},  // difficulty 1
	{5500, 25000}, // difficulty 2 (max)
}

// heistMinCashStage is the earliest stage a run can cash out (detail canCashOut).
const heistMinCashStage uint8 = 2

// heistRunsPerDay caps heist starts per dealer per day. A typical stage/run
// mission finishes in a couple of runs, so this bounds the energy/$CASH heists
// take (and any reverting-start spin) while still completing over the week.
const heistRunsPerDay = 3

// heistMissionStep drives heists to complete an incomplete WEEKLY heist mission:
// it starts a run at the highest affordable difficulty, pushes to the first
// cashable stage (≥2) and banks it — a clean, terminating run that advances
// runs/stages/cash-outs. (Pushing to the absolute top instead spins when
// commitStage reverts at the difficulty's final stage — the "hung at stage 2"
// bug.) Once the weekly heist mission is complete it launches no new runs, but
// still banks any in-flight run. Only touches the heist reads when the board
// actually has a heist mission.
func heistMissionStep(ctx context.Context, r StrategyReader, d Decision, difficulty int8) (Action, bool) {
	st := d.Snap.State
	ms, err := r.MissionStatus(ctx, d.Snap.TokenID)
	if err != nil {
		return Action{}, false
	}
	// Any incomplete heist mission (daily or weekly) drives runs.
	hasHeistMission, incomplete := false, false
	for i := range ms {
		m := &ms[i]
		if !m.Mission.Enabled || !m.CheckedIn || !isHeistMetric(m.Mission.Metric) {
			continue
		}
		hasHeistMission = true
		if !m.Claimed && m.Progress < m.Mission.Target {
			incomplete = true
		}
	}
	if !hasHeistMission {
		return Action{}, false // no heist mission on the board → no heist reads
	}

	tokenID := d.Snap.TokenID
	hid, err := r.ActiveHeist(ctx, tokenID)
	if err != nil {
		return Action{}, false
	}
	if hid != 0 {
		h, err := r.GetHeist(ctx, hid)
		if err != nil {
			return Action{}, false
		}
		switch bindings.HeistStatus(h.Status) {
		case bindings.HeistPreStage:
			// Just started → commit the first stage.
			return Action{Kind: ActionHeistStage, HeistID: hid}, true
		case bindings.HeistRevealedWin, bindings.HeistSetback:
			// Bank as soon as the run is cashable (stage ≥ 2). This cleanly ENDS the
			// run — advancing runs/stages/cash-outs — and, crucially, avoids pushing
			// past the difficulty's top stage: there commitStage reverts (no further
			// stage) and the autopilot would spin, looking "hung at stage 2". Below
			// the cashable stage, commit one more to reach it.
			if h.CurrentStage >= heistMinCashStage {
				return Action{Kind: ActionHeistCashOut, HeistID: hid}, true
			}
			if bindings.HeistStatus(h.Status) == bindings.HeistRevealedWin {
				return Action{Kind: ActionHeistStage, HeistID: hid}, true
			}
			return Action{}, false // setback below the cashable stage — leave it
		default:
			// COMMITTED (a stage is mid commit-reveal; the autopilot skips pending
			// rounds anyway) or terminal (BUSTED/CASHED_OUT/ABANDONED) → leave it.
			return Action{}, false
		}
	}

	// No active heist: start a new run only while the mission is unfinished and the
	// dealer has an attempt to spend (a start costs one). The per-day run cap is
	// enforced by the stepRunner (recipe Max, default heistRunsPerDay) counting
	// ActionStartHeist emits — so heists can't eat all of a dealer's energy/$CASH
	// (or spin on a reverting start) and starve the deal missions the strategy
	// serves; runs spread across days instead.
	if incomplete && st != nil && st.DailyAttemptsRemaining > 0 {
		if diff, ok := heistDifficultyFor(st, difficulty); ok {
			return Action{Kind: ActionStartHeist, HeistFamily: bindings.FamilyCash, HeistDifficulty: diff}, true
		}
	}
	return Action{}, false
}

// heistDifficultyFor picks the run difficulty: a fixed 0..2 override when the
// dealer meets that tier's rep gate and can afford its $CASH stake (else it does
// NOT start — no silent downgrade), otherwise the highest affordable tier.
func heistDifficultyFor(st *bindings.FullDealerState, override int8) (uint8, bool) {
	if override >= 0 && int(override) < len(heistGates) {
		d := uint8(override)
		if canAffordHeist(st, d) {
			return d, true
		}
		return 0, false
	}
	return maxHeistDifficulty(st)
}

// canAffordHeist reports whether the dealer meets tier d's rep gate and $CASH stake.
func canAffordHeist(st *bindings.FullDealerState, d uint8) bool {
	rep, cash := int64(0), int64(0)
	if st.Reputation != nil {
		rep = st.Reputation.Int64()
	}
	if st.CashBalance != nil {
		cash = st.CashBalance.Int64()
	}
	return int(d) < len(heistGates) && rep >= heistGates[d].rep && cash >= heistGates[d].cash
}

func isHeistMetric(metric uint8) bool {
	switch metric {
	case bindings.MetricHeistRuns, bindings.MetricHeistStages, bindings.MetricHeistCashouts:
		return true
	}
	return false
}

// maxHeistDifficulty returns the highest difficulty (0..2) the dealer meets the
// reputation gate for and can afford the $CASH stake of, or ok=false if not even
// the cheapest is feasible.
func maxHeistDifficulty(st *bindings.FullDealerState) (uint8, bool) {
	for d := len(heistGates) - 1; d >= 0; d-- {
		if canAffordHeist(st, uint8(d)) {
			return uint8(d), true
		}
	}
	return 0, false
}
