package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"bastiondeck/internal/apiclient"
)

// Message types carrying loaded data.
type (
	hostsLoadedMsg struct{ hosts []apiclient.Host }
	runsLoadedMsg  struct{ runs []apiclient.Run }
	auditLoadedMsg struct{ entries []apiclient.AuditEntry }
	runStartedMsg  struct{ id string }
	runDetailMsg   struct{ run *apiclient.Run }
)

func (m Model) loadHosts() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		hs, err := c.ListHosts(context.Background(), "")
		if err != nil {
			return errMsg{err}
		}
		return hostsLoadedMsg{hs}
	}
}

func (m Model) loadRuns() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		rs, err := c.ListRuns(context.Background(), 30)
		if err != nil {
			return errMsg{err}
		}
		return runsLoadedMsg{rs}
	}
}

func (m Model) loadAudit() tea.Cmd {
	c := m.client
	return func() tea.Msg {
		es, err := c.ListAudit(context.Background(), 40)
		if err != nil {
			return errMsg{err}
		}
		return auditLoadedMsg{es}
	}
}

func (m Model) loadRunDetail() tea.Cmd {
	c := m.client
	id := m.runs.selectedID()
	if id == "" {
		return nil
	}
	return func() tea.Msg {
		run, _, err := c.GetRun(context.Background(), id)
		if err != nil {
			return errMsg{err}
		}
		return runDetailMsg{run}
	}
}

func (m Model) submitExec() tea.Cmd {
	c := m.client
	cmd := m.exec.input.Value()
	hostIDs := m.hosts.selectedIDs()
	return func() tea.Msg {
		id, err := c.Exec(context.Background(), cmd, hostIDs, 60)
		if err != nil {
			return errMsg{err}
		}
		return runStartedMsg{id}
	}
}
