package sshlite

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"bastiondeck/internal/connector"
)

// Client implements connector.Client over an established *ssh.Client.
type Client struct {
	hostID  string
	raw     *ssh.Client
	keyType string
	fp      string

	done chan struct{}
	once sync.Once
	mu   sync.Mutex
}

func newClient(hostID string, raw *ssh.Client, keyType, fp string) *Client {
	c := &Client{hostID: hostID, raw: raw, keyType: keyType, fp: fp, done: make(chan struct{})}
	go func() {
		_ = raw.Wait()
		close(c.done)
	}()
	return c
}

// Kind identifies the backend.
func (c *Client) Kind() string { return "ssh" }

// HostID returns the bound host id.
func (c *Client) HostID() string { return c.hostID }

// Fingerprint returns the TOFU key material.
func (c *Client) Fingerprint() (string, string, error) {
	return c.keyType, c.fp, nil
}

// Done closes when the underlying transport drops.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close tears down the transport.
func (c *Client) Close() error {
	var err error
	c.once.Do(func() { err = c.raw.Close() })
	return err
}

// Exec runs a command with separated stdout/stderr and a definite outcome.
// A lost/indeterminate result is reported as connector.StatusLost rather than
// guessed as failure (ADR-003).
func (c *Client) Exec(ctx context.Context, req connector.ExecRequest) (*connector.ExecResult, error) {
	start := time.Now()
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sess, err := c.raw.NewSession()
	if err != nil {
		return &connector.ExecResult{
			Status: connector.StatusLost, ErrorCode: "conn_lost",
			ErrorText: err.Error(), DurationMs: ms(start),
		}, nil
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	outW := io.Writer(&stdout)
	errW := io.Writer(&stderr)
	if req.OnOutput != nil {
		outW = &teeWriter{buf: &stdout, stream: "stdout", cb: req.OnOutput}
		errW = &teeWriter{buf: &stderr, stream: "stderr", cb: req.OnOutput}
	}
	sess.Stdout, sess.Stderr = outW, errW

	type runErr struct{ err error }
	done := make(chan runErr, 1)
	go func() { done <- runErr{sess.Run(req.Command)} }()

	select {
	case <-runCtx.Done():
		_ = sess.Signal(ssh.SIGTERM)
		go func() { /* drain */ <-done }()
		time.Sleep(120 * time.Millisecond)
		_ = sess.Close()
		status := connector.StatusTimeout
		code := "timeout"
		if errors.Is(ctx.Err(), context.Canceled) {
			status, code = "cancelled", "cancelled"
		}
		return &connector.ExecResult{
			Status: status, ErrorCode: code, Stdout: stdout.Bytes(), Stderr: stderr.Bytes(),
			DurationMs: ms(start),
		}, nil
	case r := <-done:
		res := &connector.ExecResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), DurationMs: ms(start)}
		var exitErr *ssh.ExitError
		switch {
		case r.err == nil:
			res.Status, res.ExitCode = connector.StatusSuccess, 0
		case errors.As(r.err, &exitErr):
			res.Status, res.ExitCode = connector.StatusFailed, exitErr.ExitStatus()
			res.ErrorText = exitErr.Error()
		default:
			// Connection-level failure: outcome against the host is unknown.
			res.Status, res.ErrorCode = connector.StatusLost, "conn_lost"
			res.ErrorText = r.err.Error()
		}
		return res, nil
	}
}

func ms(start time.Time) int64 { return time.Since(start).Milliseconds() }

type teeWriter struct {
	buf    *bytes.Buffer
	stream string
	cb     func(stream string, b []byte)
}

func (t *teeWriter) Write(p []byte) (int, error) {
	n, err := t.buf.Write(p)
	if n > 0 {
		cp := make([]byte, n)
		copy(cp, p[:n])
		t.cb(t.stream, cp)
	}
	return n, err
}
