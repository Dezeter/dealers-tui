package i18n

// Batch daily check-in summary (the fleet 'c' action notice).
func init() {
	add(map[string][2]string{
		"checkin.none":         {"чек-ин: нет дилеров", "check-in: no dealers"},
		"checkin.prefix":       {"чек-ин: ", "check-in: "},
		"checkin.done":         {"готово", "done"},
		"checkin.already":      {"уже", "already"},
		"checkin.not_eligible": {"не подходят", "not eligible"},
		"checkin.jailed":       {"в тюрьме", "jailed"},
		"checkin.uninit":       {"не иниц.", "uninit"},
		"checkin.errors":       {"ошибки", "errors"},
	})
}
