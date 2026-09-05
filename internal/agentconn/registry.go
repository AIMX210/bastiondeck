// Package agentconn manages optional bd-agent reverse connections: enrollment
// secrets, approval workflow, an in-memory live-conn registry and the adapter
// that presents an agent as a connector.Client (parallel to the SSH backend).
package agentconn

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"bastiondeck/internal/store"
)

// View is the agent record.
type View struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	RegisteredAt *string         `json:"registeredAt,omitempty"`
	LastSeenAt   *string         `json:"lastSeenAt,omitempty"`
	Version      string          `json:"version,omitempty"`
	Status       string          `json:"status"`
	Facts        json.RawMessage `json:"facts,omitempty"`
	Online       bool            `json:"online"`
}

// Registry persists agents and tracks live connections.
type Registry struct {
	db *sql.DB

	mu       sync.Mutex
	live     map[string]*LiveConn
	sessions map[string]*connState
}

// LiveConn is a connected agent's messaging surface (filled by protocol.go).
type LiveConn struct {
	AgentID string
	Version string
	Since   time.Time
	Send    func(msg any) error
}

// New constructs the registry.
func New(db *sql.DB) *Registry {
	return &Registry{db: db, live: map[string]*LiveConn{}, sessions: map[string]*connState{}}
}

func (r *Registry) setSession(id string, st *connState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st == nil {
		delete(r.sessions, id)
	} else {
		r.sessions[id] = st
	}
}

func (r *Registry) session(id string) (*connState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.sessions[id]
	return st, ok
}

// Enroll creates an agent and returns the one-time plaintext enrollment secret.
func (r *Registry) Enroll(ctx context.Context, name string) (id, secret string, err error) {
	if name == "" {
		return "", "", errors.New("agent name required")
	}
	id = store.NewID(store.PrefixAgent)
	secret = store.RandomToken(24)
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO agents(id,name,enroll_secret_hash,status) VALUES(?,?,?,?)`,
		id, name, store.HashToken(secret), "pending")
	return id, secret, err
}

// Authenticate matches an enrollment secret at connect time.
func (r *Registry) Authenticate(ctx context.Context, secret string) (*View, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id,name,status FROM agents WHERE enroll_secret_hash=?`, store.HashToken(secret))
	var v View
	if err := row.Scan(&v.ID, &v.Name, &v.Status); err != nil {
		return nil, errors.New("invalid enrollment secret")
	}
	if v.Status == "blocked" {
		return nil, errors.New("agent blocked")
	}
	return &v, nil
}

// SetStatus approves/blocks an agent.
func (r *Registry) SetStatus(ctx context.Context, id, status string) error {
	if status != "approved" && status != "blocked" && status != "pending" {
		return errors.New("bad status")
	}
	_, err := r.db.ExecContext(ctx, `UPDATE agents SET status=? WHERE id=?`, status, id)
	return err
}

// MarkRegistered stamps first registration.
func (r *Registry) MarkRegistered(ctx context.Context, id, version string) error {
	now := store.Now()
	_, err := r.db.ExecContext(ctx,
		`UPDATE agents SET registered_at=COALESCE(registered_at,?),last_seen_at=?,version=? WHERE id=?`,
		now, now, version, id)
	return err
}

// Touch updates last-seen.
func (r *Registry) Touch(ctx context.Context, id string) {
	_, _ = r.db.ExecContext(ctx, `UPDATE agents SET last_seen_at=? WHERE id=?`, store.Now(), id)
}

// SaveFacts upserts facts JSON.
func (r *Registry) SaveFacts(ctx context.Context, id string, facts any) error {
	b, err := json.Marshal(facts)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE agents SET facts_json=? WHERE id=?`, string(b), id)
	return err
}

// List returns all agents with online state.
func (r *Registry) List(ctx context.Context) ([]View, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,name,registered_at,last_seen_at,COALESCE(version,''),status,facts_json FROM agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []View
	for rows.Next() {
		var v View
		var reg, seen sql.NullString
		var facts string
		if err := rows.Scan(&v.ID, &v.Name, &reg, &seen, &v.Version, &v.Status, &facts); err != nil {
			return nil, err
		}
		if reg.Valid {
			v.RegisteredAt = &reg.String
		}
		if seen.Valid {
			v.LastSeenAt = &seen.String
		}
		v.Facts = json.RawMessage(facts)
		r.mu.Lock()
		_, v.Online = r.live[v.ID]
		r.mu.Unlock()
		out = append(out, v)
	}
	return out, rows.Err()
}

// SetLive registers/removes a live connection.
func (r *Registry) SetLive(id string, conn *LiveConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if conn == nil {
		delete(r.live, id)
	} else {
		r.live[id] = conn
	}
}

// Available reports an approved and connected agent.
func (r *Registry) Available(id string) bool {
	r.mu.Lock()
	_, online := r.live[id]
	r.mu.Unlock()
	if !online {
		return false
	}
	var status string
	err := r.db.QueryRow(`SELECT status FROM agents WHERE id=?`, id).Scan(&status)
	return err == nil && status == "approved"
}

// GetLive returns a live connection.
func (r *Registry) GetLive(id string) (*LiveConn, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.live[id]
	return c, ok
}
