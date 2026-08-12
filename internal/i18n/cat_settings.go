package i18n

// Settings screen: title, hint, the language switch row, and per-toggle
// label/description keyed by the toggle's stable settings key.
func init() {
	add(map[string][2]string{
		"settings.title":            {"НАСТРОЙКИ", "SETTINGS"},
		"settings.hint":             {"↑/↓ выбрать · enter/пробел переключить · esc назад", "↑/↓ select · enter/space toggle · esc back"},
		"settings.language.label":   {"Язык интерфейса", "UI language"},
		"settings.language.desc":    {"Переключить язык интерфейса (Русский / English).", "Switch the interface language (Русский / English)."},
		"settings.language.changed": {"язык → %s", "language → %s"},

		// Toggle: pay_bail_after_failed_breakout
		"setting.pay_bail_after_failed_breakout.label": {"Платить залог после неудачного побега", "Pay bail after a failed breakout"},
		"setting.pay_bail_after_failed_breakout.desc":  {"Если дилер в тюрьме и бесплатный дневной побег уже использован — заплатить ETH-залог, чтобы выйти сейчас (автопилот). По умолчанию выключено.", "When a dealer is jailed and its free daily escape is used up, spend ETH on bail to get out now (autopilot). Off by default."},
	})
}
