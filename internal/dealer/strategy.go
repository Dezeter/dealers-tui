package dealer

import (
	"context"
	"strings"
	"sync"
	"time"

	"dealers/internal/chain/bindings"
)

// ActionKind tags a strategy-emitted action.
type ActionKind int

const (
	ActionNone           ActionKind = iota
	ActionPVE                       // buy/sell hustle (commit-reveal)
	ActionClearHeat                 // wanted poster (ETH-free heat clear, commit-reveal)
	ActionTravel                    // move to Action.DestArea (single-tx)
	ActionPVP                       // attack Action.DefenderID (commit-reveal)
	ActionSellDrop                  // black-market sell Action.DrugID × Action.Amount (single-tx)
	ActionBreakout                  // free jail-break attempt (commit-reveal)
	ActionPayBail                   // pay ETH bail to leave jail (opt-in; single-tx)
	ActionMissionCheckIn            // snapshot mission baseline for the epoch (single-tx)
	ActionMissionClaim              // claim mission reward Action.TemplateID (single-tx)
	ActionHeistCheckIn              // bank-heist season check-in (auto-enters season; single-tx)
	ActionStartHeist                // start a heist run (Action.HeistFamily/HeistDifficulty; single-tx)
	ActionHeistStage                // commit the next heist stage of Action.HeistID (commit-reveal)
	ActionHeistCashOut              // bank an active heist Action.HeistID (single-tx)
)

// Action is a concrete instruction the Manager can Execute.
type Action struct {
	Kind       ActionKind
	Hustle     bindings.HustleType
	DrugID     uint64
	Amount     uint64
	DestArea   uint8  // ActionTravel destination area id
	DefenderID uint64 // ActionPVP target token id
	TemplateID uint64 // ActionMissionClaim mission template id

	HeistID         uint64               // ActionHeistStage / ActionHeistCashOut
	HeistFamily     bindings.HeistFamily // ActionStartHeist
	HeistDifficulty uint8                // ActionStartHeist
}

// Decision is the context a Strategy sees for one dealer on an autopilot tick.
// Snap and Area are pre-fetched by the autopilot; anything else a strategy needs
// (stake cap, PvP targets) it reads on demand through the StrategyReader.
type Decision struct {
	Snap Snapshot
	Area []bindings.AreaDrug // current area's market
}

// StrategyReader is the read surface a strategy may pull from mid-decision.
// *bindings.Reader satisfies it; tests supply a fake.
type StrategyReader interface {
	GameState(ctx context.Context, tokenID uint64) (*bindings.GameState, error)
	StakeParams(ctx context.Context) (*bindings.PVEStakeParams, error)
	PotentialTargets(ctx context.Context, attackerID, offset, limit uint64) ([]bindings.PVPTarget, uint64, error)
	MissionStatus(ctx context.Context, tokenID uint64) ([]bindings.MissionStatus, error)
	NeedsHeistCheckIn(ctx context.Context, tokenID uint64) (bool, error)
	ActiveHeist(ctx context.Context, tokenID uint64) (uint64, error)
	GetHeist(ctx context.Context, heistID uint64) (*bindings.DailyHeist, error)
}

// Strategy decides a dealer's next autopilot action (ADR-5). ok=false means
// "do nothing this tick". Autopilot is opt-in; the default is ManualStrategy.
type Strategy interface {
	Next(ctx context.Context, r StrategyReader, d Decision) (Action, bool)
}

// ManualStrategy never acts — the app is driven by hand.
type ManualStrategy struct{}

func (ManualStrategy) Next(context.Context, StrategyReader, Decision) (Action, bool) {
	return Action{}, false
}

// MultiStrategy routes each dealer to its own strategy by token id via a live
// Resolve func, so an in-UI strategy change takes effect on the next tick with no
// engine restart. Sharing a single strategy instance across ids is safe — the
// strategies key their per-dealer state (poster / loot trackers) by token id.
type MultiStrategy struct {
	Resolve func(tokenID uint64) Strategy
}

func (m MultiStrategy) Next(ctx context.Context, r StrategyReader, d Decision) (Action, bool) {
	if m.Resolve == nil {
		return Action{}, false
	}
	s := m.Resolve(d.Snap.TokenID)
	if s == nil {
		return Action{}, false
	}
	return s.Next(ctx, r, d)
}

// cheapestBuyable returns the lowest-buy-price available drug in an area.
func cheapestBuyable(area []bindings.AreaDrug) (bindings.AreaDrug, bool) {
	var best *bindings.AreaDrug
	for i := range area {
		d := &area[i]
		if !d.IsAvailable || d.BuyPrice == nil || d.BuyPrice.Sign() <= 0 {
			continue
		}
		if best == nil || d.BuyPrice.Cmp(best.BuyPrice) < 0 {
			best = d
		}
	}
	if best == nil {
		return bindings.AreaDrug{}, false
	}
	return *best, true
}

// --- shared helpers for the money-making strategies ---

// PVPUnlockRep is the reputation at which PvP attacks unlock (DealersPVP).
const PVPUnlockRep int64 = 200

// PosterHeatThreshold is "3 stars" — at or above it the strategies spend one
// attempt on a free wanted-poster removal before doing anything else.
const PosterHeatThreshold uint8 = 3

// actionable reports whether a dealer can take any game action right now.
func actionable(st *bindings.FullDealerState) bool {
	return st != nil && st.IsInitialized && !st.IsJailed && !st.IsInSafeHouse
}

// jailbreakFirst handles a jailed dealer: spend today's FREE breakout attempt
// first (~50%, no ETH/energy). If that attempt is already used up (a prior failed
// roll), fall back to paying ETH bail ONLY when payBail is on — otherwise wait
// for tomorrow's free attempt. The breakout commit creates a pending round, so
// the autopilot skips the dealer until it resolves; a failed roll leaves
// CanBreakoutToday false, which is what routes the next tick to bail.
func jailbreakFirst(st *bindings.FullDealerState, payBail bool) (Action, bool) {
	if st == nil || !st.IsInitialized || !st.IsJailed {
		return Action{}, false
	}
	if st.CanBreakoutToday {
		return Action{Kind: ActionBreakout}, true
	}
	if payBail {
		return Action{Kind: ActionPayBail}, true
	}
	return Action{}, false
}

// utcDay is the current UTC day number (matches the contract's timestamp/1 day).
func utcDay() int64 { return time.Now().UTC().Unix() / 86400 }

// findDrug returns the area-market entry whose name contains want (case-
// insensitive) and is currently available.
func findDrug(area []bindings.AreaDrug, want string) (bindings.AreaDrug, bool) {
	want = strings.ToLower(want)
	for i := range area {
		d := &area[i]
		if d.IsAvailable && strings.Contains(strings.ToLower(d.Name), want) {
			return *d, true
		}
	}
	return bindings.AreaDrug{}, false
}

// drugHoldings returns the dealer's balance of the drug whose name contains want.
func drugHoldings(balances []bindings.DrugBalance, want string) uint64 {
	want = strings.ToLower(want)
	for i := range balances {
		b := &balances[i]
		if b.Balance != nil && strings.Contains(strings.ToLower(b.Name), want) {
			return b.Balance.Uint64()
		}
	}
	return 0
}

// oncePerDay tracks a per-dealer action that should fire at most once per UTC day
// (the "poster first" step, and each loot sale in the black market). Safe for the
// autopilot's sequential ticks; the mutex only guards against future concurrency.
type oncePerDay struct {
	mu   sync.Mutex
	seen map[string]int64 // key → day it last fired
}

func newOncePerDay() *oncePerDay { return &oncePerDay{seen: map[string]int64{}} }

// try marks key for today and reports whether it had NOT already fired today.
func (o *oncePerDay) try(key string, day int64) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.seen[key] == day {
		return false
	}
	o.seen[key] = day
	return true
}

// dailyLimiter caps how many times a per-dealer action may fire per UTC day
// (used to bound heist starts so they can't monopolise a dealer's energy/$CASH
// and starve the deal missions the strategy serves). Resets each day.
type dailyLimiter struct {
	mu  sync.Mutex
	day int64
	n   map[uint64]int
}

func newDailyLimiter() *dailyLimiter { return &dailyLimiter{n: map[uint64]int{}} }

// take reports whether tokenID is under today's cap and, if so, counts one use.
func (l *dailyLimiter) take(tokenID uint64, cap int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if today := utcDay(); today != l.day {
		l.day, l.n = today, map[uint64]int{}
	}
	if l.n[tokenID] >= cap {
		return false
	}
	l.n[tokenID]++
	return true
}

// posterFirst returns the free heat-clear action while the dealer is at/above the
// 3-star threshold and has an attempt to spend. It retries until the heat is
// actually cleared (each poster costs a daily attempt, so it can't loop forever)
// — the step's per-day cap (recipe Max) bounds it further if the template sets
// one. Between attempts the commit-reveal round makes the dealer skip ticks, so
// the retries are naturally spaced.
func posterFirst(st *bindings.FullDealerState) (Action, bool) {
	return posterAt(st, PosterHeatThreshold)
}

// posterAt is posterFirst with a caller-chosen star threshold (0 = the default
// PosterHeatThreshold). Used by the program's clear-stars step so a template can
// set at how many stars it kicks in.
func posterAt(st *bindings.FullDealerState, threshold uint8) (Action, bool) {
	if threshold == 0 {
		threshold = PosterHeatThreshold
	}
	if st.HeatLevel >= threshold && st.DailyAttemptsRemaining > 0 {
		return Action{Kind: ActionClearHeat}, true
	}
	return Action{}, false
}

// primaryActionKind maps a strategy's primary class to the game action that
// advances it — used to count the core step against its per-day cap by the RIGHT
// action (a pvp core's attacks, not its no-target fallback trades).
func primaryActionKind(p metricClass) ActionKind {
	if p == classPVP {
		return ActionPVP
	}
	return ActionPVE
}

// dayCounter caps arbitrary string-keyed events per UTC day (per-step action
// budgets). Like dailyLimiter but keyed by string so one counter serves every
// (step, dealer) pair. Resets each day.
type dayCounter struct {
	mu  sync.Mutex
	day int64
	n   map[string]int
}

func newDayCounter() *dayCounter { return &dayCounter{n: map[string]int{}} }

// take reports whether key is under cap today and, if so, counts one use.
func (c *dayCounter) take(key string, cap int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if today := utcDay(); today != c.day {
		c.day, c.n = today, map[string]int{}
	}
	if c.n[key] >= cap {
		return false
	}
	c.n[key]++
	return true
}

// heistCheckInAttemptsPerDay bounds how many times the autopilot re-attempts the
// bank-heist check-in per dealer per UTC day. A check-in that keeps failing — most
// often a dealer too poor to pay the one-time season enter() $CASH fee — must not
// spin on this first-in-line step every tick and starve the rest of the pipeline
// (heat-clearing, missions, trading). A few tries ride out a transient RPC error;
// after that the dealer yields and does everything else.
const heistCheckInAttemptsPerDay = 3

// heistCheckInStep is the very first autopilot action (once jail is handled): if
// a bank-heist season is active and the dealer hasn't checked in today, do it —
// auto-entering the season if needed. Runs before stars/missions/strategy so the
// daily focus is never missed; a jailed dealer breaks out first and checks in on
// the next tick after release. Attempts are capped per day (attempts, a
// dailyLimiter) so a check-in that can't complete — e.g. the dealer can't afford
// the season enter() fee — yields the pipeline instead of monopolising it; a
// successful check-in flips CheckedInToday and the step stops firing on its own.
// Best-effort — a read error just skips it.
func heistCheckInStep(ctx context.Context, r StrategyReader, tokenID uint64, attempts *dailyLimiter) (Action, bool) {
	need, err := r.NeedsHeistCheckIn(ctx, tokenID)
	if err != nil || !need {
		return Action{}, false
	}
	if attempts != nil && !attempts.take(tokenID, heistCheckInAttemptsPerDay) {
		return Action{}, false // tried enough today — let stars/missions/core run
	}
	return Action{Kind: ActionHeistCheckIn}, true
}

// missionStep is the universal mission-following pre-step for the autopilot:
// claim any completed mission (daily before weekly) as soon as it's claimable,
// and if the dealer hasn't checked in this epoch, snapshot the baseline so
// progress starts counting. One action per tick; the loop drains claims then
// checks in. Best-effort — a read error just skips missions this tick.
func missionStep(ctx context.Context, r StrategyReader, tokenID uint64) (Action, bool) {
	ms, err := r.MissionStatus(ctx, tokenID)
	if err != nil || len(ms) == 0 {
		return Action{}, false
	}
	if tpl, ok := bindings.FirstClaimable(ms); ok {
		return Action{Kind: ActionMissionClaim, TemplateID: tpl}, true
	}
	if bindings.NeedsCheckIn(ms) {
		return Action{Kind: ActionMissionCheckIn}, true
	}
	return Action{}, false
}

// stakeParamCache memoises the (static-ish) PVE stake params across ticks. A
// failed read leaves it unset so the next tick retries.
type stakeParamCache struct {
	mu   sync.Mutex
	sp   *bindings.PVEStakeParams
	done bool
}

func newStakeParamCache() *stakeParamCache { return &stakeParamCache{} }

func (c *stakeParamCache) get(ctx context.Context, r StrategyReader) *bindings.PVEStakeParams {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return c.sp
	}
	if sp, err := r.StakeParams(ctx); err == nil {
		c.sp, c.done = sp, true
	}
	return c.sp
}
