package i18n

// Per-dealer missions screen: progress, accept/claim actions, rewards.
func init() {
	add(map[string][2]string{
		"missions.title":        {"МИССИИ — дилер #%d", "MISSIONS — dealer #%d"},
		"missions.not_deployed": {"контракт миссий не развёрнут в этой сети", "missions contract not deployed on this network"},
		"missions.none":         {"нет активных миссий", "no active missions"},
		"missions.daily":        {"ЕЖЕДНЕВНЫЕ", "DAILY"},
		"missions.weekly":       {"ЕЖЕНЕДЕЛЬНЫЕ", "WEEKLY"},
		"missions.hint":         {"a принять (чек-ин) · c забрать всё · r обновить · esc назад", "a accept (check-in) · c claim all · r refresh · esc back"},
		"missions.hint_ro":      {"только чтение · r обновить · esc назад", "read-only · r refresh · esc back"},

		// action notices
		"missions.ro_signer":     {"только чтение — нет подписанта", "read-only — no signer"},
		"missions.accepting":     {"принимаю…", "accepting…"},
		"missions.accepted":      {"чек-ин выполнен — сегодняшние миссии приняты", "checked in — today's missions accepted"},
		"missions.nothing_claim": {"нечего забирать", "nothing to claim"},
		"missions.claiming":      {"забираю…", "claiming…"},
		"missions.claimed_then":  {"забрано %d, затем: %v", "claimed %d, then: %v"},
		"missions.claimed_n":     {"забрано наград: %d", "claimed %d mission reward(s)"},

		// per-mission state
		"missions.in_progress":  {"в процессе", "in progress"},
		"missions.claimed":      {"забрано ✓", "claimed ✓"},
		"missions.claimable":    {"МОЖНО ЗАБРАТЬ ★", "CLAIMABLE ★"},
		"missions.not_accepted": {"не принято — нажми a", "not accepted — press a"},

		// reward parts
		"missions.rew_rep":   {"+%d реп", "+%d rep"},
		"missions.rew_inf":   {"+%d инф", "+%d inf"},
		"missions.rew_cash":  {"+%s кэш", "+%s cash"},
		"missions.rew_drug":  {"+%d товар#%d", "+%d drug#%d"},
		"missions.no_reward": {"(без награды)", "(no reward)"},
	})
}
