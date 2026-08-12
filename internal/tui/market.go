package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dealers/internal/chain/bindings"
	"dealers/internal/dealer"
	"dealers/internal/i18n"

	tea "github.com/charmbracelet/bubbletea"
)

// MarketModel shows cross-area prices and the best buy-low/sell-high pairs
// (the arbitrage board). It reads getAllAreas once per open/refresh.
type MarketModel struct {
	deps    Deps
	loading bool
	areas   []bindings.AreaEconomy
	pairs   []dealer.ArbPair
	err     string
}

type marketDataMsg struct {
	areas []bindings.AreaEconomy
	err   error
}

func NewMarket(deps Deps) MarketModel {
	return MarketModel{deps: deps, loading: true}
}

func (m MarketModel) Init() tea.Cmd { return m.fetch() }

func (m MarketModel) fetch() tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		areas, err := deps.Reader.AllAreas(ctx)
		return marketDataMsg{areas: areas, err: err}
	}
}

func (m MarketModel) Update(msg tea.Msg) (MarketModel, tea.Cmd) {
	switch msg := msg.(type) {
	case marketDataMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.areas = msg.areas
		m.pairs = dealer.Arbitrage(msg.areas)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return backToFleetMsg{} }
		case "r":
			m.loading = true
			return m, m.fetch()
		}
	}
	return m, nil
}

func (m MarketModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(i18n.T("market.title")) + "\n\n")

	if m.loading {
		return b.String() + statusBarStyle.Render(i18n.T("market.scanning"))
	}
	if m.err != "" {
		return b.String() + errStyle.Render(i18n.T("market.scan_failed")+": "+m.err)
	}

	// Arbitrage board — buy cheap here, sell dear there.
	b.WriteString(sectionStyle.Render(i18n.T("market.board_title")) + "\n")
	if len(m.pairs) == 0 {
		b.WriteString(helpStyle.Render(i18n.T("market.no_spreads")))
	}
	for i, p := range m.pairs {
		if i >= 12 {
			break
		}
		gate := ""
		if p.SellMinRep != nil && p.SellMinRep.Sign() > 0 {
			gate = helpStyle.Render(i18n.T("market.rep_gate", p.SellMinRep))
		}
		travel := ""
		if fee := p.TravelWei(); fee.Sign() > 0 {
			travel = helpStyle.Render(i18n.T("market.travel_fee", dealer.EthStr(fee)))
		}
		b.WriteString(i18n.T("market.row",
			truncate(p.DrugName, 9), bigStr(p.BuyPrice), m.deps.AreaName(p.BuyArea),
			bigStr(p.SellPrice), m.deps.AreaName(p.SellArea),
			okStyle.Render(i18n.T("market.profit_unit", bigStr(p.Profit))), travel, gate))
	}
	b.WriteString(helpStyle.Render(i18n.T("market.explain")) + "\n")

	// Full price table by area.
	b.WriteString("\n" + sectionStyle.Render(i18n.T("market.prices_title")) + "\n")
	for _, a := range m.areas {
		if !a.IsActive || a.IsJail || a.IsSafeHouse || len(a.Drugs) == 0 {
			continue
		}
		gate := ""
		if a.MinReputation != nil && a.MinReputation.Sign() > 0 {
			gate = helpStyle.Render(i18n.T("market.area_gate", a.MinReputation))
		}
		fmt.Fprintf(&b, "  %s%s\n", focusStyle.Render(m.deps.AreaName(a.AreaID)), gate)
		var parts []string
		for _, d := range a.Drugs {
			if !d.IsAvailable {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s %s/%s", d.Name, bigStr(d.BuyPrice), bigStr(d.SellPrice)))
		}
		b.WriteString("    " + strings.Join(parts, " · ") + "\n")
	}

	b.WriteString("\n" + helpStyle.Render(i18n.T("market.hint")))
	return b.String()
}
