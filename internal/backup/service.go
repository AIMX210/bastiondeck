// Package backup creates and restores encrypted logical backups. Restore is
// staged (decrypt+validate in memory) and requires explicit confirmation;
// before applying, the current database is copied to a safety file.
package backup

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/crypto/argon2"
)

// bundleVersion tags the on-wire format.
const bundleVersion = 1

// logicalTables are exported in a fixed order for deterministic bundles.
var logicalTables = []string{
	"users", "credentials", "host_groups", "hosts", "snippets", "jobs",
	"job_runs", "run_targets", "tunnels", "agents", "settings",
}

// Bundle is the logical database snapshot.
type Bundle struct {
	Version int                         `json:"version"`
	At      string                      `json:"at"`
	Tables  map[string][]map[string]any `json:"tables"`
}

// Service performs backup/restore against a database file.
type Service struct {
	db     *sql.DB
	dbPath string
}

// New constructs the service.
func New(db *sql.DB, dbPath string) *Service { return &Service{db: db, dbPath: dbPath} }

func deriveKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, 2, 64*1024, 2, 32)
}

// seal: salt(16) | nonce(12) | ciphertext
func seal(plain []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := deriveKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, plain, nil)
	return append(append(salt, nonce...), ct...), nil
}

func open(blob []byte, passphrase string) ([]byte, error) {
	if len(blob) < 28 {
		return nil, errors.New("backup too short")
	}
	salt, nonce, ct := blob[:16], blob[16:28], blob[28:]
	key := deriveKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, nil)
}

// Export produces an encrypted logical backup.
func (s *Service) Export(ctx context.Context, passphrase string) ([]byte, error) {
	if len(passphrase) < 8 {
		return nil, errors.New("passphrase must be at least 8 characters")
	}
	b := Bundle{Version: bundleVersion, At: time.Now().UTC().Format(time.RFC3339), Tables: map[string][]map[string]any{}}
	for _, t := range logicalTables {
		rows, err := s.dumpTable(ctx, t)
		if err != nil {
			return nil, fmt.Errorf("dump %s: %w", t, err)
		}
		b.Tables[t] = rows
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, err
	}
	return seal(raw, passphrase)
}

func (s *Service) dumpTable(ctx context.Context, table string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM `+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				row[c] = map[string]any{"__bin__": fmt.Sprintf("%x", v)}
			default:
				row[c] = v
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Report summarises a staged backup.
type Report struct {
	Version int            `json:"version"`
	At      string         `json:"at"`
	Counts  map[string]int `json:"counts"`
}

// Inspect decrypts and validates without writing.
func Inspect(blob []byte, passphrase string) (*Bundle, *Report, error) {
	raw, err := open(blob, passphrase)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt: %w", err)
	}
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, nil, err
	}
	if b.Version != bundleVersion {
		return nil, nil, fmt.Errorf("unsupported bundle version %d", b.Version)
	}
	rep := &Report{Version: b.Version, At: b.At, Counts: map[string]int{}}
	for t, rows := range b.Tables {
		rep.Counts[t] = len(rows)
	}
	return &b, rep, nil
}

// Restore applies a staged bundle inside a transaction: tables are replaced
// in dependency order. A safety copy of the DB file is written first.
func (s *Service) Restore(ctx context.Context, b *Bundle, safetyDir string) (string, error) {
	if b == nil || b.Version != bundleVersion {
		return "", errors.New("invalid bundle")
	}
	if err := os.MkdirAll(safetyDir, 0o700); err != nil {
		return "", err
	}
	safety := filepath.Join(safetyDir, "pre-restore-"+time.Now().Format("20060102-150405")+".db")
	if err := copyFile(s.dbPath, safety); err != nil {
		return "", fmt.Errorf("safety copy: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	// Delete children before parents.
	delOrder := reverse(logicalTables)
	for _, t := range delOrder {
		if _, ok := b.Tables[t]; !ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t); err != nil {
			return "", fmt.Errorf("clear %s: %w", t, err)
		}
	}
	for _, t := range logicalTables {
		rows, ok := b.Tables[t]
		if !ok {
			continue
		}
		if err := loadRows(ctx, tx, t, rows); err != nil {
			return "", fmt.Errorf("load %s: %w", t, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return safety, nil
}

func loadRows(ctx context.Context, tx *sql.Tx, table string, rows []map[string]any) error {
	for _, row := range rows {
		cols := make([]string, 0, len(row))
		for k := range row {
			cols = append(cols, k)
		}
		sort.Strings(cols)
		placeholders := make([]string, len(cols))
		args := make([]any, len(cols))
		for i, c := range cols {
			placeholders[i] = "?"
			v := row[c]
			if m, ok := v.(map[string]any); ok {
				if hexs, ok := m["__bin__"].(string); ok {
					v = []byte(decodeHex(hexs))
				}
			}
			args[i] = normalize(v)
		}
		q := fmt.Sprintf(`INSERT INTO %s (%s) VALUES(%s)`,
			table, joinCols(cols), joinStr(placeholders, ","))
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}
	}
	return nil
}

func normalize(v any) any {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	default:
		return v
	}
}

func decodeHex(s string) []byte {
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var b byte
		for j := 0; j < 2; j++ {
			c := s[i*2+j]
			b <<= 4
			switch {
			case c >= '0' && c <= '9':
				b |= c - '0'
			case c >= 'a' && c <= 'f':
				b |= c - 'a' + 10
			}
		}
		out[i] = b
	}
	return out
}

func joinCols(cols []string) string { return joinStr(cols, ",") }

func joinStr(ss []string, sep string) string {
	var b bytes.Buffer
	for i, s := range ss {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(s)
	}
	return b.String()
}

func reverse(in []string) []string {
	out := make([]string, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
