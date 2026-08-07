// Package store is the SQLite/WAL persistence layer. Driver is
// modernc.org/sqlite (pure Go, no cgo) so the Windows build needs no gcc
// (ADR-3). Migrations are embedded and applied on Open.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the database handle.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path, applies PRAGMAs
// and any pending migrations. The DSN sets WAL + NORMAL sync + a busy timeout so
// the resolve worker and the UI-refresh goroutine don't trip over each other.
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)" +
		"&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	// modernc pools connections; each gets the DSN pragmas, but keep it to one
	// writer to avoid SQLITE_BUSY churn on WAL.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the handle for packages that own their queries.
func (s *Store) DB() *sql.DB { return s.db }

// migrate applies embedded migrations in lexical order, tracked by
// schema_migrations. Each file runs once, in its own transaction.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (datetime('now')))`,
	); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists int
		if err := s.db.QueryRow(`SELECT 1 FROM schema_migrations WHERE name = ?`, name).Scan(&exists); err == nil {
			continue // already applied
		} else if err != sql.ErrNoRows {
			return fmt.Errorf("check migration %s: %w", name, err)
		}

		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}
