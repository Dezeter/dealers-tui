package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func tmp(t *testing.T) string { return filepath.Join(t.TempDir(), "settings.json") }

func TestDefaultsOff(t *testing.T) {
	s := Load(tmp(t))
	if s.Get(KeyPayBail) {
		t.Error("toggles must default to off")
	}
}

func TestToggleAndPersist(t *testing.T) {
	p := tmp(t)
	s := Load(p)
	v, err := s.Toggle(KeyPayBail)
	if err != nil || !v {
		t.Fatalf("Toggle → (%v,%v), want (true,nil)", v, err)
	}
	// Reload from disk: the value survives.
	if !Load(p).Get(KeyPayBail) {
		t.Error("toggle did not persist")
	}
	// Toggle back off.
	if v, _ := s.Toggle(KeyPayBail); v {
		t.Error("second toggle should turn it off")
	}
	if Load(p).Get(KeyPayBail) {
		t.Error("off state did not persist")
	}
}

func TestSet(t *testing.T) {
	p := tmp(t)
	s := Load(p)
	if err := s.Set(KeyPayBail, true); err != nil {
		t.Fatal(err)
	}
	if !s.Get(KeyPayBail) || !Load(p).Get(KeyPayBail) {
		t.Error("Set(true) did not stick")
	}
}

func TestLoadIgnoresUnknownKeys(t *testing.T) {
	p := tmp(t)
	if err := os.WriteFile(p, []byte(`{"bogus_key":true,"`+KeyPayBail+`":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(p)
	if !s.Get(KeyPayBail) {
		t.Error("known key should load")
	}
	if s.Get("bogus_key") {
		t.Error("unknown key should be ignored")
	}
}
