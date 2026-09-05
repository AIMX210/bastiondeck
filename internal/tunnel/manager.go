// Package tunnel manages local and remote SSH port forwards with persisted
// state and explicit start/stop. It deliberately never auto-restarts tunnels
// after a daemon restart: stale "active" rows are marked stopped and the
// operator must re-enable them (predictable behaviour over magic).
package tunnel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"bastiondeck/internal/sshlite"
	"bastiondeck/internal/store"
)

// Spec describes a requested forward.
type Spec struct {
	ID         string `json:"id"`
	HostID     string `json:"hostId"`
	Kind       string `json:"kind"` // local | remote
	LocalHost  string `json:"localHost"`
	LocalPort  int    `json:"localPort"`
	RemoteHost string `json:"remoteHost"`
	RemotePort int    `json:"remotePort"`
	StartedBy  string `json:"startedBy,omitempty"`
}

// View is the runtime + persisted tunnel state.
type View struct {
	Spec
	Status    string `json:"status"`
	StartedAt string `json:"startedAt,omitempty"`
	StoppedAt string `json:"stoppedAt,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

// Manager runs live tunnels.
type Manager struct {
	db   *sql.DB
	pool *sshlite.Pool

	mu   sync.Mutex
	live map[string]*live
}

type live struct {
	view   View
	cancel context.CancelFunc
	l      net.Listener
	done   chan struct{}
	errMu  sync.Mutex
	err    error
}

// New constructs the manager.
func New(db *sql.DB, pool *sshlite.Pool) *Manager {
	return &Manager{db: db, pool: pool, live: map[string]*live{}}
}

// Recover marks previously-running tunnels as stopped at startup.
func (m *Manager) Recover(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx,
		`UPDATE tunnels SET status='stopped',stopped_at=? WHERE status IN ('starting','active')`, store.Now())
	return err
}

// Create persists a tunnel spec and starts forwarding.
func (m *Manager) Create(ctx context.Context, s Spec) (*View, error) {
	if s.Kind != "local" && s.Kind != "remote" {
		return nil, errors.New("kind must be local or remote")
	}
	if s.LocalHost == "" {
		s.LocalHost = "127.0.0.1"
	}
	if s.RemoteHost == "" {
		s.RemoteHost = "127.0.0.1"
	}
	if s.LocalPort < 1 || s.LocalPort > 65535 || s.RemotePort < 1 || s.RemotePort > 65535 {
		return nil, errors.New("ports must be 1..65535")
	}
	s.ID = store.NewID(store.PrefixTunnel)
	now := store.Now()
	if _, err := m.db.ExecContext(ctx, `INSERT INTO tunnels
        (id,host_id,kind,local_host,local_port,remote_host,remote_port,status,started_at,started_by)
        VALUES(?,?,?,?,?,?,?,'starting',?,?)`,
		s.ID, s.HostID, s.Kind, s.LocalHost, s.LocalPort, s.RemoteHost, s.RemotePort, now, s.StartedBy); err != nil {
		return nil, err
	}
	v := View{Spec: s, Status: "starting", StartedAt: now}
	if err := m.start(ctx, &v); err != nil {
		m.markStopped(ctx, s.ID, err)
		return nil, err
	}
	return m.Get(ctx, s.ID)
}

func (m *Manager) start(ctx context.Context, v *View) error {
	cli, err := m.pool.Connect(ctx, v.HostID)
	if err != nil {
		return err
	}
	sshCli := cli.(*sshlite.Client).SSH()
	runCtx, cancel := context.WithCancel(context.Background())
	lt := &live{view: *v, cancel: cancel, done: make(chan struct{})}

	switch v.Kind {
	case "local":
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", v.LocalHost, v.LocalPort))
		if err != nil {
			cancel()
			return fmt.Errorf("local listen: %w", err)
		}
		lt.l = ln
		go m.serveLocal(runCtx, lt, sshCli)
	case "remote":
		ln, err := sshCli.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", v.RemotePort))
		if err != nil {
			cancel()
			return fmt.Errorf("remote listen on host: %w", err)
		}
		lt.l = ln
		go m.serveRemote(runCtx, lt)
	}
	m.mu.Lock()
	m.live[v.ID] = lt
	m.mu.Unlock()
	_, _ = m.db.ExecContext(ctx, `UPDATE tunnels SET status='active' WHERE id=?`, v.ID)
	lt.view.Status = "active"
	return nil
}

// serveLocal accepts local connections and dials through SSH.
func (m *Manager) serveLocal(ctx context.Context, lt *live, dialer interface {
	Dial(network, addr string) (net.Conn, error)
}) {
	defer close(lt.done)
	go func() {
		<-ctx.Done()
		_ = lt.l.Close()
	}()
	for {
		local, err := lt.l.Accept()
		if err != nil {
			if ctx.Err() == nil {
				lt.setErr(err)
			}
			return
		}
		go m.bridge(ctx, local, func() (net.Conn, error) {
			return dialer.Dial("tcp", fmt.Sprintf("%s:%d", lt.view.RemoteHost, lt.view.RemotePort))
		})
	}
}

// serveRemote accepts server-side forwarded connections and dials the local target.
func (m *Manager) serveRemote(ctx context.Context, lt *live) {
	defer close(lt.done)
	go func() {
		<-ctx.Done()
		_ = lt.l.Close()
	}()
	for {
		remote, err := lt.l.Accept()
		if err != nil {
			if ctx.Err() == nil {
				lt.setErr(err)
			}
			return
		}
		go m.bridge(ctx, remote, func() (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", lt.view.LocalHost, lt.view.LocalPort))
		})
	}
}

// bridge pumps bytes in both directions until either side closes.
func (m *Manager) bridge(ctx context.Context, a net.Conn, dial func() (net.Conn, error)) {
	defer a.Close()
	b, err := dial()
	if err != nil {
		return
	}
	defer b.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (lt *live) setErr(err error) {
	lt.errMu.Lock()
	lt.err = err
	lt.errMu.Unlock()
}

// Stop terminates a tunnel.
func (m *Manager) Stop(ctx context.Context, id string) error {
	m.mu.Lock()
	lt, ok := m.live[id]
	m.mu.Unlock()
	if ok {
		lt.cancel()
		_ = lt.l.Close()
		select {
		case <-lt.done:
		case <-time.After(2 * time.Second):
		}
		m.mu.Lock()
		delete(m.live, id)
		m.mu.Unlock()
	}
	return m.markStopped(ctx, id, nil)
}

func (m *Manager) markStopped(ctx context.Context, id string, cause error) error {
	errText := ""
	if cause != nil {
		errText = cause.Error()
	}
	_, err := m.db.ExecContext(ctx,
		`UPDATE tunnels SET status='stopped',stopped_at=? WHERE id=?`, store.Now(), id)
	_ = errText
	return err
}

// Get returns a view (live error if any).
func (m *Manager) Get(ctx context.Context, id string) (*View, error) {
	row := m.db.QueryRowContext(ctx, `SELECT id,host_id,kind,local_host,local_port,remote_host,
        remote_port,status,COALESCE(started_at,''),COALESCE(stopped_at,''),COALESCE(started_by,'')
        FROM tunnels WHERE id=?`, id)
	var v View
	if err := row.Scan(&v.ID, &v.HostID, &v.Kind, &v.LocalHost, &v.LocalPort, &v.RemoteHost,
		&v.RemotePort, &v.Status, &v.StartedAt, &v.StoppedAt, &v.StartedBy); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if lt, ok := m.live[id]; ok {
		lt.errMu.Lock()
		if lt.err != nil {
			v.LastError = lt.err.Error()
		}
		lt.errMu.Unlock()
	}
	m.mu.Unlock()
	return &v, nil
}

// List returns all persisted tunnels with runtime status.
func (m *Manager) List(ctx context.Context) ([]View, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id,host_id,kind,local_host,local_port,remote_host,
        remote_port,status,COALESCE(started_at,''),COALESCE(stopped_at,''),COALESCE(started_by,'')
        FROM tunnels ORDER BY started_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []View
	for rows.Next() {
		var v View
		if err := rows.Scan(&v.ID, &v.HostID, &v.Kind, &v.LocalHost, &v.LocalPort, &v.RemoteHost,
			&v.RemotePort, &v.Status, &v.StartedAt, &v.StoppedAt, &v.StartedBy); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// StopAll shuts every live tunnel down (graceful shutdown).
func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.live))
	for id := range m.live {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, id := range ids {
		_ = m.Stop(ctx, id)
	}
}
