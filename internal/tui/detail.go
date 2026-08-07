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
			m.notice = errStyle.Render("submit failed: " + msg.err.Error())
		} else {
			m.notice = okStyle.Render(fmt.Sprintf("committed seq=%d — resolving in background…", msg.seq))
		}
		return m, m.Refresh()

	case actionDoneMsg:
		m.submitting = false
		if msg.err != nil {
			m.notice = errStyle.Render(msg.label + " failed: " + msg.err.Error())
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
				return m.askConfirm("bail", "Pay bail (ETH) to release now")
			}
			m.notice = errStyle.Render("not in jail")
			return m, nil
		case "a":
			return m.askConfirm("resetattempts", "Reset daily attempts")
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
				return m.askConfirm("cashout", fmt.Sprintf("Cash out heist #%d (pot %s)", m.heistID, heistPot(m.heist)))
			}
			m.notice = errStyle.Render("cash out needs a revealed-win heist at stage ≥ 2")
			return m, nil
		case "x":
			if m.heist != nil && m.heist.Status == uint8(bindings.HeistPreStage) {
				return m.askConfirm("abandon", fmt.Sprintf("Abandon heist #%d (refund stake)", m.heistID))
			}
			m.notice = errStyle.Render("abandon only works before the first stage")
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
		m.notice = errStyle.Render("read-only: no signer configured")
		return m, nil
	}
	if m.snap.State != nil && m.snap.State.IsJailed {
		m.notice = errStyle.Render("can't travel while jailed")
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
			return false, fmt.Sprintf("infamy≥%d", bindings.BlackMarketMinInfamy)
		}
		return true, ""
	}
	if a.MinReputation != nil && a.MinReputation.Sign() > 0 {
		if st.Reputation == nil || st.Reputation.Cmp(a.MinReputation) < 0 {
			return false, "rep≥" + a.MinReputation.String()
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
			m.notice = errStyle.Render(fmt.Sprintf("can't enter %s: need %s", m.deps.AreaName(dest.AreaID), why))
			return m, nil
		}
		m.tvOpen = false
		m.submitting = true
		m.notice = statusBarStyle.Render("traveling to " + m.deps.AreaName(dest.AreaID) + "…")
		area := dest.AreaID
		name := m.deps.AreaName(area)
		return m, m.managerAction("arrived at "+name, func(ctx context.Context) error {
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
	fmt.Fprintf(&b, "%s  %s\n\n", titleStyle.Render("TRAVEL"), helpStyle.Render("you're in "+here))

	switch {
	case m.tvLoading:
		b.WriteString(statusBarStyle.Render("loading areas…\n"))
	case m.tvErr != "":
		b.WriteString(errStyle.Render("failed: "+m.tvErr) + "\n")
	case len(m.tvDests) == 0:
		b.WriteString(helpStyle.Render("no destinations available\n"))
	default:
		for i, a := range m.tvDests {
			cursor := "  "
			fee := "free"
			if a.MovementFee != nil && a.MovementFee.Sign() > 0 {
				fee = dealer.EthStr(a.MovementFee)
			}
			line := fmt.Sprintf("→ %-11s %s", m.deps.AreaName(a.AreaID), helpStyle.Render(fee))
			if ok, why := m.canEnter(a); !ok {
				line += errStyle.Render("  🔒 " + why)
			} else if a.AreaID == bindings.BlackMarketArea {
				line += helpStyle.Render(fmt.Sprintf("  infamy≥%d", bindings.BlackMarketMinInfamy))
			} else if a.MinReputation != nil && a.MinReputation.Sign() > 0 {
				line += helpStyle.Render(fmt.Sprintf("  rep≥%s", a.MinReputation))
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
	b.WriteString("\n" + helpStyle.Render("↑/↓ pick · enter travel · esc back"))
	return b.String()
}

// openHeistStart opens the start-heist form (only when no active heist).
func (m DetailModel) openHeistStart() (DetailModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render("read-only: no signer configured")
		return m, nil
	}
	if m.heistID != 0 {
		m.notice = errStyle.Render(fmt.Sprintf("already running heist #%d", m.heistID))
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
		m.notice = statusBarStyle.Render("starting heist…")
		deps, tid := m.deps, m.tokenID
		fam, diff, jp := m.hsFamily, m.hsDiff, m.hsJackpot
		return m, m.managerAction("heist started", func(ctx context.Context) error {
			_, err := deps.Manager.StartHeist(ctx, tid, fam, diff, jp)
			return err
		})
	}
	return m, nil
}

// commitStageAction pushes the heist forward (commit next stage).
func (m DetailModel) commitStageAction() (DetailModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render("read-only: no signer configured")
		return m, nil
	}
	if m.heist == nil {
		m.notice = errStyle.Render("no active heist — press h to start one")
		return m, nil
	}
	st := m.heist.Status
	if st != uint8(bindings.HeistPreStage) && st != uint8(bindings.HeistRevealedWin) {
		m.notice = errStyle.Render("can't push now (stage in progress or run ended)")
		return m, nil
	}
	m.submitting = true
	m.notice = statusBarStyle.Render("committing heist stage…")
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
		m.notice = errStyle.Render("read-only: no signer configured (set wallet.source)")
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
			m.notice = errStyle.Render("target not attackable right now")
			return m, nil
		}
		def := t.TokenID.Uint64()
		m.pvpOpen = false
		m.submitting = true
		m.notice = statusBarStyle.Render(fmt.Sprintf("attacking #%d…", def))
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
		m.notice = errStyle.Render("read-only: no signer configured")
		return m, nil
	}
	if m.snap.State != nil && !m.snap.State.IsJailed {
		m.notice = errStyle.Render("not in jail")
		return m, nil
	}
	m.submitting = true
	m.notice = statusBarStyle.Render("attempting breakout (free · ~50% · 1/day)…")
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
		m.notice = errStyle.Render("read-only: no signer configured (set wallet.source)")
		return m, nil
	}
	m.submitting = true
	m.notice = statusBarStyle.Render("removing wanted poster (spends 1 attempt · ~50% to clear heat)…")
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
		m.notice = errStyle.Render("read-only: no signer configured (set wallet.source)")
		return m, nil
	}
	m.confirm = id
	m.notice = statusBarStyle.Render(label + "? (pays a small ETH fee)  y = confirm · n = cancel")
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
			m.notice = statusBarStyle.Render("resetting attempts…")
			return m, m.managerAction("attempts reset", func(ctx context.Context) error {
				return m.deps.Manager.ResetAttempts(ctx, m.tokenID)
			})
		case "cashout":
			hid := m.heistID
			m.notice = statusBarStyle.Render("cashing out…")
			return m, m.managerAction("heist cashed out", func(ctx context.Context) error {
				return m.deps.Manager.CashOut(ctx, m.tokenID, hid)
			})
		case "abandon":
			hid := m.heistID
			m.notice = statusBarStyle.Render("abandoning heist…")
			return m, m.managerAction("heist abandoned", func(ctx context.Context) error {
				return m.deps.Manager.AbandonHeist(ctx, m.tokenID, hid)
			})
		case "bail":
			m.notice = statusBarStyle.Render("paying bail…")
			return m, m.managerAction("bailed out", func(ctx context.Context) error {
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
		m.notice = errStyle.Render("read-only: no signer configured (set wallet.source)")
		return m, nil
	}
	if h == bindings.HustleBuy && m.inBlackMarket() {
		m.notice = errStyle.Render("can't buy in the black market — only sell loot here")
		return m, nil
	}
	if len(m.areaDrugs) == 0 {
		m.notice = errStyle.Render("no market data for this area yet — press r to refresh")
		return m, nil
	}
	list := m.tradeable(h)
	if len(list) == 0 {
		if h == bindings.HustleBuy {
			m.notice = errStyle.Render("nothing to buy in " + m.deps.AreaName(areaOf(m.snap)))
		} else {
			m.notice = errStyle.Render("you hold nothing sellable in " + m.deps.AreaName(areaOf(m.snap)))
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
			m.notice = statusBarStyle.Render(fmt.Sprintf("black market: selling %d × #%d (guaranteed, no energy)…", amount, drug))
			d, a := drug, amount
			return m, m.managerAction(fmt.Sprintf("black market: sold %d × #%d", a, d), func(ctx context.Context) error {
				return m.deps.Manager.SellDrop(ctx, m.tokenID, d, a)
			})
		}
		m.notice = statusBarStyle.Render(fmt.Sprintf("committing %s drug=%d amount=%d…", hustleName(m.hustle), drug, amount))
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
		return 0, fmt.Errorf("amount must be a positive number")
	}
	if max, label, ok := m.tradeLimit(); ok && amount > max {
		return 0, fmt.Errorf("max %d units here (%s)", max, label)
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
			caps = append(caps, cap{u, "stake cap"})
		}
	}
	// Resource cap: cash for buy, holdings for sell.
	if m.hustle == bindings.HustleBuy {
		if cash := m.snap.State.CashBalance; cash != nil && price != nil && price.Sign() > 0 {
			caps = append(caps, cap{new(big.Int).Div(cash, price).Uint64(), "cash"})
		}
	} else {
		caps = append(caps, cap{m.heldBalance(sel.DrugID.Uint64()).Uint64(), "held"})
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
	fmt.Fprintf(&b, "%s\n\n", titleStyle.Render(fmt.Sprintf("Dealer #%d", m.tokenID)))

	if m.snap.Err != nil {
		b.WriteString(errStyle.Render("read error: "+m.snap.Err.Error()) + "\n")
	} else if st := m.snap.State; st != nil {
		b.WriteString(kv("Rank", st.ReputationTitle))
		b.WriteString(kv("REP", bigStr(st.Reputation)))
		b.WriteString(kv("Heat", fmt.Sprintf("%d/5", st.HeatLevel)))
		b.WriteString(kv("Cash", bigStr(st.CashBalance)))
		b.WriteString(kv("Area", m.deps.AreaName(st.CurrentArea)))
		b.WriteString(kv("Energy", fmt.Sprintf("%d/%d attempts", st.DailyAttemptsRemaining, st.MaxAttempts)))
		b.WriteString(kv("Infamy", bigStr(st.Infamy)))
		b.WriteString(kv("PVE W/T/L", fmt.Sprintf("%d/%d/%d", st.PveWins, st.PveTies, st.PveLosses)))
		b.WriteString(kv("Status", m.snap.Status()))
		b.WriteString("\n" + sectionStyle.Render("Stash") + "\n")
		if len(st.DrugBalances) == 0 {
			b.WriteString(helpStyle.Render("  (empty)\n"))
		}
		for _, d := range st.DrugBalances {
			if d.Balance != nil && d.Balance.Sign() > 0 {
				fmt.Fprintf(&b, "  #%s %-10s ×%s\n", d.DrugID, truncate(d.Name, 10), d.Balance)
			}
		}
	}

	// Open rounds.
	b.WriteString("\n" + sectionStyle.Render("Pending") + "\n")
	if len(m.pending) == 0 {
		b.WriteString(helpStyle.Render("  none\n"))
	}
	for _, p := range m.pending {
		fmt.Fprintf(&b, "  seq %d · %s · reveal@%d expiry@%d %s\n",
			p.Seq, p.Kind, p.RevealBlock, p.ExpiryBlock, statusBarStyle.Render("(resolving)"))
	}

	// Heist.
	b.WriteString("\n" + sectionStyle.Render("Heist") + "\n")
	if m.hsOpen {
		b.WriteString(m.heistStartView())
	} else if m.heist == nil {
		b.WriteString(helpStyle.Render("  none · h to start (needs REP ≥ 600)\n"))
	} else {
		h := m.heist
		fmt.Fprintf(&b, "  #%d %s difficulty=%d · stage %d · %s · pot %s%s\n",
			m.heistID, bindings.HeistFamily(h.Family), h.Difficulty, h.CurrentStage,
			bindings.HeistStatus(h.Status), bigStr(h.CurrentPot), jackpotFlag(h.EthJackpot))
		b.WriteString(helpStyle.Render("  " + heistActionsHint(h) + "\n"))
	}

	// Recent log.
	b.WriteString("\n" + sectionStyle.Render("Recent") + "\n")
	if len(m.log) == 0 {
		b.WriteString(helpStyle.Render("  no activity yet\n"))
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
		hint := "b buy · s sell · p pvp · t travel · h heist · c clear-heat · a reset-attempts · r refresh · esc back"
		if m.deps.Manager == nil {
			hint = "read-only · r refresh · esc back"
		} else if m.snap.State != nil && m.snap.State.IsJailed {
			hint = "JAILED · k breakout (free ~50% · 1/day) · l bail (ETH, instant) · r refresh · esc back"
		} else if m.inBlackMarket() {
			hint = "BLACK MARKET · s sell loot (guaranteed, no energy) · t travel · r refresh · esc back"
		}
		b.WriteString(helpStyle.Render(hint))
	}
	return b.String()
}

// heistStartView renders the start-heist form.
func (m DetailModel) heistStartView() string {
	fam := "SUPPLY"
	if m.hsFamily == bindings.FamilyCash {
		fam = "CASH"
	}
	diffNames := []string{"easy (rep 600)", "medium (rep 1500)", "hard (rep 5500)"}
	jp := "off"
	if m.hsJackpot {
		jp = "on (+0.001 ETH)"
	}
	rows := []struct{ label, val string }{
		{"Family", fam},
		{"Difficulty", diffNames[m.hsDiff]},
		{"ETH jackpot", jp},
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
	b.WriteString(helpStyle.Render("  ↑/↓ field · ←/→/space change · enter start · esc cancel\n"))
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
		return " · 🎰jackpot"
	}
	return ""
}

// heistActionsHint lists the actions available for the current heist status.
func heistActionsHint(h *bindings.DailyHeist) string {
	switch bindings.HeistStatus(h.Status) {
	case bindings.HeistPreStage:
		return "g push (start stage 1) · x abandon"
	case bindings.HeistRevealedWin:
		if h.CurrentStage >= 2 {
			return "g push deeper · o cash out"
		}
		return "g push deeper (cash out unlocks at stage 2)"
	case bindings.HeistCommitted:
		return "resolving stage in background…"
	default:
		return "run ended — h to start a new heist"
	}
}

// pvpView renders the target browser.
func (m DetailModel) pvpView() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", titleStyle.Render(fmt.Sprintf("PVP TARGETS — attacker #%d", m.tokenID)))

	switch {
	case m.pvpLoading:
		b.WriteString(statusBarStyle.Render("scanning your area…\n"))
	case m.pvpErr != "":
		b.WriteString(errStyle.Render("scan failed: "+m.pvpErr) + "\n")
	case len(m.targets) == 0:
		b.WriteString(helpStyle.Render("no attackable targets here.\nPVP needs REP ≥ 200 and another dealer in the same area.\n"))
	default:
		for i, t := range m.targets {
			cursor := "  "
			line := fmt.Sprintf("#%-4s rep=%-6s win=%s%% infamy=%s",
				t.TokenID, bigStr(t.Reputation), bigStr(t.WinChance), bigStr(t.Infamy))
			if !t.CanAttackNow {
				line += helpStyle.Render("  [unavailable]")
			}
			if i == m.targetIdx {
				cursor = focusStyle.Render("▶ ")
				line = focusStyle.Render(line)
			}
			b.WriteString("  " + cursor + line + "\n")
		}
	}

	if m.alliesHidden > 0 {
		b.WriteString(helpStyle.Render(fmt.Sprintf("\n  (%d ally target(s) hidden)\n", m.alliesHidden)))
	}
	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("↑/↓ pick target · enter attack · esc back"))
	return b.String()
}

func (m DetailModel) formView() string {
	sel := m.drugs[m.drugIdx]
	price := sel.BuyPrice
	plabel := "buy"
	if m.hustle == bindings.HustleSell {
		price = sel.SellPrice
		plabel = "sell"
	}
	bal := m.heldBalance(sel.DrugID.Uint64())
	drugLine := fmt.Sprintf("  Drug:   %s #%s %s  %s",
		focusStyle.Render("↑/↓"),
		sel.DrugID, sel.Name,
		helpStyle.Render(fmt.Sprintf("(%s @%s · you have ×%s · %d/%d)", plabel, bigStr(price), bal, m.drugIdx+1, len(m.drugs))))
	amtLine := "  Amount: " + m.amount.View()
	if max, label, ok := m.tradeLimit(); ok {
		amtLine += "  " + helpStyle.Render(fmt.Sprintf("max %d (%s)", max, label))
	}

	return fmt.Sprintf("%s %s\n%s\n%s\n%s",
		titleStyle.Render(strings.ToUpper(hustleName(m.hustle))),
		helpStyle.Render("choice: deal — odds fixed"),
		drugLine, amtLine,
		helpStyle.Render("↑/↓ pick drug · type amount · enter submit · esc cancel"))
}

func kv(k, v string) string {
	return fmt.Sprintf("  %-10s %s\n", k+":", v)
}

func hustleName(h bindings.HustleType) string {
	if h == bindings.HustleSell {
		return "sell"
	}
	return "buy"
}
