// Package dealer holds the per-dealer domain layer. In Phase 0 it only builds
// read-only fleet snapshots; the DealerManager FSM and Strategy interface
// (ADR-5) arrive in Phase 1+.
package dealer

import (
	"context"
	"sort"
	"sync"
	"time"

	"dealers/internal/chain/bindings"
)

// Snapshot is one dealer's state at a point in time, or the error from reading
// it. A failed read for one dealer never hides the others (FR3: non-blocking).
type Snapshot struct {
	TokenID uint64
	State   *bindings.FullDealerState
	Err     error
	FetchedAt time.Time
	// CheckedIn is the daily check-in status for the active season: nil = unknown
	// (contract not deployed / read failed / dealer uninitialized), else whether
	// this dealer has already checked in today.
	CheckedIn *bool

	// Mission summary for the fleet indicator (Known=false when unread/undeployed).
	MissionsKnown       bool
	MissionsClaimable   int
	MissionsNeedCheckIn bool
}

// Status is the coarse fleet-view status derived purely from state flags. Heist
// activity (activeHeist) is layered in during Phase 3; for now a dealer is
// Jailed, in the Safe House, or Idle.
func (s Snapshot) Status() string {
	switch {
	case s.Err != nil:
		return "ERR"
	case s.State == nil:
		return "?"
	case !s.State.IsInitialized:
		return "UNINIT"
	case s.State.IsJailed:
		return "JAILED"
	case s.State.IsInSafeHouse:
		return "SAFEHOUSE"
	default:
		return "IDLE"
	}
}

// FetchAll reads every dealer's full state concurrently and returns snapshots
// sorted by token id. The per-call timeout is the caller's ctx; individual
// failures are captured in Snapshot.Err.
func FetchAll(ctx context.Context, r *bindings.Reader, ids []uint64, now time.Time) []Snapshot {
	// One season lookup shared by every dealer's check-in read. On networks
	// without the bank-heist contract this errors and CheckedIn stays nil.
	season, seasonErr := r.ActiveSeason(ctx)
	out := make([]Snapshot, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id uint64) {
			defer wg.Done()
			st, err := r.GetFullDealerState(ctx, id)
			snap := Snapshot{TokenID: id, State: st, Err: err, FetchedAt: now}
			if err == nil && st != nil && st.IsInitialized {
				if seasonErr == nil {
					if done, cerr := r.CheckedInToday(ctx, season, id, now.Unix()); cerr == nil {
						snap.CheckedIn = &done
					}
				}
				if ms, merr := r.MissionStatus(ctx, id); merr == nil && ms != nil {
					snap.MissionsKnown = true
					snap.MissionsNeedCheckIn = bindings.NeedsCheckIn(ms)
					for j := range ms {
						if ms[j].Claimable && !ms[j].Claimed {
							snap.MissionsClaimable++
						}
					}
				}
			}
			out[i] = snap
		}(i, id)
	}
	wg.Wait()
	sort.Slice(out, func(a, b int) bool { return out[a].TokenID < out[b].TokenID })
	return out
}
