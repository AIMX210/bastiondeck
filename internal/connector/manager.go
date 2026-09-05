package connector

import (
	"context"
	"errors"
	"fmt"

	"bastiondeck/internal/inventory"
)

// SSHProvider is implemented by the SSH connection pool.
type SSHProvider interface {
	Connect(ctx context.Context, hostID string) (Client, error)
}

// AgentProvider is implemented by agentconn (nil when agents disabled).
type AgentProvider interface {
	Connect(ctx context.Context, agentID string) (Client, error)
	Available(agentID string) bool
}

// Manager routes a host id to the right backend based on its auth_kind.
type Manager struct {
	Hosts  *inventory.Repo
	SSH    SSHProvider
	Agents AgentProvider // may be nil
}

// Connect resolves and connects a host.
func (m *Manager) Connect(ctx context.Context, hostID string) (Client, error) {
	h, err := m.Hosts.Get(ctx, hostID)
	if err != nil {
		return nil, err
	}
	switch h.AuthKind {
	case "", "credential":
		if m.SSH == nil {
			return nil, errors.New("ssh backend unavailable")
		}
		return m.SSH.Connect(ctx, hostID)
	case "agent":
		if m.Agents == nil || h.AgentID == nil {
			return nil, errors.New("host has no bound agent")
		}
		if !m.Agents.Available(*h.AgentID) {
			return nil, fmt.Errorf("agent %s offline", *h.AgentID)
		}
		return m.Agents.Connect(ctx, *h.AgentID)
	default:
		return nil, fmt.Errorf("unknown auth kind %q", h.AuthKind)
	}
}
