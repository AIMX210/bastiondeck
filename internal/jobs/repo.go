package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"bastiondeck/internal/store"
)

// Repo handles jobs persistence.
type Repo struct{ db *sql.DB }

// NewRepo constructs the repo.
func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// JobInput carries create/update fields.
type JobInput struct {
	Name         string
	Command      string
	TargetIDs    []string
	ScheduleExpr string
	Enabled      bool
	TimeoutMs    int
	Concurrency  int
	Kind         string
	CreatedBy    string
}

// CreateJob inserts a job definition.
func (r *Repo) CreateJob(ctx context.Context, in JobInput) (*Job, error) {
	if in.Kind == "" {
		in.Kind = "oneshot"
	}
	if in.TimeoutMs <= 0 {
		in.TimeoutMs = 60000
	}
	if in.Concurrency <= 0 {
		in.Concurrency = 5
	}
	j := &Job{
		ID: store.NewID(store.PrefixJob), Kind: in.Kind, Name: in.Name, Command: in.Command,
		TargetIDs: orEmpty(in.TargetIDs), ScheduleExpr: in.ScheduleExpr, Enabled: in.Enabled,
		TimeoutMs: in.TimeoutMs, Concurrency: in.Concurrency, CreatedBy: in.CreatedBy,
		CreatedAt: store.Now(), UpdatedAt: store.Now(),
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO jobs
        (id,kind,name,command,target_ids_json,schedule_expr,enabled,timeout_ms,concurrency,created_by,created_at,updated_at)
        VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.Kind, j.Name, j.Command, marshalIDs(j.TargetIDs), j.ScheduleExpr, boolInt(j.Enabled),
		j.TimeoutMs, j.Concurrency, j.CreatedBy, j.CreatedAt, j.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return j, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
func orEmpty(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

const jobCols = `id,kind,name,command,target_ids_json,COALESCE(schedule_expr,''),enabled,timeout_ms,
 concurrency,COALESCE(created_by,''),last_run_at,next_run_at,created_at,updated_at`

func scanJob(sc interface{ Scan(...any) error }) (*Job, error) {
	var j Job
	var ids string
	var enabled int
	var last, next sql.NullString
	if err := sc.Scan(&j.ID, &j.Kind, &j.Name, &j.Command, &ids, &j.ScheduleExpr, &enabled,
		&j.TimeoutMs, &j.Concurrency, &j.CreatedBy, &last, &next, &j.CreatedAt, &j.UpdatedAt); err != nil {
		return nil, err
	}
	j.TargetIDs = parseIDs(ids)
	j.Enabled = enabled == 1
	if last.Valid {
		j.LastRunAt = &last.String
	}
	if next.Valid {
		j.NextRunAt = &next.String
	}
	return &j, nil
}

// GetJob loads a job.
func (r *Repo) GetJob(ctx context.Context, id string) (*Job, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id=?`, id)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	return j, err
}

// ListJobs returns all jobs.
func (r *Repo) ListJobs(ctx context.Context) ([]Job, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// UpdateJob changes editable fields.
func (r *Repo) UpdateJob(ctx context.Context, id string, in JobInput) (*Job, error) {
	_, err := r.db.ExecContext(ctx, `UPDATE jobs SET name=?,command=?,target_ids_json=?,schedule_expr=?,
        enabled=?,timeout_ms=?,concurrency=?,updated_at=? WHERE id=?`,
		in.Name, in.Command, marshalIDs(orEmpty(in.TargetIDs)), in.ScheduleExpr, boolInt(in.Enabled),
		in.TimeoutMs, in.Concurrency, store.Now(), id)
	if err != nil {
		return nil, err
	}
	return r.GetJob(ctx, id)
}

// SetRunTimes updates scheduling bookkeeping.
func (r *Repo) SetRunTimes(ctx context.Context, id string, lastRun, nextRun *string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE jobs SET last_run_at=?,next_run_at=?,updated_at=? WHERE id=?`,
		nullable(lastRun), nullable(nextRun), store.Now(), id)
	return err
}

func nullable(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// DeleteJob removes a job (cascades to runs).
func (r *Repo) DeleteJob(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM jobs WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// InsertRun persists a run and its targets in one transaction.
func (r *Repo) InsertRun(ctx context.Context, run *Run, targets []RunTarget) error {
	return store.InTx(ctx, r.db, func(tx *sql.Tx) error {
		summary, _ := json.Marshal(run.Summary)
		if _, err := tx.Exec(`INSERT INTO job_runs
            (id,job_id,trigger,status,started_at,summary_json,created_at,updated_at)
            VALUES(?,?,?,?,?,?,?,?)`,
			run.ID, run.JobID, run.Trigger, run.Status, nullable(run.StartedAt), string(summary),
			run.CreatedAt, run.UpdatedAt); err != nil {
			return err
		}
		for _, t := range targets {
			if _, err := tx.Exec(`INSERT INTO run_targets
                (id,run_id,host_id,status,stdout_path,stderr_path)
                VALUES(?,?,?,?,?,?)`,
				t.ID, t.RunID, t.HostID, t.Status, t.StdoutPath, t.StderrPath); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetRunStatus updates run status and summary.
func (r *Repo) SetRunStatus(ctx context.Context, id, status string, summary RunSummary, ended bool) error {
	sum, _ := json.Marshal(summary)
	if ended {
		now := store.Now()
		_, err := r.db.ExecContext(ctx,
			`UPDATE job_runs SET status=?,summary_json=?,ended_at=?,updated_at=? WHERE id=?`,
			status, string(sum), now, now, id)
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE job_runs SET status=?,summary_json=?,started_at=COALESCE(started_at,?),updated_at=? WHERE id=?`,
		status, string(sum), store.Now(), store.Now(), id)
	return err
}

// TransitionTarget applies a guarded state transition to a target.
func (r *Repo) TransitionTarget(ctx context.Context, id, from, to string, apply func(*sql.Tx) error) error {
	return store.InTx(ctx, r.db, func(tx *sql.Tx) error {
		var cur string
		if err := tx.QueryRow(`SELECT status FROM run_targets WHERE id=?`, id).Scan(&cur); err != nil {
			return err
		}
		if cur != from {
			// Idempotent: terminal writes after cancellation are tolerated only
			// when moving to another terminal state from a live state.
			if IsTerminal(cur) {
				return nil
			}
		}
		if !CanTransitionTarget(cur, to) {
			return nil // illegal transitions are silently ignored, never crash a run
		}
		if apply != nil {
			if err := apply(tx); err != nil {
				return err
			}
		}
		return nil
	})
}

const targetCols = `id,run_id,host_id,status,exit_code,started_at,ended_at,COALESCE(error_code,''),
 COALESCE(error_text,''),COALESCE(stdout_path,''),COALESCE(stderr_path,''),COALESCE(stdout_preview,''),
 COALESCE(stderr_preview,''),bytes_out`

func scanTarget(sc interface{ Scan(...any) error }) (RunTarget, error) {
	var t RunTarget
	var exit sql.NullInt64
	var started, ended sql.NullString
	if err := sc.Scan(&t.ID, &t.RunID, &t.HostID, &t.Status, &exit, &started, &ended,
		&t.ErrorCode, &t.ErrorText, &t.StdoutPath, &t.StderrPath, &t.StdoutPreview,
		&t.StderrPreview, &t.BytesOut); err != nil {
		return t, err
	}
	if exit.Valid {
		v := int(exit.Int64)
		t.ExitCode = &v
	}
	if started.Valid {
		t.StartedAt = &started.String
	}
	if ended.Valid {
		t.EndedAt = &ended.String
	}
	return t, nil
}

// GetRun loads a run with its targets.
func (r *Repo) GetRun(ctx context.Context, id string) (*Run, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id,COALESCE(job_id,''),trigger,status,started_at,ended_at,
        summary_json,created_at,updated_at FROM job_runs WHERE id=?`, id)
	var run Run
	var started, ended sql.NullString
	var sum string
	if err := row.Scan(&run.ID, &run.JobID, &run.Trigger, &run.Status, &started, &ended,
		&sum, &run.CreatedAt, &run.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	if started.Valid {
		run.StartedAt = &started.String
	}
	if ended.Valid {
		run.EndedAt = &ended.String
	}
	_ = json.Unmarshal([]byte(sum), &run.Summary)
	rows, err := r.db.QueryContext(ctx, `SELECT `+targetCols+` FROM run_targets WHERE run_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		run.Targets = append(run.Targets, t)
	}
	return &run, rows.Err()
}

// ListRuns returns recent runs newest-first with cursor pagination.
func (r *Repo) ListRuns(ctx context.Context, limit int, cursor int64) ([]Run, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := `SELECT id,COALESCE(job_id,''),trigger,status,started_at,ended_at,summary_json,created_at,updated_at,rowid
        FROM job_runs`
	args := []any{}
	if cursor > 0 {
		// cursor points at the first unreturned row probed last time; include
		// it (strict < would drop one run per page).
		q += ` WHERE rowid <= ?`
		args = append(args, cursor)
	}
	q += ` ORDER BY rowid DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Run
	var next int64
	for rows.Next() {
		var run Run
		var started, ended sql.NullString
		var sum string
		var rowid int64
		if err := rows.Scan(&run.ID, &run.JobID, &run.Trigger, &run.Status, &started, &ended,
			&sum, &run.CreatedAt, &run.UpdatedAt, &rowid); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal([]byte(sum), &run.Summary)
		if len(out) == limit {
			next = rowid
			break
		}
		out = append(out, run)
	}
	return out, next, rows.Err()
}

// TargetsInStates returns target ids in given states (reconciliation/cancel).
func (r *Repo) TargetsInStates(ctx context.Context, states ...string) ([]RunTarget, error) {
	if len(states) == 0 {
		return nil, nil
	}
	q := `SELECT ` + targetCols + ` FROM run_targets WHERE status IN (` + placeholders(len(states)) + `)`
	args := make([]any, len(states))
	for i, s := range states {
		args[i] = s
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunTarget
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += "?"
	}
	return out
}
