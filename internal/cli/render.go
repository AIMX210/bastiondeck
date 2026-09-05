package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// table is a minimal aligned table renderer.
type table struct {
	w      *tabwriter.Writer
	header []string
}

func newTable(out io.Writer, header ...string) *table {
	return &table{w: tabwriter.NewWriter(out, 0, 4, 2, ' ', 0), header: header}
}

func (t *table) head() {
	fmt.Fprintln(t.w, strings.Join(t.header, "\t"))
}

func (t *table) row(cols ...string) {
	fmt.Fprintln(t.w, strings.Join(cols, "\t"))
}

func (t *table) flush() { _ = t.w.Flush() }

// truncate keeps cells readable in narrow terminals.
func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// statusGlyph renders an outcome marker.
func statusGlyph(s string) string {
	switch s {
	case "success":
		return "✓"
	case "failed":
		return "✗"
	case "running", "pending":
		return "…"
	case "timeout":
		return "⏱"
	case "cancelled":
		return "⊘"
	case "lost":
		return "?"
	default:
		return "·"
	}
}
