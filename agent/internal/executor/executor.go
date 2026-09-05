// Package executor runs the commands requested by the server with bounded
// timeouts, streamed stdout/stderr and definite outcome reporting. It never
// persists anything and runs with the agent process's own privileges.
package executor

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"bd-agent/internal/proto"
)

// Sink receives protocol frames.
type Sink func(proto.Frame)

// Executor executes local commands.
type Executor struct {
	mu   sync.Mutex
	live map[string]context.CancelFunc
}

// New constructs an executor.
func New() *Executor { return &Executor{live: map[string]context.CancelFunc{}} }

// Run executes request id and streams chunks, then emits a terminal exec_end.
func (e *Executor) Run(id, command string, timeoutMs int, sink Sink) {
	if timeoutMs <= 0 {
		timeoutMs = 60000
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	e.mu.Lock()
	e.live[id] = cancel
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.live, id)
		e.mu.Unlock()
	}()

	cmd := shellCommand(ctx, command)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.end(id, "lost", -1, "pipe", err.Error(), sink)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		e.end(id, "lost", -1, "pipe", err.Error(), sink)
		return
	}
	if err := cmd.Start(); err != nil {
		e.end(id, "failed", -1, "start", err.Error(), sink)
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go e.pump(id, "stdout", stdout, sink, &wg)
	go e.pump(id, "stderr", stderr, sink, &wg)
	wg.Wait()

	err = cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		e.end(id, "timeout", -1, "timeout", "command exceeded deadline", sink)
		return
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		e.end(id, "cancelled", -1, "cancelled", "cancelled by server", sink)
		return
	}
	exit := 0
	code := "success"
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
			code = "failed"
		} else {
			e.end(id, "lost", -1, "wait", err.Error(), sink)
			return
		}
	}
	e.end(id, code, exit, "", "", sink)
}

// Cancel aborts a running command.
func (e *Executor) Cancel(id string) {
	e.mu.Lock()
	cancel := e.live[id]
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (e *Executor) pump(id, stream string, r io.Reader, sink Sink, wg *sync.WaitGroup) {
	defer wg.Done()
	sc := bufio.NewReaderSize(r, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := sc.Read(buf)
		if n > 0 {
			sink(proto.Frame{T: "exec_chunk", ID: id, Stream: stream,
				DataB64: base64.StdEncoding.EncodeToString(buf[:n])})
		}
		if err != nil {
			return
		}
	}
}

func (e *Executor) end(id, status string, exit int, errCode, errText string, sink Sink) {
	sink(proto.Frame{T: "exec_end", ID: id, Status: status, ExitCode: exit,
		ErrorCode: errCode, ErrorText: errText})
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	prepareCmd(cmd) // platform-specific: own process group + group kill
	return cmd
}
