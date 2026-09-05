package audit_test

import (
	"context"
	"testing"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/testutil"
)

func writeN(t *testing.T, s *audit.Service, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		if _, err := s.Write(ctx, audit.Actor{ID: "usr_a", Name: "alice"},
			"test.action", "thing", "obj", "success", map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestChainVerifiesWhenIntact(t *testing.T) {
	h := testutil.NewHarness(t)
	writeN(t, h.Audit, 12)
	rep, err := h.Audit.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("intact chain must verify: %+v", rep)
	}
	if rep.Checked != 12 {
		t.Fatalf("checked = %d", rep.Checked)
	}
}

func TestChainDetectsTamperedDetail(t *testing.T) {
	h := testutil.NewHarness(t)
	writeN(t, h.Audit, 5)
	// Tamper with a middle row's detail without recomputing the hash.
	if _, err := h.Store.DB.ExecContext(context.Background(),
		`UPDATE audit_logs SET detail_json=? WHERE id=3`, `{"i":999}`); err != nil {
		t.Fatal(err)
	}
	rep, err := h.Audit.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("tampered chain must fail verification")
	}
	if rep.BrokenAt == 0 {
		t.Fatalf("broken-at position must be reported: %+v", rep)
	}
}

func TestChainDetectsDeletedRow(t *testing.T) {
	h := testutil.NewHarness(t)
	writeN(t, h.Audit, 6)
	if _, err := h.Store.DB.ExecContext(context.Background(),
		`DELETE FROM audit_logs WHERE id=4`); err != nil {
		t.Fatal(err)
	}
	rep, err := h.Audit.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("deleting a row breaks the chain")
	}
}

func TestEmptyChainVerifies(t *testing.T) {
	h := testutil.NewHarness(t)
	rep, err := h.Audit.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Checked != 0 {
		t.Fatalf("empty chain: %+v", rep)
	}
}

func TestListPageFiltersByResult(t *testing.T) {
	h := testutil.NewHarness(t)
	ctx := context.Background()
	_, _ = h.Audit.Write(ctx, audit.Actor{ID: "u", Name: "u"}, "a.good", "o", "1", "success", nil)
	_, _ = h.Audit.Write(ctx, audit.Actor{ID: "u", Name: "u"}, "a.bad", "o", "2", "failure", nil)
	_, _ = h.Audit.Write(ctx, audit.Actor{ID: "u", Name: "u"}, "a.no", "o", "3", "denied", nil)
	rows, _, err := h.Audit.ListPage(ctx, 50, 0, audit.Filter{Result: "failure"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Result != "failure" {
		t.Fatalf("filter failure: rows=%+v", rows)
	}
}

func TestListPagePaginates(t *testing.T) {
	h := testutil.NewHarness(t)
	writeN(t, h.Audit, 8)
	page1, next, err := h.Audit.ListPage(context.Background(), 3, 0, audit.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if next == 0 || len(page1) != 3 {
		t.Fatalf("page1 len=%d next=%d", len(page1), next)
	}
	page2, next2, err := h.Audit.ListPage(context.Background(), 3, next, audit.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 3 {
		t.Fatalf("page2 len=%d", len(page2))
	}
	if page1[0].EventID == page2[0].EventID {
		t.Fatal("pages must not overlap")
	}
	// Walk to the end: 8 rows → pages of 3,3,2 then cursor zero.
	page3, last, err := h.Audit.ListPage(context.Background(), 3, next2, audit.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page3) != 2 || last != 0 {
		t.Fatalf("page3 len=%d last cursor=%d", len(page3), last)
	}
}
