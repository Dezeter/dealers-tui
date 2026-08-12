package i18n

// Market / arbitrage screen.
func init() {
	add(map[string][2]string{
		"market.title":        {"РЫНОК — лучшие сделки", "MARKET — best deals"},
		"market.scanning":     {"сканирую рынки…", "scanning markets…"},
		"market.scan_failed":  {"сканирование не удалось", "scan failed"},
		"market.board_title":  {"Лучший арбитраж (купить → продать)", "Best arbitrage (buy → sell)"},
		"market.no_spreads":   {"  сейчас нет межзонных спредов\n", "  no cross-area spreads right now\n"},
		"market.rep_gate":     {" реп≥%s", " rep≥%s"},
		"market.travel_fee":   {" · переезд %s", " · travel %s"},
		"market.profit_unit":  {"+%s/ед $CASH", "+%s/u $CASH"},
		"market.row":          {"  %-9s купить %s @%-9s → продать %s @%-9s  %s%s%s\n", "  %-9s buy %s @%-9s → sell %s @%-9s  %s%s%s\n"},
		"market.explain":      {"  прибыль/ед = ожидаемый $CASH (сделка купли/продажи — честная монетка: выигрыш и\n  проигрыш гасят друг друга, ничья — обычная сделка, так что в среднем вы берёте спред\n  плюс репутацию).\n  переезд — разовая комиссия ETH, размазанная на партию. купля и продажа стоят по 1\n  энергии независимо от объёма → возите большие партии, чтобы поездка окупалась.\n", "  profit/u = expected $CASH (the buy/sell gamble is even-money — win/loss cancel,\n  tie is a normal trade — so on average you net the spread, plus you earn rep).\n  travel is a one-off ETH fee amortised over the batch. buy & sell each cost 1 energy\n  regardless of amount → carry big batches to make it worth the trip.\n"},
		"market.prices_title": {"Цены по зонам (купить / продать)", "Prices by area (buy / sell)"},
		"market.area_gate":    {" (реп %s)", " (rep %s)"},
		"market.hint":         {"r обновить · esc назад", "r refresh · esc back"},
	})
}
