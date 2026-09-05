package httpx

import (
	"strconv"
	"strings"

	"bastiondeck/internal/inventory"
	"bastiondeck/internal/snippets"
)

func itoa(n int) string { return strconv.Itoa(n) }

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func dedup(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, x := range in {
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
}

func filterByGroup(groupID string) inventory.HostFilter {
	return inventory.HostFilter{GroupID: groupID}
}

func renderBody(body string, vars map[string]string) (string, []string) {
	return snippets.Render(body, vars)
}

// atoiDefault parses a query parameter with fallback.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// cursor parses an opaque pagination cursor.
func cursorOf(s string) int64 {
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func ptrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
