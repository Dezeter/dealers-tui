package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func rune1(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestNormalizeKeyRussianLayout(t *testing.T) {
	// Physical key → Cyrillic rune it produces → expected Latin shortcut.
	cases := map[rune]string{
		'и': "b", // buy
		'ы': "s", // sell
		'с': "c", // clear heat
		'з': "p", // pvp
		'р': "h", // heist
		'п': "g", // go/commit
		'щ': "o", // cash out
		'ч': "x", // abandon
		'к': "r", // refresh
		'ф': "a", // reset attempts
		'й': "q", // quit
		'Ф': "A", // autopilot toggle (capital)
		'н': "y", // confirm yes
		'т': "n", // confirm no
	}
	for cyr, want := range cases {
		if got := normalizeKey(rune1(cyr)).String(); got != want {
			t.Errorf("normalizeKey(%q) = %q, want %q", cyr, got, want)
		}
	}
}

func TestNormalizeKeyLeavesLatinAndSpecials(t *testing.T) {
	// Latin keys pass through unchanged.
	if got := normalizeKey(rune1('b')).String(); got != "b" {
		t.Errorf("latin b changed to %q", got)
	}
	// Digits pass through.
	if got := normalizeKey(rune1('5')).String(); got != "5" {
		t.Errorf("digit 5 changed to %q", got)
	}
	// Non-rune keys (enter/esc/arrows) are untouched.
	if got := normalizeKey(tea.KeyMsg{Type: tea.KeyEnter}).Type; got != tea.KeyEnter {
		t.Errorf("enter key type changed to %v", got)
	}
	if got := normalizeKey(tea.KeyMsg{Type: tea.KeyUp}).Type; got != tea.KeyUp {
		t.Errorf("up key type changed to %v", got)
	}
}
