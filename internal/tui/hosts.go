package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"bastiondeck/internal/apiclient"
)

// hostItem adapts a Host to the bubbles.list.Item interface.
type hostItem struct{ h apiclient.Host }

func (i hostItem) Title() string {
	tags := ""
	if len(i.h.Tags) > 0 {
		tags = " [" + strings.Join(i.h.Tags, ",") + "]"
	}
	return fmt.Sprintf("%-22s %s:%d%s", i.h.Name, i.h.Address, i.h.Port, tags)
}
func (i hostItem) Description() string {
	st := i.h.Status
	if st == "" {
		st = "unknown"
	}
	return fmt.Sprintf("%s  user=%s  id=%s", st, i.h.Username, short(i.h.ID))
}
func (i hostItem) FilterValue() string { return i.h.Name + " " + i.h.Address }

type hostsPane struct {
	list    list.Model
	items   []apiclient.Host
	spinner spinner.Model
	loaded  bool
}

func newHostsPane() hostsPane {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 80, 20)
	l.Title = "Hosts"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = stTitle
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return hostsPane{list: l, spinner: sp}
}

func (p *hostsPane) rebuildItems() tea.Cmd {
	items := make([]list.Item, 0, len(p.items))
	for _, h := range p.items {
		items = append(items, hostItem{h})
	}
	return p.list.SetItems(items)
}

func (p hostsPane) update(msg tea.Msg) (hostsPane, tea.Cmd) {
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

func (p hostsPane) view(w, h int) string {
	p.list.SetSize(w-2, h)
	if !p.loaded {
		return stPanel.Render(p.spinner.View() + " loading hosts…")
	}
	return stPanel.Render(p.list.View())
}

// selectedIDs returns checked/selected host ids for execution.
func (p hostsPane) selectedIDs() []string {
	if sel, ok := p.list.SelectedItem().(hostItem); ok {
		return []string{sel.h.ID}
	}
	return nil
}

func short(id string) string {
	if len(id) > 10 {
		return id[:10]
	}
	return id
}
