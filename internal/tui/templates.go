package tui

import (
	"fmt"
	"strconv"
	"strings"

	"dealers/internal/dealer"
	"dealers/internal/template"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// TemplatesModel manages the named strategy presets: a list view to create/clone/
// delete templates, and an editor for a template's base strategy, trade route,
// heist difficulty, mission priority, and per-template step recipe with per-step
// daily caps. Edits persist immediately and apply to the autopilot on the next
// tick (the strategies read the template live).
type TemplatesModel struct {
	deps     Deps
	mode     int // modeList | modeEdit
	cursor   int
	editName string // template being edited (modeEdit)
	input    textinput.Model
	editing  bool   // text input focused
	editKind string // which row the input is editing ("name"/"drug"/"buy"/"sell"/"max")
	editStep int    // step index when editKind=="max"
	notice   string
}

const (
	modeList = iota
	modeEdit
)

func NewTemplates(deps Deps) TemplatesModel {
	ti := textinput.New()
	ti.CharLimit = 24
	ti.Width = 24
	return TemplatesModel{deps: deps, input: ti}
}

func (m TemplatesModel) Init() tea.Cmd { return nil }

// names returns the current template names.
func (m TemplatesModel) names() []string {
	if m.deps.Templates == nil {
		return nil
	}
	return m.deps.Templates.Names()
}

func (m TemplatesModel) Update(msg tea.Msg) (TemplatesModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok || m.deps.Templates == nil {
		return m, nil
	}
	if m.editing {
		return m.updateEditing(key)
	}
	if m.mode == modeList {
		return m.updateList(key)
	}
	return m.updateEdit(key)
}

// ---- list mode ----

func (m TemplatesModel) updateList(key tea.KeyMsg) (TemplatesModel, tea.Cmd) {
	names := m.names()
	n := len(names)
	switch key.String() {
	case "esc", "q":
		return m, func() tea.Msg { return backToFleetMsg{} }
	case "up", "k":
		if n > 0 {
			m.cursor = (m.cursor - 1 + n) % n
		}
	case "down", "j":
		if n > 0 {
			m.cursor = (m.cursor + 1) % n
		}
	case "enter":
		if n > 0 {
			m.editName = names[m.cursor]
			m.mode, m.cursor = modeEdit, 0
		}
	case "n": // new blank template → jump straight into editing it
		t := withUniqueName(m.deps.Templates, template.Template{Name: "template", Strategy: "pve", Params: template.Params{HeistDifficulty: -1}})
		if err := m.deps.Templates.Add(t); err != nil {
			m.notice = errStyle.Render("create failed: " + err.Error())
		} else {
			m.editName, m.mode, m.cursor, m.notice = t.Name, modeEdit, 0, ""
		}
	case "c": // clone selected
		if n > 0 {
			if nm, err := m.deps.Templates.Clone(names[m.cursor]); err != nil {
				m.notice = errStyle.Render("clone failed: " + err.Error())
			} else {
				m.notice = okStyle.Render("cloned → " + nm)
			}
		}
	case "d": // delete selected
		if n > 0 {
			nm := names[m.cursor]
			if err := m.deps.Templates.Delete(nm); err != nil {
				m.notice = errStyle.Render(err.Error())
			} else {
				m.notice = statusBarStyle.Render("deleted " + nm)
				if m.cursor >= n-1 && m.cursor > 0 {
					m.cursor--
				}
			}
		}
	}
	return m, nil
}

// withUniqueName defers to the store's own de-dup by trying Add-friendly names.
func withUniqueName(s *template.Store, t template.Template) template.Template {
	base := t.Name
	taken := map[string]bool{}
	for _, n := range s.Names() {
		taken[n] = true
	}
	if !taken[base] {
		return t
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if !taken[cand] {
			t.Name = cand
			return t
		}
	}
}

// ---- edit mode ----

// editRow is one navigable line of the editor.
type editRow struct {
	kind    string // name|strategy|drug|buy|sell|heist|mission|stepsToggle|step
	stepIdx int
}

func editRows(t template.Template) []editRow {
	rows := []editRow{
		{kind: "name"}, {kind: "strategy"}, {kind: "drug"},
		{kind: "buy"}, {kind: "sell"}, {kind: "heist"}, {kind: "mission"},
	}
	if len(t.Steps) == 0 {
		rows = append(rows, editRow{kind: "stepsToggle"})
	} else {
		for i := range t.Steps {
			rows = append(rows, editRow{kind: "step", stepIdx: i})
		}
	}
	return rows
}

func (m TemplatesModel) updateEdit(key tea.KeyMsg) (TemplatesModel, tea.Cmd) {
	t, ok := m.deps.Templates.Get(m.editName)
	if !ok {
		m.mode = modeList
		return m, nil
	}
	rows := editRows(t)
	n := len(rows)
	switch key.String() {
	case "esc":
		m.mode, m.cursor = modeList, 0
		return m, nil
	case "up", "k":
		m.cursor = (m.cursor - 1 + n) % n
		return m, nil
	case "down", "j":
		m.cursor = (m.cursor + 1) % n
		return m, nil
	}
	row := rows[m.cursor]
	switch row.kind {
	case "name":
		if key.String() == "enter" {
			return m.startEditing("name", 0, m.editName)
		}
	case "drug":
		if key.String() == "enter" {
			return m.startEditing("drug", 0, t.Params.Drug)
		}
	case "buy":
		if key.String() == "enter" {
			return m.startEditing("buy", 0, t.Params.BuyArea)
		}
	case "sell":
		if key.String() == "enter" {
			return m.startEditing("sell", 0, t.Params.SellArea)
		}
	case "strategy":
		if isCycleKey(key) {
			m.persist(func(x *template.Template) { x.Strategy = nextStrategy(x.Strategy) })
		}
	case "heist":
		if isCycleKey(key) {
			m.persist(func(x *template.Template) { x.Params.HeistDifficulty = nextDiff(x.Params.HeistDifficulty) })
		}
	case "mission":
		if isCycleKey(key) {
			m.persist(func(x *template.Template) {
				if x.Params.MissionPriority == "weekly" {
					x.Params.MissionPriority = "daily"
				} else {
					x.Params.MissionPriority = "weekly"
				}
			})
		}
	case "stepsToggle":
		if key.String() == " " || key.String() == "enter" {
			if err := m.deps.Templates.EnsureSteps(m.editName, dealer.DefaultStepOrder()); err != nil {
				m.notice = errStyle.Render(err.Error())
			} else {
				m.notice = okStyle.Render("steps customised — this template no longer inherits the global recipe")
			}
		}
	case "step":
		return m.updateStepRow(key, row.stepIdx, len(t.Steps))
	}
	return m, nil
}

func (m TemplatesModel) updateStepRow(key tea.KeyMsg, idx, count int) (TemplatesModel, tea.Cmd) {
	switch key.String() {
	case " ", "x":
		m.persist(func(x *template.Template) {
			if idx < len(x.Steps) {
				x.Steps[idx].On = !x.Steps[idx].On
			}
		})
	case "[", "-", "K": // move up
		if idx > 0 {
			m.persist(func(x *template.Template) { x.Steps[idx-1], x.Steps[idx] = x.Steps[idx], x.Steps[idx-1] })
			m.cursor--
		}
	case "]", "+", "J": // move down
		if idx < count-1 {
			m.persist(func(x *template.Template) { x.Steps[idx+1], x.Steps[idx] = x.Steps[idx], x.Steps[idx+1] })
			m.cursor++
		}
	case "m", "enter": // edit the per-day cap
		cur := ""
		if t, ok := m.deps.Templates.Get(m.editName); ok && idx < len(t.Steps) && t.Steps[idx].Max > 0 {
			cur = strconv.Itoa(t.Steps[idx].Max)
		}
		return m.startEditing("max", idx, cur)
	case "r": // reset steps → inherit global again
		m.persist(func(x *template.Template) { x.Steps = nil })
		m.cursor = 7 // back to the steps toggle row
	}
	return m, nil
}

// startEditing focuses the shared input, prefilled, to edit a text/number field.
func (m TemplatesModel) startEditing(kind string, stepIdx int, cur string) (TemplatesModel, tea.Cmd) {
	m.editing, m.editKind, m.editStep = true, kind, stepIdx
	m.input.SetValue(cur)
	m.input.CursorEnd()
	if kind == "max" {
		m.input.Placeholder = "0 = no limit"
	} else {
		m.input.Placeholder = ""
	}
	return m, m.input.Focus()
}

func (m TemplatesModel) updateEditing(key tea.KeyMsg) (TemplatesModel, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.editing = false
		m.input.Blur()
		return m, nil
	case "enter":
		m.commitEditing()
		m.editing = false
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(key)
	return m, cmd
}

func (m *TemplatesModel) commitEditing() {
	val := strings.TrimSpace(m.input.Value())
	switch m.editKind {
	case "name":
		if val == "" || val == m.editName {
			return
		}
		if err := m.deps.Templates.Rename(m.editName, val); err != nil {
			m.notice = errStyle.Render(err.Error())
			return
		}
		m.editName = val
		m.notice = okStyle.Render("renamed — reassign dealers to it with s (old assignments fall back to default)")
	case "drug":
		m.persist(func(x *template.Template) { x.Params.Drug = val })
	case "buy":
		m.persist(func(x *template.Template) { x.Params.BuyArea = val })
	case "sell":
		m.persist(func(x *template.Template) { x.Params.SellArea = val })
	case "max":
		n, err := strconv.Atoi(val)
		if val == "" {
			n = 0
		} else if err != nil || n < 0 {
			m.notice = errStyle.Render("cap must be a number ≥ 0")
			return
		}
		idx := m.editStep
		m.persist(func(x *template.Template) {
			if idx < len(x.Steps) {
				x.Steps[idx].Max = n
			}
		})
	}
}

// persist applies mutate to the edited template and records any save error.
func (m *TemplatesModel) persist(mutate func(*template.Template)) {
	if err := m.deps.Templates.Update(m.editName, mutate); err != nil {
		m.notice = errStyle.Render("save failed: " + err.Error())
	}
}

func isCycleKey(key tea.KeyMsg) bool {
	switch key.String() {
	case " ", "left", "right", "h", "l", "enter":
		return true
	}
	return false
}

func nextStrategy(s string) string {
	switch s {
	case "pve":
		return "pvp"
	case "pvp":
		return "manual"
	default:
		return "pve"
	}
}

func nextDiff(d int) int {
	if d < 0 {
		return 0
	}
	if d >= 2 {
		return -1
	}
	return d + 1
}

func diffLabel(d int) string {
	if d < 0 {
		return "auto (max affordable)"
	}
	return strconv.Itoa(d)
}

func missionLabelFor(p string) string {
	if p == "weekly" {
		return "weekly-first"
	}
	return "daily-first"
}

// ---- view ----

func (m TemplatesModel) View() string {
	if m.deps.Templates == nil {
		return titleStyle.Render("TEMPLATES") + "\n\n" + helpStyle.Render("unavailable")
	}
	if m.mode == modeList {
		return m.viewList()
	}
	return m.viewEdit()
}

func (m TemplatesModel) viewList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("STRATEGY TEMPLATES") + "\n")
	b.WriteString(helpStyle.Render("named presets assigned per-NFT (cycle with s on the fleet); autopilot reads them live") + "\n\n")
	for i, t := range m.deps.Templates.All() {
		cursor := "  "
		name := t.Name
		if i == m.cursor {
			cursor = focusStyle.Render("▸ ")
			name = focusStyle.Render(name)
		}
		fmt.Fprintf(&b, "%s%s  %s\n", cursor, name, helpStyle.Render(summarize(t)))
	}
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("↑/↓ · enter edit · n new · c clone · d delete · esc back"))
	return b.String()
}

// summarize renders a one-line preview of a template.
func summarize(t template.Template) string {
	if t.Strategy == "manual" {
		return "manual — does nothing"
	}
	route := "weed Manhattan→Amsterdam"
	if t.Params.Drug != "" || t.Params.BuyArea != "" || t.Params.SellArea != "" {
		drug := t.Params.Drug
		if drug == "" {
			drug = "weed"
		}
		buy, sell := t.Params.BuyArea, t.Params.SellArea
		if buy == "" {
			buy = "Manhattan"
		}
		if sell == "" {
			sell = "Amsterdam"
		}
		route = fmt.Sprintf("%s %s→%s", drug, buy, sell)
	}
	steps := "inherits global steps"
	if len(t.Steps) > 0 {
		steps = fmt.Sprintf("%d custom steps", len(t.Steps))
	}
	return fmt.Sprintf("%s · %s · heist %s · %s · %s",
		t.Strategy, route, diffLabel(t.Params.HeistDifficulty), missionLabelFor(t.Params.MissionPriority), steps)
}

func (m TemplatesModel) viewEdit() string {
	t, ok := m.deps.Templates.Get(m.editName)
	if !ok {
		return titleStyle.Render("TEMPLATE") + "\n\n" + helpStyle.Render("gone — esc back")
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("TEMPLATE · "+t.Name) + "\n")
	b.WriteString(helpStyle.Render("↑/↓ move · enter edit/cycle a field · esc back · changes apply live") + "\n\n")

	rows := editRows(t)
	for i, row := range rows {
		b.WriteString(m.renderRow(i, row, t) + "\n")
	}
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	b.WriteString("\n" + helpStyle.Render(m.editHelp(rows)))
	return b.String()
}

func (m TemplatesModel) renderRow(i int, row editRow, t template.Template) string {
	sel := i == m.cursor
	cur := "  "
	if sel {
		cur = focusStyle.Render("▸ ")
	}
	label := func(name, val string) string {
		l := fmt.Sprintf("%-16s %s", name, val)
		if sel {
			l = focusStyle.Render(fmt.Sprintf("%-16s ", name)) + val
		}
		return cur + l
	}
	// While editing a text field, show the input inline for that row.
	editingThis := m.editing && sel
	inputView := m.input.View()

	switch row.kind {
	case "name":
		if editingThis {
			return label("name", inputView)
		}
		return label("name", t.Name)
	case "strategy":
		return label("strategy", t.Strategy+helpStyle.Render("  (space cycles pve/pvp/manual)"))
	case "drug":
		if editingThis {
			return label("drug", inputView)
		}
		return label("drug", orDim(t.Params.Drug, "weed"))
	case "buy":
		if editingThis {
			return label("buy zone", inputView)
		}
		return label("buy zone", orDim(t.Params.BuyArea, "Manhattan"))
	case "sell":
		if editingThis {
			return label("sell zone", inputView)
		}
		return label("sell zone", orDim(t.Params.SellArea, "Amsterdam"))
	case "heist":
		return label("heist difficulty", diffLabel(t.Params.HeistDifficulty))
	case "mission":
		return label("mission priority", missionLabelFor(t.Params.MissionPriority))
	case "stepsToggle":
		return cur + helpStyle.Render("steps: inherits the global recipe — space to customise")
	case "step":
		return cur + m.renderStep(row.stepIdx, t, sel, editingThis, inputView)
	}
	return ""
}

func (m TemplatesModel) renderStep(idx int, t template.Template, sel, editingThis bool, inputView string) string {
	if idx >= len(t.Steps) {
		return ""
	}
	st := t.Steps[idx]
	box := helpStyle.Render("[ ]")
	if st.On {
		box = okStyle.Render("[x]")
	}
	name := stepMeta(st.ID).Label
	if !st.On {
		name = helpStyle.Render(name)
	} else if sel {
		name = focusStyle.Render(name)
	}
	cap := "no limit"
	if st.Max > 0 {
		cap = fmt.Sprintf("%d/day", st.Max)
	}
	if editingThis {
		cap = inputView
	}
	return fmt.Sprintf("%s %s  %s", box, name, helpStyle.Render("cap: ")+cap)
}

func (m TemplatesModel) editHelp(rows []editRow) string {
	if m.editing {
		return "type · enter save · esc cancel"
	}
	if m.cursor < len(rows) && rows[m.cursor].kind == "step" {
		return "space toggle · [ ] move · m set daily cap · r reset to global · esc back"
	}
	return "enter edit/cycle · esc back"
}

// orDim shows val, or a dimmed default when val is empty.
func orDim(val, def string) string {
	if val == "" {
		return helpStyle.Render(def + " (default)")
	}
	return val
}
