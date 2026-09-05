package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bastiondeck/internal/audit"
	"bastiondeck/internal/connector"
	"bastiondeck/internal/realtime"
	"bastiondeck/internal/store"
)

// Engine orchestrates run execution.
type Engine struct {
	repo      *Repo
	db        *sql.DB
	resolver  connector.Resolver
	hub       *realtime.Hub
	logs      *audit.Service
	dataDir   string
	maxOutput int64

	mu   sync.Mutex
	live map[string]*liveRun
}

type liveRun struct {
	cancel  context.CancelFunc
	targets sync.Map // targetID -> bool
}

// NewEngine constructs the engine.
func NewEngine(repo *Repo, db *sql.DB, resolver connector.Resolver, hub *realtime.Hub,
	logs *audit.Service, dataDir string, maxOutput int64) *Engine {
	if maxOutput <= 0 {
		maxOutput = 4 << 20
	}
	return &Engine{repo: repo, db: db, resolver: resolver, hub: hub, logs: logs,
		dataDir: dataDir, maxOutput: maxOutput, live: map[string]*liveRun{}}
}

// StartInput describes a requested run.
type StartInput struct {
	JobID       string // empty => create an adhoc job
	JobName     string
	Command     string
	TargetIDs   []string
	Timeout     time.Duration
	Concurrency int
	Trigger     string // manual | schedule | retry
	Actor       audit.Actor
}

// Validation errors with stable codes.
var (
	ErrEmptyTargets = errors.New("empty_targets")
	ErrEmptyCommand = errors.New("empty_command")
)

// StartRun persists first, then executes asynchronously.
func (e *Engine) StartRun(ctx context.Context, in StartInput) (string, error) {
	if len(in.TargetIDs) == 0 {
		return "", ErrEmptyTargets
	}
	if in.Command == "" {
		return "", ErrEmptyCommand
	}
	if in.Trigger == "" {
		in.Trigger = "manual"
	}
	if in.Timeout <= 0 {
		in.Timeout = 60 * time.Second
	}
	if in.Concurrency <= 0 {
		in.Concurrency = 5
	}
	jobID := in.JobID
	if jobID == "" {
		j, err := e.repo.CreateJob(ctx, JobInput{
			Kind: "adhoc", Name: in.JobName, Command: in.Command, TargetIDs: in.TargetIDs,
			Enabled: true, TimeoutMs: int(in.Timeout / time.Millisecond),
			Concurrency: in.Concurrency, CreatedBy: in.Actor.ID,
		})
		if err != nil {
			return "", err
		}
		jobID = j.ID
	}
	now := store.Now()
	run := &Run{ID: store.NewID(store.PrefixRun), JobID: jobID, Trigger: in.Trigger,
		Status: StatusPending, StartedAt: &now, CreatedAt: now, UpdatedAt: now,
		Summary: RunSummary{Total: len(in.TargetIDs), Pending: len(in.TargetIDs)}}
	targets := make([]RunTarget, 0, len(in.TargetIDs))
	for _, hostID := range in.TargetIDs {
		targets = append(targets, RunTarget{
			ID: store.NewID(store.PrefixTarget), RunID: run.ID, HostID: hostID,
			Status:     StatusPending,
			StdoutPath: filepath.Join(e.dataDir, "runs", run.ID, ""), // directory; files per target
			StderrPath: filepath.Join(e.dataDir, "runs", run.ID, ""),
		})
	}
	if err := os.MkdirAll(filepath.Join(e.dataDir, "runs", run.ID), 0o700); err != nil {
		return "", err
	}
	if err := e.repo.InsertRun(ctx, run, targets); err != nil {
		return "", err
	}
	_, _ = e.db.ExecContext(ctx, `UPDATE jobs SET last_run_at=? WHERE id=?`, now, jobID)

	runCtx, cancel := context.WithCancel(context.Background())
	lr := &liveRun{cancel: cancel}
	e.mu.Lock()
	e.live[run.ID] = lr
	e.mu.Unlock()

	go e.execute(runCtx, run.ID, jobID, in, targets)
	if e.logs != nil {
		_, _ = e.logs.Write(context.Background(), in.Actor, "job.run", "job_run", run.ID, "success",
			map[string]any{"targets": len(in.TargetIDs), "trigger": in.Trigger})
	}
	e.publish("run_update", map[string]any{"runId": run.ID, "status": StatusPending})
	return run.ID, nil
}

func (e *Engine) publish(t string, data any) {
	if e.hub != nil {
		e.hub.Publish(t, data)
	}
}

// execute fans out across targets with a bounded semaphore and aggregates.
func (e *Engine) execute(ctx context.Context, runID, jobID string, in StartInput, targets []RunTarget) {
	_ = jobID // 保留参数以对齐签名；run 级状态由 SetRunStatus 汇总
	defer func() {
		e.mu.Lock()
		delete(e.live, runID)
		e.mu.Unlock()
	}()
	_ = e.repo.SetRunStatus(ctx, runID, StatusRunning, RunSummary{Total: len(targets), Running: len(targets)}, false)
	sem := make(chan struct{}, in.Concurrency)
	var wg sync.WaitGroup
	statuses := make([]string, len(targets))
	var stMu sync.Mutex
	for i := range targets {
		wg.Add(1)
		go func(idx int, t RunTarget) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				e.finishTarget(ctx, t, StatusCancelled, nil, "")
				stMu.Lock()
				statuses[idx] = StatusCancelled
				stMu.Unlock()
				return
			}
			defer func() { <-sem }()
			st := e.runOne(ctx, t, in)
			stMu.Lock()
			statuses[idx] = st
			stMu.Unlock()
		}(i, targets[i])
	}
	wg.Wait()
	agg := Aggregate(statuses)
	summary := Summarise(statuses)
	_ = e.repo.SetRunStatus(context.Background(), runID, agg, summary, true)
	e.publish("run_update", map[string]any{"runId": runID, "status": agg, "summary": summary})
}

// runOne connects and executes against one target.
func (e *Engine) runOne(ctx context.Context, t RunTarget, in StartInput) string {
	e.markRunning(t)
	outPath := filepath.Join(e.dataDir, "runs", t.RunID, t.ID+".stdout")
	errPath := filepath.Join(e.dataDir, "runs", t.RunID, t.ID+".stderr")
	outW := newCappedWriter(outPath, e.maxOutput)
	errW := newCappedWriter(errPath, e.maxOutput)
	defer outW.Close()
	defer errW.Close()

	cli, err := e.resolver.Connect(ctx, t.HostID)
	if err != nil {
		return e.finishTarget(ctx, t, StatusLost, nil, classifyConn(err))
	}
	res, err := cli.Exec(ctx, connector.ExecRequest{
		Command:        in.Command,
		Timeout:        in.Timeout,
		MaxBufferBytes: e.maxOutput,
		OnOutput: func(stream string, b []byte) {
			if stream == "stdout" {
				_, _ = outW.Write(b)
			} else {
				_, _ = errW.Write(b)
			}
		},
	})
	if err != nil {
		return e.finishTarget(ctx, t, StatusLost, nil, err.Error())
	}
	// OnOutput 已把全部流式输出写入产物文件（sshlite/agent 两个后端均保证
	// 回调覆盖完整输出）；此处不得再 flush res.Stdout/Stderr，否则产物内容翻倍。
	_ = outW.Close()
	_ = errW.Close()
	exit := res.ExitCode
	final := res.Status
	e.persistTargetResult(ctx, t, final, &exit, res.ErrorCode, res.ErrorText,
		outW.Preview(), errW.Preview(), outW.Bytes())
	e.publish("target_update", map[string]any{"runId": t.RunID, "targetId": t.ID, "hostId": t.HostID, "status": final})
	return final
}

func (e *Engine) markRunning(t RunTarget) {
	now := store.Now()
	_ = e.repo.TransitionTarget(context.Background(), t.ID, StatusPending, StatusRunning,
		func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE run_targets SET status='running',started_at=? WHERE id=?`, now, t.ID)
			return err
		})
}

func (e *Engine) finishTarget(ctx context.Context, t RunTarget, status string, exit *int, errText string) string {
	code := ""
	if status == StatusLost {
		code = "conn_lost"
	}
	e.persistTargetResult(ctx, t, status, exit, code, errText, "", "", 0)
	e.publish("target_update", map[string]any{"runId": t.RunID, "targetId": t.ID, "hostId": t.HostID, "status": status})
	return status
}

func (e *Engine) persistTargetResult(ctx context.Context, t RunTarget, status string, exit *int,
	code, errText, outPrev, errPrev string, bytes int64) {
	now := store.Now()
	_ = store.InTx(ctx, e.db, func(tx *sql.Tx) error {
		var cur string
		if err := tx.QueryRow(`SELECT status FROM run_targets WHERE id=?`, t.ID).Scan(&cur); err != nil {
			return err
		}
		if !CanTransitionTarget(cur, status) {
			return nil
		}
		_, err := tx.Exec(`UPDATE run_targets SET status=?,exit_code=?,ended_at=?,error_code=?,
            error_text=?,stdout_preview=?,stderr_preview=?,bytes_out=? WHERE id=?`,
			status, exit, now, code, truncate(errText, 2000), truncate(outPrev, 2048),
			truncate(errPrev, 2048), bytes, t.ID)
		return err
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func classifyConn(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// CancelRun aborts a run: live context cancelled, pending targets become
// cancelled; running targets unwind through their own timeout/cancel path.
func (e *Engine) CancelRun(ctx context.Context, runID string) error {
	e.mu.Lock()
	lr, ok := e.live[runID]
	e.mu.Unlock()
	if ok {
		lr.cancel()
	}
	pending, err := e.repo.TargetsInStates(ctx, StatusPending)
	if err != nil {
		return err
	}
	for _, t := range pending {
		if t.RunID != runID {
			continue
		}
		now := store.Now()
		_, _ = e.db.ExecContext(ctx,
			`UPDATE run_targets SET status='cancelled',ended_at=? WHERE id=? AND status='pending'`, now, t.ID)
	}
	return nil
}

// Reconcile marks orphaned targets/runs as lost after a restart or a stalled
// worker (no live owner). This is the ambiguous-outcome holder (ADR-003).
func (e *Engine) Reconcile(ctx context.Context) (int, error) {
	e.mu.Lock()
	liveIDs := map[string]bool{}
	for id := range e.live {
		liveIDs[id] = true
	}
	e.mu.Unlock()
	open, err := e.repo.TargetsInStates(ctx, StatusPending, StatusRunning)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range open {
		if liveIDs[t.RunID] {
			continue
		}
		now := store.Now()
		res, err := e.db.ExecContext(ctx,
			`UPDATE run_targets SET status='lost',ended_at=?,error_code='orphaned' WHERE id=?`, now, t.ID)
		if err != nil {
			return n, err
		}
		if x, _ := res.RowsAffected(); x > 0 {
			n++
		}
	}
	// Recompute parent run states for affected runs.
	rows, err := e.db.QueryContext(ctx,
		`SELECT DISTINCT run_id FROM run_targets WHERE status IN ('lost')`)
	if err != nil {
		return n, err
	}
	var runIDs []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		runIDs = append(runIDs, id)
	}
	rows.Close()
	for _, id := range runIDs {
		run, err := e.repo.GetRun(ctx, id)
		if err != nil {
			continue
		}
		states := make([]string, len(run.Targets))
		for i, t := range run.Targets {
			states[i] = t.Status
		}
		agg := Aggregate(states)
		if IsTerminal(agg) {
			_ = e.repo.SetRunStatus(ctx, id, agg, Summarise(states), true)
		}
	}
	return n, nil
}

// RetryFailed creates a new run over failed/lost/timeout targets.
func (e *Engine) RetryFailed(ctx context.Context, runID string, actor audit.Actor) (string, error) {
	run, err := e.repo.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	job, err := e.repo.GetJob(ctx, run.JobID)
	if err != nil {
		return "", err
	}
	var ids []string
	for _, t := range run.Targets {
		if t.Status == StatusFailed || t.Status == StatusLost || t.Status == StatusTimeout {
			ids = append(ids, t.HostID)
		}
	}
	if len(ids) == 0 {
		return "", errors.New("no failed targets to retry")
	}
	return e.StartRun(ctx, StartInput{
		JobID: job.ID, JobName: job.Name + " (retry)", Command: job.Command, TargetIDs: ids,
		Timeout: time.Duration(job.TimeoutMs) * time.Millisecond, Concurrency: job.Concurrency,
		Trigger: "retry", Actor: actor,
	})
}

// ReadOutput streams captured output from offset for one target.
func (e *Engine) ReadOutput(ctx context.Context, runID, targetID, stream string, offset int64) ([]byte, int64, error) {
	if stream != "stdout" && stream != "stderr" {
		return nil, 0, fmt.Errorf("bad stream")
	}
	// 纵深防御：ID 必须符合内部前缀，防止拼路径逃逸（正常路由已由 mux 限制）。
	if !strings.HasPrefix(runID, store.PrefixRun) || !strings.HasPrefix(targetID, store.PrefixTarget) {
		return nil, 0, fmt.Errorf("bad id")
	}
	p := filepath.Join(e.dataDir, "runs", runID, targetID+"."+stream)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, offset, nil
		}
		return nil, offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, offset, err
	}
	return b, offset + int64(len(b)), nil
}

// IsLive reports whether a run is currently executing.
func (e *Engine) IsLive(runID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.live[runID]
	return ok
}
