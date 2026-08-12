package tui

import (
	"fmt"
	"strconv"
	"strings"

	"dealers/internal/i18n"
	"dealers/internal/template"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// TemplatesModel edits the named autopilot programs. A template is an ordered list
// of steps; each step names an action (trade / pvp / heist / clear stars /
// breakout / heist check-in / missions) and carries that action's params plus a
// repeat count (0 = "until done"). Three screens: the template list, a template's
// step list, and a single step's fields. Edits persist immediately and apply to
// the autopilot on the next tick.
type TemplatesModel struct {
	deps     Deps
	mode     int // modeList | modeSteps | modeStep
	cursor   int // list index (modeList) or step index (modeSteps)
	field    int // field index (modeStep)
	editName string
	stepIdx  int // step being edited (modeStep)
	input    textinput.Model
	editing  bool   // text input focused
	editKind string // field under text edit ("name"/"drug"/"buy"/"sell"/"count")
	notice   string
}

const (
	modeList = iota
	modeSteps
	modeStep
)

func NewTemplates(deps Deps) TemplatesModel {
	ti := textinput.New()
	ti.CharLimit = 24
	ti.Width = 24
	return TemplatesModel{deps: deps, input: ti}
}

func (m TemplatesModel) Init() tea.Cmd { return nil }

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
	switch m.mode {
	case modeList:
		return m.updateList(key)
	case modeSteps:
		return m.updateSteps(key)
	default:
		return m.updateStep(key)
	}
}

// ---- template list ----

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
			m.editName, m.mode, m.cursor = names[m.cursor], modeSteps, 0
		}
	case "n":
		t := withUniqueName(m.deps.Templates, template.Template{Name: "template"})
		if err := m.deps.Templates.Add(t); err != nil {
			m.notice = errStyle.Render(i18n.T("templates.create_failed") + ": " + err.Error())
		} else {
			m.editName, m.mode, m.cursor, m.notice = t.Name, modeSteps, 0, ""
		}
	case "c":
		if n > 0 {
			if nm, err := m.deps.Templates.Clone(names[m.cursor]); err != nil {
				m.notice = errStyle.Render(i18n.T("templates.clone_failed") + ": " + err.Error())
			} else {
				m.notice = okStyle.Render(i18n.T("templates.cloned", nm))
			}
		}
	case "d":
		if n > 0 {
			nm := names[m.cursor]
			if err := m.deps.Templates.Delete(nm); err != nil {
				m.notice = errStyle.Render(err.Error())
			} else {
				m.notice = statusBarStyle.Render(i18n.T("templates.deleted", nm))
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

// ---- step list ----

func (m TemplatesModel) updateSteps(key tea.KeyMsg) (TemplatesModel, tea.Cmd) {
	t, ok := m.deps.Templates.Get(m.editName)
	if !ok {
		m.mode = modeList
		return m, nil
	}
	n := len(t.Steps)
	switch key.String() {
	case "esc":
		m.mode, m.cursor = modeList, 0
	case "up", "k":
		if n > 0 {
			m.cursor = (m.cursor - 1 + n) % n
		}
	case "down", "j":
		if n > 0 {
			m.cursor = (m.cursor + 1) % n
		}
	case "a": // add a step (defaults to trade) and edit it
		m.persist(func(x *template.Template) {
			x.Steps = append(x.Steps, template.Step{Action: template.ActionTrade})
		})
		if t2, ok := m.deps.Templates.Get(m.editName); ok {
			m.stepIdx, m.mode, m.field = len(t2.Steps)-1, modeStep, 0
		}
	case "enter":
		if n > 0 {
			m.stepIdx, m.mode, m.field = m.cursor, modeStep, 0
		}
	case "d":
		if n > 0 {
			idx := m.cursor
			m.persist(func(x *template.Template) {
				x.Steps = append(x.Steps[:idx], x.Steps[idx+1:]...)
			})
			if m.cursor >= n-1 && m.cursor > 0 {
				m.cursor--
			}
		}
	case "[", "-", "K":
		if m.cursor > 0 {
			idx := m.cursor
			m.persist(func(x *template.Template) { x.Steps[idx-1], x.Steps[idx] = x.Steps[idx], x.Steps[idx-1] })
			m.cursor--
		}
	case "]", "+", "J":
		if m.cursor < n-1 {
			idx := m.cursor
			m.persist(func(x *template.Template) { x.Steps[idx+1], x.Steps[idx] = x.Steps[idx], x.Steps[idx+1] })
			m.cursor++
		}
	case "r": // rename the template
		return m.startEditing("name", m.editName)
	}
	return m, nil
}

// ---- single step ----

// stepFields returns the editable field ids for a step, by action.
func stepFields(s template.Step) []string {
	f := []string{"action"}
	switch {
	case template.UsesTradeParams(s.Action):
		f = append(f, "drug", "buy", "sell")
	case template.UsesHeistParams(s.Action):
		f = append(f, "difficulty")
	case template.UsesHeatParam(s.Action):
		f = append(f, "heat_at")
	case template.UsesBailParam(s.Action):
		f = append(f, "pay_bail")
	}
	return append(f, "count")
}

func (m TemplatesModel) step() (template.Step, bool) {
	t, ok := m.deps.Templates.Get(m.editName)
	if !ok || m.stepIdx < 0 || m.stepIdx >= len(t.Steps) {
		return template.Step{}, false
	}
	return t.Steps[m.stepIdx], true
}

func (m TemplatesModel) updateStep(key tea.KeyMsg) (TemplatesModel, tea.Cmd) {
	s, ok := m.step()
	if !ok {
		m.mode, m.cursor = modeSteps, 0
		return m, nil
	}
	fields := stepFields(s)
	nf := len(fields)
	switch key.String() {
	case "esc":
		m.mode = modeSteps
		return m, nil
	case "up", "k":
		m.field = (m.field - 1 + nf) % nf
		return m, nil
	case "down", "j":
		m.field = (m.field + 1) % nf
		return m, nil
	}
	idx := m.stepIdx
	switch fields[m.field] {
	case "action":
		if isCycleKey(key) {
			m.persistStep(idx, func(x *template.Step) { x.Action = nextAction(x.Action) })
			m.field = 0 // field set changes with the action
		}
	case "difficulty":
		if isCycleKey(key) {
			m.persistStep(idx, func(x *template.Step) { x.HeistDifficulty = nextDiff8(x.HeistDifficulty) })
		}
	case "pay_bail":
		if isCycleKey(key) {
			m.persistStep(idx, func(x *template.Step) { x.PayBail = !x.PayBail })
		}
	case "heat_at":
		if key.String() == "enter" {
			cur := ""
			if s.HeatAt > 0 {
				cur = strconv.Itoa(int(s.HeatAt))
			}
			return m.startEditing("heat_at", cur)
		}
	case "drug":
		if key.String() == "enter" {
			return m.startEditing("drug", s.Drug)
		}
	case "buy":
		if key.String() == "enter" {
			return m.startEditing("buy", s.BuyArea)
		}
	case "sell":
		if key.String() == "enter" {
			return m.startEditing("sell", s.SellArea)
		}
	case "count":
		if key.String() == "enter" {
			cur := ""
			if s.Count > 0 {
				cur = strconv.Itoa(s.Count)
			}
			return m.startEditing("count", cur)
		}
	}
	return m, nil
}

// ---- text editing ----

func (m TemplatesModel) startEditing(kind, cur string) (TemplatesModel, tea.Cmd) {
	m.editing, m.editKind = true, kind
	m.input.SetValue(cur)
	m.input.CursorEnd()
	switch kind {
	case "count":
		m.input.Placeholder = i18n.T("templates.count_placeholder")
	case "heat_at":
		m.input.Placeholder = i18n.T("templates.heat_placeholder")
	default:
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
		m.notice = okStyle.Render(i18n.T("templates.renamed"))
	case "drug":
		m.persistStep(m.stepIdx, func(x *template.Step) { x.Drug = val })
	case "buy":
		m.persistStep(m.stepIdx, func(x *template.Step) { x.BuyArea = val })
	case "sell":
		m.persistStep(m.stepIdx, func(x *template.Step) { x.SellArea = val })
	case "count":
		n, err := strconv.Atoi(val)
		if val == "" {
			n = 0
		} else if err != nil || n < 0 {
			m.notice = errStyle.Render(i18n.T("templates.count_invalid"))
			return
		}
		m.persistStep(m.stepIdx, func(x *template.Step) { x.Count = n })
	case "heat_at":
		n, err := strconv.Atoi(val)
		if val == "" {
			n = 0
		} else if err != nil || n < 0 || n > 5 {
			m.notice = errStyle.Render(i18n.T("templates.heat_invalid"))
			return
		}
		m.persistStep(m.stepIdx, func(x *template.Step) { x.HeatAt = int8(n) })
	}
}

func (m *TemplatesModel) persist(mutate func(*template.Template)) {
	if err := m.deps.Templates.Update(m.editName, mutate); err != nil {
		m.notice = errStyle.Render(i18n.T("common.save_failed") + ": " + err.Error())
	}
}

func (m *TemplatesModel) persistStep(idx int, mutate func(*template.Step)) {
	m.persist(func(t *template.Template) {
		if idx >= 0 && idx < len(t.Steps) {
			mutate(&t.Steps[idx])
		}
	})
}

func isCycleKey(key tea.KeyMsg) bool {
	switch key.String() {
	case " ", "left", "right", "h", "l", "enter":
		return true
	}
	return false
}

func nextAction(a string) string {
	order := template.ActionOrder
	for i, x := range order {
		if x == a {
			return order[(i+1)%len(order)]
		}
	}
	return order[0]
}

func nextDiff8(d int8) int8 {
	if d < 0 {
		return 0
	}
	if d >= 2 {
		return -1
	}
	return d + 1
}

func diffLabel8(d int8) string {
	if d < 0 {
		return i18n.T("templates.diff_auto")
	}
	return strconv.Itoa(int(d))
}

// heatAtValue is the effective clear-stars threshold (0 = the default 3).
func heatAtValue(h int8) int {
	if h <= 0 {
		return 3
	}
	return int(h)
}

// heatAtLabel renders the threshold, marking the default.
func heatAtLabel(h int8) string {
	if h <= 0 {
		return i18n.T("templates.heat_default")
	}
	return strconv.Itoa(int(h)) + "★"
}

func boolLabel(v bool) string {
	if v {
		return i18n.T("templates.yes")
	}
	return i18n.T("templates.no")
}

func actionLabel(action string) string {
	return i18n.T("templates.act_" + action)
}

// countLabel renders a step's repeat count: "×N" or "до успеха" for 0.
func countLabel(count int) string {
	if count > 0 {
		return fmt.Sprintf("×%d", count)
	}
	return i18n.T("templates.until_done")
}

// ---- view ----

func (m TemplatesModel) View() string {
	if m.deps.Templates == nil {
		return titleStyle.Render(i18n.T("templates.title")) + "\n\n" + helpStyle.Render(i18n.T("common.unavailable"))
	}
	switch m.mode {
	case modeList:
		return m.viewList()
	case modeSteps:
		return m.viewSteps()
	default:
		return m.viewStep()
	}
}

func (m TemplatesModel) viewList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(i18n.T("templates.list_title")) + "\n")
	b.WriteString(helpStyle.Render(i18n.T("templates.subtitle")) + "\n\n")
	for i, t := range m.deps.Templates.All() {
		cursor, name := "  ", t.Name
		if i == m.cursor {
			cursor, name = focusStyle.Render("▸ "), focusStyle.Render(name)
		}
		fmt.Fprintf(&b, "%s%s  %s\n", cursor, name, helpStyle.Render(i18n.T("templates.steps_count", len(t.Steps))))
	}
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	b.WriteString("\n" + helpStyle.Render(i18n.T("templates.list_hint")))
	return b.String()
}

func (m TemplatesModel) viewSteps() string {
	t, ok := m.deps.Templates.Get(m.editName)
	if !ok {
		return titleStyle.Render(i18n.T("templates.edit_title")) + "\n\n" + helpStyle.Render(i18n.T("templates.gone"))
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(i18n.T("templates.edit_title")+" · "+t.Name) + "\n")
	b.WriteString(helpStyle.Render(i18n.T("templates.steps_hint")) + "\n\n")
	if m.editing && m.editKind == "name" {
		b.WriteString(i18n.T("templates.f_name") + ": " + m.input.View() + "\n\n")
	}
	if len(t.Steps) == 0 {
		b.WriteString(helpStyle.Render(i18n.T("templates.no_steps")) + "\n")
	}
	for i, s := range t.Steps {
		cursor := "  "
		if i == m.cursor {
			cursor = focusStyle.Render("▸ ")
		}
		num := helpStyle.Render(fmt.Sprintf("%d.", i+1))
		fmt.Fprintf(&b, "%s%s %s\n", cursor, num, m.stepSummary(s, i == m.cursor))
	}
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	return b.String()
}

// stepSummary renders one step line: action · params · count.
func (m TemplatesModel) stepSummary(s template.Step, sel bool) string {
	label := actionLabel(s.Action)
	if sel {
		label = focusStyle.Render(label)
	}
	detail := ""
	switch {
	case template.UsesTradeParams(s.Action):
		detail = orDim(s.Drug, "weed") + " " + orDim(s.BuyArea, "Manhattan") + "→" + orDim(s.SellArea, "Amsterdam")
	case template.UsesHeistParams(s.Action):
		detail = i18n.T("templates.f_heist") + " " + diffLabel8(s.HeistDifficulty)
	case template.UsesHeatParam(s.Action):
		detail = i18n.T("templates.at_stars", heatAtValue(s.HeatAt))
	case template.UsesBailParam(s.Action):
		if s.PayBail {
			detail = i18n.T("templates.with_bail")
		}
	}
	return fmt.Sprintf("%-18s %s   %s", label, detail, helpStyle.Render(countLabel(s.Count)))
}

func (m TemplatesModel) viewStep() string {
	s, ok := m.step()
	if !ok {
		return titleStyle.Render(i18n.T("templates.edit_title")) + "\n\n" + helpStyle.Render(i18n.T("templates.gone"))
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("%s %d · %s", i18n.T("templates.step_word"), m.stepIdx+1, actionLabel(s.Action))) + "\n")
	b.WriteString(helpStyle.Render(i18n.T("templates.step_hint")) + "\n\n")

	fields := stepFields(s)
	for i, f := range fields {
		b.WriteString(m.renderField(i, f, s) + "\n")
	}
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	return b.String()
}

func (m TemplatesModel) renderField(i int, field string, s template.Step) string {
	sel := i == m.field
	cur := "  "
	if sel {
		cur = focusStyle.Render("▸ ")
	}
	row := func(name, val string) string {
		if sel {
			return cur + focusStyle.Render(fmt.Sprintf("%-16s ", name)) + val
		}
		return cur + fmt.Sprintf("%-16s %s", name, val)
	}
	editingThis := m.editing && sel
	switch field {
	case "action":
		return row(i18n.T("templates.f_action"), actionLabel(s.Action)+helpStyle.Render("  "+i18n.T("templates.action_hint")))
	case "drug":
		if editingThis {
			return row(i18n.T("templates.f_drug"), m.input.View())
		}
		return row(i18n.T("templates.f_drug"), orDim(s.Drug, "weed"))
	case "buy":
		if editingThis {
			return row(i18n.T("templates.f_buy"), m.input.View())
		}
		return row(i18n.T("templates.f_buy"), orDim(s.BuyArea, "Manhattan"))
	case "sell":
		if editingThis {
			return row(i18n.T("templates.f_sell"), m.input.View())
		}
		return row(i18n.T("templates.f_sell"), orDim(s.SellArea, "Amsterdam"))
	case "difficulty":
		return row(i18n.T("templates.f_heist"), diffLabel8(s.HeistDifficulty))
	case "heat_at":
		if editingThis {
			return row(i18n.T("templates.f_heat_at"), m.input.View())
		}
		return row(i18n.T("templates.f_heat_at"), heatAtLabel(s.HeatAt)+helpStyle.Render("  "+i18n.T("templates.heat_hint")))
	case "pay_bail":
		return row(i18n.T("templates.f_pay_bail"), boolLabel(s.PayBail))
	case "count":
		if editingThis {
			return row(i18n.T("templates.f_count"), m.input.View())
		}
		return row(i18n.T("templates.f_count"), countLabel(s.Count)+helpStyle.Render("  "+i18n.T("templates.count_hint")))
	}
	return ""
}

// orDim shows val, or a dimmed default when val is empty.
func orDim(val, def string) string {
	if val == "" {
		return helpStyle.Render(i18n.T("templates.default_suffix", def))
	}
	return val
}
