package tunnel_test

import (
	"context"
	"testing"
	"time"

	"bastiondeck/internal/connector"
	"bastiondeck/internal/sshlite"
	"bastiondeck/internal/testutil"
	"bastiondeck/internal/tunnel"
)

func newManager(t *testing.T) (*tunnel.Manager, *testutil.Harness) {
	t.Helper()
	h := testutil.NewHarness(t)
	dialer := &sshlite.Dialer{Hosts: h.Hosts, Creds: h.Creds, DialTimeout: time.Second}
	pool := sshlite.NewPool(dialer, time.Minute)
	t.Cleanup(pool.CloseAll)
	_ = &connector.Manager{Hosts: h.Hosts, SSH: pool}
	return tunnel.New(h.Store.DB, pool), h
}

func TestCreateRejectsBadKind(t *testing.T) {
	m, h := newManager(t)
	srv := testutil.NewFakeSSH(t, "pw", "", nil)
	defer srv.Close()
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	hst := h.MustHost("a", addr, port, "tester", cred)
	_, err := m.Create(context.Background(), tunnel.Spec{
		HostID: hst.ID, Kind: "sideways", LocalPort: 2200, RemotePort: 80,
	})
	if err == nil {
		t.Fatal("bad kind must be rejected")
	}
}

func TestCreateRejectsBadPorts(t *testing.T) {
	m, h := newManager(t)
	srv := testutil.NewFakeSSH(t, "pw", "", nil)
	defer srv.Close()
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	hst := h.MustHost("a", addr, port, "tester", cred)
	bad := []tunnel.Spec{
		{HostID: hst.ID, Kind: "local", LocalPort: 0, RemotePort: 80},
		{HostID: hst.ID, Kind: "local", LocalPort: 70000, RemotePort: 80},
		{HostID: hst.ID, Kind: "remote", LocalPort: 2200, RemotePort: -1},
	}
	for i, s := range bad {
		if _, err := m.Create(context.Background(), s); err == nil {
			t.Fatalf("case %d: bad port must be rejected", i)
		}
	}
}

func TestCreateRejectsUnknownHost(t *testing.T) {
	m, _ := newManager(t)
	_, err := m.Create(context.Background(), tunnel.Spec{
		HostID: "hst_missing", Kind: "local", LocalPort: 2200, RemotePort: 80,
	})
	if err == nil {
		t.Fatal("unknown host must fail and not leave an active tunnel")
	}
}

func TestRecoverMarksStale(t *testing.T) {
	m, h := newManager(t)
	ctx := context.Background()
	// Seed a tunnel row that was active when the previous process died.
	_, err := h.Store.DB.ExecContext(ctx, `INSERT INTO tunnels
        (id,host_id,kind,local_host,local_port,remote_host,remote_port,status,started_at)
        VALUES('tun_old','hst_x','local','127.0.0.1',2200,'127.0.0.1',80,'active',?)`, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := h.Store.DB.QueryRowContext(ctx, `SELECT status FROM tunnels WHERE id='tun_old'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "stopped" {
		t.Fatalf("stale tunnel status = %s want stopped", status)
	}
}

func TestListAndGetMissing(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	if _, err := m.Get(ctx, "tun_nope"); err == nil {
		t.Fatal("missing tunnel must error")
	}
	views, err := m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Fatalf("fresh manager has %d views", len(views))
	}
}
