// Package recipe manages the UI-editable autopilot step order — a persisted,
// ordered list of steps with on/off flags (recipes.json), so people can change
// what the autopilot does and in what order without editing config or code. The
// step ids and their meaning live in the dealer package; this store is generic
// over ids.
package recipe

import (
	"encoding/json"
	"os"
	"sync"
)

// Step is one entry in the ordered recipe.
type Step struct {
	ID string `json:"id"`
	On bool   `json:"on"`
	// Max is the per-UTC-day cap on how many times this step may perform its
	// primary action (a PvE deal, a PvP attack, a heist start, a poster attempt).
	// 0 = the step's built-in default (unbounded for most, 3 for heists). Only the
	// action-emitting steps honour it; plumbing (travel/check-in/claim) ignores it.
	Max int `json:"max,omitempty"`
}

// Store is the ordered, toggleable step recipe, persisted to a JSON file.
type Store struct {
	path  string
	mu    sync.RWMutex
	steps []Step
	def   []string // default order (every known step id)
}

// Load merges the default order (every id, all on) with the persisted file: the
// file's order/on wins for known ids, ids no longer known are dropped, and any
// new default id missing from the file is appended (on). This keeps saved recipes
// valid across versions that add or remove steps.
func Load(path string, defaultOrder []string) *Store {
	s := &Store{path: path, def: append([]string(nil), defaultOrder...)}
	valid := map[string]bool{}
	for _, id := range defaultOrder {
		valid[id] = true
	}
	var file []Step
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &file)
	}
	seen := map[string]bool{}
	for _, st := range file {
		if valid[st.ID] && !seen[st.ID] {
			s.steps = append(s.steps, st)
			seen[st.ID] = true
		}
	}
	for _, id := range defaultOrder {
		if !seen[id] {
			s.steps = append(s.steps, Step{ID: id, On: true})
		}
	}
	return s
}

// Enabled returns the ordered ids of the on steps — the live recipe the autopilot
// runs.
func (s *Store) Enabled() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.steps))
	for _, st := range s.steps {
		if st.On {
			out = append(out, st.ID)
		}
	}
	return out
}

// All returns the full ordered recipe (for the editor).
func (s *Store) All() []Step {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Step(nil), s.steps...)
}

// Toggle flips the on/off of the step at index and persists.
func (s *Store) Toggle(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.steps) {
		return nil
	}
	s.steps[index].On = !s.steps[index].On
	return s.save()
}

// Move shifts the step at index by delta (−1 up, +1 down), clamped, and persists;
// returns the new index.
func (s *Store) Move(index, delta int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := index + delta
	if index < 0 || index >= len(s.steps) || j < 0 || j >= len(s.steps) {
		return index, nil
	}
	s.steps[index], s.steps[j] = s.steps[j], s.steps[index]
	return j, s.save()
}

// Reset restores the default order (all on) and persists.
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = s.steps[:0]
	for _, id := range s.def {
		s.steps = append(s.steps, Step{ID: id, On: true})
	}
	return s.save()
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.steps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
