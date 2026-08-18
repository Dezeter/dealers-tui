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

		// Batch season-reward claim (the fleet 'C' action notice).
		"claim.none":    {"награды: нет дилеров", "rewards: no dealers"},
		"claim.prefix":  {"награды: ", "rewards: "},
		"claim.claimed": {"собрано", "claimed"},
		"claim.nothing": {"нечего", "nothing"},
		"claim.uninit":  {"не иниц.", "uninit"},
		"claim.errors":  {"ошибки", "errors"},
		"claim.seasons": {"сезонов", "seasons"},
	})
}
