package jobs

import (
	"context"
	"time"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/schedule"
)

// Scheduler ticks enabled scheduled jobs and starts runs when due. It never
// double-starts: a due job whose next_run_at is advanced atomically before
// launching.
type Scheduler struct {
	engine *Engine
	repo   *Repo
	actor  audit.Actor
}

// NewScheduler constructs the scheduler.
func NewScheduler(e *Engine, r *Repo) *Scheduler {
	return &Scheduler{engine: e, repo: r, actor: audit.Actor{ID: "system", Name: "scheduler"}}
}

// RecomputeNext sets next_run_at for a job from its cron expression.
func (s *Scheduler) RecomputeNext(ctx context.Context, jobID, expr string, from time.Time) error {
	parsed, err := schedule.Parse(expr)
	if err != nil {
		return err
	}
	next := parsed.NextAfter(from).UTC().Format(time.RFC3339Nano)
	return s.repo.SetRunTimes(ctx, jobID, nil, &next)
}

// Tick performs one scheduling pass. It is invoked every minute by the daemon.
func (s *Scheduler) Tick(ctx context.Context, now time.Time) (started []string, err error) {
	jobs, err := s.repo.ListJobs(ctx)
	if err != nil {
		return nil, err
	}
	for _, j := range jobs {
		if !j.Enabled || j.Kind != "scheduled" || j.ScheduleExpr == "" {
			continue
		}
		parsed, perr := schedule.Parse(j.ScheduleExpr)
		if perr != nil {
			continue
		}
		due := false
		if j.NextRunAt == nil {
			next := parsed.NextAfter(now).UTC().Format(time.RFC3339Nano)
			_ = s.repo.SetRunTimes(ctx, j.ID, nil, &next)
			continue
		}
		t, perr := time.Parse(time.RFC3339Nano, *j.NextRunAt)
		if perr == nil && !t.After(now) {
			due = true
		}
		if !due {
			continue
		}
		next := parsed.NextAfter(now)
		nextStr := next.UTC().Format(time.RFC3339Nano)
		lastStr := now.UTC().Format(time.RFC3339Nano)
		// Advance bookkeeping BEFORE starting to prevent double-fire.
		if err := s.repo.SetRunTimes(ctx, j.ID, &lastStr, &nextStr); err != nil {
			continue
		}
		runID, err := s.engine.StartRun(ctx, StartInput{
			JobID: j.ID, Command: j.Command, TargetIDs: j.TargetIDs,
			Timeout:     time.Duration(j.TimeoutMs) * time.Millisecond,
			Concurrency: j.Concurrency, Trigger: "schedule", Actor: s.actor,
		})
		if err == nil {
			started = append(started, runID)
		}
	}
	return started, nil
}

// Run blocks, ticking every minute until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			_, _ = s.Tick(ctx, now)
		}
	}
}
