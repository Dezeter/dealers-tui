// Package template manages named, reusable autopilot presets ("strategy
// templates"). A template bundles a base strategy (pve/pvp/manual), an optional
// per-template step recipe with per-step daily caps, and tuning parameters
// (trade route, heist difficulty, mission priority). Templates are assigned to
// dealers per NFT (via internal/autostrat, whose values are template names), so
// different dealers can run genuinely different jobs on autopilot.
//
// The definitions live in templates.json (hand-editable now, UI-editable later).
// A template with no Steps inherits the global step recipe, and zero-valued
// Params mean "the strategy's built-in default" — so the seeded pve/pvp/manual
// templates reproduce today's behaviour exactly, and only customised templates
// change anything.
package template

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"dealers/internal/recipe"
)

// Params tunes the parameterised behaviours on top of a template's base strategy.
// Every zero value means "use the strategy default".
type Params struct {
	Drug     string `json:"drug,omitempty"`      // trade this drug; "" = weed
	BuyArea  string `json:"buy_area,omitempty"`  // buy-zone name; "" = Manhattan
	SellArea string `json:"sell_area,omitempty"` // sell-zone name; "" = Amsterdam

	// HeistDifficulty fixes the heist tier: -1 (or 0-value via NormalizeParams) =
	// highest the dealer can afford; 0/1/2 = that exact difficulty.
	HeistDifficulty int `json:"heist_difficulty"`

	// MissionPriority orders mission steering: "daily" (default) or "weekly".
	MissionPriority string `json:"mission_priority,omitempty"`
}

// Template is a named autopilot preset.
type Template struct {
	Name     string        `json:"name"`
	Strategy string        `json:"strategy"`        // pve | pvp | manual
	Steps    []recipe.Step `json:"steps,omitempty"` // empty = inherit the global recipe
	Params   Params        `json:"params"`
}

// Defaults returns the seeded pve/pvp/manual templates: no step override (inherit
// the global recipe), neutral params (HeistDifficulty -1). They reproduce the
// app's current behaviour so existing per-dealer assignments keep working.
func Defaults() []Template {
	mk := func(name string) Template {
		return Template{Name: name, Strategy: name, Params: Params{HeistDifficulty: -1}}
	}
	return []Template{mk("pve"), mk("pvp"), mk("manual")}
}

// Store holds the template definitions, persisted to a JSON file.
type Store struct {
	path string
	mu   sync.RWMutex
	list []Template
}

// Load reads templates.json; on a missing/empty/invalid file it seeds the
// defaults and writes them so the file is there to edit.
func Load(path string) *Store {
	s := &Store{path: path}
	if data, err := os.ReadFile(path); err == nil {
		var list []Template
		if json.Unmarshal(data, &list) == nil {
			s.list = sanitize(list)
		}
	}
	if len(s.list) == 0 {
		s.list = Defaults()
		_ = s.save()
	}
	return s
}

// sanitize drops nameless templates and defaults an unset HeistDifficulty (the
// JSON zero) to -1 ("max affordable") only when the template also omits it — a
// template that explicitly wants difficulty 0 must set it as 0 and is preserved.
// Since we can't tell 0-because-omitted from 0-because-chosen in plain JSON, we
// treat 0 as "difficulty 0" and rely on Defaults()/UI to write -1 for "auto".
func sanitize(list []Template) []Template {
	out := list[:0]
	for _, t := range list {
		if t.Name == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Names returns the template names in file order (the fleet selector's cycle).
func (s *Store) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.list))
	for i, t := range s.list {
		out[i] = t.Name
	}
	return out
}

// All returns a copy of the templates.
func (s *Store) All() []Template {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Template(nil), s.list...)
}

// Get returns the named template.
func (s *Store) Get(name string) (Template, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.list {
		if t.Name == name {
			return t, true
		}
	}
	return Template{}, false
}

// EnabledSteps returns the ordered enabled step ids for the named template,
// falling back to global (the shared recipe) when the template has no own steps.
func (s *Store) EnabledSteps(name string, global func() []string) []string {
	t, ok := s.Get(name)
	if !ok || len(t.Steps) == 0 {
		if global != nil {
			return global()
		}
		return nil
	}
	out := make([]string, 0, len(t.Steps))
	for _, st := range t.Steps {
		if st.On {
			out = append(out, st.ID)
		}
	}
	return out
}

// StepMax returns the per-day action cap the named template sets for a step id
// (0 = the step's built-in default). A template inheriting the global recipe has
// no per-step caps.
func (s *Store) StepMax(name, stepID string) int {
	t, ok := s.Get(name)
	if !ok {
		return 0
	}
	for _, st := range t.Steps {
		if st.ID == stepID {
			return st.Max
		}
	}
	return 0
}

// Update mutates the named template in place and persists. The mutate callback
// receives a pointer to the stored template.
func (s *Store) Update(name string, mutate func(*Template)) error {
	s.mu.Lock()
	for i := range s.list {
		if s.list[i].Name == name {
			mutate(&s.list[i])
			s.mu.Unlock()
			return s.save()
		}
	}
	s.mu.Unlock()
	return fmt.Errorf("template %q not found", name)
}

// Add appends a new template (name must be non-empty and unique) and persists.
func (s *Store) Add(t Template) error {
	if t.Name == "" {
		return fmt.Errorf("template name required")
	}
	s.mu.Lock()
	for _, e := range s.list {
		if e.Name == t.Name {
			s.mu.Unlock()
			return fmt.Errorf("template %q already exists", t.Name)
		}
	}
	s.list = append(s.list, t)
	s.mu.Unlock()
	return s.save()
}

// Clone copies the named template under a fresh unique name and persists,
// returning the new name.
func (s *Store) Clone(name string) (string, error) {
	src, ok := s.Get(name)
	if !ok {
		return "", fmt.Errorf("template %q not found", name)
	}
	dup := src
	dup.Steps = append([]recipe.Step(nil), src.Steps...)
	dup.Name = s.uniqueName(name + "-copy")
	if err := s.Add(dup); err != nil {
		return "", err
	}
	return dup.Name, nil
}

// Delete removes the named template and persists. Removing the last template is
// refused so the fleet always has something to assign.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	if len(s.list) <= 1 {
		s.mu.Unlock()
		return fmt.Errorf("can't delete the last template")
	}
	for i := range s.list {
		if s.list[i].Name == name {
			s.list = append(s.list[:i], s.list[i+1:]...)
			s.mu.Unlock()
			return s.save()
		}
	}
	s.mu.Unlock()
	return fmt.Errorf("template %q not found", name)
}

// Rename changes a template's name (must stay unique) and persists. Per-dealer
// assignments reference templates BY NAME, so a renamed template's dealers fall
// back to the default until reassigned — the caller should warn.
func (s *Store) Rename(old, neu string) error {
	if neu == "" {
		return fmt.Errorf("name required")
	}
	s.mu.Lock()
	var target *Template
	for i := range s.list {
		if s.list[i].Name == neu && s.list[i].Name != old {
			s.mu.Unlock()
			return fmt.Errorf("template %q already exists", neu)
		}
		if s.list[i].Name == old {
			target = &s.list[i]
		}
	}
	if target == nil {
		s.mu.Unlock()
		return fmt.Errorf("template %q not found", old)
	}
	target.Name = neu
	s.mu.Unlock()
	return s.save()
}

// EnsureSteps fills a template's Steps from the default order (all on) when it has
// none, so the editor can toggle/reorder/cap them. Persists if it changed.
func (s *Store) EnsureSteps(name string, defaultOrder []string) error {
	return s.Update(name, func(t *Template) {
		if len(t.Steps) == 0 {
			t.Steps = make([]recipe.Step, len(defaultOrder))
			for i, id := range defaultOrder {
				t.Steps[i] = recipe.Step{ID: id, On: true}
			}
		}
	})
}

// uniqueName returns base, or base-2/base-3/… if taken. Caller holds no lock.
func (s *Store) uniqueName(base string) string {
	taken := map[string]bool{}
	for _, e := range s.All() {
		taken[e.Name] = true
	}
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if !taken[cand] {
			return cand
		}
	}
}

func (s *Store) save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.list, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
