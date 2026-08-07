package tui

import (
	"regexp"

	"github.com/charmbracelet/lipgloss"
)

// Outcome line colors for the activity log (applied at render time only — stored
// summaries stay plain text).
var (
	posStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // green: wins / gains
	negStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")) // red: losses
)

var (
	// Positive outcomes → whole line green.
	reWin = regexp.MustCompile(`\b(WIN|CLEAN|SUCCESS|CLEARED|cleared|escaped|arrived|bailed|reset|sold|cashed)\b`)
	// Negative outcomes → whole line red.
	reLoss = regexp.MustCompile(`\b(LOSS|LOSE|BUST|BUSTED|FAILED|FAIL|EXPIRED|ARRESTED|arrested|jailed|abandoned)\b`)
)

// colorizeLog tints a whole activity-log line by its outcome: green for a win /
// positive action, red for a loss / bust, neutral otherwise (TIE, SETBACK, plain
// commits). Simple and scan-friendly — the win/loss reads at a glance.
func colorizeLog(s string) string {
	switch {
	case reLoss.MatchString(s):
		return negStyle.Render(s)
	case reWin.MatchString(s):
		return posStyle.Render(s)
	default:
		return s
	}
}
