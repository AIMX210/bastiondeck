package agentconn

import (
	"context"
	"testing"

	"bastiondeck/internal/testutil"
)

func newReg(t *testing.T) *Registry {
	t.Helper()
	h := testutil.NewHarness(t)
	return New(h.Store.DB)
}

func TestEnrollAuthenticate(t *testing.T) {
	r := newReg(t)
	ctx := context.Background()
	id, secret, err := r.Enroll(ctx, "node-a")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || len(secret) < 16 {
		t.Fatalf("bad enrollment id=%q secret-len=%d", id, len(secret))
	}
	// The enrollment secret doubles as the persistent reconnect credential
	// (it is shown only once in the UI, but remains valid for reconnects).
	v, err := r.Authenticate(ctx, secret)
	if err != nil {
		t.Fatalf("first auth: %v", err)
	}
	if v.ID != id || v.Name != "node-a" {
		t.Fatalf("view = %+v", v)
	}
	if _, err := r.Authenticate(ctx, secret); err != nil {
		t.Fatalf("secret must survive reconnect: %v", err)
	}
	// Garbage secret fails.
	if _, err := r.Authenticate(ctx, "not-a-secret"); err == nil {
		t.Fatal("garbage secret must fail")
	}
	// Empty secret fails.
	if _, err := r.Authenticate(ctx, ""); err == nil {
		t.Fatal("empty secret must fail")
	}
}

func TestBlockedAgentRejected(t *testing.T) {
	r := newReg(t)
	ctx := context.Background()
	id, secret, _ := r.Enroll(ctx, "z")
	if err := r.SetStatus(ctx, id, "blocked"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Authenticate(ctx, secret); err == nil {
		t.Fatal("blocked agent must be rejected at handshake")
	}
}

func TestApprovalGating(t *testing.T) {
	r := newReg(t)
	ctx := context.Background()
	id, _, err := r.Enroll(ctx, "node-b")
	if err != nil {
		t.Fatal(err)
	}
	// Pending agents are never "available".
	if r.Available(id) {
		t.Fatal("pending agent cannot be available")
	}
	if err := r.SetStatus(ctx, id, "blocked"); err != nil {
		t.Fatal(err)
	}
	if err := r.SetStatus(ctx, id, "approved"); err != nil {
		t.Fatal(err)
	}
	// Approved but no live connection: still unavailable.
	if r.Available(id) {
		t.Fatal("approved without live session cannot be available")
	}
	r.SetLive(id, &LiveConn{AgentID: id})
	if !r.Available(id) {
		t.Fatal("approved + live must be available")
	}
	// Blocking must immediately revoke availability even with a live conn.
	if err := r.SetStatus(ctx, id, "blocked"); err != nil {
		t.Fatal(err)
	}
	if r.Available(id) {
		t.Fatal("blocked agent must not be available")
	}
}

func TestMarkRegisteredAndFacts(t *testing.T) {
	r := newReg(t)
	ctx := context.Background()
	id, _, _ := r.Enroll(ctx, "node-c")
	_ = r.SetStatus(ctx, id, "approved")
	if err := r.MarkRegistered(ctx, id, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	r.Touch(context.Background(), id)
	if err := r.SaveFacts(ctx, id, map[string]any{"hostname": "c"}); err != nil {
		t.Fatal(err)
	}
	list, err := r.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Version != "1.2.3" {
		t.Fatalf("list = %+v", list)
	}
}
