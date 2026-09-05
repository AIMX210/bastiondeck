package jobs_test

import (
	"context"
	"testing"
	"time"

	"bastiondeck/internal/jobs"
	"bastiondeck/internal/testutil"
)

func TestSchedulerSeedsNextRunWithoutFiring(t *testing.T) {
	eng, repo, h := newEngine(t)
	sched := jobs.NewScheduler(eng, repo)
	srv := testutil.NewFakeSSH(t, "pw", "", func(string) ([]byte, []byte, int) {
		return []byte("ok"), nil, 0
	})
	defer srv.Close()
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	hst := h.MustHost("a", addr, port, "tester", cred)

	job, err := repo.CreateJob(context.Background(), jobs.JobInput{
		Name: "every-minute", Kind: "scheduled", Command: "echo ok",
		TargetIDs: []string{hst.ID}, ScheduleExpr: "* * * * *",
		Enabled: true, TimeoutMs: 3000, Concurrency: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 5, 13, 30, 0, 0, time.UTC)
	started, err := sched.Tick(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != 0 {
		t.Fatalf("first tick must only seed next_run, started=%v", started)
	}
	got, err := repo.GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextRunAt == nil {
		t.Fatal("next_run_at must be seeded")
	}
	if got.LastRunAt != nil {
		t.Fatal("last_run_at must stay nil before first fire")
	}
}

func TestSchedulerFiresDueJobAndAdvances(t *testing.T) {
	eng, repo, h := newEngine(t)
	sched := jobs.NewScheduler(eng, repo)
	srv := testutil.NewFakeSSH(t, "pw", "", func(string) ([]byte, []byte, int) {
		return []byte("ok"), nil, 0
	})
	defer srv.Close()
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	hst := h.MustHost("a", addr, port, "tester", cred)

	job, err := repo.CreateJob(context.Background(), jobs.JobInput{
		Name: "due", Kind: "scheduled", Command: "echo ok",
		TargetIDs: []string{hst.ID}, ScheduleExpr: "* * * * *",
		Enabled: true, TimeoutMs: 3000, Concurrency: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 5, 13, 30, 0, 0, time.UTC)
	// Seed.
	if _, err := sched.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	// Two minutes later: due.
	started, err := sched.Tick(context.Background(), now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 {
		t.Fatalf("want exactly one run started, got %v", started)
	}
	waitTerminal(t, repo, started[0])
	got, _ := repo.GetJob(context.Background(), job.ID)
	if got.LastRunAt == nil {
		t.Fatal("last_run_at must be recorded")
	}
	// Next fire is in the future again: immediate second tick must not refire.
	again, err := sched.Tick(context.Background(), now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("job must not double-fire, got %v", again)
	}
}

func TestSchedulerSkipsDisabledAndManual(t *testing.T) {
	eng, repo, _ := newEngine(t)
	sched := jobs.NewScheduler(eng, repo)
	ctx := context.Background()
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	mk := func(name, kind string, enabled bool) string {
		j, err := repo.CreateJob(ctx, jobs.JobInput{
			Name: name, Kind: kind, Command: "echo x", ScheduleExpr: "* * * * *",
			Enabled: enabled, TimeoutMs: 1000, Concurrency: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		next := time.Date(2020, 1, 1, 0, 1, 0, 0, time.UTC).Format(time.RFC3339Nano)
		if err := repo.SetRunTimes(ctx, j.ID, &past, &next); err != nil {
			t.Fatal(err)
		}
		return j.ID
	}
	mk("disabled", "scheduled", false)
	mk("oneshot", "oneshot", true)
	started, err := sched.Tick(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != 0 {
		t.Fatalf("disabled and manual jobs never fire, got %v", started)
	}
}

func TestRecomputeNextRejectsBadExpr(t *testing.T) {
	eng, repo, _ := newEngine(t)
	sched := jobs.NewScheduler(eng, repo)
	j, err := repo.CreateJob(context.Background(), jobs.JobInput{
		Name: "bad", Kind: "scheduled", Command: "x", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sched.RecomputeNext(context.Background(), j.ID, "not a cron", time.Now()); err == nil {
		t.Fatal("bad cron expression must be rejected")
	}
}
