package audit_test

import (
	"context"
	"testing"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/testutil"
)

func newSvc(t *testing.T) (*audit.Service, func()) {
	h := testutil.NewHarness(t)
	return audit.New(h.Store.DB), func() { _ = h.Store.Close() }
}

func TestChainLinksAndVerifies(t *testing.T) {
	s, closeFn := newSvc(t)
	defer closeFn()
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if _, err := s.Write(ctx, audit.Actor{ID: "u", Name: "alice"}, "test.action", "thing", "id", "success",
			map[string]any{"i": i}); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := s.Verify(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Checked != 10 {
		t.Fatalf("chain verify failed: %+v", rep)
	}
}

func TestTamperBreaksChain(t *testing.T) {
	h := testutil.NewHarness(t)
	s := audit.New(h.Store.DB)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _ = s.Write(ctx, audit.Actor{Name: "a"}, "a", "", "", "success", nil)
	}
	if _, err := h.Store.DB.ExecContext(ctx, `UPDATE audit_logs SET action='hacked' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	rep, err := s.Verify(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || rep.BrokenAt != 2 {
		t.Fatalf("expected break at 2, got %+v", rep)
	}
}

func TestListPagination(t *testing.T) {
	s, closeFn := newSvc(t)
	defer closeFn()
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, _ = s.Write(ctx, audit.Actor{Name: "bob"}, "login", "", "", "success", nil)
	}
	page1, next, err := s.ListPage(ctx, 2, 0, audit.Filter{})
	if err != nil || len(page1) != 2 || next == 0 {
		t.Fatalf("first page wrong: %d next=%d err=%v", len(page1), next, err)
	}
	page2, _, err := s.ListPage(ctx, 2, next, audit.Filter{})
	if err != nil || len(page2) != 2 {
		t.Fatalf("second page wrong: %d err=%v", len(page2), err)
	}
	if page1[0].Seq <= page2[0].Seq {
		t.Fatal("expected newest-first ordering")
	}
}
