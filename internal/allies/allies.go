// Package allies manages the do-not-attack list: the operator's own fleet
// (fixed, auto-protected) plus a user-managed list persisted to allies.json.
package allies

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
)

// Allies is a thread-safe do-not-attack set.
type Allies struct {
	path  string
	mu    sync.RWMutex
	fixed map[uint64]bool // own fleet + config seed — always allies, never removable
	user  map[uint64]bool // user-managed, persisted to path
}

// Load builds the set from the fixed ids and the user list at path (missing file
// = empty user list).
func Load(path string, fixed []uint64) *Allies {
	a := &Allies{path: path, fixed: set(fixed), user: map[uint64]bool{}}
	if data, err := os.ReadFile(path); err == nil {
		var ids []uint64
		if json.Unmarshal(data, &ids) == nil {
			a.user = set(ids)
		}
	}
	return a
}

// IsAlly reports whether a dealer is protected (fixed or user-added).
func (a *Allies) IsAlly(id uint64) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.fixed[id] || a.user[id]
}

// List returns the user-managed allies, sorted (fixed/own-fleet excluded).
func (a *Allies) List() []uint64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return keys(a.user)
}

// FixedCount is how many dealers are auto-protected (own fleet + config seed).
func (a *Allies) FixedCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.fixed)
}

// Toggle adds id to the user list if absent, removes it if present, and
// persists. A fixed id (own dealer) is always an ally and cannot be toggled off
// (returns fixed=true, no change).
func (a *Allies) Toggle(id uint64) (added, fixed bool, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fixed[id] {
		return true, true, nil
	}
	if a.user[id] {
		delete(a.user, id)
		return false, false, a.save()
	}
	a.user[id] = true
	return true, false, a.save()
}

func (a *Allies) save() error {
	data, err := json.MarshalIndent(keys(a.user), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.path, data, 0o600)
}

func set(ids []uint64) map[uint64]bool {
	m := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func keys(m map[uint64]bool) []uint64 {
	out := make([]uint64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
