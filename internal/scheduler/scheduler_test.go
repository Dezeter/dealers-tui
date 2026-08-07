package scheduler

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"dealers/internal/store"
)

func quietLogger() *log.Logger { return log.New(io.Discard, "", 0) }

// fakeResolver records which seqs it was asked to resolve and, on request,
// actually marks them resolved in the store (to mimic a real resolver so the
// row leaves COMMITTED).
type fakeResolver struct {
	st      *store.Store
	mu      sync.Mutex
	calls   []uint64
	markDone bool
	failSeq  uint64 // if non-zero, Resolve returns error for this seq (leaves COMMITTED)
	count    atomic.Int64
}

func (f *fakeResolver) Resolve(_ context.Context, p store.Pending) error {
	f.mu.Lock()
	f.calls = append(f.calls, p.Seq)
	f.mu.Unlock()
	f.count.Add(1)
	if p.Seq == f.failSeq {
		return errContext
	}
	if f.markDone {
		return f.st.MarkResolved(p.Seq, "0xresolved")
	}
	return nil
}

var errContext = context.Canceled // any error value

func newSched(t *testing.T, r Resolver) (*Scheduler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(st, r, quietLogger())
	// Run dispatched work synchronously for deterministic assertions.
	s.dispatch = func(f func()) { f() }
	return s, st
}

func seed(t *testing.T, st *store.Store, p store.Pending) {
	t.Helper()
	if err := st.UpsertDealer(store.Dealer{TokenID: p.TokenID, WalletAddress: "0xagw", Network: "testnet"}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertPending(p); err != nil {
		t.Fatal(err)
	}
}

func TestResolvesWhenDue(t *testing.T) {
	fr := &fakeResolver{markDone: true}
	s, st := newSched(t, fr)
	fr.st = st
	seed(t, st, store.Pending{Seq: 1, TokenID: 10, Kind: store.KindPVE, CommitBlock: 100, RevealBlock: 102, ExpiryBlock: 300, TxHashCommit: "0xa"})

	s.OnBlock(context.Background(), 101) // before reveal
	if fr.count.Load() != 0 {
		t.Fatalf("resolved too early: %d calls", fr.count.Load())
	}
	s.OnBlock(context.Background(), 102) // reveal reached
	if fr.count.Load() != 1 {
		t.Fatalf("expected 1 resolve at reveal, got %d", fr.count.Load())
	}
	if rows, _ := st.ListCommitted(); len(rows) != 0 {
		t.Errorf("row still COMMITTED after resolve: %d", len(rows))
	}
}

func TestExpiresPastWindow(t *testing.T) {
	fr := &fakeResolver{}
	s, st := newSched(t, fr)
	fr.st = st
	seed(t, st, store.Pending{Seq: 2, TokenID: 11, Kind: store.KindPVE, CommitBlock: 100, RevealBlock: 102, ExpiryBlock: 300, TxHashCommit: "0xb"})

	s.OnBlock(context.Background(), 301) // past expiry
	if fr.count.Load() != 0 {
		t.Errorf("expired round should not be resolved, got %d calls", fr.count.Load())
	}
	rows, _ := st.ListCommitted()
	if len(rows) != 0 {
		t.Errorf("expired row still COMMITTED: %d", len(rows))
	}
	var status string
	st.DB().QueryRow(`SELECT status FROM pending_actions WHERE seq=2`).Scan(&status)
	if status != store.StatusExpired {
		t.Errorf("status = %q, want EXPIRED", status)
	}
}

func TestDedupeInflight(t *testing.T) {
	// markDone=false so the row stays COMMITTED; without dedupe a second OnBlock
	// at the same height would resolve it again.
	fr := &fakeResolver{}
	s, st := newSched(t, fr)
	fr.st = st
	// Make dispatch defer execution so the inflight marker is observed as "still
	// running" across two OnBlock calls, then drain.
	var pending []func()
	s.dispatch = func(f func()) { pending = append(pending, f) }
	seed(t, st, store.Pending{Seq: 3, TokenID: 12, Kind: store.KindPVE, CommitBlock: 1, RevealBlock: 2, ExpiryBlock: 200, TxHashCommit: "0xc"})

	s.OnBlock(context.Background(), 5)
	s.OnBlock(context.Background(), 6) // second tick while first still "in flight"
	if len(pending) != 1 {
		t.Fatalf("expected 1 dispatched resolve (deduped), got %d", len(pending))
	}
	for _, f := range pending {
		f()
	}
}

func TestResumeFromReopen(t *testing.T) {
	// Persist a committed round, close, reopen, and confirm the first OnBlock
	// resolves it — the resume guarantee end to end at the scheduler level.
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDealer(store.Dealer{TokenID: 20, WalletAddress: "0xagw", Network: "testnet"}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertPending(store.Pending{Seq: 99, TokenID: 20, Kind: store.KindPVE, CommitBlock: 500, RevealBlock: 502, ExpiryBlock: 700, TxHashCommit: "0xd"}); err != nil {
		t.Fatal(err)
	}
	st.Close()

	st2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	fr := &fakeResolver{st: st2, markDone: true}
	s := New(st2, fr, quietLogger())
	s.dispatch = func(f func()) { f() }

	s.OnBlock(context.Background(), 503) // reveal reached, mid-window
	if fr.count.Load() != 1 || len(fr.calls) != 1 || fr.calls[0] != 99 {
		t.Fatalf("resume did not resolve seq 99: calls=%v", fr.calls)
	}
}
