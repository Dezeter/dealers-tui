package tui

import (
	"fmt"
	"strconv"
	"strings"

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
	ti.Placeholder = "token id"
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
		m.notice = errStyle.Render("ally list unavailable")
		return m, nil
	}
	id, err := strconv.ParseUint(strings.TrimSpace(m.input.Value()), 10, 64)
	if err != nil || id == 0 {
		m.notice = errStyle.Render("enter a valid token id")
		return m, nil
	}
	added, fixed, serr := m.deps.Allies.Toggle(id)
	m.input.SetValue("")
	switch {
	case serr != nil:
		m.notice = errStyle.Render("save failed: " + serr.Error())
	case fixed:
		m.notice = helpStyle.Render(fmt.Sprintf("#%d is your own dealer — always protected", id))
	case added:
		m.notice = okStyle.Render(fmt.Sprintf("added #%d to allies (won't show in PVP)", id))
	default:
		m.notice = statusBarStyle.Render(fmt.Sprintf("removed #%d from allies", id))
	}
	return m, nil
}

func (m AlliesModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("ALLIES — do-not-attack") + "\n\n")
	if m.deps.Allies == nil {
		return b.String() + helpStyle.Render("unavailable")
	}

	b.WriteString(sectionStyle.Render("Your list") + "\n")
	list := m.deps.Allies.List()
	if len(list) == 0 {
		b.WriteString(helpStyle.Render("  (none yet)\n"))
	}
	for _, id := range list {
		fmt.Fprintf(&b, "  #%d\n", id)
	}
	b.WriteString(helpStyle.Render(fmt.Sprintf("  + your %d dealers (auto-protected)\n", m.deps.Allies.FixedCount())))

	b.WriteString("\n  add / remove by id: " + m.input.View() + "\n")
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("type token id + enter to toggle · esc back"))
	return b.String()
}
