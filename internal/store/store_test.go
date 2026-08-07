package store

import (
	"path/filepath"
	"testing"
)

// TestMigrateAndReopen verifies migrations apply once, are idempotent across a
// reopen, and that a COMMITTED pending_actions row survives a close/reopen —
// the FR8 resume-after-restart guarantee, at the storage layer.
func TestMigrateAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dealers.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if _, err := s.DB().Exec(
		`INSERT INTO dealers (token_id, wallet_address, network) VALUES (1, '0xabc', 'testnet')`,
	); err != nil {
		t.Fatalf("insert dealer: %v", err)
	}
	if _, err := s.DB().Exec(
		`INSERT INTO pending_actions (seq, token_id, kind, commit_block, reveal_block, expiry_block, tx_hash_commit)
		 VALUES (42, 1, 'pve', 100, 102, 300, '0xdeadbeef')`,
	); err != nil {
		t.Fatalf("insert pending: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen: migrations must not re-run, data must persist.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	var status string
	var revealBlock int64
	if err := s2.DB().QueryRow(
		`SELECT status, reveal_block FROM pending_actions WHERE seq = 42`,
	).Scan(&status, &revealBlock); err != nil {
		t.Fatalf("read pending after reopen: %v", err)
	}
	if status != "COMMITTED" {
		t.Errorf("status = %q, want COMMITTED (default)", status)
	}
	if revealBlock != 102 {
		t.Errorf("reveal_block = %d, want 102", revealBlock)
	}

	// Migrations tracked exactly once.
	var count int
	if err := s2.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_migrations count = %d, want 1", count)
	}
}

// TestForeignKeyEnforced confirms the FK pragma is active: a pending_action for
// a non-existent dealer must be rejected.
func TestForeignKeyEnforced(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "fk.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	_, err = s.DB().Exec(
		`INSERT INTO pending_actions (seq, token_id, kind, commit_block, reveal_block, expiry_block, tx_hash_commit)
		 VALUES (1, 999, 'pve', 1, 3, 200, '0x00')`,
	)
	if err == nil {
		t.Fatal("expected FK violation inserting pending_action for missing dealer, got nil")
	}
}
