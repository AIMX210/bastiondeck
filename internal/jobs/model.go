// Package jobs is the execution engine: persisted runs, bounded-concurrency
// fan-out across hosts, a strict target/run state machine, cancellation,
// output capture, restart reconciliation (lost outcomes) and cron scheduling.
package jobs

import "encoding/json"

// Run/target status constants — the single source of truth mirrored by the DB
// CHECK constraints and the frontend.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusSuccess   = "success"
	StatusFailed    = "failed"
	StatusTimeout   = "timeout"
	StatusCancelled = "cancelled"
	StatusLost      = "lost"
	StatusSkipped   = "skipped"
)

// Job is a (possibly scheduled) job definition.
type Job struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Name         string   `json:"name"`
	Command      string   `json:"command"`
	TargetIDs    []string `json:"targetIds"`
	SnippetID    string   `json:"snippetId,omitempty"`
	ScheduleExpr string   `json:"scheduleExpr,omitempty"`
	Enabled      bool     `json:"enabled"`
	TimeoutMs    int      `json:"timeoutMs"`
	Concurrency  int      `json:"concurrency"`
	CreatedBy    string   `json:"createdBy,omitempty"`
	LastRunAt    *string  `json:"lastRunAt,omitempty"`
	NextRunAt    *string  `json:"nextRunAt,omitempty"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

// Run is one execution of a job.
type Run struct {
	ID        string      `json:"id"`
	JobID     string      `json:"jobId"`
	Trigger   string      `json:"trigger"`
	Status    string      `json:"status"`
	StartedAt *string     `json:"startedAt,omitempty"`
	EndedAt   *string     `json:"endedAt,omitempty"`
	Summary   RunSummary  `json:"summary"`
	CreatedAt string      `json:"createdAt"`
	UpdatedAt string      `json:"updatedAt"`
	Targets   []RunTarget `json:"targets,omitempty"`
}

// RunSummary counts target outcomes.
type RunSummary struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Success   int `json:"success"`
	Failed    int `json:"failed"`
	Timeout   int `json:"timeout"`
	Cancelled int `json:"cancelled"`
	Lost      int `json:"lost"`
	Skipped   int `json:"skipped"`
}

// RunTarget is the per-host outcome.
type RunTarget struct {
	ID            string  `json:"id"`
	RunID         string  `json:"runId"`
	HostID        string  `json:"hostId"`
	Status        string  `json:"status"`
	ExitCode      *int    `json:"exitCode,omitempty"`
	StartedAt     *string `json:"startedAt,omitempty"`
	EndedAt       *string `json:"endedAt,omitempty"`
	ErrorCode     string  `json:"errorCode,omitempty"`
	ErrorText     string  `json:"errorText,omitempty"`
	StdoutPath    string  `json:"-"`
	StderrPath    string  `json:"-"`
	StdoutPreview string  `json:"stdoutPreview,omitempty"`
	StderrPreview string  `json:"stderrPreview,omitempty"`
	BytesOut      int64   `json:"bytesOut"`
}

func parseIDs(s string) []string {
	var out []string
	if s == "" {
		return []string{}
	}
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func marshalIDs(ids []string) string {
	if ids == nil {
		ids = []string{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}
