package template

// Migration from the old template shape (a base strategy + shared params + a
// recipe of toggled pipeline steps) to the new step-program shape. New-format
// files pass through unchanged; old-format ones are converted once on load.

// rawTemplate accepts BOTH the new and the old on-disk shapes so one unmarshal
// pass can tell them apart.
type rawTemplate struct {
	Name  string    `json:"name"`
	Steps []rawStep `json:"steps,omitempty"`

	// Old-format only:
	Strategy string     `json:"strategy,omitempty"`
	Params   *rawParams `json:"params,omitempty"`
}

type rawStep struct {
	// New-format fields:
	Action          string `json:"action,omitempty"`
	Drug            string `json:"drug,omitempty"`
	BuyArea         string `json:"buy_area,omitempty"`
	SellArea        string `json:"sell_area,omitempty"`
	HeistDifficulty int8   `json:"heist_difficulty,omitempty"`
	HeatAt          int8   `json:"heat_at,omitempty"`
	PayBail         bool   `json:"pay_bail,omitempty"`
	Count           int    `json:"count,omitempty"`

	// Old-format recipe fields:
	ID  string `json:"id,omitempty"`
	On  *bool  `json:"on,omitempty"`
	Max int    `json:"max,omitempty"`
}

type rawParams struct {
	Drug            string `json:"drug,omitempty"`
	BuyArea         string `json:"buy_area,omitempty"`
	SellArea        string `json:"sell_area,omitempty"`
	HeistDifficulty int    `json:"heist_difficulty,omitempty"`
	MissionPriority string `json:"mission_priority,omitempty"`
}

// isOld reports whether this record is in the old strategy/recipe shape.
func (rt rawTemplate) isOld() bool {
	if rt.Strategy != "" || rt.Params != nil {
		return true
	}
	for _, s := range rt.Steps {
		if s.Action == "" && (s.ID != "" || s.On != nil) {
			return true
		}
	}
	return false
}

// compile returns the template in the new shape, migrating if needed.
func (rt rawTemplate) compile() Template {
	if !rt.isOld() {
		steps := make([]Step, 0, len(rt.Steps))
		for _, s := range rt.Steps {
			if s.Action == "" {
				continue
			}
			steps = append(steps, Step{
				Action: s.Action, Drug: s.Drug, BuyArea: s.BuyArea, SellArea: s.SellArea,
				HeistDifficulty: s.HeistDifficulty, HeatAt: s.HeatAt, PayBail: s.PayBail, Count: s.Count,
			})
		}
		return Template{Name: rt.Name, Steps: steps}
	}
	return Template{Name: rt.Name, Steps: rt.migrate()}
}

// migrate builds a step program from the old strategy + recipe + params. A manual
// strategy becomes an empty program (does nothing). Otherwise the program starts
// with a breakout step (the old implicit jailbreak) then the enabled recipe steps
// in order, with the trade/pvp "core" and the heist difficulty pulled in from the
// shared params.
func (rt rawTemplate) migrate() []Step {
	p := rt.Params
	if p == nil {
		p = &rawParams{}
	}
	if rt.Strategy == "manual" {
		return nil
	}
	core := Step{Action: ActionTrade, Drug: p.Drug, BuyArea: p.BuyArea, SellArea: p.SellArea}
	if rt.Strategy == "pvp" {
		core = Step{Action: ActionPvP}
	}

	// Determine the enabled recipe order. If the old template had no explicit
	// recipe it inherited the global default order — reproduce that.
	order := rt.enabledRecipe()
	if len(order) == 0 {
		for _, id := range []string{"clear_stars", "heist_checkin", "missions", "heists", "core"} {
			order = append(order, recipeEntry{id: id})
		}
	}

	steps := []Step{{Action: ActionBreakout}}
	for _, r := range order {
		switch r.id {
		case "clear_stars":
			steps = append(steps, Step{Action: ActionClearStars, Count: r.max})
		case "heist_checkin":
			steps = append(steps, Step{Action: ActionHeistCheckIn, Count: r.max})
		case "missions":
			steps = append(steps, Step{Action: ActionMissions, Count: r.max})
		case "heists":
			steps = append(steps, Step{Action: ActionHeist, HeistDifficulty: int8(p.HeistDifficulty), Count: r.max})
		case "core":
			c := core
			c.Count = r.max
			steps = append(steps, c)
		case "follow_missions":
			// folded into the mission/trade steps in the new model — dropped
		}
	}
	return steps
}

type recipeEntry struct {
	id  string
	max int
}

// enabledRecipe returns the old recipe's enabled step ids in order (empty if the
// template had no explicit recipe).
func (rt rawTemplate) enabledRecipe() []recipeEntry {
	var out []recipeEntry
	for _, s := range rt.Steps {
		if s.ID == "" {
			continue
		}
		if s.On == nil || *s.On {
			out = append(out, recipeEntry{id: s.ID, max: s.Max})
		}
	}
	return out
}
