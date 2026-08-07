package dealer

import "context"

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
// run in turn. The concrete strategy supplies the core (trade or raid).
type stepRunner struct {
	recipe    func() []string // ordered enabled step ids (live; nil/empty = default)
	isAlly    func(uint64) bool
	payBail   func() bool
	primary   metricClass // classPVE or classPVP — for follow_missions
	tried     *oncePerDay
	heistRuns *dailyLimiter
	core      stepFn
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
	for _, id := range sr.order() {
		var a Action
		var ok bool
		switch id {
		case StepHeistCheckIn:
			a, ok = heistCheckInStep(ctx, r, tokenID)
		case StepClearStars:
			a, ok = posterFirst(st, tokenID, sr.tried)
		case StepMissions:
			a, ok = missionStep(ctx, r, tokenID)
		case StepFollowMissions:
			a, ok = missionSteer(ctx, r, d, sr.primary, sr.isAlly)
		case StepHeists:
			a, ok = heistMissionStep(ctx, r, d, sr.heistRuns)
		case StepCore:
			a, ok = sr.core(ctx, r, d)
		}
		if ok {
			return a, true
		}
	}
	return Action{}, false
}
