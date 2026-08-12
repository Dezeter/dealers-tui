package template

import (
	"os"
	"path/filepath"
	"testing"
)

func tmp(t *testing.T) string { return filepath.Join(t.TempDir(), "templates.json") }

func TestLoadSeedsDefaults(t *testing.T) {
	p := tmp(t)
	s := Load(p)
	names := s.Names()
	if len(names) != 3 || names[0] != "pve" || names[2] != "manual" {
		t.Fatalf("default names = %v, want [pve pvp manual]", names)
	}
	pve, ok := s.Get("pve")
	if !ok || len(pve.Steps) == 0 || pve.Steps[len(pve.Steps)-1].Action != ActionTrade {
		t.Fatalf("pve program should end in a trade step: %+v", pve.Steps)
	}
	if m, _ := s.Get("manual"); len(m.Steps) != 0 {
		t.Errorf("manual should be an empty program, got %+v", m.Steps)
	}
	// File was written and reloads to the same program.
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("templates.json not written: %v", err)
	}
	if got := Load(p).Names(); len(got) != 3 {
		t.Errorf("reloaded names = %v", got)
	}
}

func TestAddCloneRenameDelete(t *testing.T) {
	s := Load(tmp(t))
	if err := s.Add(Template{Name: "raid", Steps: []Step{{Action: ActionPvP, Count: 4}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(Template{Name: "raid"}); err == nil {
		t.Error("duplicate name should fail")
	}
	dup, err := s.Clone("raid")
	if err != nil || dup != "raid-copy" {
		t.Fatalf("clone → (%q,%v)", dup, err)
	}
	if err := s.Rename("raid-copy", "raid2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("raid2"); !ok {
		t.Error("rename lost the template")
	}
	if err := s.Delete("raid2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("raid2"); ok {
		t.Error("delete left the template")
	}
}

func TestUpdatePersists(t *testing.T) {
	p := tmp(t)
	s := Load(p)
	err := s.Update("pve", func(tpl *Template) {
		tpl.Steps = append(tpl.Steps, Step{Action: ActionHeist, HeistDifficulty: 1, Count: 2})
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := Load(p).Get("pve")
	last := got.Steps[len(got.Steps)-1]
	if last.Action != ActionHeist || last.HeistDifficulty != 1 || last.Count != 2 {
		t.Errorf("update did not persist: %+v", last)
	}
}

func TestMigrateOldFormat(t *testing.T) {
	p := tmp(t)
	old := `[
	  {"name":"pve","strategy":"pve","params":{"drug":"coke","buy_area":"Miami","sell_area":"Berlin","heist_difficulty":2},
	   "steps":[{"id":"clear_stars","on":true},{"id":"core","on":true,"max":3},{"id":"heists","on":true}]},
	  {"name":"idle","strategy":"manual","params":{"heist_difficulty":-1}}
	]`
	if err := os.WriteFile(p, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load(p)
	pve, ok := s.Get("pve")
	if !ok {
		t.Fatal("migrated pve missing")
	}
	// First step is the breakout (old implicit jailbreak).
	if pve.Steps[0].Action != ActionBreakout {
		t.Errorf("migrated program should start with breakout, got %q", pve.Steps[0].Action)
	}
	// The core step became a trade carrying the old params + max→count.
	var trade *Step
	for i := range pve.Steps {
		if pve.Steps[i].Action == ActionTrade {
			trade = &pve.Steps[i]
		}
	}
	if trade == nil || trade.Drug != "coke" || trade.BuyArea != "Miami" || trade.SellArea != "Berlin" || trade.Count != 3 {
		t.Errorf("trade step not migrated correctly: %+v", trade)
	}
	// The heists step carried the difficulty.
	var heist *Step
	for i := range pve.Steps {
		if pve.Steps[i].Action == ActionHeist {
			heist = &pve.Steps[i]
		}
	}
	if heist == nil || heist.HeistDifficulty != 2 {
		t.Errorf("heist step not migrated: %+v", heist)
	}
	// manual → empty program.
	if m, _ := s.Get("idle"); len(m.Steps) != 0 {
		t.Errorf("manual should migrate to empty, got %+v", m.Steps)
	}
}
