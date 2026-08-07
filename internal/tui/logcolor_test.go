package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestLogColorRules(t *testing.T) {
	if !reWin.MatchString("PVP WIN vs #1: rep +12") {
		t.Error("WIN not matched as positive")
	}
	if !reLoss.MatchString("heist BUST at stage 3") {
		t.Error("BUST not matched as negative")
	}
	// Neutral outcomes get no color.
	if reWin.MatchString("PVE TIE (rep +18)") || reLoss.MatchString("PVE TIE (rep +18)") {
		t.Error("TIE should be neutral")
	}
	if reWin.MatchString("heist SETBACK at stage 2") {
		t.Error("SETBACK should be neutral (not win)")
	}
}

func TestColorizeWholeLine(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	win := colorizeLog("WIN (rep +36, cash -120, heat→1)")
	loss := colorizeLog("PVP LOSS vs #100: rep -10")
	tie := colorizeLog("TIE (rep +18)")

	if !strings.Contains(win, "\x1b[") || !strings.Contains(loss, "\x1b[") {
		t.Fatal("win/loss lines not colored")
	}
	if win == loss {
		t.Error("win and loss render the same color")
	}
	// Neutral line stays plain (no ANSI).
	if strings.Contains(tie, "\x1b[") {
		t.Errorf("TIE line should be uncolored: %q", tie)
	}
	// Positive and negative styles differ.
	if posStyle.Render("x") == negStyle.Render("x") {
		t.Error("positive and negative render the same color")
	}
}
