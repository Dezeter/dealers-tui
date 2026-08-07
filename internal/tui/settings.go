package tui

import (
	"fmt"
	"strings"

	"dealers/internal/settings"

	tea "github.com/charmbracelet/bubbletea"
)

// SettingsModel is the global-toggles screen. Each row is a switch from
// settings.Registry; enter/space flips it and persists to settings.json.
type SettingsModel struct {
	deps   Deps
	cursor int
	notice string
}

func NewSettings(deps Deps) SettingsModel { return SettingsModel{deps: deps} }

func (m SettingsModel) Init() tea.Cmd { return nil }

func (m SettingsModel) Update(msg tea.Msg) (SettingsModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	n := len(settings.Registry)
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
		return m.toggle()
	}
	return m, nil
}

func (m SettingsModel) toggle() (SettingsModel, tea.Cmd) {
	if m.deps.Settings == nil || m.cursor < 0 || m.cursor >= len(settings.Registry) {
		return m, nil
	}
	t := settings.Registry[m.cursor]
	v, err := m.deps.Settings.Toggle(t.Key)
	if err != nil {
		m.notice = errStyle.Render("save failed: " + err.Error())
		return m, nil
	}
	if v {
		m.notice = okStyle.Render(t.Label + " → ON")
	} else {
		m.notice = statusBarStyle.Render(t.Label + " → off")
	}
	return m, nil
}

func (m SettingsModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("SETTINGS") + "\n\n")
	if m.deps.Settings == nil {
		return b.String() + helpStyle.Render("unavailable")
	}
	for i, t := range settings.Registry {
		on := m.deps.Settings.Get(t.Key)
		box := helpStyle.Render("[ ]")
		if on {
			box = okStyle.Render("[x]")
		}
		label := t.Label
		if i == m.cursor {
			label = focusStyle.Render("▸ " + label)
		} else {
			label = "  " + label
		}
		fmt.Fprintf(&b, "%s %s\n", box, label)
		b.WriteString("     " + helpStyle.Render(t.Desc) + "\n\n")
	}
	if m.notice != "" {
		b.WriteString(m.notice + "\n\n")
	}
	b.WriteString(helpStyle.Render("↑/↓ select · enter/space toggle · esc back"))
	return b.String()
}
