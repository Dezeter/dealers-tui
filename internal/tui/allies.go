package tui

import (
	"fmt"
	"strconv"
	"strings"

	"dealers/internal/i18n"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// AlliesModel is the do-not-attack manager: shows the user list, and a numeric
// input that toggles a token id on/off (add if absent, remove if present).
type AlliesModel struct {
	deps   Deps
	input  textinput.Model
	notice string
}

func NewAllies(deps Deps) AlliesModel {
	ti := textinput.New()
	ti.Placeholder = i18n.T("allies.placeholder")
	ti.CharLimit = 8
	ti.Width = 10
	ti.Focus()
	return AlliesModel{deps: deps, input: ti}
}

func (m AlliesModel) Init() tea.Cmd { return textinput.Blink }

func (m AlliesModel) Update(msg tea.Msg) (AlliesModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			return m, func() tea.Msg { return backToFleetMsg{} }
		case "enter":
			return m.toggle()
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m AlliesModel) toggle() (AlliesModel, tea.Cmd) {
	if m.deps.Allies == nil {
		m.notice = errStyle.Render(i18n.T("allies.unavailable_list"))
		return m, nil
	}
	id, err := strconv.ParseUint(strings.TrimSpace(m.input.Value()), 10, 64)
	if err != nil || id == 0 {
		m.notice = errStyle.Render(i18n.T("allies.invalid_id"))
		return m, nil
	}
	added, fixed, serr := m.deps.Allies.Toggle(id)
	m.input.SetValue("")
	switch {
	case serr != nil:
		m.notice = errStyle.Render(i18n.T("common.save_failed") + ": " + serr.Error())
	case fixed:
		m.notice = helpStyle.Render(i18n.T("allies.own_dealer", id))
	case added:
		m.notice = okStyle.Render(i18n.T("allies.added", id))
	default:
		m.notice = statusBarStyle.Render(i18n.T("allies.removed", id))
	}
	return m, nil
}

func (m AlliesModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(i18n.T("allies.title")) + "\n\n")
	if m.deps.Allies == nil {
		return b.String() + helpStyle.Render(i18n.T("common.unavailable"))
	}

	b.WriteString(sectionStyle.Render(i18n.T("allies.your_list")) + "\n")
	list := m.deps.Allies.List()
	if len(list) == 0 {
		b.WriteString(helpStyle.Render(i18n.T("allies.none")))
	}
	for _, id := range list {
		fmt.Fprintf(&b, "  #%d\n", id)
	}
	b.WriteString(helpStyle.Render(i18n.T("allies.your_dealers", m.deps.Allies.FixedCount())))

	b.WriteString(i18n.T("allies.add_remove") + m.input.View() + "\n")
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	b.WriteString("\n" + helpStyle.Render(i18n.T("allies.hint")))
	return b.String()
}
