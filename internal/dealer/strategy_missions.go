package dealer

import (
	"context"

	"dealers/internal/chain/bindings"
)

// metricClass buckets a MetricType into the concrete activity that advances it.
// Score/heist/capstone/defend metrics are "passive" — normal play advances them
// (or they can't be forced by a single action), so the strategy handles them.
type metricClass int

const (
	classPassive metricClass = iota
	classPVE                 // a PvE deal advances it
	classPVP                 // a PvP attack advances it
	classAny                 // any game advances it (covered by every active strategy)
)

func classifyMetric(metric uint8) metricClass {
	switch metric {
	case bindings.MetricPVEWins, bindings.MetricPVEGames:
		return classPVE
	case bindings.MetricPVPAttackWins, bindings.MetricPVPGames:
		return classPVP
	case bindings.MetricAnyGames:
		return classAny
	default: // defend wins, heists, rep/infamy gain, missions-claimed capstone
		return classPassive
	}
}

// missionAcceptStep accepts (checks in) the current epoch's missions so their
// progress starts counting; idle once they're accepted. Best-effort.
func missionAcceptStep(ctx context.Context, r StrategyReader, tokenID uint64) (Action, bool) {
	ms, err := r.MissionStatus(ctx, tokenID)
	if err != nil || len(ms) == 0 {
		return Action{}, false
	}
	if bindings.NeedsCheckIn(ms) {
		return Action{Kind: ActionMissionCheckIn}, true
	}
	return Action{}, false
}

// missionClaimStep claims the first claimable finished mission (daily before
// weekly); idle when nothing is claimable. Best-effort.
func missionClaimStep(ctx context.Context, r StrategyReader, tokenID uint64) (Action, bool) {
	ms, err := r.MissionStatus(ctx, tokenID)
	if err != nil {
		return Action{}, false
	}
	if tpl, ok := bindings.FirstClaimable(ms); ok {
		return Action{Kind: ActionMissionClaim, TemplateID: tpl}, true
	}
	return Action{}, false
}

// missionFollowStep does the activity an incomplete, ACCEPTED mission needs — a
// PvE deal for PvE missions, a PvP attack for PvP missions (daily before weekly).
// It's opportunistic: no target / can't afford / wrong area → idle, so "if it
// completes it completes, if not it doesn't". Only accepted, unfinished missions
// are followed; passive metrics (rep/infamy/heist/defend) are left to other steps.
func missionFollowStep(ctx context.Context, r StrategyReader, d Decision, isAlly func(uint64) bool, drug string) (Action, bool) {
	st := d.Snap.State
	if st == nil || st.DailyAttemptsRemaining == 0 {
		return Action{}, false
	}
	ms, err := r.MissionStatus(ctx, d.Snap.TokenID)
	if err != nil {
		return Action{}, false
	}
	for _, cadence := range []uint8{bindings.CadenceDaily, bindings.CadenceWeekly} {
		for i := range ms {
			m := &ms[i]
			if m.Mission.Cadence != cadence || !m.Mission.Enabled || !m.CheckedIn || m.Claimed {
				continue
			}
			if m.Progress >= m.Mission.Target {
				continue
			}
			switch classifyMetric(m.Mission.Metric) {
			case classPVE:
				if a, ok := pveDealAction(ctx, r, st, d.Snap.TokenID, d.Area, drug); ok {
					return a, true
				}
			case classPVP:
				if a, ok := pvpAttackAction(ctx, r, st, d.Snap.TokenID, isAlly); ok {
					return a, true
				}
			}
		}
	}
	return Action{}, false
}

// missionSteer implements "follow the missions": if an incomplete mission needs
// an activity the running strategy (its primary class) does NOT itself produce,
// do that activity instead — the mission takes priority over the strategy.
// Priority is DAILY first, then WEEKLY, then the strategy: only once no daily
// mission needs steering do weekly missions get to override. Missions the
// strategy already advances (matching class, ANY_GAMES, or passive metrics) are
// left to it, and a completed mission stops steering. Returns no action when
// nothing needs steering or the off-strategy action isn't possible right now
// (afford/target/black-market), so the caller falls through to normal behaviour.
func missionSteer(ctx context.Context, r StrategyReader, d Decision, primary metricClass, isAlly func(uint64) bool, priority, drug string) (Action, bool) {
	st := d.Snap.State
	if st == nil || st.DailyAttemptsRemaining == 0 {
		return Action{}, false // PvE/PvP games cost a daily attempt
	}
	ms, err := r.MissionStatus(ctx, d.Snap.TokenID)
	if err != nil {
		return Action{}, false
	}
	// Steer in the template's cadence priority (default daily-first); both cadences
	// still take priority over the strategy.
	order := []uint8{bindings.CadenceDaily, bindings.CadenceWeekly}
	if priority == "weekly" {
		order = []uint8{bindings.CadenceWeekly, bindings.CadenceDaily}
	}
	for _, cadence := range order {
		if a, ok := steerForCadence(ctx, r, d, primary, isAlly, ms, cadence, drug); ok {
			return a, true
		}
	}
	return Action{}, false
}

// steerForCadence returns the off-strategy action for the first incomplete
// mission of the given cadence whose class the strategy doesn't cover.
func steerForCadence(ctx context.Context, r StrategyReader, d Decision, primary metricClass, isAlly func(uint64) bool, ms []bindings.MissionStatus, cadence uint8, drug string) (Action, bool) {
	st := d.Snap.State
	for i := range ms {
		m := &ms[i]
		if m.Mission.Cadence != cadence || !m.Mission.Enabled || !m.CheckedIn || m.Claimed {
			continue
		}
		if m.Progress >= m.Mission.Target {
			continue // done — leave the claim to missionStep, return to strategy
		}
		switch classifyMetric(m.Mission.Metric) {
		case classPVE:
			if primary != classPVE {
				if a, ok := pveDealAction(ctx, r, st, d.Snap.TokenID, d.Area, drug); ok {
					return a, true
				}
			}
		case classPVP:
			if primary != classPVP {
				if a, ok := pvpAttackAction(ctx, r, st, d.Snap.TokenID, isAlly); ok {
					return a, true
				}
			}
		}
	}
	return Action{}, false
}

// pveDealAction emits a PvE buy of the traded drug (weed) — or the cheapest
// buyable when it isn't sold in this area — to advance PVE_GAMES/PVE_WINS. It
// sizes the buy by the SAME rule as the trade core (the most the rep stake cap
// allows, capped at ⅓ of cash), so following a PvE mission is a real trade
// rather than a throwaway 1-unit buy. Skips the black market (commitGame
// reverts) and yields when nothing is affordable within the ⅓-cash bound.
func pveDealAction(ctx context.Context, r StrategyReader, st *bindings.FullDealerState, tokenID uint64, area []bindings.AreaDrug, drug string) (Action, bool) {
	if st.CurrentArea == bindings.BlackMarketArea {
		return Action{}, false
	}
	if drug == "" {
		drug = weedName
	}
	d, ok := findDrug(area, drug)
	if !ok || d.BuyPrice == nil || d.BuyPrice.Sign() <= 0 {
		d, ok = cheapestBuyable(area)
		if !ok || d.BuyPrice == nil {
			return Action{}, false
		}
	}
	amount := pveBuyUnits(ctx, r, tokenID, st.CashBalance, d.BuyPrice)
	if amount == 0 {
		return Action{}, false
	}
	return Action{Kind: ActionPVE, Hustle: bindings.HustleBuy, DrugID: d.DrugID.Uint64(), Amount: amount}, true
}

// pvpAttackAction attacks the first attackable non-ally in the current area — the
// action that advances PVP_GAMES/PVP_ATTACK_WINS. Opportunistic: if no target is
// present it yields (no roaming/gas-burn); the strategy's own movement re-checks
// other zones over time. Needs REP ≥ the PvP unlock.
func pvpAttackAction(ctx context.Context, r StrategyReader, st *bindings.FullDealerState, tokenID uint64, isAlly func(uint64) bool) (Action, bool) {
	if st.Reputation == nil || st.Reputation.Int64() < PVPUnlockRep {
		return Action{}, false
	}
	targets, _, err := r.PotentialTargets(ctx, tokenID, 0, targetPageLimit)
	if err != nil {
		return Action{}, false
	}
	for i := range targets {
		t := &targets[i]
		if t.TokenID == nil || !t.CanAttackNow {
			continue
		}
		if id := t.TokenID.Uint64(); isAlly == nil || !isAlly(id) {
			return Action{Kind: ActionPVP, DefenderID: id}, true
		}
	}
	return Action{}, false
}
