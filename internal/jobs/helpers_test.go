package jobs_test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"

	"bastiondeck/internal/jobs"
	"bastiondeck/internal/store"
	"bastiondeck/internal/testutil"
)

func mustListen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func splitAddr(ln net.Listener, t *testing.T) (string, int, error) {
	t.Helper()
	host, p, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return "", 0, err
	}
	port, _ := strconv.Atoi(p)
	return host, port, nil
}

// eng2StartStale fabricates a run whose target is stuck "running" with no
// live owner, exactly as a restarted daemon would find it.
func eng2StartStale(t *testing.T, repo *jobs.Repo, h *testutil.Harness, hostID string) (string, error) {
	t.Helper()
	ctx := context.Background()
	j, err := repo.CreateJob(ctx, jobs.JobInput{
		Kind: "adhoc", Name: "stale", Command: "x", TargetIDs: []string{hostID}})
	if err != nil {
		return "", err
	}
	now := store.Now()
	run := &jobs.Run{ID: store.NewID(store.PrefixRun), JobID: j.ID, Trigger: "manual",
		Status: jobs.StatusRunning, StartedAt: &now, CreatedAt: now, UpdatedAt: now}
	tgt := jobs.RunTarget{ID: store.NewID(store.PrefixTarget), RunID: run.ID, HostID: hostID,
		Status: jobs.StatusRunning}
	if err := repo.InsertRun(ctx, run, []jobs.RunTarget{tgt}); err != nil {
		return "", err
	}
	// keep the target id referenced through run lookup
	_ = strings.TrimSpace
	return run.ID, nil
}
