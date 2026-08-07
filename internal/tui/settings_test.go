package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"dealers/internal/settings"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSettingsScreenToggles(t *testing.T) {
	store := settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	m := NewSettings(Deps{Settings: store})

	// enter flips the selected (first) toggle on.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !store.Get(settings.KeyPayBail) {
		t.Fatal("enter should turn the first toggle ON")
	}
	// View shows it checked.
	if !strings.Contains(stripANSI(m.View()), "[x]") {
		t.Errorf("view should render a checked box:\n%s", stripANSI(m.View()))
	}
	// enter again flips it off.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if store.Get(settings.KeyPayBail) {
		t.Error("second enter should turn it OFF")
	}
}

func TestSettingsScreenBack(t *testing.T) {
	m := NewSettings(Deps{Settings: settings.Load(filepath.Join(t.TempDir(), "s.json"))})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should emit a back command")
	}
	if _, ok := cmd().(backToFleetMsg); !ok {
		t.Error("esc should return to the fleet")
	}
}
