package store

import "fmt"

// Pending kinds and statuses mirror the pending_actions schema (TZ §8).
const (
	KindPVE          = "pve"
	KindPVP          = "pvp"
	KindHeistStage   = "heist_stage"
	KindBreakout     = "breakout"
	KindWantedPoster = "wanted_poster"

	StatusCommitted = "COMMITTED"
	StatusResolved  = "RESOLVED"
	StatusExpired   = "EXPIRED"
	StatusFailed    = "FAILED"
)

// Dealer is a row in the dealers table.
type Dealer struct {
	TokenID       uint64
	Nickname      string
	WalletAddress string
	Network       string
}

// Pending is a row in pending_actions — one in-flight commit-reveal round.
type Pending struct {
	Seq           uint64
	TokenID       uint64
	Kind          string
	CommitBlock   uint64
	RevealBlock   uint64
	ExpiryBlock   uint64
	Status        string
	TxHashCommit  string
	TxHashResolve string
	MetaJSON      string
}

// LogEntry is an append to action_log.
type LogEntry struct {
	TokenID   uint64
	Kind      string
	Summary   string
	CashDelta *int64
	RepDelta  *int64
	HeatAfter *int64
	TxHash    string
}

// UpsertDealer inserts or updates a dealer row (idempotent on token_id).
func (s *Store) UpsertDealer(d Dealer) error {
	_, err := s.db.Exec(`
		INSERT INTO dealers (token_id, nickname, wallet_address, network)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(token_id) DO UPDATE SET
			nickname       = excluded.nickname,
			wallet_address = excluded.wallet_address,
			network        = excluded.network`,
		d.TokenID, d.Nickname, d.WalletAddress, d.Network)
	if err != nil {
		return fmt.Errorf("upsert dealer %d: %w", d.TokenID, err)
	}
	return nil
}

// InsertPending records a freshly-committed action. Written immediately after
// the commit tx confirms so a crash before resolve is recoverable (FR8). The
// dealer row must exist (FK); callers UpsertDealer first.
func (s *Store) InsertPending(p Pending) error {
	if p.Status == "" {
		p.Status = StatusCommitted
	}
	_, err := s.db.Exec(`
		INSERT INTO pending_actions
			(seq, token_id, kind, commit_block, reveal_block, expiry_block, status, tx_hash_commit, meta_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Seq, p.TokenID, p.Kind, p.CommitBlock, p.RevealBlock, p.ExpiryBlock,
		p.Status, p.TxHashCommit, nullIfEmpty(p.MetaJSON))
	if err != nil {
		return fmt.Errorf("insert pending seq=%d: %w", p.Seq, err)
	}
	return nil
}

// ListCommitted returns every still-open round, oldest first. This is the
// resume set loaded at startup (FR8) and the working set the scheduler scans.
func (s *Store) ListCommitted() ([]Pending, error) {
	rows, err := s.db.Query(`
		SELECT seq, token_id, kind, commit_block, reveal_block, expiry_block,
		       status, tx_hash_commit, COALESCE(tx_hash_resolve, ''), COALESCE(meta_json, '')
		FROM pending_actions
		WHERE status = ?
		ORDER BY reveal_block ASC, seq ASC`, StatusCommitted)
	if err != nil {
		return nil, fmt.Errorf("list committed: %w", err)
	}
	defer rows.Close()

	var out []Pending
	for rows.Next() {
		var p Pending
		if err := rows.Scan(&p.Seq, &p.TokenID, &p.Kind, &p.CommitBlock, &p.RevealBlock,
			&p.ExpiryBlock, &p.Status, &p.TxHashCommit, &p.TxHashResolve, &p.MetaJSON); err != nil {
			return nil, fmt.Errorf("scan pending: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PendingForToken returns the open (COMMITTED) rounds for one dealer.
func (s *Store) PendingForToken(tokenID uint64) ([]Pending, error) {
	rows, err := s.db.Query(`
		SELECT seq, token_id, kind, commit_block, reveal_block, expiry_block,
		       status, tx_hash_commit, COALESCE(tx_hash_resolve, ''), COALESCE(meta_json, '')
		FROM pending_actions
		WHERE token_id = ? AND status = ?
		ORDER BY reveal_block ASC`, tokenID, StatusCommitted)
	if err != nil {
		return nil, fmt.Errorf("pending for token %d: %w", tokenID, err)
	}
	defer rows.Close()
	var out []Pending
	for rows.Next() {
		var p Pending
		if err := rows.Scan(&p.Seq, &p.TokenID, &p.Kind, &p.CommitBlock, &p.RevealBlock,
			&p.ExpiryBlock, &p.Status, &p.TxHashCommit, &p.TxHashResolve, &p.MetaJSON); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// LogRow is a rendered action_log entry.
type LogRow struct {
	TS      string
	Kind    string
	Summary string
	TxHash  string
}

// RecentActions returns the newest action_log rows for a dealer.
func (s *Store) RecentActions(tokenID uint64, limit int) ([]LogRow, error) {
	rows, err := s.db.Query(`
		SELECT ts, kind, COALESCE(summary,''), COALESCE(tx_hash,'')
		FROM action_log WHERE token_id = ?
		ORDER BY id DESC LIMIT ?`, tokenID, limit)
	if err != nil {
		return nil, fmt.Errorf("recent actions token %d: %w", tokenID, err)
	}
	defer rows.Close()
	var out []LogRow
	for rows.Next() {
		var r LogRow
		if err := rows.Scan(&r.TS, &r.Kind, &r.Summary, &r.TxHash); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FleetLogRow is an action_log entry tagged with its dealer (for the fleet-wide
// activity feed).
type FleetLogRow struct {
	TokenID uint64
	TS      string
	Kind    string
	Summary string
}

// RecentFleetActions returns the newest action_log rows across all dealers.
func (s *Store) RecentFleetActions(limit int) ([]FleetLogRow, error) {
	rows, err := s.db.Query(`
		SELECT token_id, ts, kind, COALESCE(summary,'')
		FROM action_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("recent fleet actions: %w", err)
	}
	defer rows.Close()
	var out []FleetLogRow
	for rows.Next() {
		var r FleetLogRow
		if err := rows.Scan(&r.TokenID, &r.TS, &r.Kind, &r.Summary); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkResolved transitions a round to RESOLVED and records the resolve tx.
func (s *Store) MarkResolved(seq uint64, resolveTx string) error {
	return s.setStatus(seq, StatusResolved, resolveTx)
}

// MarkExpired transitions a round to EXPIRED (reveal window missed, >200 blocks).
func (s *Store) MarkExpired(seq uint64) error {
	return s.setStatus(seq, StatusExpired, "")
}

// MarkFailed transitions a round to FAILED (resolve tx errored terminally).
func (s *Store) MarkFailed(seq uint64) error {
	return s.setStatus(seq, StatusFailed, "")
}

func (s *Store) setStatus(seq uint64, status, resolveTx string) error {
	res, err := s.db.Exec(`
		UPDATE pending_actions
		SET status = ?, tx_hash_resolve = ?, resolved_at = datetime('now')
		WHERE seq = ? AND status = ?`,
		status, nullIfEmpty(resolveTx), seq, StatusCommitted)
	if err != nil {
		return fmt.Errorf("set status %s seq=%d: %w", status, seq, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("pending seq=%d not in COMMITTED state (already resolved/expired?)", seq)
	}
	return nil
}

// AppendLog writes an action_log entry.
func (s *Store) AppendLog(e LogEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO action_log (token_id, kind, summary, cash_delta, rep_delta, heat_after, tx_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.TokenID, e.Kind, nullIfEmpty(e.Summary),
		nullInt(e.CashDelta), nullInt(e.RepDelta), nullInt(e.HeatAfter), nullIfEmpty(e.TxHash))
	if err != nil {
		return fmt.Errorf("append log token=%d: %w", e.TokenID, err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
