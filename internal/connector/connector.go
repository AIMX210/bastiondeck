// Package connector defines the provider-neutral backend abstraction
// (ADR-002): jobs/files/metrics code talks to Client, while concrete
// implementations (SSH, bd-agent) live behind adapters at the boundary.
package connector

import (
	"context"
	"io"
	"time"
)

// ExecStatus values mirror the persisted run-target state machine.
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusTimeout = "timeout"
	StatusLost    = "lost"
)

// ExecRequest describes one command execution.
type ExecRequest struct {
	Command    string
	Timeout    time.Duration
	Cols, Rows int
	OnOutput   func(stream string, b []byte) // optional incremental callback
	// MaxBufferBytes bounds the memory accumulated for Stdout/Stderr in the
	// ExecResult (streaming callbacks are unaffected). 0 = unlimited.
	MaxBufferBytes int64
}

// ExecResult is the definite outcome of an execution.
type ExecResult struct {
	Status     string `json:"status"`
	ExitCode   int    `json:"exitCode"`
	Stdout     []byte `json:"-"`
	Stderr     []byte `json:"-"`
	DurationMs int64  `json:"durationMs"`
	ErrorCode  string `json:"errorCode,omitempty"`
	ErrorText  string `json:"errorText,omitempty"`
}

// DirEntry is a filesystem listing row.
type DirEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Mode  uint32 `json:"mode"`
	IsDir bool   `json:"isDir"`
	MTime int64  `json:"mtime"`
}

// FileStat describes a path.
type FileStat struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Mode  uint32 `json:"mode"`
	IsDir bool   `json:"isDir"`
	MTime int64  `json:"mtime"`
}

// FS is the minimal remote file system surface.
type FS interface {
	List(ctx context.Context, path string) ([]DirEntry, error)
	Stat(ctx context.Context, path string) (*FileStat, error)
	Read(ctx context.Context, path string, limit int64) ([]byte, error)
	// Write atomically replaces path: temp file + rename; expectedSHA enables
	// optimistic concurrency (409 modified on mismatch).
	Write(ctx context.Context, path string, content []byte, expectedSHA string) (newSHA string, err error)
	Mkdir(ctx context.Context, path string) error
	Rename(ctx context.Context, from, to string) error
	Remove(ctx context.Context, path string) error
	Upload(ctx context.Context, r io.Reader, remote string, progress func(int64)) error
	Download(ctx context.Context, remote string, w io.Writer, progress func(int64)) error
}

// Facts are basic host facts.
type Facts struct {
	Hostname string            `json:"hostname"`
	OS       string            `json:"os"`
	Kernel   string            `json:"kernel"`
	Arch     string            `json:"arch"`
	UptimeS  int64             `json:"uptimeSec"`
	CPUModel string            `json:"cpuModel"`
	CPUCores int               `json:"cpuCores"`
	MemTotal int64             `json:"memTotal"`
	Disk     []DiskUsage       `json:"disk"`
	Extra    map[string]string `json:"extra,omitempty"`
}

// DiskUsage is a mounted filesystem usage row.
type DiskUsage struct {
	Filesystem string `json:"filesystem"`
	Mount      string `json:"mount"`
	Total      int64  `json:"total"`
	Used       int64  `json:"used"`
	Available  int64  `json:"available"`
}

// PtySession is an interactive terminal.
type PtySession interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	Wait() error
}

// Client is a live connection to a managed host.
type Client interface {
	Kind() string // "ssh" | "agent"
	HostID() string
	Fingerprint() (keyType, fp string, err error)
	Exec(ctx context.Context, req ExecRequest) (*ExecResult, error)
	OpenPty(ctx context.Context, cols, rows int) (PtySession, error)
	FS() (FS, error)
	Facts(ctx context.Context) (*Facts, error)
	Done() <-chan struct{}
	Close() error
}

// Resolver resolves a host id to a connected Client.
type Resolver interface {
	Connect(ctx context.Context, hostID string) (Client, error)
}
