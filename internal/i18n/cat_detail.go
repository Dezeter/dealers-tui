package i18n

// Detail screen: one dealer's stats/stash/pending/heist/log view plus its PVE
// buy/sell form, PVP target browser, travel picker, and heist-start form.
func init() {
	add(map[string][2]string{
		// --- submit / action results ---
		"detail.submit_failed": {"не удалось отправить: %s", "submit failed: %s"},
		"detail.committed_seq": {"коммит seq=%d — резолв в фоне…", "committed seq=%d — resolving in background…"},
		"detail.action_failed": {"%s — сбой: %s", "%s failed: %s"},

		// --- confirmations (single-tx, spend ETH) ---
		"detail.confirm_bail":           {"Оплатить залог (ETH) для освобождения", "Pay bail (ETH) to release now"},
		"detail.confirm_reset_attempts": {"Сбросить дневные попытки", "Reset daily attempts"},
		"detail.confirm_cashout":        {"Вывести хайст #%d (банк %s)", "Cash out heist #%d (pot %s)"},
		"detail.confirm_abandon":        {"Бросить хайст #%d (возврат ставки)", "Abandon heist #%d (refund stake)"},
		"detail.confirm_prompt":         {"%s? (небольшая комиссия ETH)  y = подтвердить · n = отмена", "%s? (pays a small ETH fee)  y = confirm · n = cancel"},

		// --- notices / blockers ---
		"detail.not_in_jail":             {"не в тюрьме", "not in jail"},
		"detail.cashout_needs":           {"для вывода нужна раскрытая победа на стадии ≥ 2", "cash out needs a revealed-win heist at stage ≥ 2"},
		"detail.abandon_only":            {"бросить можно только до первой стадии", "abandon only works before the first stage"},
		"detail.read_only_no_signer":     {"только чтение: подписант не настроен", "read-only: no signer configured"},
		"detail.read_only_no_signer_src": {"только чтение: подписант не настроен (укажите wallet.source)", "read-only: no signer configured (set wallet.source)"},
		"detail.cant_travel_jailed":      {"нельзя переехать, пока в тюрьме", "can't travel while jailed"},
		"detail.already_running_heist":   {"уже идёт хайст #%d", "already running heist #%d"},
		"detail.no_active_heist":         {"нет активного хайста — h чтобы начать", "no active heist — press h to start one"},
		"detail.cant_push_now":           {"сейчас нельзя (стадия идёт или забег окончен)", "can't push now (stage in progress or run ended)"},
		"detail.target_not_attackable":   {"цель сейчас нельзя атаковать", "target not attackable right now"},
		"detail.cant_buy_bm":             {"на чёрном рынке нельзя купить — только продать лут", "can't buy in the black market — only sell loot here"},
		"detail.no_market_data":          {"пока нет данных рынка для зоны — r чтобы обновить", "no market data for this area yet — press r to refresh"},
		"detail.nothing_to_buy":          {"нечего купить в %s", "nothing to buy in %s"},
		"detail.nothing_sellable":        {"нет ничего на продажу в %s", "you hold nothing sellable in %s"},

		// --- status-bar progress notices ---
		"detail.traveling_to":        {"переезд в %s…", "traveling to %s…"},
		"detail.starting_heist":      {"запуск хайста…", "starting heist…"},
		"detail.committing_stage":    {"коммит стадии хайста…", "committing heist stage…"},
		"detail.attacking":           {"атака #%d…", "attacking #%d…"},
		"detail.attempting_breakout": {"попытка побега (бесплатно · ~50% · 1/день)…", "attempting breakout (free · ~50% · 1/day)…"},
		"detail.removing_poster":     {"снятие ориентировки (тратит 1 попытку · ~50% снять розыск)…", "removing wanted poster (spends 1 attempt · ~50% to clear heat)…"},
		"detail.resetting_attempts":  {"сброс попыток…", "resetting attempts…"},
		"detail.cashing_out":         {"вывод…", "cashing out…"},
		"detail.abandoning_heist":    {"бросаю хайст…", "abandoning heist…"},
		"detail.paying_bail":         {"оплата залога…", "paying bail…"},
		"detail.bm_selling":          {"чёрный рынок: продажа %d × #%d (гарантия, без энергии)…", "black market: selling %d × #%d (guaranteed, no energy)…"},
		"detail.committing_trade":    {"коммит %s препарат=%d кол-во=%d…", "committing %s drug=%d amount=%d…"},

		// --- manager-action success labels ---
		"detail.arrived_at":       {"прибыл в %s", "arrived at %s"},
		"detail.heist_started":    {"хайст запущен", "heist started"},
		"detail.attempts_reset":   {"попытки сброшены", "attempts reset"},
		"detail.heist_cashed_out": {"хайст выведен", "heist cashed out"},
		"detail.heist_abandoned":  {"хайст брошен", "heist abandoned"},
		"detail.bailed_out":       {"залог оплачен", "bailed out"},
		"detail.bm_sold":          {"чёрный рынок: продано %d × #%d", "black market: sold %d × #%d"},

		// --- travel picker ---
		"detail.travel_title":    {"ПЕРЕЕЗД", "TRAVEL"},
		"detail.youre_in":        {"вы в %s", "you're in %s"},
		"detail.loading_areas":   {"загрузка зон…\n", "loading areas…\n"},
		"detail.failed":          {"сбой: %s", "failed: %s"},
		"detail.no_destinations": {"нет доступных зон\n", "no destinations available\n"},
		"detail.free":            {"бесплатно", "free"},
		"detail.gate_infamy":     {"бесславие≥%d", "infamy≥%d"},
		"detail.gate_rep":        {"rep≥", "rep≥"},
		"detail.cant_enter":      {"нельзя войти в %s: нужно %s", "can't enter %s: need %s"},
		"detail.travel_hint":     {"↑/↓ выбор · enter переезд · esc назад", "↑/↓ pick · enter travel · esc back"},

		// --- main detail view ---
		"detail.dealer_title": {"Дилер #%d", "Dealer #%d"},
		"detail.read_error":   {"ошибка чтения: %s", "read error: %s"},
		"detail.kv_rank":      {"Ранг", "Rank"},
		"detail.kv_heat":      {"Розыск", "Heat"},
		"detail.kv_cash":      {"Кэш", "Cash"},
		"detail.kv_area":      {"Зона", "Area"},
		"detail.kv_energy":    {"Энергия", "Energy"},
		"detail.kv_infamy":    {"Бесславие", "Infamy"},
		"detail.kv_pve_wtl":   {"PVE В/Н/П", "PVE W/T/L"},
		"detail.kv_status":    {"Статус", "Status"},
		"detail.attempts_fmt": {"%d/%d попыток", "%d/%d attempts"},
		"detail.sec_stash":    {"Запас", "Stash"},
		"detail.empty":        {"  (пусто)\n", "  (empty)\n"},
		"detail.sec_pending":  {"В ожидании", "Pending"},
		"detail.none":         {"  нет\n", "  none\n"},
		"detail.pending_row":  {"  seq %d · %s · раскрытие@%d истечение@%d %s\n", "  seq %d · %s · reveal@%d expiry@%d %s\n"},
		"detail.resolving":    {"(резолв)", "(resolving)"},
		"detail.sec_heist":    {"Хайст", "Heist"},
		"detail.heist_none":   {"  нет · h чтобы начать (нужен REP ≥ 600)\n", "  none · h to start (needs REP ≥ 600)\n"},
		"detail.heist_row":    {"  #%d %s сложность=%d · стадия %d · %s · банк %s%s\n", "  #%d %s difficulty=%d · stage %d · %s · pot %s%s\n"},
		"detail.sec_recent":   {"Недавнее", "Recent"},
		"detail.no_activity":  {"  пока нет активности\n", "  no activity yet\n"},

		// --- footer hints ---
		"detail.hint_main":     {"b купить · s продать · p pvp · t переезд · h хайст · c снять розыск · a сброс попыток · r обновить · esc назад", "b buy · s sell · p pvp · t travel · h heist · c clear-heat · a reset-attempts · r refresh · esc back"},
		"detail.hint_readonly": {"только чтение · r обновить · esc назад", "read-only · r refresh · esc back"},
		"detail.hint_jailed":   {"В ТЮРЬМЕ · k побег (бесплатно ~50% · 1/день) · l залог (ETH, сразу) · r обновить · esc назад", "JAILED · k breakout (free ~50% · 1/day) · l bail (ETH, instant) · r refresh · esc back"},
		"detail.hint_bm":       {"ЧЁРНЫЙ РЫНОК · s продать лут (гарантия, без энергии) · t переезд · r обновить · esc назад", "BLACK MARKET · s sell loot (guaranteed, no energy) · t travel · r refresh · esc back"},

		// --- heist start form ---
		"detail.fam_cash":         {"КЭШ", "CASH"},
		"detail.fam_supply":       {"ПОСТАВКА", "SUPPLY"},
		"detail.diff_easy":        {"лёгкий (rep 600)", "easy (rep 600)"},
		"detail.diff_medium":      {"средний (rep 1500)", "medium (rep 1500)"},
		"detail.diff_hard":        {"тяжёлый (rep 5500)", "hard (rep 5500)"},
		"detail.jp_on":            {"вкл (+0.001 ETH)", "on (+0.001 ETH)"},
		"detail.lbl_family":       {"Семья", "Family"},
		"detail.lbl_difficulty":   {"Сложность", "Difficulty"},
		"detail.lbl_eth_jackpot":  {"ETH джекпот", "ETH jackpot"},
		"detail.heist_start_hint": {"  ↑/↓ поле · ←/→/space изменить · enter начать · esc отмена\n", "  ↑/↓ field · ←/→/space change · enter start · esc cancel\n"},
		"detail.jackpot_flag":     {" · 🎰джекпот", " · 🎰jackpot"},

		// --- heist action hints ---
		"detail.hint_prestage":           {"g продвинуть (начать стадию 1) · x бросить", "g push (start stage 1) · x abandon"},
		"detail.hint_pushdeeper_cashout": {"g продвинуть глубже · o вывести", "g push deeper · o cash out"},
		"detail.hint_pushdeeper":         {"g продвинуть глубже (вывод со стадии 2)", "g push deeper (cash out unlocks at stage 2)"},
		"detail.hint_resolving_stage":    {"стадия резолвится в фоне…", "resolving stage in background…"},
		"detail.hint_run_ended":          {"забег окончен — h чтобы начать новый хайст", "run ended — h to start a new heist"},

		// --- PVP target browser ---
		"detail.pvp_title":     {"PVP ЦЕЛИ — атакующий #%d", "PVP TARGETS — attacker #%d"},
		"detail.scanning_area": {"сканирую зону…\n", "scanning your area…\n"},
		"detail.scan_failed":   {"сбой сканирования: %s", "scan failed: %s"},
		"detail.no_targets":    {"нет целей для атаки.\nPVP требует REP ≥ 200 и другого дилера в этой зоне.\n", "no attackable targets here.\nPVP needs REP ≥ 200 and another dealer in the same area.\n"},
		"detail.pvp_row":       {"#%-4s rep=%-6s шанс=%s%% инф=%s", "#%-4s rep=%-6s win=%s%% infamy=%s"},
		"detail.allies_hidden": {"\n  (скрыто целей-союзников: %d)\n", "\n  (%d ally target(s) hidden)\n"},
		"detail.pvp_hint":      {"↑/↓ выбор цели · enter атака · esc назад", "↑/↓ pick target · enter attack · esc back"},

		// --- PVE buy/sell form ---
		"detail.buy":          {"купить", "buy"},
		"detail.sell":         {"продать", "sell"},
		"detail.drug_line":    {"  Препарат: %s #%s %s  %s", "  Drug:   %s #%s %s  %s"},
		"detail.drug_meta":    {"(%s @%s · у вас ×%s · %d/%d)", "(%s @%s · you have ×%s · %d/%d)"},
		"detail.amount_label": {"  Кол-во: ", "  Amount: "},
		"detail.max_label":    {"макс %d (%s)", "max %d (%s)"},
		"detail.choice_deal":  {"выбор: deal — шансы фиксированы", "choice: deal — odds fixed"},
		"detail.form_hint":    {"↑/↓ выбор препарата · ввод кол-ва · enter отправить · esc отмена", "↑/↓ pick drug · type amount · enter submit · esc cancel"},

		// --- trade-limit labels + amount validation ---
		"detail.amount_positive": {"кол-во должно быть положительным числом", "amount must be a positive number"},
		"detail.max_units":       {"макс %d ед. здесь (%s)", "max %d units here (%s)"},
		"detail.cap_stake":       {"лимит ставки", "stake cap"},
		"detail.cap_cash":        {"кэш", "cash"},
		"detail.cap_held":        {"на руках", "held"},
	})
}
