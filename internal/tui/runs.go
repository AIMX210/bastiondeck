package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"bastiondeck/internal/apiclient"
)

type runsPane struct {
	items    []apiclient.Run
	cursor   int
	loaded   bool
	viewport viewport.Model
	detail   *apiclient.Run
}

func newRunsPane(vp viewport.Model) runsPane {
	return runsPane{viewport: vp}
}

func (p runsPane) update(msg tea.Msg) (runsPane, tea.Cmd) {
	switch msg := msg.(type) {
	case runDetailMsg:
		p.detail = msg.run
		p.viewport.SetContent(p.detailText())
		return p, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "down", "j":
			if p.cursor < len(p.items)-1 {
				p.cursor++
			}
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
			}
		case "esc":
			p.detail = nil
		}
	}
	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return p, cmd
}

func (p *runsPane) selectedID() string {
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return ""
	}
	return p.items[p.cursor].ID
}

func (p runsPane) view(w, h int) string {
	if !p.loaded {
		return stPanel.Render("loading runs…")
	}
	if p.detail != nil {
		return stPanel.Width(w - 2).Render("Run detail (esc to close)\n\n" + p.viewport.View())
	}
	var b strings.Builder
	b.WriteString(stTitle.Render("Recent runs") + stMuted.Render("  (enter for detail)\n\n"))
	for i, r := range p.items {
		cursor := " "
		line := fmt.Sprintf("%s %-12s %-10s ok=%d fail=%d lost=%d",
			cursor, short(r.ID), r.Status, r.Summary.Success, r.Summary.Failed, r.Summary.Lost)
		style := statusColor(r.Status)
		if i == p.cursor {
			line = "▸ " + line
		} else {
			line = "  " + line
		}
		b.WriteString(style.Render(line) + "\n")
	}
	return stPanel.Width(w - 2).Render(b.String())
}

func (p runsPane) detailText() string {
	if p.detail == nil {
		return ""
	}
	var b strings.Builder
	r := p.detail
	fmt.Fprintf(&b, "run %s  status=%s  job=%s\n\n", r.ID, r.Status, r.JobID)
	if len(r.Targets) == 0 {
		b.WriteString("(targets not loaded)\n")
	}
	for _, t := range r.Targets {
		exit := "-"
		if t.ExitCode != nil {
			exit = fmt.Sprint(*t.ExitCode)
		}
		fmt.Fprintf(&b, "host %s  %s  exit=%s\n", short(t.HostID), t.Status, exit)
		if t.ErrorText != "" {
			fmt.Fprintf(&b, "  error: %s\n", t.ErrorText)
		}
		if t.StdoutPreview != "" {
			fmt.Fprintf(&b, "  stdout: %s\n", strings.TrimSpace(t.StdoutPreview))
		}
		if t.StderrPreview != "" {
			fmt.Fprintf(&b, "  stderr: %s\n", strings.TrimSpace(t.StderrPreview))
		}
		b.WriteString("\n")
	}
	return b.String()
}
