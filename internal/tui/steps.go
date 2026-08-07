package tui

import (
	"fmt"
	"strings"

	"dealers/internal/dealer"

	tea "github.com/charmbracelet/bubbletea"
)

// StepsModel is the autopilot recipe editor: reorder and enable/disable the
// steps the autopilot runs each tick. The order is global (applies to every
// autopilot dealer); the "core" step is that dealer's trade or raid job.
type StepsModel struct {
	deps   Deps
	cursor int
	notice string
}

func NewSteps(deps Deps) StepsModel { return StepsModel{deps: deps} }

func (m StepsModel) Init() tea.Cmd { return nil }

// stepLabels maps step id → catalog metadata for display.
func stepMeta(id string) dealer.StepMeta {
	for _, s := range dealer.StepCatalog {
		if s.ID == id {
			return s
		}
	}
	return dealer.StepMeta{ID: id, Label: id}
}

func (m StepsModel) Update(msg tea.Msg) (StepsModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok || m.deps.Recipe == nil {
		return m, nil
	}
	n := len(m.deps.Recipe.All())
	switch key.String() {
	case "esc":
		return m, func() tea.Msg { return backToFleetMsg{} }
	case "up", "k":
		if n > 0 {
			m.cursor = (m.cursor - 1 + n) % n
		}
	case "down", "j":
		if n > 0 {
			m.cursor = (m.cursor + 1) % n
		}
	case "enter", " ", "x":
		if err := m.deps.Recipe.Toggle(m.cursor); err != nil {
			m.notice = errStyle.Render("save failed: " + err.Error())
		}
	case "[", "-", "K": // move up
		m.cursor, _ = m.deps.Recipe.Move(m.cursor, -1)
	case "]", "+", "J": // move down
		m.cursor, _ = m.deps.Recipe.Move(m.cursor, 1)
	case "r": // reset to default
		if err := m.deps.Recipe.Reset(); err == nil {
			m.cursor = 0
			m.notice = okStyle.Render("reset to default order")
		}
	}
	return m, nil
}

func (m StepsModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("AUTOPILOT STEPS") + "\n")
	b.WriteString(helpStyle.Render("each tick the autopilot runs the ON steps top-to-bottom; the first that acts wins") + "\n\n")
	if m.deps.Recipe == nil {
		return b.String() + helpStyle.Render("unavailable")
	}
	for i, st := range m.deps.Recipe.All() {
		meta := stepMeta(st.ID)
		box := helpStyle.Render("[ ]")
		name := meta.Label
		if st.On {
			box = okStyle.Render("[x]")
		} else {
			name = helpStyle.Render(name) // dim disabled steps
		}
		num := fmt.Sprintf("%d.", i+1)
		if i == m.cursor {
			num = focusStyle.Render("▸")
			name = focusStyle.Render(meta.Label)
		}
		fmt.Fprintf(&b, "%2s %s %s\n", num, box, name)
		if i == m.cursor {
			b.WriteString("      " + helpStyle.Render(meta.Desc) + "\n")
		}
	}
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("↑/↓ select · space toggle · [ ] move up/down · r reset · esc back"))
	return b.String()
}
