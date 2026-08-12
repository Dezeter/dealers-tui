package i18n

// Fleet dashboard: status line, key hints, grid scroll markers, activity feed,
// card labels, mission/status cells, and fleet-level action notices.
func init() {
	add(map[string][2]string{
		"fleet.dealers":    {"%d дилеров", "%d dealers"},
		"fleet.refreshing": {" · обновление…", " · refreshing…"},
		"fleet.updated":    {" · обновлено %s", " · updated %s"},
		"fleet.no_dealers": {"нет дилеров", "no dealers"},
		"fleet.more_up":    {"  ↑ ещё %d", "  ↑ %d more"},
		"fleet.more_down":  {"  ↓ ещё %d", "  ↓ %d more"},
		"fleet.activity":   {"Активность", "Activity"},

		"fleet.hint":      {"↑↓←→ · enter · n миссии · c чек-ин · s стратегия · t шаблоны · m рынок · f союзники · o настройки · r · q", "↑↓←→ · enter · n missions · c check-in · s strategy · t templates · m market · f allies · o settings · r · q"},
		"fleet.hint_auto": {"↑↓←→ · enter · n миссии · c чек-ин · s стратегия · t шаблоны · A авто · m рынок · f союзники · o · r · q", "↑↓←→ · enter · n missions · c check-in · s strategy · t templates · A auto · m market · f allies · o · r · q"},
		"fleet.hint_ro":   {"только чтение · ↑↓←→ · enter · n миссии · t шаблоны · m рынок · f союзники · o настройки · r · q", "read-only · ↑↓←→ · enter · n missions · t templates · m market · f allies · o settings · r · q"},

		// card body
		"fleet.read_error":     {"ошибка чтения: ", "read error: "},
		"fleet.no_data":        {"нет данных", "no data"},
		"fleet.uninitialised":  {"не инициализирован", "uninitialised"},
		"fleet.err_dealer":     {"дилер %d: %v", "dealer %d: %v"},
		"fleet.lbl_energy":     {"Энергия", "Energy"},
		"fleet.lbl_heat":       {"Розыск", "Heat"},
		"fleet.lbl_checkin":    {"чек-ин", "check-in"},
		"fleet.lbl_missions":   {"миссии", "missions"},
		"fleet.lbl_auto":       {"авто", "auto"},
		"fleet.miss_claimable": {"★%d забрать", "★%d claimable"},
		"fleet.miss_accept":    {"○ принять", "○ accept"},
		"fleet.miss_uptodate":  {"✓ готово", "✓ up to date"},

		// action notices
		"fleet.notice.ro_templates":         {"только чтение — шаблоны недоступны", "read-only — templates unavailable"},
		"fleet.notice.ro_checkin":           {"только чтение — нет подписанта для чек-ина", "read-only — no signer configured for check-in"},
		"fleet.notice.checking_in":          {"чек-ин…", "checking in…"},
		"fleet.notice.ro_strategy":          {"только чтение — стратегия автопилота недоступна", "read-only — autopilot strategy unavailable"},
		"fleet.notice.strategy_save_failed": {"не удалось сохранить стратегию", "strategy save failed"},
		"fleet.notice.strategy_set":         {"#%d стратегия → %s", "#%d strategy → %s"},
	})
}
