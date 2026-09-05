package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

	"bastiondeck/internal/apiclient"
)

// ---------- Audit pane ----------
type auditPane struct {
	items  []apiclient.AuditEntry
	loaded bool
}

func newAuditPane() auditPane { return auditPane{} }

func (p auditPane) view(w, h int) string {
	if !p.loaded {
		return stPanel.Render("loading audit…")
	}
	var b strings.Builder
	b.WriteString(stTitle.Render("Audit trail (hash-chained)\n\n"))
	for _, e := range p.items {
		res := e.Result
		line := fmt.Sprintf("%-20s %-18s %-22s %s", e.At, trunc(e.ActorName, 16),
			trunc(e.Action, 20), res)
		switch res {
		case "success":
			b.WriteString(stOK.Render(line))
		case "denied", "failure":
			b.WriteString(stErr.Render(line))
		default:
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return stPanel.Width(w - 2).Render(b.String())
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// ---------- Help pane ----------
type helpPane struct{ lines []string }

func newHelpPane() helpPane {
	return helpPane{lines: []string{
		"Navigation",
		"  tab / shift+tab   switch pane",
		"  1..4              jump to pane",
		"  ↑/↓ or j/k        move selection",
		"  enter             open run detail",
		"  r                 force refresh",
		"",
		"Actions",
		"  e or :            open exec prompt for the selected host",
		"  enter (exec)      submit command; view switches to Runs",
		"  q / ctrl+c        quit",
		"",
		"Model",
		"  Hosts, credentials and runs live server-side; the TUI is a",
		"  polled console and never stores secrets on disk.",
	}}
}

func (p helpPane) view(w, h int) string {
	return stPanel.Width(w - 2).Render(strings.Join(p.lines, "\n"))
}

// ---------- Exec dialog ----------
type execDialog struct {
	input  textinput.Model
	active bool
}
