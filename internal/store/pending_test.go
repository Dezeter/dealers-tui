package store

import (
	"path/filepath"
	"testing"
)

func openTmp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedDealer(t *testing.T, s *Store, id uint64) {
	t.Helper()
	if err := s.UpsertDealer(Dealer{TokenID: id, WalletAddress: "0xagw", Network: "testnet"}); err != nil {
		t.Fatalf("upsert dealer: %v", err)
	}
}

func TestPendingLifecycle(t *testing.T) {
	s := openTmp(t)
	seedDealer(t, s, 1)

	p := Pending{
		Seq: 7, TokenID: 1, Kind: KindPVE,
		CommitBlock: 100, RevealBlock: 102, ExpiryBlock: 300,
		TxHashCommit: "0xcommit", MetaJSON: `{"choice":0,"drugId":0,"amount":5}`,
	}
	if err := s.InsertPending(p); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.ListCommitted()
	if err != nil || len(got) != 1 {
		t.Fatalf("ListCommitted = %v, %d rows", err, len(got))
	}
	if got[0].Status != StatusCommitted || got[0].MetaJSON == "" || got[0].TxHashResolve != "" {
		t.Errorf("unexpected pending: %+v", got[0])
	}

	if err := s.MarkResolved(7, "0xresolve"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if rows, _ := s.ListCommitted(); len(rows) != 0 {
		t.Errorf("expected 0 committed after resolve, got %d", len(rows))
	}

	// Double-resolve must fail (already left COMMITTED) — guards against the
	// resolver firing twice for one round.
	if err := s.MarkResolved(7, "0xagain"); err == nil {
		t.Error("expected error re-resolving an already-resolved round")
	}
}

// TestResumeAfterReopen is the FR8 guarantee: committed rounds reload verbatim
// after a process restart, ready for the scheduler to continue resolving.
func TestResumeAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resume.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.UpsertDealer(Dealer{TokenID: 2, WalletAddress: "0xagw", Network: "testnet"}); err != nil {
		t.Fatalf("dealer: %v", err)
	}
	rounds := []Pending{
		{Seq: 10, TokenID: 2, Kind: KindPVE, CommitBlock: 50, RevealBlock: 52, ExpiryBlock: 250, TxHashCommit: "0xa"},
		{Seq: 11, TokenID: 2, Kind: KindHeistStage, CommitBlock: 60, RevealBlock: 62, ExpiryBlock: 260, TxHashCommit: "0xb"},
	}
	for _, p := range rounds {
		if err := s.InsertPending(p); err != nil {
			t.Fatalf("insert %d: %v", p.Seq, err)
		}
	}
	s.MarkResolved(10, "0xdone") // one already resolved before "crash"
	s.Close()

	// Restart.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	open, err := s2.ListCommitted()
	if err != nil {
		t.Fatalf("list after reopen: %v", err)
	}
	if len(open) != 1 || open[0].Seq != 11 || open[0].Kind != KindHeistStage || open[0].ExpiryBlock != 260 {
		t.Fatalf("resume set wrong: %+v", open)
	}
}

func TestActionLog(t *testing.T) {
	s := openTmp(t)
	seedDealer(t, s, 3)
	cash, rep, heat := int64(-120), int64(36), int64(1)
	if err := s.AppendLog(LogEntry{
		TokenID: 3, Kind: KindPVE, Summary: "WIN buy weed", CashDelta: &cash, RepDelta: &rep, HeatAfter: &heat, TxHash: "0xr",
	}); err != nil {
		t.Fatalf("append log: %v", err)
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM action_log WHERE token_id=3 AND rep_delta=36`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("log row missing: n=%d err=%v", n, err)
	}
}

func TestRecentFleetActions(t *testing.T) {
	s := openTmp(t)
	seedDealer(t, s, 3)
	seedDealer(t, s, 4)
	if err := s.AppendLog(LogEntry{TokenID: 3, Kind: KindPVE, Summary: "bought 5 weed — WIN"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendLog(LogEntry{TokenID: 4, Kind: KindPVP, Summary: "PVP WIN vs #142"}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.RecentFleetActions(10)
	if err != nil {
		t.Fatalf("RecentFleetActions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 across dealers", len(rows))
	}
	// Newest first: dealer 4's PVP entry leads, tagged with its token id.
	if rows[0].TokenID != 4 || rows[0].Summary != "PVP WIN vs #142" {
		t.Errorf("row0 = %+v, want dealer 4 PVP", rows[0])
	}
	if rows[1].TokenID != 3 {
		t.Errorf("row1 token = %d, want 3", rows[1].TokenID)
	}
}
