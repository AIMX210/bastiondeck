// Package migrations embeds SQL migration files and applies them in order
// inside a single transaction per migration. A failed migration is rolled
// back and never partially recorded, matching the charter's reliability
// requirements.
package migrations

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed *.sql
var FS embed.FS

// CurrentVersion is the highest migration version shipped with this binary.
func CurrentVersion() (int, error) {
	names, err := FS.ReadDir(".")
	if err != nil {
		return 0, err
	}
	var maxV int
	for _, n := range names {
		var v int
		if _, err := fmt.Sscanf(n.Name(), "%d_", &v); err == nil && v > maxV {
			maxV = v
		}
	}
	return maxV, nil
}

// Applyer abstracts the database surface needed to migrate. *sql.DB and
// *sql.Tx both satisfy it.
type Applyer interface {
	Exec(query string, args ...any) (any, error)
}

// PlannedMigration pairs an ordered version with its SQL body.
type PlannedMigration struct {
	Version int
	Name    string
	SQL     string
}

// List returns all embedded migrations ordered by version number.
func List() ([]PlannedMigration, error) {
	entries, err := FS.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var out []PlannedMigration
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		var v int
		if _, err := fmt.Sscanf(name, "%d_", &v); err != nil {
			return nil, fmt.Errorf("migration %q has invalid name: %w", name, err)
		}
		body, err := FS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		out = append(out, PlannedMigration{Version: v, Name: name, SQL: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// StampUTC is a small indirection so tests can freeze migration timestamps.
func StampUTC() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }
