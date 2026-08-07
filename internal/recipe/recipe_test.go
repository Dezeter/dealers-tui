package recipe

import (
	"path/filepath"
	"reflect"
	"testing"
)

func tmp(t *testing.T) string { return filepath.Join(t.TempDir(), "recipes.json") }

var def = []string{"a", "b", "c", "d"}

func TestDefaultAllOn(t *testing.T) {
	s := Load(tmp(t), def)
	if got := s.Enabled(); !reflect.DeepEqual(got, def) {
		t.Errorf("default enabled = %v, want %v", got, def)
	}
}

func TestToggleAndPersist(t *testing.T) {
	p := tmp(t)
	s := Load(p, def)
	if err := s.Toggle(1); err != nil { // turn off "b"
		t.Fatal(err)
	}
	want := []string{"a", "c", "d"}
	if got := s.Enabled(); !reflect.DeepEqual(got, want) {
		t.Errorf("after toggle enabled = %v, want %v", got, want)
	}
	// Reload from disk: the off state survives.
	if got := Load(p, def).Enabled(); !reflect.DeepEqual(got, want) {
		t.Errorf("persisted enabled = %v, want %v", got, want)
	}
}

func TestMoveReorders(t *testing.T) {
	p := tmp(t)
	s := Load(p, def)
	// Move "c" (index 2) up to index 1.
	ni, err := s.Move(2, -1)
	if err != nil || ni != 1 {
		t.Fatalf("Move → (%d,%v)", ni, err)
	}
	want := []string{"a", "c", "b", "d"}
	if got := s.Enabled(); !reflect.DeepEqual(got, want) {
		t.Errorf("after move = %v, want %v", got, want)
	}
	// Clamped at the top: moving index 0 up is a no-op.
	if ni, _ := s.Move(0, -1); ni != 0 {
		t.Errorf("move up from 0 should clamp, got %d", ni)
	}
	// Persisted.
	if got := Load(p, def).Enabled(); !reflect.DeepEqual(got, want) {
		t.Errorf("persisted order = %v, want %v", got, want)
	}
}

func TestResetRestoresDefault(t *testing.T) {
	s := Load(tmp(t), def)
	s.Toggle(0)
	s.Move(3, -1)
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	if got := s.Enabled(); !reflect.DeepEqual(got, def) {
		t.Errorf("after reset = %v, want %v", got, def)
	}
}

func TestLoadDropsUnknownAndAppendsNew(t *testing.T) {
	p := tmp(t)
	// Persist an old recipe that references a removed step "x" and misses new "d".
	Load(p, []string{"a", "x", "b"}) // writes nothing (no save); construct file manually instead
	// Simulate a saved file with a stale id and a missing new id via Toggle to save.
	s := Load(p, []string{"a", "b", "c"})
	s.Toggle(0) // forces a save of [a(off),b,c]
	// Now load with a catalog that dropped "a" and added "d".
	s2 := Load(p, []string{"b", "c", "d"})
	all := s2.All()
	if len(all) != 3 {
		t.Fatalf("want 3 steps (dropped a, kept b,c, appended d), got %d: %+v", len(all), all)
	}
	if all[2].ID != "d" || !all[2].On {
		t.Errorf("new step d should be appended and on, got %+v", all[2])
	}
}
