package agentconn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"bastiondeck/internal/connector"
)

// Provider adapts live agents to the connector abstraction.
type Provider struct{ reg *Registry }

// NewProvider returns the AgentProvider for connector.Manager.
func (r *Registry) NewProvider() *Provider { return &Provider{reg: r} }

// Available reports whether the agent is approved and currently connected.
func (p *Provider) Available(agentID string) bool { return p.reg.Available(agentID) }

// Connect returns a Client bound to a live approved agent.
func (p *Provider) Connect(ctx context.Context, agentID string) (connector.Client, error) {
	st, ok := p.reg.session(agentID)
	if !ok {
		return nil, errors.New("agent offline")
	}
	return &AgentClient{id: agentID, st: st, done: make(chan struct{})}, nil
}

// AgentClient is a connector.Client over one live agent WebSocket.
type AgentClient struct {
	id   string
	st   *connState
	once sync.Once
	done chan struct{}
}

func (a *AgentClient) Kind() string          { return "agent" }
func (a *AgentClient) HostID() string        { return a.id }
func (a *AgentClient) Done() <-chan struct{} { return a.done }
func (a *AgentClient) Close() error {
	a.once.Do(func() { close(a.done) })
	return nil
}
func (a *AgentClient) Fingerprint() (string, string, error) {
	return "agent", a.id, nil
}

// Exec runs a command and aggregates streamed output.
func (a *AgentClient) Exec(ctx context.Context, req connector.ExecRequest) (*connector.ExecResult, error) {
	id, ch := a.st.register()
	defer a.st.unregister(id)
	start := time.Now()
	if err := a.st.send(Frame{T: "exec_req", ID: id, Command: req.Command,
		TimeoutMs: int(req.Timeout / time.Millisecond)}); err != nil {
		return nil, err
	}
	var stdout, stderr []byte
	for {
		select {
		case <-ctx.Done():
			_ = a.st.send(Frame{T: "cancel_req", ID: id})
			return nil, ctx.Err()
		case f, ok := <-ch:
			if !ok {
				return nil, errors.New("agent disconnected")
			}
			switch f.T {
			case "exec_chunk":
				b, _ := base64.StdEncoding.DecodeString(f.DataB64)
				if req.OnOutput != nil {
					req.OnOutput(f.Stream, b)
				}
				if f.Stream == "stderr" {
					stderr = append(stderr, b...)
				} else {
					stdout = append(stdout, b...)
				}
			case "exec_end":
				return &connector.ExecResult{
					Status: f.Status, ExitCode: f.ExitCode, Stdout: stdout, Stderr: stderr,
					ErrorCode: f.ErrorCode, ErrorText: f.ErrorText,
					DurationMs: time.Since(start).Milliseconds(),
				}, nil
			}
		}
	}
}

// OpenPty is not supported for the first agent protocol revision.
func (a *AgentClient) OpenPty(ctx context.Context, cols, rows int) (connector.PtySession, error) {
	return nil, errors.New("pty not supported over agent protocol v1")
}

// FS returns the remote filesystem bridge.
func (a *AgentClient) FS() (connector.FS, error) { return &agentFS{a: a}, nil }

// Facts requests host facts from the agent.
func (a *AgentClient) Facts(ctx context.Context) (*connector.Facts, error) {
	id, ch := a.st.register()
	defer a.st.unregister(id)
	if err := a.st.send(Frame{T: "facts_req", ID: id}); err != nil {
		return nil, err
	}
	select {
	case f := <-ch:
		var out connector.Facts
		if err := json.Unmarshal(f.Facts, &out); err != nil {
			return nil, err
		}
		return &out, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(15 * time.Second):
		return nil, errors.New("facts timeout")
	}
}

// ---------- FS bridge ----------

type agentFS struct{ a *AgentClient }

func (fs *agentFS) request(ctx context.Context, f Frame) (Frame, error) {
	id, ch := fs.a.st.register()
	defer fs.a.st.unregister(id)
	f.T = "fs_req"
	f.ID = id
	if err := fs.a.st.send(f); err != nil {
		return Frame{}, err
	}
	select {
	case res := <-ch:
		if res.ErrorText != "" {
			return res, errors.New(res.ErrorText)
		}
		return res, nil
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	case <-time.After(30 * time.Second):
		return Frame{}, errors.New("fs request timeout")
	}
}

func (fs *agentFS) List(ctx context.Context, p string) ([]connector.DirEntry, error) {
	res, err := fs.request(ctx, Frame{Op: "list", Path: p})
	if err != nil {
		return nil, err
	}
	var out []connector.DirEntry
	return out, json.Unmarshal(res.Payload, &out)
}

func (fs *agentFS) Stat(ctx context.Context, p string) (*connector.FileStat, error) {
	res, err := fs.request(ctx, Frame{Op: "stat", Path: p})
	if err != nil {
		return nil, err
	}
	var out connector.FileStat
	return &out, json.Unmarshal(res.Payload, &out)
}

func (fs *agentFS) Read(ctx context.Context, p string, limit int64) ([]byte, error) {
	res, err := fs.request(ctx, Frame{Op: "read", Path: p})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(res.ContentB64)
}

func (fs *agentFS) Write(ctx context.Context, p string, content []byte, expectedSHA string) (string, error) {
	res, err := fs.request(ctx, Frame{Op: "write", Path: p,
		ContentB64: base64.StdEncoding.EncodeToString(content), ExpectedSHA: expectedSHA})
	if err != nil {
		return "", err
	}
	var out struct {
		SHA string `json:"sha256"`
	}
	return out.SHA, json.Unmarshal(res.Payload, &out)
}

func (fs *agentFS) Mkdir(ctx context.Context, p string) error {
	_, err := fs.request(ctx, Frame{Op: "mkdir", Path: p})
	return err
}
func (fs *agentFS) Rename(ctx context.Context, from, to string) error {
	_, err := fs.request(ctx, Frame{Op: "rename", Path: from, Dest: to})
	return err
}
func (fs *agentFS) Remove(ctx context.Context, p string) error {
	_, err := fs.request(ctx, Frame{Op: "remove", Path: p})
	return err
}

func (fs *agentFS) Upload(ctx context.Context, r io.Reader, remote string, progress func(int64)) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	_, err = fs.Write(ctx, remote, b, "")
	if progress != nil {
		progress(int64(len(b)))
	}
	return err
}

func (fs *agentFS) Download(ctx context.Context, remote string, w io.Writer, progress func(int64)) error {
	b, err := fs.Read(ctx, remote, 0)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	if progress != nil {
		progress(int64(len(b)))
	}
	return err
}

func (fs *agentFS) Close() error { return nil }
