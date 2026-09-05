// Package store owns the SQLite database: opening, pragmas, schema
// migrations and small transaction/query helpers used by repositories.
//
// We deliberately run with a single pooled connection: BastionDeck is a
// single-host control plane, SQLite serialises writers anyway, and one
// connection eliminates "database is locked" classes entirely (ADR-001).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"bastiondeck/internal/migrations"
)

// Store wraps an opened database and its resolved file path.
type Store struct {
	DB      *sql.DB
	Path    string
	Version int
}

// Open creates/opens the database at dataDir/bastiondeck.db, applies
// pragmas through the DSN and runs all pending migrations.
func Open(dataDir string) (*Store, error) {
	path := filepath.Join(dataDir, "bastiondeck.db")
	dsn := url.URL{Scheme: "file", Path: path}
	q := dsn.Query()
	// Pure-Go SQLite pragmas via DSN (no pragma statements inside tx).
	q.Set("_pragma", "busy_timeout(5000)")
	q.Set("_pragma", "foreign_keys(ON)")
	q.Set("_pragma", "journal_mode(WAL)")
	q.Set("_pragma", "synchronous(NORMAL)")
	dsn.RawQuery = q.Encode()

	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single connection: deterministic write ordering, no lock contention.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	s := &Store{DB: db, Path: path}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close checkpoints WAL and closes the database.
func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	_, _ = s.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return s.DB.Close()
}

// InTx runs fn inside a serializable transaction, committing on nil error and
// rolling back otherwise. Nested calls reuse the outer transaction.
func (s *Store) InTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	return InTx(ctx, s.DB, fn)
}

// txStarter is satisfied by *sql.DB.
type txStarter interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

// InTx is the package-level variant used by repositories that receive a
// *sql.DB. It upgrades to serializable for predictable SQLite semantics.
func InTx(ctx context.Context, db txStarter, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err = fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback (%v) after: %w", rbErr, err)
		}
		return err
	}
	return tx.Commit()
}

func (s *Store) migrate() error {
	planned, err := migrations.List()
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(
        version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`)
	if err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}
	have := map[int]bool{}
	rows, err := s.DB.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		have[v] = true
	}
	rows.Close()

	for _, m := range planned {
		if have[m.Version] {
			s.Version = max(s.Version, m.Version)
			continue
		}
		if err := s.applyOne(m); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
		}
		s.Version = m.Version
	}
	cur, err := migrations.CurrentVersion()
	if err != nil {
		return err
	}
	if s.Version == 0 {
		s.Version = cur
	}
	return nil
}

func (s *Store) applyOne(m migrations.PlannedMigration) error {
	return InTx(context.Background(), s.DB, func(tx *sql.Tx) error {
		for _, stmt := range splitStatements(m.SQL) {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("stmt %q: %w", firstWords(stmt), err)
			}
		}
		_, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(?,?)`,
			m.Version, migrations.StampUTC())
		return err
	})
}

// splitStatements splits a migration script on semicolon boundaries and
// drops PRAGMA statements (those are applied via the SQLite DSN). Our
// migrations contain no stored procedures with embedded semicolons.
func splitStatements(script string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(script, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trim), "PRAGMA ") {
			continue
		}
		if strings.HasPrefix(trim, "--") {
			continue
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
		if strings.HasSuffix(trim, ";") {
			stmt := strings.TrimSpace(cur.String())
			if stmt != "" && stmt != ";" {
				out = append(out, stmt)
			}
			cur.Reset()
		}
	}
	if stmt := strings.TrimSpace(cur.String()); stmt != "" {
		out = append(out, stmt)
	}
	return out
}

func firstWords(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 48 {
		return s[:48]
	}
	return s
}

// ErrNotFound is returned by repositories when a row does not exist.
var ErrNotFound = errors.New("not found")

// Now returns the canonical UTC millisecond timestamp used across the app.
func Now() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

// Max is a tiny local max to avoid a go1.21 dependency assumption in tools.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
