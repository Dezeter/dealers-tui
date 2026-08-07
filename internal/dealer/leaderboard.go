package dealer

import (
	"context"
	"math/big"
	"sort"
	"sync"
	"time"

	"dealers/internal/chain/bindings"
)

// Ranks is a dealer's 1-based leaderboard position on each board (0 = unknown).
// PVE ranks by reputation, PVP by infamy — two independent boards, tie-broken by
// smaller token id (TZ §A.9). There is no on-chain getter (TODO-7), so this is
// computed by enumerating all minted dealers.
type Ranks struct {
	Pve int
	Pvp int
}

// leaderReader is the read surface leaderboard computation needs (mockable).
type leaderReader interface {
	TotalSupply(context.Context) (uint64, error)
	TokenByIndex(context.Context, uint64) (uint64, error)
	GetFullDealerState(context.Context, uint64) (*bindings.FullDealerState, error)
}

// dealerScore is one dealer's ranking inputs.
type dealerScore struct {
	ID     uint64
	Rep    *big.Int
	Infamy *big.Int
}

// rankDealers assigns PVE (by rep) and PVP (by infamy) positions, tie-broken by
// smaller id. Pure and testable.
func rankDealers(scores []dealerScore) map[uint64]Ranks {
	out := make(map[uint64]Ranks, len(scores))

	pve := make([]dealerScore, len(scores))
	copy(pve, scores)
	sort.Slice(pve, func(i, j int) bool {
		if c := cmpBig(pve[i].Rep, pve[j].Rep); c != 0 {
			return c > 0 // higher rep first
		}
		return pve[i].ID < pve[j].ID
	})
	for i, d := range pve {
		r := out[d.ID]
		r.Pve = i + 1
		out[d.ID] = r
	}

	pvp := make([]dealerScore, len(scores))
	copy(pvp, scores)
	sort.Slice(pvp, func(i, j int) bool {
		if c := cmpBig(pvp[i].Infamy, pvp[j].Infamy); c != 0 {
			return c > 0
		}
		return pvp[i].ID < pvp[j].ID
	})
	for i, d := range pvp {
		r := out[d.ID]
		r.Pvp = i + 1
		out[d.ID] = r
	}
	return out
}

func cmpBig(a, b *big.Int) int {
	if a == nil {
		a = big.NewInt(0)
	}
	if b == nil {
		b = big.NewInt(0)
	}
	return a.Cmp(b)
}

// LeaderboardCache holds computed ranks for concurrent read from the UI while a
// background goroutine refreshes them.
type LeaderboardCache struct {
	mu    sync.RWMutex
	ranks map[uint64]Ranks
	at    time.Time
}

// NewLeaderboardCache returns an empty cache.
func NewLeaderboardCache() *LeaderboardCache {
	return &LeaderboardCache{ranks: map[uint64]Ranks{}}
}

// Get returns a dealer's ranks (ok=false if not computed yet).
func (c *LeaderboardCache) Get(id uint64) (Ranks, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.ranks[id]
	return r, ok
}

// ComputedAt returns when the ranks were last refreshed.
func (c *LeaderboardCache) ComputedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.at
}

// Refresh enumerates all minted dealers, reads rep+infamy, ranks them, and
// stores the result. Bounded concurrency keeps the RPC burst reasonable.
func (c *LeaderboardCache) Refresh(ctx context.Context, r leaderReader) error {
	total, err := r.TotalSupply(ctx)
	if err != nil {
		return err
	}

	ids := make([]uint64, total)
	if err := forEachBounded(ctx, int(total), 8, func(i int) error {
		id, err := r.TokenByIndex(ctx, uint64(i))
		if err != nil {
			return err
		}
		ids[i] = id
		return nil
	}); err != nil {
		return err
	}

	scores := make([]dealerScore, total)
	_ = forEachBounded(ctx, int(total), 8, func(i int) error {
		st, err := r.GetFullDealerState(ctx, ids[i])
		if err != nil || st == nil {
			return nil // skip unreadable dealers; they just don't rank
		}
		scores[i] = dealerScore{ID: ids[i], Rep: st.Reputation, Infamy: st.Infamy}
		return nil
	})

	// Drop skipped (zero-id) entries.
	filtered := scores[:0]
	for _, s := range scores {
		if s.ID != 0 || s.Rep != nil {
			filtered = append(filtered, s)
		}
	}

	ranks := rankDealers(filtered)
	c.mu.Lock()
	c.ranks = ranks
	c.at = time.Now()
	c.mu.Unlock()
	return nil
}

// forEachBounded runs fn(0..n-1) with at most `limit` concurrent calls and
// returns the first error.
func forEachBounded(ctx context.Context, n, limit int, fn func(i int) error) error {
	if n == 0 {
		return nil
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error
	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fn(i); err != nil {
				once.Do(func() { firstErr = err })
			}
		}(i)
	}
	wg.Wait()
	return firstErr
}
