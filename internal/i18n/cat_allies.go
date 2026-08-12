package i18n

// Allies (do-not-attack) screen.
func init() {
	add(map[string][2]string{
		"allies.title":            {"СОЮЗНИКИ — не атаковать", "ALLIES — do-not-attack"},
		"allies.your_list":        {"Ваш список", "Your list"},
		"allies.none":             {"  (пока пусто)\n", "  (none yet)\n"},
		"allies.your_dealers":     {"  + ваши %d дилеров (авто-защита)\n", "  + your %d dealers (auto-protected)\n"},
		"allies.add_remove":       {"\n  добавить / убрать по id: ", "\n  add / remove by id: "},
		"allies.hint":             {"введите id токена + enter чтобы переключить · esc назад", "type token id + enter to toggle · esc back"},
		"allies.placeholder":      {"id токена", "token id"},
		"allies.unavailable_list": {"список союзников недоступен", "ally list unavailable"},
		"allies.invalid_id":       {"введите корректный id токена", "enter a valid token id"},
		"allies.own_dealer":       {"#%d — ваш дилер, всегда защищён", "#%d is your own dealer — always protected"},
		"allies.added":            {"#%d добавлен в союзники (не появится в PVP)", "added #%d to allies (won't show in PVP)"},
		"allies.removed":          {"#%d убран из союзников", "removed #%d from allies"},
	})
}
