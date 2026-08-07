// Package scheduler drives the commit-reveal lifecycle (FR7/FR8). It owns no
// chain or signing logic: on every new block it scans the store's COMMITTED
// rounds and either marks them EXPIRED (reveal window missed) or hands them to a
// Resolver when their reveal block is reached. Resume is implicit — the first
// block after startup scans ListCommitted, which already includes rounds
// persisted by a previous process.
package scheduler

import (
	"context"
	"log"
	"sync"

	"dealers/internal/store"
)

// Resolver performs the on-chain resolve for one due round and records the
// result itself (MarkResolved/MarkFailed + action_log). The scheduler calls it
// at most once concurrently per seq; if it errors and leaves the row COMMITTED,
// the next block retries.
type Resolver interface {
	Resolve(ctx context.Context, p store.Pending) error
}

// Scheduler couples the store to a Resolver, driven by a block stream.
type Scheduler struct {
	store    *store.Store
	resolver Resolver
	logger   *log.Logger

	inflight sync.Map // seq -> struct{}: resolves currently being attempted

	// dispatch runs a resolve attempt. Overridable in tests for synchronous
	// execution; defaults to a goroutine so a slow resolve tx never blocks
	// processing the rest of the fleet.
	dispatch func(func())
}

// New builds a scheduler. logger may be nil.
func New(st *store.Store, r Resolver, logger *log.Logger) *Scheduler {
	if logger == nil {
		logger = log.Default()
	}
	return &Scheduler{
		store:    st,
		resolver: r,
		logger:   logger,
		dispatch: func(f func()) { go f() },
	}
}

// Run consumes block numbers until ctx is cancelled or blocks closes.
func (s *Scheduler) Run(ctx context.Context, blocks <-chan uint64) {
	for {
		select {
		case <-ctx.Done():
			return
		case bn, ok := <-blocks:
			if !ok {
				return
			}
			s.OnBlock(ctx, bn)
		}
	}
}

// OnBlock evaluates every open round against the current height. Exported so it
// can be unit-tested and called for a one-shot catch-up at startup.
func (s *Scheduler) OnBlock(ctx context.Context, current uint64) {
	committed, err := s.store.ListCommitted()
	if err != nil {
		s.logger.Printf("scheduler: list committed: %v", err)
		return
	}
	for _, p := range committed {
		switch {
		case current > p.ExpiryBlock:
			// Reveal window (commit+200) lapsed. On chain reveal() now reverts
			// Expired; the round is terminal (loss/bust). Record it once.
			if err := s.store.MarkExpired(p.Seq); err != nil {
				s.logger.Printf("scheduler: mark expired seq=%d: %v", p.Seq, err)
			} else {
				s.logger.Printf("scheduler: seq=%d token=%d EXPIRED at block %d (expiry %d)",
					p.Seq, p.TokenID, current, p.ExpiryBlock)
			}

		case current >= p.RevealBlock:
			// Due. Deduplicate: skip if a resolve for this seq is already running.
			if _, running := s.inflight.LoadOrStore(p.Seq, struct{}{}); running {
				continue
			}
			p := p
			s.dispatch(func() {
				defer s.inflight.Delete(p.Seq)
				if err := s.resolver.Resolve(ctx, p); err != nil {
					s.logger.Printf("scheduler: resolve seq=%d token=%d: %v (will retry next block)",
						p.Seq, p.TokenID, err)
				}
			})
		}
	}
}
