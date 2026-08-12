package tui

import (
	"fmt"
	"strings"

	"dealers/internal/i18n"
	"dealers/internal/settings"

	tea "github.com/charmbracelet/bubbletea"
)

// SettingsModel is the global-toggles screen. Each toggle row is a switch from
// settings.Registry; enter/space flips it and persists to settings.json. A final
// row switches the UI language (persisted to language.json).
type SettingsModel struct {
	deps   Deps
	cursor int
	notice string
}

func NewSettings(deps Deps) SettingsModel { return SettingsModel{deps: deps} }

func (m SettingsModel) Init() tea.Cmd { return nil }

// rowCount is the number of navigable rows: the toggles plus the language row
// (only when a language store is wired).
func (m SettingsModel) rowCount() int {
	n := len(settings.Registry)
	if m.deps.Lang != nil {
		n++
	}
	return n
}

// langRow returns the index of the language row (-1 when not shown).
func (m SettingsModel) langRow() int {
	if m.deps.Lang == nil {
		return -1
	}
	return len(settings.Registry)
}

func (m SettingsModel) Update(msg tea.Msg) (SettingsModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	n := m.rowCount()
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
		if m.cursor == m.langRow() {
			return m.cycleLang()
		}
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
		m.notice = errStyle.Render(i18n.T("common.save_failed") + ": " + err.Error())
		return m, nil
	}
	label := settingLabel(t)
	if v {
		m.notice = okStyle.Render(label + " → " + i18n.T("common.on"))
	} else {
		m.notice = statusBarStyle.Render(label + " → " + i18n.T("common.off"))
	}
	return m, nil
}

// cycleLang flips the UI language and persists it; the change applies on the
// next render (the catalog is a process-global).
func (m SettingsModel) cycleLang() (SettingsModel, tea.Cmd) {
	if m.deps.Lang == nil {
		return m, nil
	}
	l, err := m.deps.Lang.Toggle()
	if err != nil {
		m.notice = errStyle.Render(i18n.T("common.save_failed") + ": " + err.Error())
		return m, nil
	}
	m.notice = okStyle.Render(i18n.T("settings.language.changed", l.Name()))
	return m, nil
}

// settingLabel/settingDesc localize a toggle by its stable key, falling back to
// the Registry's English text for any key without a translation.
func settingLabel(t settings.Toggle) string {
	if s := i18n.T("setting." + t.Key + ".label"); s != "setting."+t.Key+".label" {
		return s
	}
	return t.Label
}

func settingDesc(t settings.Toggle) string {
	if s := i18n.T("setting." + t.Key + ".desc"); s != "setting."+t.Key+".desc" {
		return s
	}
	return t.Desc
}

func (m SettingsModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(i18n.T("settings.title")) + "\n\n")
	if m.deps.Settings == nil && m.deps.Lang == nil {
		return b.String() + helpStyle.Render(i18n.T("common.unavailable"))
	}
	for i, t := range settings.Registry {
		on := m.deps.Settings != nil && m.deps.Settings.Get(t.Key)
		box := helpStyle.Render("[ ]")
		if on {
			box = okStyle.Render("[x]")
		}
		label := settingLabel(t)
		if i == m.cursor {
			label = focusStyle.Render("▸ " + label)
		} else {
			label = "  " + label
		}
		fmt.Fprintf(&b, "%s %s\n", box, label)
		b.WriteString("     " + helpStyle.Render(settingDesc(t)) + "\n\n")
	}
	if m.deps.Lang != nil {
		label := i18n.T("settings.language.label") + ": " + m.deps.Lang.Lang().Name()
		if m.cursor == m.langRow() {
			label = focusStyle.Render("▸ " + label)
		} else {
			label = "  " + label
		}
		fmt.Fprintf(&b, "%s %s\n", helpStyle.Render(" ⇄ "), label)
		b.WriteString("     " + helpStyle.Render(i18n.T("settings.language.desc")) + "\n\n")
	}
	if m.notice != "" {
		b.WriteString(m.notice + "\n\n")
	}
	b.WriteString(helpStyle.Render(i18n.T("settings.hint")))
	return b.String()
}
