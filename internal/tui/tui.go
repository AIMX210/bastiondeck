// Package tui is the terminal user interface built on bubbletea. It is a
// read-mostly operator console: browse hosts/runs/audit, fire commands at
// selected hosts and watch run outcomes live (polled through apiclient).
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"bastiondeck/internal/apiclient"
)

// tab identifiers.
const (
	tabHosts = iota
	tabRuns
	tabAudit
	tabHelp
	tabCount
)

var tabNames = [tabCount]string{"Hosts", "Runs", "Audit", "Help"}

// Model is the root TUI model.
type Model struct {
	client *apiclient.Client
	width  int
	height int

	tab int

	hosts  hostsPane
	runs   runsPane
	audit  auditPane
	help   helpPane
	exec   execDialog
	status string
	err    string

	quitting bool
}

type tickMsg time.Time
type errMsg struct{ err error }
type refreshMsg struct{ when time.Time }

// New builds the root model.
func New(c *apiclient.Client) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	ti := textinput.New()
	ti.Placeholder = "command to run on selected host(s), e.g. uptime"
	ti.CharLimit = 512
	ti.Width = 60
	vp := viewport.New(80, 20)
	m := Model{
		client: c,
		hosts:  newHostsPane(),
		runs:   newRunsPane(vp),
		audit:  newAuditPane(),
		help:   newHelpPane(),
		exec:   execDialog{input: ti, active: false},
		status: "connecting…",
	}
	return m
}

// Init starts the polling loop and first loads.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadHosts(), m.loadRuns(), m.loadAudit(), tickEvery(3*time.Second), spinner.Tick)
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Update routes messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.runs.viewport.Width = msg.Width - 4
		m.runs.viewport.Height = msg.Height - 8
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.hosts.spinner, cmd = m.hosts.spinner.Update(msg)
		return m, cmd
	case tickMsg:
		cmds := []tea.Cmd{}
		if m.tab == tabHosts {
			cmds = append(cmds, m.loadHosts())
		}
		if m.tab == tabRuns {
			cmds = append(cmds, m.loadRuns())
		}
		if m.tab == tabAudit {
			cmds = append(cmds, m.loadAudit())
		}
		cmds = append(cmds, tickEvery(3*time.Second))
		return m, tea.Batch(cmds...)
	case hostsLoadedMsg:
		m.hosts.items = msg.hosts
		m.hosts.loaded = true
		m.status = fmt.Sprintf("%d hosts", len(msg.hosts))
		return m, m.hosts.rebuildItems()
	case runsLoadedMsg:
		m.runs.items = msg.runs
		m.runs.loaded = true
		return m, nil
	case auditLoadedMsg:
		m.audit.items = msg.entries
		m.audit.loaded = true
		return m, nil
	case runStartedMsg:
		m.status = "started run " + msg.id
		m.exec.active = false
		m.exec.input.Reset()
		m.tab = tabRuns
		return m, tea.Batch(m.loadRuns(), tickEvery(800*time.Millisecond))
	case errMsg:
		m.err = msg.err.Error()
		m.status = "error"
		return m, nil
	}

	// Delegate to focused pane.
	var cmd tea.Cmd
	switch m.tab {
	case tabHosts:
		if !m.exec.active {
			m.hosts, cmd = m.hosts.update(msg)
		}
	case tabRuns:
		m.runs, cmd = m.runs.update(msg)
	}
	if m.exec.active {
		m.exec.input, cmd = m.exec.input.Update(msg)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.exec.active {
		switch msg.String() {
		case "esc":
			m.exec.active = false
			return m, nil
		case "enter":
			return m, m.submitExec()
		}
		var cmd tea.Cmd
		m.exec.input, cmd = m.exec.input.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "tab", "right":
		m.tab = (m.tab + 1) % tabCount
		return m, nil
	case "shift+tab", "left":
		m.tab = (m.tab - 1 + tabCount) % tabCount
		return m, nil
	case "1", "2", "3", "4":
		m.tab = int(msg.String()[0] - '1')
		return m, nil
	case "r":
		return m, tea.Batch(m.loadHosts(), m.loadRuns(), m.loadAudit())
	case "e", ":":
		m.exec.active = true
		m.exec.input.Focus()
		return m, textinput.Blink
	case "enter":
		if m.tab == tabRuns {
			return m, m.loadRunDetail()
		}
	}
	return m, nil
}

// View renders the whole screen.
func (m Model) View() string {
	if m.quitting {
		return "bye.\n"
	}
	header := m.renderHeader()
	var body string
	switch m.tab {
	case tabHosts:
		body = m.hosts.view(m.width, m.height-6)
	case tabRuns:
		body = m.runs.view(m.width, m.height-6)
	case tabAudit:
		body = m.audit.view(m.width, m.height-6)
	case tabHelp:
		body = m.help.view(m.width, m.height-6)
	}
	footer := m.renderFooter()
	out := header + "\n" + body + "\n" + footer
	if m.exec.active {
		out += "\n" + stPanel.Render("exec ▸ "+m.exec.input.View())
	}
	if m.err != "" {
		out += "\n" + stErr.Render("! "+m.err)
	}
	return stApp.Render(out)
}

func (m Model) renderHeader() string {
	tabs := ""
	for i, name := range tabNames {
		if i == m.tab {
			tabs += stTabActive.Render(name) + " "
		} else {
			tabs += stTab.Render(name) + " "
		}
	}
	title := stTitle.Render("◆ BastionDeck")
	return title + "  " + tabs
}

func (m Model) renderFooter() string {
	left := stMuted.Render("[tab] switch  [e] exec  [r] refresh  [↑/↓] move  [enter] detail  [q] quit")
	right := stMuted.Render(m.status)
	pad := m.width - len(left) - len(right) - 2
	if pad < 1 {
		pad = 1
	}
	spaces := ""
	for i := 0; i < pad; i++ {
		spaces += " "
	}
	return left + spaces + right
}

// Run launches the TUI.
func Run(c *apiclient.Client) error {
	p := tea.NewProgram(New(c), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
