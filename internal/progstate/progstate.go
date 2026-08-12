// Package progstate persists each dealer's position in its autopilot program —
// which step it's on and how many repetitions of that step it has done — so a
// sequential template program resumes exactly where it left off across restarts.
// It mirrors the other UI-managed JSON stores (one small file, thread-safe).
package progstate

import (
	"encoding/json"
	"os"
	"strconv"
	"sync"
)

// Pos is a dealer's program position: the current step index and how many reps of
// that step have completed.
type Pos struct {
	Step int `json:"step"`
	Reps int `json:"reps"`
}

// Store maps token id → program position, persisted to a JSON file.
type Store struct {
	path string
	mu   sync.Mutex
	pos  map[string]Pos // keyed by decimal token id (JSON object keys are strings)
}

// Load reads the state file (missing/invalid = empty).
func Load(path string) *Store {
	s := &Store{path: path, pos: map[string]Pos{}}
	if data, err := os.ReadFile(path); err == nil {
		var m map[string]Pos
		if json.Unmarshal(data, &m) == nil && m != nil {
			s.pos = m
		}
	}
	return s
}

// Get returns the stored position for a dealer (zero value = start of program).
func (s *Store) Get(tokenID uint64) Pos {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pos[key(tokenID)]
}

// Set stores a dealer's position and persists it — but only when it actually
// changed, so the fast autopilot tick doesn't rewrite the file every second.
func (s *Store) Set(tokenID uint64, p Pos) error {
	s.mu.Lock()
	k := key(tokenID)
	if s.pos[k] == p {
		s.mu.Unlock()
		return nil
	}
	s.pos[k] = p
	data, err := json.MarshalIndent(s.pos, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func key(tokenID uint64) string { return strconv.FormatUint(tokenID, 10) }
