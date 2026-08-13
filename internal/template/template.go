// Package template manages named autopilot programs ("strategy templates"). A
// template is an ORDERED LIST OF STEPS the autopilot runs sequentially per dealer:
// step 1 repeats until it's satisfied (a fixed count, or "until it has nothing
// left to do"), then the program advances to step 2, and so on; at the end it
// loops back to the start. Each step is self-contained — it names an action
// (trade / pvp / heist / clear stars / breakout / heist check-in / missions) and
// carries that action's own parameters — so there is no separate "strategy"
// concept: the steps ARE the strategy.
//
// Templates are assigned to dealers per NFT (via internal/autostrat, whose values
// are template names) so different dealers run different programs. The per-dealer
// program position (which step, how many reps done) lives in internal/progstate.
//
// Definitions persist to templates.json. An older strategy/params/recipe file is
// migrated to the step-program shape on load.
package template

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Action identifiers for a program step. Stable JSON values — never rename.
const (
	ActionBreakout     = "breakout"      // escape jail (free daily attempt, then bail if the setting is on)
	ActionClearStars   = "clear_stars"   // free wanted-poster heat clear (at heat ≥ 3)
	ActionHeistCheckIn = "heist_checkin" // daily bank-heist season check-in (auto-joins, pays $CASH)
	ActionTrade        = "trade"         // buy/haul/sell a drug between two zones
	ActionPvP          = "pvp"           // attack a present non-ally target
	ActionHeist        = "heist"         // run a heist at a difficulty and bank it

	// Missions, split into separate concepts you can place independently:
	ActionMissionsAccept = "missions_accept" // accept (check-in) the epoch's daily missions so they track
	ActionMissionsFollow = "missions_follow" // do the activity an accepted mission needs (opportunistic)
	ActionMissionsClaim  = "missions_claim"  // claim finished missions' rewards

	// ActionMissions is the legacy combined step (claim + accept). Kept so older
	// templates keep running; it's no longer offered when adding a new step.
	ActionMissions = "missions"
)

// ActionOrder is the catalog order the editor cycles through.
var ActionOrder = []string{
	ActionTrade, ActionPvP, ActionHeist, ActionClearStars, ActionBreakout, ActionHeistCheckIn,
	ActionMissionsAccept, ActionMissionsFollow, ActionMissionsClaim,
}

// usesTrade / usesHeist report which params a given action reads (for the editor).
func UsesTradeParams(action string) bool { return action == ActionTrade }
func UsesHeistParams(action string) bool { return action == ActionHeist }

// Step is one program step: an action plus the params that action needs, and how
// many times to repeat it before advancing.
type Step struct {
	Action string `json:"action"`

	// Trade params (Action == "trade"). Empty = weed / Manhattan / Amsterdam.
	Drug     string `json:"drug,omitempty"`
	BuyArea  string `json:"buy_area,omitempty"`
	SellArea string `json:"sell_area,omitempty"`

	// Heist param (Action == "heist"): -1 = highest affordable; 0..2 = fixed tier.
	HeistDifficulty int8 `json:"heist_difficulty,omitempty"`

	// HeatAt (Action == "clear_stars"): the wanted-star level at which the step
	// activates. 0 = the default of 3. It won't clear below this threshold.
	HeatAt int8 `json:"heat_at,omitempty"`

	// PayBail (Action == "breakout"): when the free daily breakout attempt is used
	// up, pay ETH bail to leave now instead of waiting for tomorrow's free attempt.
	PayBail bool `json:"pay_bail,omitempty"`

	// Count is how many times to repeat the action before advancing to the next
	// step. 0 means "until it has nothing left to do" (до успеха): keep running the
	// step while it can still act, then advance. N means "advance after N actions".
	Count int `json:"count,omitempty"`
}

// UsesHeatParam / UsesBailParam report which extra param a given action reads.
func UsesHeatParam(action string) bool { return action == ActionClearStars }
func UsesBailParam(action string) bool { return action == ActionBreakout }

// Template is a named autopilot program.
type Template struct {
	Name  string `json:"name"`
	Steps []Step `json:"steps"`
}

// Defaults returns the seeded programs. They keep the classic pve/pvp/manual names
// (so existing per-dealer assignments, which reference templates by name, still
// resolve) and reproduce roughly the old behaviour as explicit step programs.
func Defaults() []Template {
	head := []Step{
		{Action: ActionBreakout},
		{Action: ActionClearStars},
		{Action: ActionHeistCheckIn},
		{Action: ActionMissionsAccept},
	}
	claim := Step{Action: ActionMissionsClaim}
	pve := append(append(append([]Step(nil), head...), Step{Action: ActionTrade}), claim)
	pvp := append(append(append([]Step(nil), head...), Step{Action: ActionPvP}), claim)
	return []Template{
		{Name: "pve", Steps: pve},
		{Name: "pvp", Steps: pvp},
		{Name: "manual", Steps: nil}, // does nothing
	}
}

// Store holds the template definitions, persisted to a JSON file.
type Store struct {
	path string
	mu   sync.RWMutex
	list []Template
}

// Load reads templates.json (migrating an old strategy/params file); a missing or
// empty file seeds the defaults and writes them so the file is there to edit.
func Load(path string) *Store {
	s := &Store{path: path}
	if data, err := os.ReadFile(path); err == nil {
		s.list = parse(data)
	}
	if len(s.list) == 0 {
		s.list = Defaults()
		_ = s.save()
	}
	return s
}

// parse unmarshals the file, migrating any templates still in the old
// strategy/params/recipe shape into step programs.
func parse(data []byte) []Template {
	var raw []rawTemplate
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	out := make([]Template, 0, len(raw))
	for _, rt := range raw {
		if rt.Name == "" {
			continue
		}
		out = append(out, rt.compile())
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
	return cloneAll(s.list)
}

// Get returns a deep copy of the named template.
func (s *Store) Get(name string) (Template, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.list {
		if t.Name == name {
			return cloneTemplate(t), true
		}
	}
	return Template{}, false
}

// Update mutates the named template in place and persists.
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

// Add appends a new template (non-empty, unique name) and persists.
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
	s.list = append(s.list, cloneTemplate(t))
	s.mu.Unlock()
	return s.save()
}

// Clone copies the named template under a fresh unique name and persists.
func (s *Store) Clone(name string) (string, error) {
	src, ok := s.Get(name)
	if !ok {
		return "", fmt.Errorf("template %q not found", name)
	}
	src.Name = s.uniqueName(name + "-copy")
	if err := s.Add(src); err != nil {
		return "", err
	}
	return src.Name, nil
}

// Delete removes the named template (refusing the last one) and persists.
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

// Rename changes a template's name (must stay unique) and persists.
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

// uniqueName returns base, or base-2/base-3/… if taken. Caller holds no lock.
func (s *Store) uniqueName(base string) string {
	taken := map[string]bool{}
	for _, n := range s.Names() {
		taken[n] = true
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

func cloneTemplate(t Template) Template {
	t.Steps = append([]Step(nil), t.Steps...)
	return t
}

func cloneAll(in []Template) []Template {
	out := make([]Template, len(in))
	for i, t := range in {
		out[i] = cloneTemplate(t)
	}
	return out
}
