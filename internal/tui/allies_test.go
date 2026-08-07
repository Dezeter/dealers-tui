package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"dealers/internal/allies"
)

func TestAlliesScreenToggle(t *testing.T) {
	dir := t.TempDir()
	m := NewAllies(Deps{Allies: allies.Load(filepath.Join(dir, "a.json"), []uint64{24})})

	// Add #99 via the input.
	m.input.SetValue("99")
	m, _ = m.toggle()
	if !m.deps.Allies.IsAlly(99) {
		t.Fatal("99 not added")
	}
	if !strings.Contains(m.notice, "added #99") {
		t.Errorf("notice = %q", m.notice)
	}

	// Toggling own dealer #24 says it's auto-protected, no change.
	m.input.SetValue("24")
	m, _ = m.toggle()
	if !strings.Contains(m.notice, "own dealer") {
		t.Errorf("fixed notice = %q", m.notice)
	}

	// Remove #99.
	m.input.SetValue("99")
	m, _ = m.toggle()
	if m.deps.Allies.IsAlly(99) {
		t.Error("99 not removed")
	}

	// Bad input.
	m.input.SetValue("abc")
	m, _ = m.toggle()
	if !strings.Contains(m.notice, "valid token id") {
		t.Errorf("bad-input notice = %q", m.notice)
	}

	// View renders the fixed count.
	if v := m.View(); !strings.Contains(v, "ALLIES") || !strings.Contains(v, "auto-protected") {
		t.Errorf("allies view missing content:\n%s", v)
	}
}
