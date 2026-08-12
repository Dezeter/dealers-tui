package tui

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"dealers/internal/chain/bindings"
	"dealers/internal/dealer"
	"dealers/internal/i18n"
	"dealers/internal/store"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// DetailModel is one dealer's detail view plus a PVE action form. Submitting an
// action commits via the DealerManager; the background engine resolves it and
// the periodic refresh reflects the outcome (pending → log).
type DetailModel struct {
	deps      Deps
	tokenID   uint64
	snap      dealer.Snapshot
	gameState *bindings.GameState // for the per-action stake limit
	pending   []store.Pending
	log       []store.LogRow
	height    int

	// action form
	formOpen   bool
	hustle     bindings.HustleType
	areaDrugs  []bindings.AreaDrug // full market of the current area (last fetch)
	drugs      []bindings.AreaDrug // filtered tradeable list for the open form
	drugIdx    int                 // selected index into drugs
	amount     textinput.Model
	confirm    string // pending single-tx action awaiting y/n ("" = none)
	submitting bool
	notice     string

	// PVP target browser
	pvpOpen      bool
	pvpLoading   bool
	targets      []bindings.PVPTarget
	targetIdx    int
	pvpErr       string
	alliesHidden int

	// Travel destination picker
	tvOpen    bool
	tvLoading bool
	tvDests   []bindings.AreaEconomy
	tvIdx     int
	tvErr     string

	// Heist state (live from chain) + start form
	heistID   uint64
	heist     *bindings.DailyHeist
	hsOpen    bool // heist start form open
	hsField   int  // 0 family, 1 difficulty, 2 jackpot
	hsFamily  bindings.HeistFamily
	hsDiff    uint8
	hsJackpot bool
}

type detailDataMsg struct {
	snap      dealer.Snapshot
	gameState *bindings.GameState
	pending   []store.Pending
	log       []store.LogRow
	heistID   uint64
	heist     *bindings.DailyHeist
	areaDrugs []bindings.AreaDrug
}

type submitDoneMsg struct {
	seq uint64
	err error
}

// actionDoneMsg is the result of a single-tx action (clear heat, reset attempts…).
type actionDoneMsg struct {
	label string
	err   error
}

// pvpTargetsMsg carries a completed potential-targets scan.
type pvpTargetsMsg struct {
	targets []bindings.PVPTarget
	total   uint64
	err     error
}

// travelAreasMsg carries the destination list for the travel picker.
type travelAreasMsg struct {
	areas []bindings.AreaEconomy
	err   error
}

// NewDetail builds the detail model seeded with the fleet's cached snapshot.
func NewDetail(deps Deps, tokenID uint64, seed dealer.Snapshot) DetailModel {
	amount := textinput.New()
	amount.Placeholder = "1"
	amount.CharLimit = 10
	amount.Width = 10
	return DetailModel{deps: deps, tokenID: tokenID, snap: seed, amount: amount}
}

func (m DetailModel) Init() tea.Cmd { return m.fetchCmd() }

// Refresh re-reads the dealer state, its open rounds, and recent log.
func (m DetailModel) Refresh() tea.Cmd { return m.fetchCmd() }

func (m DetailModel) fetchCmd() tea.Cmd {
	deps, tid := m.deps, m.tokenID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		st, err := deps.Reader.GetFullDealerState(ctx, tid)
		msg := detailDataMsg{snap: dealer.Snapshot{TokenID: tid, State: st, Err: err, FetchedAt: time.Now()}}
		if deps.Store != nil {
			msg.pending, _ = deps.Store.PendingForToken(tid)
			msg.log, _ = deps.Store.RecentActions(tid, 8)
		}
		if hid, herr := deps.Reader.ActiveHeist(ctx, tid); herr == nil && hid != 0 {
			msg.heistID = hid
			msg.heist, _ = deps.Reader.GetHeist(ctx, hid)
		}
		if st != nil {
			if econ, eerr := deps.Reader.AreaEconomy(ctx, st.CurrentArea); eerr == nil {
				msg.areaDrugs = econ.Drugs
			}
		}
		msg.gameState, _ = deps.Reader.GameState(ctx, tid)
		return msg
	}
}

func (m DetailModel) submitCmd(drug, amount uint64) tea.Cmd {
	deps, tid, hustle := m.deps, m.tokenID, m.hustle
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		seq, err := deps.Manager.SubmitPVE(ctx, tid, bindings.ChoiceDeal, hustle, drug, amount)
		return submitDoneMsg{seq: seq, err: err}
	}
}

func (m DetailModel) Update(msg tea.Msg) (DetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case detailDataMsg:
		m.snap = msg.snap
		m.gameState = msg.gameState
		m.pending = msg.pending
		m.log = msg.log
		m.heistID = msg.heistID
		m.heist = msg.heist
		m.areaDrugs = msg.areaDrugs
		return m, nil

	case submitDoneMsg:
		m.submitting = false
		m.formOpen = false
		if msg.err != nil {
			m.notice = errStyle.Render(i18n.T("detail.submit_failed", msg.err.Error()))
		} else {
			m.notice = okStyle.Render(i18n.T("detail.committed_seq", msg.seq))
		}
		return m, m.Refresh()

	case actionDoneMsg:
		m.submitting = false
		if msg.err != nil {
			m.notice = errStyle.Render(i18n.T("detail.action_failed", msg.label, msg.err.Error()))
		} else {
			m.notice = okStyle.Render(msg.label)
		}
		return m, m.Refresh()

	case pvpTargetsMsg:
		m.pvpLoading = false
		m.targetIdx = 0
		if msg.err != nil {
			m.pvpErr = msg.err.Error()
			return m, nil
		}
		m.pvpErr = ""
		// Hide allies (never attack the do-not-attack list).
		kept := msg.targets[:0]
		m.alliesHidden = 0
		for _, t := range msg.targets {
			if t.TokenID != nil && m.deps.IsAlly(t.TokenID.Uint64()) {
				m.alliesHidden++
				continue
			}
			kept = append(kept, t)
		}
		m.targets = kept
		return m, nil

	case travelAreasMsg:
		m.tvLoading = false
		m.tvIdx = 0
		if msg.err != nil {
			m.tvErr = msg.err.Error()
			return m, nil
		}
		m.tvErr = ""
		m.tvDests = m.filterDestinations(msg.areas)
		return m, nil

	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.submitting {
			return m, nil // ignore input while a tx is in flight
		}
		if m.pvpOpen {
			return m.updatePVP(msg)
		}
		if m.tvOpen {
			return m.updateTravel(msg)
		}
		if m.formOpen {
			return m.updateForm(msg)
		}
		if m.hsOpen {
			return m.updateHeistStart(msg)
		}
		if m.confirm != "" {
			return m.updateConfirm(msg)
		}
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return backToFleetMsg{} }
		case "r":
			return m, m.Refresh()
		case "b":
			return m.openForm(bindings.HustleBuy)
		case "s":
			return m.openForm(bindings.HustleSell)
		case "c":
			return m.clearHeat()
		case "k":
			return m.breakout()
		case "l":
			if m.snap.State != nil && m.snap.State.IsJailed {
				return m.askConfirm("bail", i18n.T("detail.confirm_bail"))
			}
			m.notice = errStyle.Render(i18n.T("detail.not_in_jail"))
			return m, nil
		case "a":
			return m.askConfirm("resetattempts", i18n.T("detail.confirm_reset_attempts"))
		case "p":
			return m.openPVP()
		case "t":
			return m.openTravel()
		case "h":
			return m.openHeistStart()
		case "g":
			return m.commitStageAction()
		case "o":
			if m.canCashOut() {
				return m.askConfirm("cashout", i18n.T("detail.confirm_cashout", m.heistID, heistPot(m.heist)))
			}
			m.notice = errStyle.Render(i18n.T("detail.cashout_needs"))
			return m, nil
		case "x":
			if m.heist != nil && m.heist.Status == uint8(bindings.HeistPreStage) {
				return m.askConfirm("abandon", i18n.T("detail.confirm_abandon", m.heistID))
			}
			m.notice = errStyle.Render(i18n.T("detail.abandon_only"))
			return m, nil
		}
	}
	return m, nil
}

func (m DetailModel) canCashOut() bool {
	return m.heist != nil && m.heist.Status == uint8(bindings.HeistRevealedWin) && m.heist.CurrentStage >= 2
}

// openTravel opens the destination picker and fetches the area list.
func (m DetailModel) openTravel() (DetailModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render(i18n.T("detail.read_only_no_signer"))
		return m, nil
	}
	if m.snap.State != nil && m.snap.State.IsJailed {
		m.notice = errStyle.Render(i18n.T("detail.cant_travel_jailed"))
		return m, nil
	}
	m.tvOpen = true
	m.tvLoading = true
	m.tvDests = nil
	m.tvIdx = 0
	m.tvErr = ""
	m.notice = ""
	deps := m.deps
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		areas, err := deps.Reader.AllAreas(ctx)
		return travelAreasMsg{areas: areas, err: err}
	}
}

// filterDestinations keeps active, travellable areas other than the current one.
// Jail and the Safe House are excluded: DealersCore.forceMove reverts on both
// (CannotEnterJail / CannotEnterSafeHouse), so travelling there just wastes gas —
// the Safe House entry isn't a shipped feature yet.
func (m DetailModel) filterDestinations(areas []bindings.AreaEconomy) []bindings.AreaEconomy {
	cur := uint8(255)
	if m.snap.State != nil {
		cur = m.snap.State.CurrentArea
	}
	var out []bindings.AreaEconomy
	for _, a := range areas {
		if !a.IsActive || a.IsJail || a.IsSafeHouse || a.AreaID == cur {
			continue
		}
		out = append(out, a)
	}
	return out
}

// canEnter reports whether the dealer meets an area's gates and, if not, the
// blocker. Normal areas gate on reputation; the black market gates on infamy
// (not reputation) — the reason travel there fails for low-infamy dealers.
func (m DetailModel) canEnter(a bindings.AreaEconomy) (bool, string) {
	st := m.snap.State
	if st == nil {
		return true, ""
	}
	if a.AreaID == bindings.BlackMarketArea {
		min := big.NewInt(bindings.BlackMarketMinInfamy)
		if st.Infamy == nil || st.Infamy.Cmp(min) < 0 {
			return false, i18n.T("detail.gate_infamy", bindings.BlackMarketMinInfamy)
		}
		return true, ""
	}
	if a.MinReputation != nil && a.MinReputation.Sign() > 0 {
		if st.Reputation == nil || st.Reputation.Cmp(a.MinReputation) < 0 {
			return false, i18n.T("detail.gate_rep") + a.MinReputation.String()
		}
	}
	return true, ""
}

func (m DetailModel) updateTravel(msg tea.KeyMsg) (DetailModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.tvOpen = false
		return m, nil
	case "up":
		if n := len(m.tvDests); n > 0 {
			m.tvIdx = ((m.tvIdx-1)%n + n) % n
		}
		return m, nil
	case "down":
		if n := len(m.tvDests); n > 0 {
			m.tvIdx = (m.tvIdx + 1) % n
		}
		return m, nil
	case "enter":
		if m.tvIdx < 0 || m.tvIdx >= len(m.tvDests) {
			return m, nil
		}
		dest := m.tvDests[m.tvIdx]
		if ok, why := m.canEnter(dest); !ok {
			m.notice = errStyle.Render(i18n.T("detail.cant_enter", m.deps.AreaName(dest.AreaID), why))
			return m, nil
		}
		m.tvOpen = false
		m.submitting = true
		m.notice = statusBarStyle.Render(i18n.T("detail.traveling_to", m.deps.AreaName(dest.AreaID)))
		area := dest.AreaID
		name := m.deps.AreaName(area)
		return m, m.managerAction(i18n.T("detail.arrived_at", name), func(ctx context.Context) error {
			return m.deps.Manager.Travel(ctx, m.tokenID, area)
		})
	}
	return m, nil
}

// travelView renders the destination picker.
func (m DetailModel) travelView() string {
	var b strings.Builder
	here := "?"
	if m.snap.State != nil {
		here = m.deps.AreaName(m.snap.State.CurrentArea)
	}
	fmt.Fprintf(&b, "%s  %s\n\n", titleStyle.Render(i18n.T("detail.travel_title")), helpStyle.Render(i18n.T("detail.youre_in", here)))

	switch {
	case m.tvLoading:
		b.WriteString(statusBarStyle.Render(i18n.T("detail.loading_areas")))
	case m.tvErr != "":
		b.WriteString(errStyle.Render(i18n.T("detail.failed", m.tvErr)) + "\n")
	case len(m.tvDests) == 0:
		b.WriteString(helpStyle.Render(i18n.T("detail.no_destinations")))
	default:
		for i, a := range m.tvDests {
			cursor := "  "
			fee := i18n.T("detail.free")
			if a.MovementFee != nil && a.MovementFee.Sign() > 0 {
				fee = dealer.EthStr(a.MovementFee)
			}
			line := fmt.Sprintf("→ %-11s %s", m.deps.AreaName(a.AreaID), helpStyle.Render(fee))
			if ok, why := m.canEnter(a); !ok {
				line += errStyle.Render("  🔒 " + why)
			} else if a.AreaID == bindings.BlackMarketArea {
				line += helpStyle.Render("  " + i18n.T("detail.gate_infamy", bindings.BlackMarketMinInfamy))
			} else if a.MinReputation != nil && a.MinReputation.Sign() > 0 {
				line += helpStyle.Render("  " + i18n.T("detail.gate_rep") + a.MinReputation.String())
			}
			if i == m.tvIdx {
				cursor = focusStyle.Render("▶ ")
				line = focusStyle.Render(line)
			}
			b.WriteString("  " + cursor + line + "\n")
		}
	}
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	b.WriteString("\n" + helpStyle.Render(i18n.T("detail.travel_hint")))
	return b.String()
}

// openHeistStart opens the start-heist form (only when no active heist).
func (m DetailModel) openHeistStart() (DetailModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render(i18n.T("detail.read_only_no_signer"))
		return m, nil
	}
	if m.heistID != 0 {
		m.notice = errStyle.Render(i18n.T("detail.already_running_heist", m.heistID))
		return m, nil
	}
	m.hsOpen = true
	m.hsField = 0
	m.hsFamily = bindings.FamilyCash
	m.hsDiff = 0
	m.hsJackpot = false
	m.notice = ""
	return m, nil
}

func (m DetailModel) updateHeistStart(msg tea.KeyMsg) (DetailModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.hsOpen = false
		return m, nil
	case "tab", "down":
		m.hsField = (m.hsField + 1) % 3
		return m, nil
	case "shift+tab", "up":
		m.hsField = (m.hsField + 2) % 3
		return m, nil
	case "left", "right", " ":
		switch m.hsField {
		case 0:
			if m.hsFamily == bindings.FamilyCash {
				m.hsFamily = bindings.FamilySupply
			} else {
				m.hsFamily = bindings.FamilyCash
			}
		case 1:
			m.hsDiff = (m.hsDiff + 1) % 3
		case 2:
			m.hsJackpot = !m.hsJackpot
		}
		return m, nil
	case "enter":
		m.hsOpen = false
		m.submitting = true
		m.notice = statusBarStyle.Render(i18n.T("detail.starting_heist"))
		deps, tid := m.deps, m.tokenID
		fam, diff, jp := m.hsFamily, m.hsDiff, m.hsJackpot
		return m, m.managerAction(i18n.T("detail.heist_started"), func(ctx context.Context) error {
			_, err := deps.Manager.StartHeist(ctx, tid, fam, diff, jp)
			return err
		})
	}
	return m, nil
}

// commitStageAction pushes the heist forward (commit next stage).
func (m DetailModel) commitStageAction() (DetailModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render(i18n.T("detail.read_only_no_signer"))
		return m, nil
	}
	if m.heist == nil {
		m.notice = errStyle.Render(i18n.T("detail.no_active_heist"))
		return m, nil
	}
	st := m.heist.Status
	if st != uint8(bindings.HeistPreStage) && st != uint8(bindings.HeistRevealedWin) {
		m.notice = errStyle.Render(i18n.T("detail.cant_push_now"))
		return m, nil
	}
	m.submitting = true
	m.notice = statusBarStyle.Render(i18n.T("detail.committing_stage"))
	deps, tid, hid := m.deps, m.tokenID, m.heistID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		seq, err := deps.Manager.CommitStage(ctx, tid, hid)
		return submitDoneMsg{seq: seq, err: err}
	}
}

// openPVP enters the target browser and kicks off a scan of the attacker's area.
func (m DetailModel) openPVP() (DetailModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render(i18n.T("detail.read_only_no_signer_src"))
		return m, nil
	}
	m.pvpOpen = true
	m.pvpLoading = true
	m.targets = nil
	m.targetIdx = 0
	m.pvpErr = ""
	m.notice = ""
	deps, tid := m.deps, m.tokenID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		targets, total, err := deps.Reader.PotentialTargets(ctx, tid, 0, 25)
		return pvpTargetsMsg{targets: targets, total: total, err: err}
	}
}

func (m DetailModel) updatePVP(msg tea.KeyMsg) (DetailModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.pvpOpen = false
		return m, nil
	case "up":
		if n := len(m.targets); n > 0 {
			m.targetIdx = ((m.targetIdx-1)%n + n) % n
		}
		return m, nil
	case "down":
		if n := len(m.targets); n > 0 {
			m.targetIdx = (m.targetIdx + 1) % n
		}
		return m, nil
	case "enter":
		if m.pvpLoading || m.targetIdx < 0 || m.targetIdx >= len(m.targets) {
			return m, nil
		}
		t := m.targets[m.targetIdx]
		if !t.CanAttackNow {
			m.notice = errStyle.Render(i18n.T("detail.target_not_attackable"))
			return m, nil
		}
		def := t.TokenID.Uint64()
		m.pvpOpen = false
		m.submitting = true
		m.notice = statusBarStyle.Render(i18n.T("detail.attacking", def))
		deps, tid := m.deps, m.tokenID
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			seq, err := deps.Manager.SubmitPVPAttack(ctx, tid, def)
			return submitDoneMsg{seq: seq, err: err}
		}
	}
	return m, nil
}

// breakout starts a free jailbreak (commit-reveal, ~50%, once/day, no ETH/energy).
func (m DetailModel) breakout() (DetailModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render(i18n.T("detail.read_only_no_signer"))
		return m, nil
	}
	if m.snap.State != nil && !m.snap.State.IsJailed {
		m.notice = errStyle.Render(i18n.T("detail.not_in_jail"))
		return m, nil
	}
	m.submitting = true
	m.notice = statusBarStyle.Render(i18n.T("detail.attempting_breakout"))
	deps, tid := m.deps, m.tokenID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		seq, err := deps.Manager.SubmitBreakout(ctx, tid)
		return submitDoneMsg{seq: seq, err: err}
	}
}

// clearHeat starts the ETH-free heat clear (wanted poster): a commit-reveal
// round that spends 1 attempt and clears heat on a ~50% roll. No ETH, so no
// confirm — treated like a hustle; the engine resolves it in the background.
func (m DetailModel) clearHeat() (DetailModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render(i18n.T("detail.read_only_no_signer_src"))
		return m, nil
	}
	m.submitting = true
	m.notice = statusBarStyle.Render(i18n.T("detail.removing_poster"))
	deps, tid := m.deps, m.tokenID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		seq, err := deps.Manager.SubmitWantedPoster(ctx, tid)
		return submitDoneMsg{seq: seq, err: err}
	}
}

// askConfirm arms a y/n confirmation for a single-tx action that spends ETH.
func (m DetailModel) askConfirm(id, label string) (DetailModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render(i18n.T("detail.read_only_no_signer_src"))
		return m, nil
	}
	m.confirm = id
	m.notice = statusBarStyle.Render(i18n.T("detail.confirm_prompt", label))
	return m, nil
}

func (m DetailModel) updateConfirm(msg tea.KeyMsg) (DetailModel, tea.Cmd) {
	switch msg.String() {
	case "y":
		id := m.confirm
		m.confirm = ""
		m.submitting = true
		switch id {
		case "resetattempts":
			m.notice = statusBarStyle.Render(i18n.T("detail.resetting_attempts"))
			return m, m.managerAction(i18n.T("detail.attempts_reset"), func(ctx context.Context) error {
				return m.deps.Manager.ResetAttempts(ctx, m.tokenID)
			})
		case "cashout":
			hid := m.heistID
			m.notice = statusBarStyle.Render(i18n.T("detail.cashing_out"))
			return m, m.managerAction(i18n.T("detail.heist_cashed_out"), func(ctx context.Context) error {
				return m.deps.Manager.CashOut(ctx, m.tokenID, hid)
			})
		case "abandon":
			hid := m.heistID
			m.notice = statusBarStyle.Render(i18n.T("detail.abandoning_heist"))
			return m, m.managerAction(i18n.T("detail.heist_abandoned"), func(ctx context.Context) error {
				return m.deps.Manager.AbandonHeist(ctx, m.tokenID, hid)
			})
		case "bail":
			m.notice = statusBarStyle.Render(i18n.T("detail.paying_bail"))
			return m, m.managerAction(i18n.T("detail.bailed_out"), func(ctx context.Context) error {
				return m.deps.Manager.PayBail(ctx, m.tokenID)
			})
		}
		m.submitting = false
		return m, nil
	case "n", "esc":
		m.confirm = ""
		m.notice = ""
		return m, nil
	}
	return m, nil
}

// managerAction runs a single-tx manager call off the UI goroutine.
func (m DetailModel) managerAction(okLabel string, fn func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		return actionDoneMsg{label: okLabel, err: fn(ctx)}
	}
}

// inBlackMarket reports whether the dealer is currently in the black market,
// where regular PVE buy/sell is disabled and loot is sold via sellDrop.
func (m DetailModel) inBlackMarket() bool {
	return m.snap.State != nil && m.snap.State.CurrentArea == bindings.BlackMarketArea
}

func (m DetailModel) openForm(h bindings.HustleType) (DetailModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render(i18n.T("detail.read_only_no_signer_src"))
		return m, nil
	}
	if h == bindings.HustleBuy && m.inBlackMarket() {
		m.notice = errStyle.Render(i18n.T("detail.cant_buy_bm"))
		return m, nil
	}
	if len(m.areaDrugs) == 0 {
		m.notice = errStyle.Render(i18n.T("detail.no_market_data"))
		return m, nil
	}
	list := m.tradeable(h)
	if len(list) == 0 {
		if h == bindings.HustleBuy {
			m.notice = errStyle.Render(i18n.T("detail.nothing_to_buy", m.deps.AreaName(areaOf(m.snap))))
		} else {
			m.notice = errStyle.Render(i18n.T("detail.nothing_sellable", m.deps.AreaName(areaOf(m.snap))))
		}
		return m, nil
	}

	m.formOpen = true
	m.hustle = h
	m.notice = ""
	m.drugs = list
	m.seedDrugIdx() // restore the remembered drug (if still in this list)
	m.amount.SetValue("")
	m.amount.Focus()
	return m, textinput.Blink
}

// tradeable returns the drugs the dealer can actually buy/sell in the current
// area: for BUY the area must sell them (isAvailable); for SELL you must hold
// them and the area must buy them. This prevents picking a drug that isn't in
// the location (which would revert on chain).
func (m DetailModel) tradeable(h bindings.HustleType) []bindings.AreaDrug {
	var list []bindings.AreaDrug
	for _, d := range m.areaDrugs {
		if h == bindings.HustleBuy {
			if d.IsAvailable && d.BuyPrice != nil && d.BuyPrice.Sign() > 0 {
				list = append(list, d)
			}
		} else {
			if d.SellPrice != nil && d.SellPrice.Sign() > 0 && m.heldBalance(d.DrugID.Uint64()).Sign() > 0 {
				list = append(list, d)
			}
		}
	}
	return list
}

// heldBalance returns how much of a drug the dealer currently holds.
func (m DetailModel) heldBalance(drugID uint64) *big.Int {
	if m.snap.State != nil {
		for _, d := range m.snap.State.DrugBalances {
			if d.DrugID != nil && d.DrugID.Uint64() == drugID {
				if d.Balance != nil {
					return d.Balance
				}
			}
		}
	}
	return big.NewInt(0)
}

func areaOf(s dealer.Snapshot) uint8 {
	if s.State != nil {
		return s.State.CurrentArea
	}
	return 0
}

// seedDrugIdx points drugIdx at the remembered drug id (else index 0).
func (m *DetailModel) seedDrugIdx() {
	want := m.deps.lastDrugID()
	for i, d := range m.drugs {
		if d.DrugID != nil && d.DrugID.Uint64() == want {
			m.drugIdx = i
			return
		}
	}
	m.drugIdx = 0
}

// applyDrugDelta moves the selection by delta (wrapping) and remembers it.
func (m *DetailModel) applyDrugDelta(delta int) {
	n := len(m.drugs)
	if n == 0 {
		return
	}
	m.drugIdx = ((m.drugIdx+delta)%n + n) % n
	if id := m.drugs[m.drugIdx].DrugID; id != nil {
		m.deps.setLastDrugID(id.Uint64())
	}
}

func (m DetailModel) selectedDrugID() uint64 {
	if m.drugIdx < 0 || m.drugIdx >= len(m.drugs) || m.drugs[m.drugIdx].DrugID == nil {
		return 0
	}
	return m.drugs[m.drugIdx].DrugID.Uint64()
}

func (m DetailModel) updateForm(msg tea.KeyMsg) (DetailModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.formOpen = false
		return m, nil
	case "up":
		m.applyDrugDelta(-1)
		return m, nil
	case "down":
		m.applyDrugDelta(+1)
		return m, nil
	case "enter":
		amount, err := m.parseAmount()
		if err != nil {
			m.notice = errStyle.Render(err.Error())
			return m, nil
		}
		drug := m.selectedDrugID()
		m.formOpen = false
		m.submitting = true
		// In the black market, selling loot is a guaranteed single-tx (sellDrop),
		// not the PVE gamble — and it costs no energy.
		if m.inBlackMarket() {
			m.notice = statusBarStyle.Render(i18n.T("detail.bm_selling", amount, drug))
			d, a := drug, amount
			return m, m.managerAction(i18n.T("detail.bm_sold", a, d), func(ctx context.Context) error {
				return m.deps.Manager.SellDrop(ctx, m.tokenID, d, a)
			})
		}
		m.notice = statusBarStyle.Render(i18n.T("detail.committing_trade", hustleName(m.hustle), drug, amount))
		return m, m.submitCmd(drug, amount)
	}

	var cmd tea.Cmd
	m.amount, cmd = m.amount.Update(msg)
	return m, cmd
}

func (m DetailModel) parseAmount() (uint64, error) {
	as := strings.TrimSpace(m.amount.Value())
	if as == "" {
		as = "1"
	}
	amount, err := strconv.ParseUint(as, 10, 64)
	if err != nil || amount == 0 {
		return 0, fmt.Errorf("%s", i18n.T("detail.amount_positive"))
	}
	if max, label, ok := m.tradeLimit(); ok && amount > max {
		return 0, fmt.Errorf("%s", i18n.T("detail.max_units", max, label))
	}
	return amount, nil
}

// tradeLimit returns the largest amount tradeable for the selected drug and the
// binding constraint's label. Combines the per-action stake cap (rep-based),
// buy cash, and sell holdings — whichever is smallest binds.
func (m DetailModel) tradeLimit() (max uint64, label string, ok bool) {
	if len(m.drugs) == 0 || m.drugIdx < 0 || m.drugIdx >= len(m.drugs) {
		return 0, "", false
	}
	sel := m.drugs[m.drugIdx]
	price := sel.BuyPrice
	if m.hustle == bindings.HustleSell {
		price = sel.SellPrice
	}

	type cap struct {
		n     uint64
		label string
	}
	var caps []cap

	// Per-action stake cap (rep-based) — PVE only; sellDrop in the black market
	// has no stake limit.
	if !m.inBlackMarket() && m.deps.StakeParams != nil && m.gameState != nil {
		if u := dealer.MaxUnitsAtPrice(dealer.MaxStake(m.gameState, m.deps.StakeParams), price); u > 0 {
			caps = append(caps, cap{u, i18n.T("detail.cap_stake")})
		}
	}
	// Resource cap: cash for buy, holdings for sell.
	if m.hustle == bindings.HustleBuy {
		if cash := m.snap.State.CashBalance; cash != nil && price != nil && price.Sign() > 0 {
			caps = append(caps, cap{new(big.Int).Div(cash, price).Uint64(), i18n.T("detail.cap_cash")})
		}
	} else {
		caps = append(caps, cap{m.heldBalance(sel.DrugID.Uint64()).Uint64(), i18n.T("detail.cap_held")})
	}

	if len(caps) == 0 {
		return 0, "", false
	}
	min := caps[0]
	for _, c := range caps[1:] {
		if c.n < min.n {
			min = c
		}
	}
	return min.n, min.label, true
}

func (m DetailModel) View() string {
	if m.pvpOpen {
		return m.pvpView()
	}
	if m.tvOpen {
		return m.travelView()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", titleStyle.Render(i18n.T("detail.dealer_title", m.tokenID)))

	if m.snap.Err != nil {
		b.WriteString(errStyle.Render(i18n.T("detail.read_error", m.snap.Err.Error())) + "\n")
	} else if st := m.snap.State; st != nil {
		b.WriteString(kv(i18n.T("detail.kv_rank"), st.ReputationTitle))
		b.WriteString(kv("REP", bigStr(st.Reputation)))
		b.WriteString(kv(i18n.T("detail.kv_heat"), fmt.Sprintf("%d/5", st.HeatLevel)))
		b.WriteString(kv(i18n.T("detail.kv_cash"), bigStr(st.CashBalance)))
		b.WriteString(kv(i18n.T("detail.kv_area"), m.deps.AreaName(st.CurrentArea)))
		b.WriteString(kv(i18n.T("detail.kv_energy"), i18n.T("detail.attempts_fmt", st.DailyAttemptsRemaining, st.MaxAttempts)))
		b.WriteString(kv(i18n.T("detail.kv_infamy"), bigStr(st.Infamy)))
		b.WriteString(kv(i18n.T("detail.kv_pve_wtl"), fmt.Sprintf("%d/%d/%d", st.PveWins, st.PveTies, st.PveLosses)))
		b.WriteString(kv(i18n.T("detail.kv_status"), m.snap.Status()))
		b.WriteString("\n" + sectionStyle.Render(i18n.T("detail.sec_stash")) + "\n")
		if len(st.DrugBalances) == 0 {
			b.WriteString(helpStyle.Render(i18n.T("detail.empty")))
		}
		for _, d := range st.DrugBalances {
			if d.Balance != nil && d.Balance.Sign() > 0 {
				fmt.Fprintf(&b, "  #%s %-10s ×%s\n", d.DrugID, truncate(d.Name, 10), d.Balance)
			}
		}
	}

	// Open rounds.
	b.WriteString("\n" + sectionStyle.Render(i18n.T("detail.sec_pending")) + "\n")
	if len(m.pending) == 0 {
		b.WriteString(helpStyle.Render(i18n.T("detail.none")))
	}
	for _, p := range m.pending {
		fmt.Fprint(&b, i18n.T("detail.pending_row",
			p.Seq, p.Kind, p.RevealBlock, p.ExpiryBlock, statusBarStyle.Render(i18n.T("detail.resolving"))))
	}

	// Heist.
	b.WriteString("\n" + sectionStyle.Render(i18n.T("detail.sec_heist")) + "\n")
	if m.hsOpen {
		b.WriteString(m.heistStartView())
	} else if m.heist == nil {
		b.WriteString(helpStyle.Render(i18n.T("detail.heist_none")))
	} else {
		h := m.heist
		fmt.Fprint(&b, i18n.T("detail.heist_row",
			m.heistID, bindings.HeistFamily(h.Family), h.Difficulty, h.CurrentStage,
			bindings.HeistStatus(h.Status), bigStr(h.CurrentPot), jackpotFlag(h.EthJackpot)))
		b.WriteString(helpStyle.Render("  " + heistActionsHint(h) + "\n"))
	}

	// Recent log.
	b.WriteString("\n" + sectionStyle.Render(i18n.T("detail.sec_recent")) + "\n")
	if len(m.log) == 0 {
		b.WriteString(helpStyle.Render(i18n.T("detail.no_activity")))
	}
	for _, l := range m.log {
		fmt.Fprintf(&b, "  %s %s: %s\n", helpStyle.Render(l.TS), l.Kind, colorizeLog(l.Summary))
	}

	// Footer: form or hints.
	b.WriteString("\n")
	if m.notice != "" {
		b.WriteString(m.notice + "\n")
	}
	if m.formOpen {
		b.WriteString(m.formView())
	} else {
		hint := i18n.T("detail.hint_main")
		if m.deps.Manager == nil {
			hint = i18n.T("detail.hint_readonly")
		} else if m.snap.State != nil && m.snap.State.IsJailed {
			hint = i18n.T("detail.hint_jailed")
		} else if m.inBlackMarket() {
			hint = i18n.T("detail.hint_bm")
		}
		b.WriteString(helpStyle.Render(hint))
	}
	return b.String()
}

// heistStartView renders the start-heist form.
func (m DetailModel) heistStartView() string {
	fam := i18n.T("detail.fam_supply")
	if m.hsFamily == bindings.FamilyCash {
		fam = i18n.T("detail.fam_cash")
	}
	diffNames := []string{i18n.T("detail.diff_easy"), i18n.T("detail.diff_medium"), i18n.T("detail.diff_hard")}
	jp := i18n.T("common.off")
	if m.hsJackpot {
		jp = i18n.T("detail.jp_on")
	}
	rows := []struct{ label, val string }{
		{i18n.T("detail.lbl_family"), fam},
		{i18n.T("detail.lbl_difficulty"), diffNames[m.hsDiff]},
		{i18n.T("detail.lbl_eth_jackpot"), jp},
	}
	var b strings.Builder
	for i, r := range rows {
		marker := "  "
		label := r.label
		if i == m.hsField {
			marker = focusStyle.Render("▶ ")
			label = focusStyle.Render(r.label)
		}
		fmt.Fprintf(&b, "  %s%-12s %s\n", marker, label, r.val)
	}
	b.WriteString(helpStyle.Render(i18n.T("detail.heist_start_hint")))
	return b.String()
}

func heistPot(h *bindings.DailyHeist) string {
	if h == nil {
		return "0"
	}
	return bigStr(h.CurrentPot)
}

func jackpotFlag(on bool) string {
	if on {
		return i18n.T("detail.jackpot_flag")
	}
	return ""
}

// heistActionsHint lists the actions available for the current heist status.
func heistActionsHint(h *bindings.DailyHeist) string {
	switch bindings.HeistStatus(h.Status) {
	case bindings.HeistPreStage:
		return i18n.T("detail.hint_prestage")
	case bindings.HeistRevealedWin:
		if h.CurrentStage >= 2 {
			return i18n.T("detail.hint_pushdeeper_cashout")
		}
		return i18n.T("detail.hint_pushdeeper")
	case bindings.HeistCommitted:
		return i18n.T("detail.hint_resolving_stage")
	default:
		return i18n.T("detail.hint_run_ended")
	}
}

// pvpView renders the target browser.
func (m DetailModel) pvpView() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", titleStyle.Render(i18n.T("detail.pvp_title", m.tokenID)))

	switch {
	case m.pvpLoading:
		b.WriteString(statusBarStyle.Render(i18n.T("detail.scanning_area")))
	case m.pvpErr != "":
		b.WriteString(errStyle.Render(i18n.T("detail.scan_failed", m.pvpErr)) + "\n")
	case len(m.targets) == 0:
		b.WriteString(helpStyle.Render(i18n.T("detail.no_targets")))
	default:
		for i, t := range m.targets {
			cursor := "  "
			line := i18n.T("detail.pvp_row",
				t.TokenID, bigStr(t.Reputation), bigStr(t.WinChance), bigStr(t.Infamy))
			if !t.CanAttackNow {
				line += helpStyle.Render("  [" + i18n.T("common.unavailable") + "]")
			}
			if i == m.targetIdx {
				cursor = focusStyle.Render("▶ ")
				line = focusStyle.Render(line)
			}
			b.WriteString("  " + cursor + line + "\n")
		}
	}

	if m.alliesHidden > 0 {
		b.WriteString(helpStyle.Render(i18n.T("detail.allies_hidden", m.alliesHidden)))
	}
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	b.WriteString("\n" + helpStyle.Render(i18n.T("detail.pvp_hint")))
	return b.String()
}

func (m DetailModel) formView() string {
	sel := m.drugs[m.drugIdx]
	price := sel.BuyPrice
	plabel := i18n.T("detail.buy")
	if m.hustle == bindings.HustleSell {
		price = sel.SellPrice
		plabel = i18n.T("detail.sell")
	}
	bal := m.heldBalance(sel.DrugID.Uint64())
	drugLine := i18n.T("detail.drug_line",
		focusStyle.Render("↑/↓"),
		sel.DrugID, sel.Name,
		helpStyle.Render(i18n.T("detail.drug_meta", plabel, bigStr(price), bal, m.drugIdx+1, len(m.drugs))))
	amtLine := i18n.T("detail.amount_label") + m.amount.View()
	if max, label, ok := m.tradeLimit(); ok {
		amtLine += "  " + helpStyle.Render(i18n.T("detail.max_label", max, label))
	}

	return fmt.Sprintf("%s %s\n%s\n%s\n%s",
		titleStyle.Render(strings.ToUpper(hustleName(m.hustle))),
		helpStyle.Render(i18n.T("detail.choice_deal")),
		drugLine, amtLine,
		helpStyle.Render(i18n.T("detail.form_hint")))
}

func kv(k, v string) string {
	return fmt.Sprintf("  %-10s %s\n", k+":", v)
}

func hustleName(h bindings.HustleType) string {
	if h == bindings.HustleSell {
		return i18n.T("detail.sell")
	}
	return i18n.T("detail.buy")
}
