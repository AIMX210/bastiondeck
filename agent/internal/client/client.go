// Package client maintains the agent's reverse WebSocket connection to the
// BastionDeck server: exponential-backoff reconnect, hello handshake, request
// dispatch and heartbeats.
package client

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"time"

	"github.com/coder/websocket"

	"bd-agent/internal/executor"
	"bd-agent/internal/facts"
	"bd-agent/internal/proto"
	"bd-agent/internal/remotefs"
)

// Config configures the agent connection.
type Config struct {
	ServerURL string // ws://host:port or http(s)://host:port
	Secret    string
	Version   string
	Name      string
}

// Agent is the running agent.
type Agent struct {
	cfg  Config
	exec *executor.Executor
}

// New constructs an agent.
func New(cfg Config) *Agent {
	return &Agent{cfg: cfg, exec: executor.New()}
}

// Run connects and reconnects forever until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := a.session(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			log.Printf("session ended: %v; reconnecting in %s", err, backoff)
		}
		jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff + jitter):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (a *Agent) wsURL() (string, error) {
	u, err := url.Parse(a.cfg.ServerURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	u.Path = "/agent/connect"
	q := u.Query()
	q.Set("secret", a.cfg.Secret)
	q.Set("version", a.cfg.Version)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (a *Agent) session(ctx context.Context) error {
	endpoint, err := a.wsURL()
	if err != nil {
		return err
	}
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, endpoint, nil)
	if err != nil {
		return err
	}
	pc := proto.Wrap(conn)
	defer pc.Close()
	log.Printf("connected to %s", a.cfg.ServerURL)

	// Hello with current facts.
	f := facts.Gather()
	pc.Send(proto.Frame{T: "hello", Version: a.cfg.Version, Facts: f.JSON()})

	// Heartbeat ticker.
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go a.heartbeats(hbCtx, pc)

	for {
		frame, err := pc.Read(ctx)
		if err != nil {
			return err
		}
		a.dispatch(ctx, pc, frame)
	}
}

func (a *Agent) heartbeats(ctx context.Context, pc *proto.Conn) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pc.Send(proto.Frame{T: "heartbeat"})
		}
	}
}

func (a *Agent) dispatch(ctx context.Context, pc *proto.Conn, f proto.Frame) {
	switch f.T {
	case "welcome":
		log.Printf("registered as agent %s", f.ID)
	case "ping":
		pc.Send(proto.Frame{T: "pong"})
	case "exec_req":
		go a.exec.Run(f.ID, f.Command, f.TimeoutMs, pc.Send)
	case "cancel_req":
		a.exec.Cancel(f.ID)
	case "facts_req":
		go func(id string) {
			pc.Send(proto.Frame{T: "facts_res", ID: id, Facts: facts.Gather().JSON()})
		}(f.ID)
	case "fs_req":
		go func(req proto.Frame) {
			pc.Send(remotefs.Handle(ctx, req))
		}(f)
	}
}
