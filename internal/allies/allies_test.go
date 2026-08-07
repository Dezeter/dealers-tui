package allies

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToggleAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allies.json")
	a := Load(path, []uint64{24, 25}) // own fleet (fixed)

	// Fixed dealers are always allies and can't be toggled off.
	if !a.IsAlly(24) {
		t.Error("own dealer 24 should be an ally")
	}
	if added, fixed, err := a.Toggle(24); err != nil || !fixed || !added {
		t.Errorf("toggling a fixed id: added=%v fixed=%v err=%v", added, fixed, err)
	}
	if len(a.List()) != 0 {
		t.Error("fixed ids must not appear in the user list")
	}

	// Add a user ally.
	added, fixed, err := a.Toggle(99)
	if err != nil || fixed || !added {
		t.Fatalf("add 99: added=%v fixed=%v err=%v", added, fixed, err)
	}
	if !a.IsAlly(99) || len(a.List()) != 1 {
		t.Errorf("99 not added: isAlly=%v list=%v", a.IsAlly(99), a.List())
	}

	// Persisted to disk and reloaded.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("allies.json not written: %v", err)
	}
	b := Load(path, []uint64{24, 25})
	if !b.IsAlly(99) {
		t.Error("user ally 99 not persisted across reload")
	}

	// Toggle 99 off.
	if added, _, _ := a.Toggle(99); added {
		t.Error("second toggle should remove")
	}
	if a.IsAlly(99) {
		t.Error("99 should be removed")
	}
}

func TestFixedCount(t *testing.T) {
	a := Load(filepath.Join(t.TempDir(), "a.json"), []uint64{1, 2, 3})
	if a.FixedCount() != 3 {
		t.Errorf("FixedCount = %d, want 3", a.FixedCount())
	}
}
