package engine

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"dealers/internal/chain/bindings"
	"dealers/internal/dealer"
	"dealers/internal/store"
)

// Autopilot drives idle dealers through a Strategy (ADR-5). It is opt-in and
// starts DISABLED — SetEnabled(true) turns it on. Each tick it acts only on
// dealers with no in-flight commit-reveal round (so it never double-submits),
// executing whatever the strategy decides. All actions flow through the same
// Manager + scheduler as manual ones.
type Autopilot struct {
	mgr      *dealer.Manager
	store    *store.Store
	reader   *bindings.Reader
	strategy dealer.Strategy
	ids      []uint64
	logger   *log.Logger
	enabled  atomic.Bool

	// settling gates a dealer after a travel until the on-chain state reflects the
	// new area. A travel is single-tx (no pending round), so if the next tick's
	// state read is stale (RPC node lag) the strategy would re-issue the SAME
	// travel — a visible double-move. Only the tick goroutine touches this.
	settling map[uint64]settleMove
}

// settleMove is the destination a dealer is expected to arrive at, with a
// deadline after which we give up waiting (in case the move never lands).
type settleMove struct {
	area     uint8
	deadline time.Time
}

// settleTimeout bounds how long we wait for a travel to reflect before acting again.
const settleTimeout = 90 * time.Second

// NewAutopilot builds a disabled autopilot.
func NewAutopilot(mgr *dealer.Manager, st *store.Store, reader *bindings.Reader, strategy dealer.Strategy, ids []uint64, logger *log.Logger) *Autopilot {
	if logger == nil {
		logger = log.Default()
	}
	return &Autopilot{mgr: mgr, store: st, reader: reader, strategy: strategy, ids: ids, logger: logger, settling: map[uint64]settleMove{}}
}

// SetEnabled turns the autopilot on or off at runtime (TUI toggle).
func (a *Autopilot) SetEnabled(on bool) {
	if a.enabled.Swap(on) != on {
		a.logger.Printf("autopilot %s", map[bool]string{true: "ENABLED", false: "disabled"}[on])
	}
}

// Enabled reports whether the autopilot is currently acting.
func (a *Autopilot) Enabled() bool { return a.enabled.Load() }

// Run ticks on the interval until ctx is cancelled. Ticks are no-ops while
// disabled.
func (a *Autopilot) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 12 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if a.enabled.Load() {
				a.tick(ctx)
			}
		}
	}
}

// travelGate returns the settle-gate to arm for an action: set for a real travel
// (destination differs from where we are), skipped otherwise. Applied on ATTEMPT
// so an errored-but-landed AGW send still gates against a duplicate move.
func travelGate(action dealer.Action, currentArea uint8, now time.Time) (settleMove, bool) {
	if action.Kind == dealer.ActionTravel && action.DestArea != currentArea {
		return settleMove{area: action.DestArea, deadline: now.Add(settleTimeout)}, true
	}
	return settleMove{}, false
}

// clearedToAct reports whether a dealer may act now, honouring a pending
// travel-settle gate: while a move is outstanding it returns false until the
// state shows arrival (or the deadline passes), clearing the gate either way.
func (a *Autopilot) clearedToAct(id uint64, currentArea uint8, now time.Time) bool {
	mv, ok := a.settling[id]
	if !ok {
		return true
	}
	if currentArea == mv.area || now.After(mv.deadline) {
		delete(a.settling, id)
		return true
	}
	return false
}

// tick evaluates every idle dealer once.
func (a *Autopilot) tick(ctx context.Context) {
	for _, id := range a.ids {
		if !a.enabled.Load() {
			return // disabled mid-tick
		}
		// Skip dealers with a commit-reveal round still open.
		if pend, err := a.store.PendingForToken(id); err != nil || len(pend) > 0 {
			continue
		}
		st, err := a.reader.GetFullDealerState(ctx, id)
		if err != nil {
			a.logger.Printf("autopilot #%d: state read: %v", id, err)
			continue
		}
		// Wait out a just-issued travel until the state reflects arrival (guards
		// the stale-read double-move); give up after settleTimeout.
		if !a.clearedToAct(id, st.CurrentArea, time.Now()) {
			continue // still in transit as far as the chain shows
		}
		dec := dealer.Decision{Snap: dealer.Snapshot{TokenID: id, State: st}}
		if econ, err := a.reader.AreaEconomy(ctx, st.CurrentArea); err == nil {
			dec.Area = econ.Drugs
		}

		action, ok := a.strategy.Next(ctx, a.reader, dec)
		if !ok {
			continue
		}
		// Gate a travel BEFORE sending, not after a successful send: an AGW
		// transaction can report an error while it still lands on chain, so gating
		// only on success left the next (stale-read) tick free to fire a DUPLICATE
		// travel — the double-move. Gating on attempt closes that: a move that
		// really failed just re-tries once the deadline lapses.
		if mv, gate := travelGate(action, st.CurrentArea, time.Now()); gate {
			a.settling[id] = mv
		}
		if seq, err := a.mgr.Execute(ctx, id, action); err != nil {
			a.logger.Printf("autopilot #%d: execute: %v", id, err)
		} else {
			a.logger.Printf("autopilot #%d: submitted action kind=%d seq=%d", id, action.Kind, seq)
		}
	}
}
