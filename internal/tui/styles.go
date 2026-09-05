package tui

import "github.com/charmbracelet/lipgloss"

// Palette is intentionally restrained (no purple/indigo): slate + teal accents.
var (
	colBg     = lipgloss.Color("#0f1419")
	colPanel  = lipgloss.Color("#1a2027")
	colBorder = lipgloss.Color("#3a4451")
	colText   = lipgloss.Color("#d7dde3")
	colMuted  = lipgloss.Color("#8a96a3")
	colAccent = lipgloss.Color("#2dd4bf")
	colOK     = lipgloss.Color("#4ade80")
	colWarn   = lipgloss.Color("#fbbf24")
	colErr    = lipgloss.Color("#f87171")
	colInfo   = lipgloss.Color("#60a5fa")
)

var (
	stApp   = lipgloss.NewStyle().Background(colBg).Foreground(colText)
	stTitle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stMuted = lipgloss.NewStyle().Foreground(colMuted)
	stPanel = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(colBorder).Padding(0, 1).Background(colPanel)
	stTabActive = lipgloss.NewStyle().Foreground(colBg).Background(colAccent).Bold(true).
			Padding(0, 1)
	stTab  = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	stOK   = lipgloss.NewStyle().Foreground(colOK)
	stWarn = lipgloss.NewStyle().Foreground(colWarn)
	stErr  = lipgloss.NewStyle().Foreground(colErr)
	stKey  = lipgloss.NewStyle().Foreground(colInfo).Bold(true)
)

func statusColor(s string) lipgloss.Style {
	switch s {
	case "success":
		return stOK
	case "failed", "lost":
		return stErr
	case "running", "pending":
		return stWarn
	case "timeout", "cancelled":
		return stMuted
	default:
		return lipgloss.NewStyle().Foreground(colText)
	}
}
