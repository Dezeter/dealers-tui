package autostrat

import (
	"os"
	"path/filepath"
	"testing"
)

func tmpPath(t *testing.T) string { return filepath.Join(t.TempDir(), "strategies.json") }

func TestGetFallsBackToDefault(t *testing.T) {
	s := Load(tmpPath(t), "pvp", nil)
	if s.Get(42) != "pvp" {
		t.Errorf("unassigned dealer = %q, want default pvp", s.Get(42))
	}
	if s.Default() != "pvp" {
		t.Errorf("default = %q, want pvp", s.Default())
	}
}

func TestSeedAndFileMerge(t *testing.T) {
	p := tmpPath(t)
	if err := os.WriteFile(p, []byte(`{"25":"pvp"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Seed sets 24=pve; file sets 25=pvp and wins for its keys; 26 → default.
	s := Load(p, "manual", map[uint64]string{24: "pve"})
	if s.Get(24) != "pve" || s.Get(25) != "pvp" || s.Get(26) != "manual" {
		t.Errorf("merge wrong: 24=%s 25=%s 26=%s", s.Get(24), s.Get(25), s.Get(26))
	}
}

func TestSetPersistsAndClearsOnDefault(t *testing.T) {
	p := tmpPath(t)
	s := Load(p, "manual", nil)
	if err := s.Set(7, "pvp"); err != nil {
		t.Fatal(err)
	}
	// Reload from disk → override survives.
	if Load(p, "manual", nil).Get(7) != "pvp" {
		t.Error("override did not persist")
	}
	// Setting back to the default clears the override (file stays minimal).
	if err := s.Set(7, "manual"); err != nil {
		t.Fatal(err)
	}
	if reloaded := Load(p, "manual", nil); reloaded.Get(7) != "manual" {
		t.Errorf("expected default after clear, got %q", reloaded.Get(7))
	}
}

func TestCycleAdvancesThroughOrder(t *testing.T) {
	s := Load(tmpPath(t), "pve", nil) // start = default "pve" (Order[0])
	want := []string{"pvp", "manual", "pve"}
	for _, w := range want {
		got, err := s.Cycle(1)
		if err != nil {
			t.Fatal(err)
		}
		if got != w {
			t.Fatalf("cycle → %q, want %q", got, w)
		}
	}
}

func TestSetRejectsUnknown(t *testing.T) {
	s := Load(tmpPath(t), "pve", nil)
	if err := s.Set(1, "bogus"); err == nil {
		t.Error("expected error for unknown strategy")
	}
}

func TestLoadDropsInvalidDefault(t *testing.T) {
	if s := Load(tmpPath(t), "nonsense", nil); s.Default() != "pve" {
		t.Errorf("invalid default should fall back to pve, got %q", s.Default())
	}
}

func TestMigratesLegacyFarm(t *testing.T) {
	p := tmpPath(t)
	if err := os.WriteFile(p, []byte(`{"9":"farm"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(p, "farm", map[uint64]string{8: "farm"}) // legacy default + seed
	if s.Default() != "pve" {
		t.Errorf("legacy default farm should migrate to pve, got %q", s.Default())
	}
	if s.Get(8) != "pve" || s.Get(9) != "pve" {
		t.Errorf("legacy farm tags should migrate to pve: 8=%s 9=%s", s.Get(8), s.Get(9))
	}
}
