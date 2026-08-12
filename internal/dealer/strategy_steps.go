package dealer

import (
	"context"
	"strconv"
)

// Autopilot step ids. A recipe is an ordered list of these; the pipeline runs the
// enabled ones in order and the first to act wins the tick. "core" resolves to the
// dealer's assigned job (trade for a pve dealer, raid for a pvp dealer).
const (
	StepHeistCheckIn   = "heist_checkin"
	StepClearStars     = "clear_stars"
	StepMissions       = "missions"
	StepFollowMissions = "follow_missions"
	StepHeists         = "heists"
	StepCore           = "core"
)

// StepMeta describes a step for the recipe-editor UI.
type StepMeta struct {
	ID    string
	Label string
	Desc  string
}

// StepCatalog is every configurable step in its default order. The recipe editor
// renders this; the store persists the user's order + on/off over it.
var StepCatalog = []StepMeta{
	{StepHeistCheckIn, "Heist season check-in", "Daily heist-season check-in (auto-joins the season, pays the $CASH entry)."},
	{StepClearStars, "Clear wanted stars", "At 3★+ spend one free wanted-poster removal to shed heat and avoid jail."},
	{StepMissions, "Claim & accept missions", "Claim finished missions (daily→weekly) and accept the new epoch's missions."},
	{StepFollowMissions, "Follow missions", "Do what a mission needs when the core doesn't (e.g. a PvP mission on a trading dealer)."},
	{StepHeists, "Run heists", "Run heists to finish a weekly heist mission (capped per day)."},
	{StepCore, "Core — trade / raid", "The dealer's main job. It's greedy: steps AFTER it rarely run, so put things you want prioritised ABOVE it."},
}

// DefaultStepOrder returns the built-in ordered step ids (all enabled).
func DefaultStepOrder() []string {
	out := make([]string, len(StepCatalog))
	for i, m := range StepCatalog {
		out[i] = m.ID
	}
	return out
}

// stepFn is one step's behaviour.
type stepFn func(ctx context.Context, r StrategyReader, d Decision) (Action, bool)

// stepRunner drives the configurable pipeline: jail handling and the actionable
// gate are fixed (never user-removable), then the recipe's ordered enabled steps
// run in turn. The concrete strategy supplies the core (trade or raid). A template
// tunes it via recipe (step order), stepMax (per-step daily action caps), the
// heist difficulty override, and the mission-steering priority.
type stepRunner struct {
	recipe  func() []string         // ordered enabled step ids (live; nil/empty = default)
	stepMax func(stepID string) int // per-step daily action cap (live; nil = defaults)
	live    func() LiveParams       // per-tick tunables (heist difficulty, mission priority)
	isAlly  func(uint64) bool
	payBail func() bool
	primary metricClass // classPVE or classPVP — core kind + follow_missions

	heistCk   *dailyLimiter // bounds bank-heist check-in retries (see heistCheckInStep)
	stepCount *dayCounter   // per-step daily action budgets (recipe Max)
	core      stepFn
}

// params returns the live tunables (or a neutral default if unset).
func (sr *stepRunner) params() LiveParams {
	if sr.live != nil {
		return sr.live()
	}
	return LiveParams{HeistDifficulty: -1}
}

func (sr *stepRunner) order() []string {
	if sr.recipe != nil {
		if o := sr.recipe(); len(o) > 0 {
			return o
		}
	}
	return DefaultStepOrder()
}

func (sr *stepRunner) Next(ctx context.Context, r StrategyReader, d Decision) (Action, bool) {
	st := d.Snap.State
	// Fixed safety head: jail (free breakout, then bail if enabled) then the
	// actionable gate — these are never part of the configurable recipe.
	if a, ok := jailbreakFirst(st, sr.payBail != nil && sr.payBail()); ok {
		return a, true
	}
	if !actionable(st) {
		return Action{}, false
	}
	tokenID := d.Snap.TokenID
	lp := sr.params()
	for _, id := range sr.order() {
		var a Action
		var ok bool
		switch id {
		case StepHeistCheckIn:
			a, ok = heistCheckInStep(ctx, r, tokenID, sr.heistCk)
		case StepClearStars:
			a, ok = posterFirst(st)
		case StepMissions:
			a, ok = missionStep(ctx, r, tokenID)
		case StepFollowMissions:
			a, ok = missionSteer(ctx, r, d, sr.primary, sr.isAlly, lp.MissionPriority, lp.Drug)
		case StepHeists:
			a, ok = heistMissionStep(ctx, r, d, lp.HeistDifficulty)
		case StepCore:
			a, ok = sr.core(ctx, r, d)
		}
		if !ok {
			continue
		}
		if !sr.underCap(id, tokenID, a.Kind) {
			continue // this step spent its daily action budget → let the next step run
		}
		return a, true
	}
	return Action{}, false
}

// underCap enforces a step's per-day action budget: plumbing actions (travel,
// check-in, claim, sell-drop) are never capped, only the step's primary action
// (a deal/attack/heist-start/poster). Counting happens on emit — and because a
// commit-reveal action makes the dealer skip ticks until it resolves, one emit ≈
// one completed action.
func (sr *stepRunner) underCap(stepID string, tokenID uint64, k ActionKind) bool {
	if !countsForCap(sr.primary, stepID, k) {
		return true
	}
	cap := sr.capFor(stepID)
	if cap <= 0 || sr.stepCount == nil {
		return true // unbounded
	}
	return sr.stepCount.take(stepKey(stepID, tokenID), cap)
}

// capFor returns the effective per-day cap for a step: the template override if
// set (>0), else the step's built-in default (heists 3, everything else 0 =
// unbounded).
func (sr *stepRunner) capFor(stepID string) int {
	if sr.stepMax != nil {
		if m := sr.stepMax(stepID); m > 0 {
			return m
		}
	}
	if stepID == StepHeists {
		return heistRunsPerDay
	}
	return 0
}

// countsForCap reports whether an emitted action counts against a step's budget.
// For the core step only its primary action counts (a pvp core's attacks, not its
// no-target fallback trades); for other steps every game action counts.
func countsForCap(primary metricClass, stepID string, k ActionKind) bool {
	if stepID == StepCore {
		return k == primaryActionKind(primary)
	}
	switch k {
	case ActionPVE, ActionPVP, ActionStartHeist, ActionClearHeat:
		return true
	}
	return false
}

func stepKey(stepID string, tokenID uint64) string {
	return stepID + ":" + strconv.FormatUint(tokenID, 10)
}
