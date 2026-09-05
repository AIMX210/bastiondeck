package jobs_test

import (
	"context"
	"testing"

	"bastiondeck/internal/jobs"
	"bastiondeck/internal/testutil"
)

// Walking nextCursor must return every run exactly once — regression for an
// off-by-one that dropped one row at each page boundary.
func TestListRunsPaginationNoDroppedRows(t *testing.T) {
	eng, repo, h := newEngine(t)
	srv := testutil.NewFakeSSH(t, "pw", "", func(string) ([]byte, []byte, int) {
		return nil, nil, 0
	})
	defer srv.Close()
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	hst := h.MustHost("a", addr, port, "tester", cred)

	const n = 5
	want := map[string]bool{}
	for i := 0; i < n; i++ {
		id, err := eng.StartRun(context.Background(), jobs.StartInput{
			Command: "true", TargetIDs: []string{hst.ID}, Concurrency: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		want[id] = true
	}
	ctx := context.Background()
	seen := map[string]bool{}
	var cursor int64
	pages := 0
	for {
		runs, next, err := repo.ListRuns(ctx, 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range runs {
			if seen[r.ID] {
				t.Fatalf("run %s returned twice", r.ID)
			}
			seen[r.ID] = true
		}
		pages++
		if next == 0 {
			break
		}
		cursor = next
		if pages > n+1 {
			t.Fatal("pagination did not converge")
		}
	}
	if len(seen) != n {
		t.Fatalf("walked %d runs, want %d (dropped rows)", len(seen), n)
	}
}
