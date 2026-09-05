package jobs_test

import (
	"context"
	"testing"
	"time"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/connector"
	"bastiondeck/internal/jobs"
	"bastiondeck/internal/realtime"
	"bastiondeck/internal/sshlite"
	"bastiondeck/internal/testutil"
)

func newEngine(t *testing.T) (*jobs.Engine, *jobs.Repo, *testutil.Harness) {
	h := testutil.NewHarness(t)
	dialer := &sshlite.Dialer{Hosts: h.Hosts, Creds: h.Creds, DialTimeout: 3 * time.Second}
	pool := sshlite.NewPool(dialer, time.Minute)
	t.Cleanup(pool.CloseAll)
	mgr := &connector.Manager{Hosts: h.Hosts, SSH: pool}
	repo := jobs.NewRepo(h.Store.DB)
	hub := realtime.NewHub()
	eng := jobs.NewEngine(repo, h.Store.DB, mgr, hub, audit.New(h.Store.DB), h.DataDir, 1<<20)
	return eng, repo, h
}

func waitTerminal(t *testing.T, repo *jobs.Repo, runID string) *jobs.Run {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		run, err := repo.GetRun(context.Background(), runID)
		if err != nil {
			t.Fatal(err)
		}
		if jobs.IsTerminal(run.Status) {
			return run
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("run did not finish in time")
	return nil
}

func TestRunSuccessAcrossHosts(t *testing.T) {
	eng, repo, h := newEngine(t)
	srv := testutil.NewFakeSSH(t, "pw", "", func(string) ([]byte, []byte, int) {
		return []byte("ok\n"), nil, 0
	})
	defer srv.Close()
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	h1 := h.MustHost("a", addr, port, "tester", cred)
	h2 := h.MustHost("b", addr, port, "tester", cred)
	runID, err := eng.StartRun(context.Background(), jobs.StartInput{
		Command: "echo ok", TargetIDs: []string{h1.ID, h2.ID}, Concurrency: 2,
		Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	run := waitTerminal(t, repo, runID)
	if run.Status != jobs.StatusSuccess {
		t.Fatalf("want success got %s: %+v", run.Status, run.Targets)
	}
	if run.Summary.Success != 2 {
		t.Fatalf("summary %+v", run.Summary)
	}
}

func TestRunMixedFailure(t *testing.T) {
	eng, repo, h := newEngine(t)
	srv := testutil.NewFakeSSH(t, "pw", "", func(cmd string) ([]byte, []byte, int) {
		if cmd == "bad" {
			return nil, []byte("nope"), 2
		}
		return []byte("yes"), nil, 0
	})
	defer srv.Close()
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	good := h.MustHost("good", addr, port, "tester", cred)
	bad := h.MustHost("bad", addr, port, "tester", cred)
	runID, _ := eng.StartRun(context.Background(), jobs.StartInput{
		Command: "bad", TargetIDs: []string{good.ID, bad.ID}, Concurrency: 2, Timeout: 3 * time.Second})
	run := waitTerminal(t, repo, runID)
	if run.Status != jobs.StatusFailed {
		t.Fatalf("want failed got %s", run.Status)
	}
	var failed int
	for _, tg := range run.Targets {
		if tg.Status == jobs.StatusFailed {
			failed++
			if tg.ExitCode == nil || *tg.ExitCode != 2 {
				t.Fatalf("exit code not captured: %+v", tg)
			}
		}
	}
	if failed != 2 { // both ran "bad"
		t.Fatalf("want 2 failed targets got %d", failed)
	}
}

func TestUnreachableHostIsLost(t *testing.T) {
	eng, repo, h := newEngine(t)
	// Reserve a closed port.
	ln := mustListen(t)
	addr, port, _ := splitAddr(ln, t)
	_ = ln.Close()
	cred := h.MustCredential("c", "pw")
	host := h.MustHost("ghost", addr, port, "tester", cred)
	runID, err := eng.StartRun(context.Background(), jobs.StartInput{
		Command: "x", TargetIDs: []string{host.ID}, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	run := waitTerminal(t, repo, runID)
	if run.Status != jobs.StatusLost {
		t.Fatalf("want lost got %s", run.Status)
	}
}

func TestCancelRun(t *testing.T) {
	eng, repo, h := newEngine(t)
	srv := testutil.NewFakeSSH(t, "pw", "", func(string) ([]byte, []byte, int) {
		time.Sleep(800 * time.Millisecond)
		return []byte("late"), nil, 0
	})
	defer srv.Close()
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	h1 := h.MustHost("a", addr, port, "tester", cred)
	h2 := h.MustHost("b", addr, port, "tester", cred)
	runID, err := eng.StartRun(context.Background(), jobs.StartInput{
		Command: "slow", TargetIDs: []string{h1.ID, h2.ID}, Concurrency: 1, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := eng.CancelRun(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	run := waitTerminal(t, repo, runID)
	if run.Status != jobs.StatusCancelled {
		t.Fatalf("want cancelled got %s", run.Status)
	}
}

func TestReconcileMarksOrphansLost(t *testing.T) {
	eng, repo, h := newEngine(t)
	srv := testutil.NewFakeSSH(t, "pw", "", func(string) ([]byte, []byte, int) {
		return []byte("ok"), nil, 0
	})
	defer srv.Close()
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	host := h.MustHost("a", addr, port, "tester", cred)
	runID, _ := eng.StartRun(context.Background(), jobs.StartInput{
		Command: "echo ok", TargetIDs: []string{host.ID}, Timeout: 3 * time.Second})
	_ = waitTerminal(t, repo, runID)
	// A fresh engine (simulating daemon restart) sees no live runs.
	eng2 := jobs.NewEngine(repo, h.Store.DB, nil, realtime.NewHub(), audit.New(h.Store.DB), h.DataDir, 1<<20)
	// Insert an orphaned running target directly.
	run2, _ := eng2StartStale(t, repo, h, host.ID)
	n, err := eng2.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected orphans reconciled")
	}
	r, _ := repo.GetRun(context.Background(), run2)
	if r.Status != jobs.StatusLost {
		t.Fatalf("orphan run should be lost, got %s", r.Status)
	}
}
