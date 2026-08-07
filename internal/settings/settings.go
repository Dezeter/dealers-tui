// Package settings holds UI-managed, persisted global toggles (settings.json),
// so behaviours like auto-paying bail can be flipped in the interface without
// hand-editing config. Add a new switch by appending to Registry and reading it
// where it applies.
package settings

import (
	"encoding/json"
	"os"
	"sync"
)

// Toggle keys (stable JSON keys — never rename an existing one).
const (
	// KeyPayBail: when jailed and the free daily breakout is used up, pay ETH
	// bail to leave now instead of waiting for tomorrow's free attempt.
	KeyPayBail = "pay_bail_after_failed_breakout"
)

// Toggle is one switch shown on the settings screen.
type Toggle struct {
	Key   string
	Label string
	Desc  string
}

// Registry is the ordered list of toggles the settings screen renders. Append
// here to add a switch.
var Registry = []Toggle{
	{
		Key:   KeyPayBail,
		Label: "Pay bail after a failed breakout",
		Desc:  "When a dealer is jailed and its free daily escape is used up, spend ETH on bail to get out now (autopilot). Off by default.",
	},
}

// Store is a thread-safe set of boolean toggles persisted to a JSON file.
type Store struct {
	path string
	mu   sync.RWMutex
	on   map[string]bool
}

// Load reads the toggles at path (missing file = all off / defaults). Only known
// keys are kept.
func Load(path string) *Store {
	s := &Store{path: path, on: map[string]bool{}}
	if data, err := os.ReadFile(path); err == nil {
		var m map[string]bool
		if json.Unmarshal(data, &m) == nil {
			for _, t := range Registry {
				if v, ok := m[t.Key]; ok {
					s.on[t.Key] = v
				}
			}
		}
	}
	return s
}

// Get reports whether a toggle is on.
func (s *Store) Get(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.on[key]
}

// Set assigns a toggle and persists.
func (s *Store) Set(key string, v bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.on[key] = v
	return s.save()
}

// Toggle flips a toggle and persists, returning the new value.
func (s *Store) Toggle(key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.on[key] = !s.on[key]
	v := s.on[key]
	return v, s.save()
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.on, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
