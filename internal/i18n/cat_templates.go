package i18n

// Strategy templates: the program list, a template's step list, and the
// single-step field editor.
func init() {
	add(map[string][2]string{
		"templates.title":       {"ШАБЛОНЫ", "TEMPLATES"},
		"templates.list_title":  {"ШАБЛОНЫ (ПРОГРАММЫ)", "TEMPLATES (PROGRAMS)"},
		"templates.subtitle":    {"именованные программы шагов, назначаются на NFT (листать s на флоте); автопилот идёт по шагам по порядку", "named step programs assigned per-NFT (cycle with s on the fleet); the autopilot runs the steps in order"},
		"templates.list_hint":   {"↑/↓ · enter шаги · n новый · c клон · d удалить · esc назад", "↑/↓ · enter steps · n new · c clone · d delete · esc back"},
		"templates.steps_count": {"%d шагов", "%d steps"},

		// notices
		"templates.create_failed": {"не удалось создать", "create failed"},
		"templates.clone_failed":  {"не удалось клонировать", "clone failed"},
		"templates.cloned":        {"клонирован → %s", "cloned → %s"},
		"templates.deleted":       {"удалён %s", "deleted %s"},
		"templates.renamed":       {"переименован — переназначьте дилеров клавишей s (старые назначения вернутся к умолчанию)", "renamed — reassign dealers with s (old assignments fall back to default)"},

		// step list
		"templates.edit_title": {"ШАБЛОН", "TEMPLATE"},
		"templates.gone":       {"удалён — esc назад", "gone — esc back"},
		"templates.steps_hint": {"↑/↓ шаг · enter изменить · a добавить · d удалить · r переименовать · [ ] двигать · esc назад", "↑/↓ step · enter edit · a add · d delete · r rename · [ ] move · esc back"},
		"templates.no_steps":   {"нет шагов — нажми a чтобы добавить", "no steps — press a to add one"},
		"templates.until_done": {"до успеха", "until done"},

		// single-step editor
		"templates.step_word":         {"ШАГ", "STEP"},
		"templates.step_hint":         {"↑/↓ поле · enter/пробел изменить · esc назад", "↑/↓ field · enter/space edit · esc back"},
		"templates.f_action":          {"действие", "action"},
		"templates.f_count":           {"сколько", "count"},
		"templates.f_name":            {"имя", "name"},
		"templates.f_drug":            {"товар", "drug"},
		"templates.f_buy":             {"зона покупки", "buy zone"},
		"templates.f_sell":            {"зона продажи", "sell zone"},
		"templates.f_heist":           {"сложн. хайста", "heist difficulty"},
		"templates.f_heat_at":         {"порог (звёзд)", "threshold (stars)"},
		"templates.f_pay_bail":        {"платный залог", "pay bail"},
		"templates.action_hint":       {"(пробел листает)", "(space cycles)"},
		"templates.count_hint":        {"(0 = до успеха)", "(0 = until done)"},
		"templates.count_placeholder": {"0 = до успеха", "0 = until done"},
		"templates.count_invalid":     {"должно быть число ≥ 0", "must be a number ≥ 0"},
		"templates.heat_hint":         {"(при скольки ★ снимать; макс 5)", "(clear at this many ★; max 5)"},
		"templates.heat_placeholder":  {"3 (по умолчанию)", "3 (default)"},
		"templates.heat_default":      {"3★ (по умолчанию)", "3★ (default)"},
		"templates.heat_invalid":      {"порог: число 0..5", "threshold: a number 0..5"},
		"templates.at_stars":          {"при %d★", "at %d★"},
		"templates.with_bail":         {"+ платный залог", "+ pay bail"},
		"templates.yes":               {"да", "yes"},
		"templates.no":                {"нет", "no"},
		"templates.diff_auto":         {"авто (макс. доступная)", "auto (max affordable)"},
		"templates.default_suffix":    {"%s (по умолчанию)", "%s (default)"},

		// action names
		"templates.act_trade":           {"Торговать", "Trade"},
		"templates.act_pvp":             {"PvP-рейд", "PvP raid"},
		"templates.act_heist":           {"Хайст", "Heist"},
		"templates.act_clear_stars":     {"Снять звёзды", "Clear stars"},
		"templates.act_breakout":        {"Побег из тюрьмы", "Jail breakout"},
		"templates.act_heist_checkin":   {"Чек-ин сезона", "Heist check-in"},
		"templates.act_missions":        {"Миссии", "Missions"},
		"templates.act_missions_accept": {"Принять миссии", "Accept missions"},
		"templates.act_missions_follow": {"Следовать миссиям", "Follow missions"},
		"templates.act_missions_claim":  {"Забрать награды", "Claim rewards"},
	})
}
