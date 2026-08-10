package template

import (
	"os"
	"path/filepath"
	"testing"

	"dealers/internal/recipe"
)

func TestLoadSeedsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "templates.json")
	s := Load(path) // missing file → seed
	if got := s.Names(); len(got) != 3 || got[0] != "pve" || got[1] != "pvp" || got[2] != "manual" {
		t.Fatalf("default names = %v, want [pve pvp manual]", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Load should have written the seed file: %v", err)
	}
	// Default templates carry -1 (max affordable) heist difficulty and no step
	// override, so they inherit the global recipe.
	pve, ok := s.Get("pve")
	if !ok || pve.Params.HeistDifficulty != -1 || len(pve.Steps) != 0 {
		t.Fatalf("pve default = %+v, want HeistDifficulty -1 and no steps", pve)
	}
}

func TestEnabledStepsFallbackAndOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "templates.json")
	s := Load(path)
	global := func() []string { return []string{"a", "b", "c"} }

	// A default (no-steps) template inherits the global recipe.
	if got := s.EnabledSteps("pve", global); len(got) != 3 || got[0] != "a" {
		t.Errorf("inherited steps = %v, want the global recipe", got)
	}
	// StepMax on an inheriting template is 0 (no caps).
	if m := s.StepMax("pve", "core"); m != 0 {
		t.Errorf("StepMax on inheriting template = %d, want 0", m)
	}
}

func TestCustomTemplateStepsAndCaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "templates.json")
	// Seed first, then overwrite with a custom template.
	_ = Load(path)
	custom := `[{"name":"heister","strategy":"pve","params":{"heist_difficulty":2},
	  "steps":[{"id":"heist_checkin","on":true},{"id":"clear_stars","on":false},
	           {"id":"heists","on":true,"max":2},{"id":"core","on":true,"max":4}]}]`
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(path)
	if names := s.Names(); len(names) != 1 || names[0] != "heister" {
		t.Fatalf("names = %v, want [heister]", names)
	}
	// Own steps used (clear_stars off is dropped), global ignored.
	got := s.EnabledSteps("heister", func() []string { return []string{"should-not-appear"} })
	want := []string{"heist_checkin", "heists", "core"}
	if len(got) != len(want) {
		t.Fatalf("enabled steps = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("enabled steps = %v, want %v", got, want)
		}
	}
	if m := s.StepMax("heister", "core"); m != 4 {
		t.Errorf("core cap = %d, want 4", m)
	}
	if m := s.StepMax("heister", "heists"); m != 2 {
		t.Errorf("heists cap = %d, want 2", m)
	}
	tpl, _ := s.Get("heister")
	if tpl.Params.HeistDifficulty != 2 {
		t.Errorf("heist difficulty = %d, want 2", tpl.Params.HeistDifficulty)
	}
}

// guard against accidental removal of the recipe.Step Max field the caps ride on.
var _ = recipe.Step{Max: 1}
