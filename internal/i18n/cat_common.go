package i18n

// Common, cross-screen messages: shared words, dealer status chips, autopilot
// step catalog labels/descriptions, alert-bar text, and the header/update/
// autopilot chrome. Per-screen strings live in their own cat_*.go files.
func init() {
	add(map[string][2]string{
		// --- shared words ---
		"common.unavailable": {"недоступно", "unavailable"},
		"common.read_only":   {"только чтение", "read-only"},
		"common.loading":     {"загрузка…", "loading…"},
		"common.save_failed": {"не удалось сохранить", "save failed"},
		"common.on":          {"ВКЛ", "ON"},
		"common.off":         {"выкл", "off"},

		// --- dealer status chips (fleet cards) ---
		"status.jailed":    {"ТЮРЬМА", "JAILED"},
		"status.safehouse": {"УБЕЖИЩЕ", "SAFEHOUSE"},
		"status.uninit":    {"НЕ АКТ.", "UNINIT"},
		"status.err":       {"ОШИБКА", "ERR"},
		"status.idle":      {"ПРОСТОЙ", "IDLE"},
		"status.unknown":   {"?", "?"},

		// --- alert bar ---
		"alert.jailed": {"#%d ТЮРЬМА", "#%d JAILED"},
		"alert.heat":   {"#%d розыск %d/5", "#%d heat %d/5"},
		"alert.runway": {"мало ETH: %s < %s", "ETH runway low: %s < %s"},

		// --- header / wallet ---
		"header.bal":   {"баланс %s", "bal %s"},
		"header.spent": {"потрачено %s", "spent %s"},

		// --- update notice ---
		"update.bar":  {" ⬆ доступно обновление: %s ", " ⬆ update available: %s "},
		"update.have": {" (у вас %s)", " (you have %s)"},

		// --- autopilot chip / notices ---
		"auto.on_tagged":      {" АВТО:%s ВКЛ ", " AUTO:%s ON "},
		"auto.on":             {" АВТО ВКЛ ", " AUTO ON "},
		"auto.off_tagged":     {"авто:%s выкл", "auto:%s off"},
		"auto.off":            {"авто выкл", "auto off"},
		"auto.mixed":          {"разные", "mixed"},
		"auto.notice_on":      {" АВТОПИЛОТ ВКЛ ", " AUTOPILOT ON "},
		"auto.notice_on_tail": {" — действует сам, тратит энергию/кэш", " — acting on its own, spending energy/cash"},
		"auto.notice_off":     {"автопилот выключен", "autopilot off"},
	})
}
