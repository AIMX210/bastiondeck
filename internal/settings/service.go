// Package settings is a typed key-value store for instance preferences with
// documented defaults.
package settings

import (
	"context"
	"database/sql"
	"strconv"

	"bastiondeck/internal/store"
)

// Defaults applied when a key is absent.
var Defaults = map[string]string{
	"session.ttlMinutes":     "720",
	"audit.retentionDays":    "365",
	"metrics.enabled":        "true",
	"metrics.intervalSec":    "60",
	"exec.defaultTimeoutSec": "60",
	"exec.maxConcurrency":    "20",
	"theme.default":          "system",
}

// Service wraps the settings table.
type Service struct{ db *sql.DB }

// New constructs the service.
func New(db *sql.DB) *Service { return &Service{db: db} }

// Get returns the stored value or default.
func (s *Service) Get(key string) string {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return Defaults[key]
	}
	if err != nil {
		return Defaults[key]
	}
	return v
}

// GetInt parses an integer setting.
func (s *Service) GetInt(key string, def int) int {
	v, err := strconv.Atoi(s.Get(key))
	if err != nil || v <= 0 {
		return def
	}
	return v
}

// GetBool parses a boolean setting.
func (s *Service) GetBool(key string, def bool) bool {
	v := s.Get(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// Set upserts a value.
func (s *Service) Set(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings(key,value,updated_at) VALUES(?,?,?)
         ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`,
		key, value, store.Now())
	return err
}

// All returns effective settings (defaults merged with overrides).
func (s *Service) All() map[string]string {
	out := map[string]string{}
	for k, v := range Defaults {
		out[k] = v
	}
	rows, err := s.db.Query(`SELECT key,value FROM settings`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		_ = rows.Scan(&k, &v)
		out[k] = v
	}
	return out
}
