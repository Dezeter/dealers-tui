// Package autostrat manages the per-dealer autopilot strategy selection: a
// UI-managed, persisted map of token id → policy tag layered over a fleet
// default. It exists so the strategy per NFT can be chosen from the interface
// (and survive restarts) without hand-editing config.json.
package autostrat

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Order is the cycle order used by the fleet-screen selector.
var Order = []string{"pve", "pvp", "manual"}

// migrate maps a retired tag to its replacement (so old files/configs still load).
var migrate = map[string]string{"farm": "pve"}

// Canonical returns tag, migrating a retired tag (e.g. the removed "farm") to its
// replacement; unknown tags are returned unchanged.
func Canonical(tag string) string {
	if to, ok := migrate[tag]; ok {
		return to
	}
	return tag
}

// Valid reports whether tag is a known policy.
func Valid(tag string) bool {
	for _, o := range Order {
		if o == tag {
			return true
		}
	}
	return false
}

// Store is a thread-safe token id → policy-tag map with a fleet default,
// persisted to a JSON file (missing file = empty; all dealers use the default).
type Store struct {
	path string
	mu   sync.RWMutex
	def  string            // fleet default when a dealer has no explicit tag
	tags map[uint64]string // per-dealer overrides, persisted to path
}

// Load builds the store from the default, an initial seed (e.g. config
// dealer_strategies), and the persisted file at path. File entries win over the
// seed; both must be valid tags or they're dropped.
func Load(path, def string, seed map[uint64]string) *Store {
	def = Canonical(def)
	if !Valid(def) {
		def = "pve"
	}
	s := &Store{path: path, def: def, tags: map[uint64]string{}}
	for id, tag := range seed {
		if tag = Canonical(tag); Valid(tag) {
			s.tags[id] = tag
		}
	}
	if data, err := os.ReadFile(path); err == nil {
		var m map[uint64]string
		if json.Unmarshal(data, &m) == nil {
			for id, tag := range m {
				if tag = Canonical(tag); Valid(tag) {
					s.tags[id] = tag
				}
			}
		}
	}
	return s
}

// Default returns the fleet-default tag.
func (s *Store) Default() string { return s.def }

// Get returns a dealer's policy tag (its override, else the fleet default).
func (s *Store) Get(id uint64) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if t, ok := s.tags[id]; ok {
		return t
	}
	return s.def
}

// Set assigns a dealer's policy tag and persists. A tag equal to the fleet
// default clears the override (keeps the file minimal).
func (s *Store) Set(id uint64, tag string) error {
	if !Valid(tag) {
		return fmt.Errorf("unknown strategy %q", tag)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tag == s.def {
		delete(s.tags, id)
	} else {
		s.tags[id] = tag
	}
	return s.save()
}

// Cycle advances a dealer to the next policy in Order and persists, returning
// the new tag.
func (s *Store) Cycle(id uint64) (string, error) {
	cur := s.Get(id)
	next := Order[0]
	for i, o := range Order {
		if o == cur {
			next = Order[(i+1)%len(Order)]
			break
		}
	}
	return next, s.Set(id, next)
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.tags, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
