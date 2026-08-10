package tui

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"dealers/internal/allies"
	"dealers/internal/autostrat"
	"dealers/internal/chain/bindings"
	"dealers/internal/dealer"
	"dealers/internal/recipe"
	"dealers/internal/settings"
	"dealers/internal/store"
	"dealers/internal/template"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ethereum/go-ethereum/common"
)

// Deps is everything the UI needs. Manager and Store are nil in read-only mode
// (no signer configured) — the UI then disables actions.
type Deps struct {
	Reader  *bindings.Reader
	Manager *dealer.Manager
	Store   *store.Store
	IDs     []uint64
	Net     string
	Owner   common.Address
	Poll    time.Duration
	UI      *UIState // shared session UI state (remembered selections); may be nil

	// Phase 4 guardrails (all optional / nil in read-only).
	BalanceFn    func(context.Context) (*big.Int, error) // owner ETH balance
	MinRunwayWei *big.Int                                // low-runway alert threshold
	SpentFn      func() *big.Int                         // cumulative session ETH spend
	AreaNames    map[uint8]string                        // area id → name (FR12 cache)

	// Phase 5 autopilot (nil in read-only).
	AutopilotOn     func() bool      // current enabled state
	ToggleAutopilot func() bool      // flip and return the new state
	Strategies      *autostrat.Store // per-dealer autopilot policy (UI-managed); nil in read-only

	// Leaderboard positions (PvE by rep, PvP by infamy); nil if not computed.
	Leaderboard *dealer.LeaderboardCache

	// PVE stake-config params for the per-action trade limit; nil if unavailable.
	StakeParams *bindings.PVEStakeParams

	// Allies is the do-not-attack set (own fleet + user list); nil = none.
	Allies *allies.Allies

	// Settings holds UI-managed global toggles (e.g. auto-pay bail); nil = none.
	Settings *settings.Store

	// Recipe is the UI-editable autopilot step order (global fallback); nil = none.
	Recipe *recipe.Store

	// Templates is the named-preset store (per-NFT strategy templates); nil = none.
	Templates *template.Store

	// Version is the running build version (main.version), shown in the header.
	Version string

	// UpdateCheck queries GitHub once on startup for a newer release. It returns
	// the newer version tag and its release-page URL, or ("","") when up to date,
	// disabled, or the check failed (best-effort). nil = feature off.
	UpdateCheck func(context.Context) (latest, url string)
}

// IsAlly reports whether a dealer is on the do-not-attack list.
func (d Deps) IsAlly(id uint64) bool { return d.Allies != nil && d.Allies.IsAlly(id) }

// Ranks returns a dealer's PvE/PvP leaderboard positions (0 = unknown).
func (d Deps) Ranks(id uint64) (pve, pvp int) {
	if d.Leaderboard == nil {
		return 0, 0
	}
	if r, ok := d.Leaderboard.Get(id); ok {
		return r.Pve, r.Pvp
	}
	return 0, 0
}

// StrategyTag returns a dealer's autopilot policy tag ("-" if none/read-only).
func (d Deps) StrategyTag(id uint64) string {
	if d.Strategies == nil {
		return "-"
	}
	return d.Strategies.Get(id)
}

// strategyChip summarizes the fleet's policies for the header: the shared tag, or
// "mixed" when dealers differ. Empty when there's no autopilot.
func (d Deps) strategyChip() string {
	return strategySummary(d.IDs, d.Strategies)
}

// strategySummary returns the single shared tag over ids, or "mixed" when they
// differ ("" when no store).
func strategySummary(ids []uint64, s *autostrat.Store) string {
	if s == nil || len(ids) == 0 {
		return ""
	}
	first := s.Get(ids[0])
	for _, id := range ids {
		if s.Get(id) != first {
			return "mixed"
		}
	}
	return first
}

// AreaName renders an area id as its name if known, else the number.
func (d Deps) AreaName(id uint8) string {
	if n, ok := d.AreaNames[id]; ok && n != "" {
		return n
	}
	return fmt.Sprintf("%d", id)
}

// UIState holds selections remembered across screens for the session. Drug ids
// are global to the game, so one remembered id serves every dealer.
type UIState struct {
	LastDrugID uint64
}

// lastDrugID returns the remembered drug id (0 if none / no state).
func (d Deps) lastDrugID() uint64 {
	if d.UI == nil {
		return 0
	}
	return d.UI.LastDrugID
}

// setLastDrugID records the drug id for next time.
func (d Deps) setLastDrugID(id uint64) {
	if d.UI != nil {
		d.UI.LastDrugID = id
	}
}

type screen int

const (
	screenFleet screen = iota
	screenDetail
	screenMarket
	screenAllies
	screenMissions
	screenSettings
	screenSteps
	screenTemplates
)

// App is the root Elm model. It multiplexes the fleet and detail sub-models
// (ADR-4) and owns the single refresh ticker.
type App struct {
	deps     Deps
	screen   screen
	fleet    FleetModel
	detail   DetailModel
	market   MarketModel
	allies   AlliesModel
	missions MissionsModel
	settings SettingsModel
	steps    StepsModel
	tmpls    TemplatesModel
	alerts   []dealer.Alert
	balance  *big.Int
	update   updateInfo // populated once if a newer release is found
}

// updateInfo carries the result of the startup GitHub release check.
type updateInfo struct {
	latest string // newer version tag, e.g. "v0.2.0" ("" = up to date / unknown)
	url    string // release-page URL
}

// updateMsg delivers the release-check result into the Elm loop.
type updateMsg updateInfo

// tickMsg drives periodic refresh of the active screen.
type tickMsg time.Time

// backToFleetMsg is emitted by the detail screen to pop back to the fleet.
type backToFleetMsg struct{}

func tickAfter(d time.Duration) tea.Cmd {
	if d <= 0 {
		d = 15 * time.Second
	}
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// refreshInterval speeds the poll to 1s while the autopilot is acting (so its
// actions and the activity log stream in live), else uses the configured poll.
func (a App) refreshInterval() time.Duration {
	if a.deps.AutopilotOn != nil && a.deps.AutopilotOn() {
		return time.Second
	}
	return a.deps.Poll
}

// NewApp builds the root model.
func NewApp(deps Deps) App {
	return App{deps: deps, fleet: NewFleet(deps)}
}

func (a App) Init() tea.Cmd {
	return tea.Batch(a.fleet.Refresh(), tickAfter(a.deps.Poll), a.checkUpdateCmd())
}

// checkUpdateCmd runs the GitHub release check once, off the UI goroutine, and
// yields an updateMsg only when a newer version is available. Returns nil (a
// no-op for tea.Batch) when the feature is disabled.
func (a App) checkUpdateCmd() tea.Cmd {
	fn := a.deps.UpdateCheck
	if fn == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		latest, url := fn(ctx)
		if latest == "" {
			return nil // up to date, disabled, or check failed — say nothing
		}
		return updateMsg{latest: latest, url: url}
	}
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		// Translate Russian-layout runes to their Latin equivalents so shortcuts
		// work under any keyboard layout.
		msg = normalizeKey(msg)
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		switch a.screen {
		case screenFleet:
			return a.updateFleetKey(msg)
		case screenMarket:
			var cmd tea.Cmd
			a.market, cmd = a.market.Update(msg)
			return a, cmd
		case screenAllies:
			var cmd tea.Cmd
			a.allies, cmd = a.allies.Update(msg)
			return a, cmd
		case screenMissions:
			var cmd tea.Cmd
			a.missions, cmd = a.missions.Update(msg)
			return a, cmd
		case screenSettings:
			var cmd tea.Cmd
			a.settings, cmd = a.settings.Update(msg)
			return a, cmd
		case screenSteps:
			var cmd tea.Cmd
			a.steps, cmd = a.steps.Update(msg)
			return a, cmd
		case screenTemplates:
			var cmd tea.Cmd
			a.tmpls, cmd = a.tmpls.Update(msg)
			return a, cmd
		default:
			var cmd tea.Cmd
			a.detail, cmd = a.detail.Update(msg)
			return a, cmd
		}

	case updateMsg:
		a.update = updateInfo(msg)
		return a, nil

	case backToFleetMsg:
		a.screen = screenFleet
		return a, a.fleet.Refresh()

	case marketDataMsg:
		var cmd tea.Cmd
		a.market, cmd = a.market.Update(msg)
		return a, cmd

	case tickMsg:
		var cmd tea.Cmd
		switch a.screen {
		case screenFleet:
			// Skip if a refresh is still running (fast 1s poll must not stack
			// overlapping fleet fetches and pile onto the RPC rate limit).
			if !a.fleet.refreshing {
				a.fleet.refreshing = true
				cmd = a.fleet.Refresh()
			}
		case screenDetail:
			cmd = a.detail.Refresh()
		case screenMarket, screenAllies:
			cmd = nil // static screens; no periodic refresh
		}
		return a, tea.Batch(cmd, tickAfter(a.refreshInterval()))

	case checkInDoneMsg:
		a.fleet.notice = checkInSummary(msg.results)
		// Flip the Chk column now instead of waiting for the cache TTL.
		if a.deps.Reader != nil {
			a.deps.Reader.InvalidateCheckins()
		}
		return a, a.fleet.Refresh()

	case snapshotsMsg:
		a.balance = msg.balance
		a.alerts = dealer.FleetAlerts(msg.snaps, msg.balance, a.deps.MinRunwayWei)
		var cmd tea.Cmd
		a.fleet, cmd = a.fleet.Update(msg)
		return a, cmd

	case tea.WindowSizeMsg:
		a.fleet, _ = a.fleet.Update(msg)
		a.detail, _ = a.detail.Update(msg)
		return a, nil

	default:
		// Screen-owned async messages (data loads, action results).
		switch a.screen {
		case screenDetail:
			var cmd tea.Cmd
			a.detail, cmd = a.detail.Update(msg)
			return a, cmd
		case screenMissions:
			var cmd tea.Cmd
			a.missions, cmd = a.missions.Update(msg)
			return a, cmd
		}
		var cmd tea.Cmd
		a.fleet, cmd = a.fleet.Update(msg)
		return a, cmd
	}
}

func (a App) updateFleetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return a, tea.Quit
	case "r":
		return a, a.fleet.Refresh()
	case "m":
		a.market = NewMarket(a.deps)
		a.screen = screenMarket
		return a, a.market.Init()
	case "f":
		a.allies = NewAllies(a.deps)
		a.screen = screenAllies
		return a, a.allies.Init()
	case "n":
		// Missions for the selected dealer (display + manual accept/claim).
		tid := a.fleet.SelectedTokenID()
		if tid == 0 {
			return a, nil
		}
		a.missions = NewMissions(a.deps, tid)
		a.screen = screenMissions
		return a, a.missions.Init()
	case "o":
		// Settings (global toggles).
		a.settings = NewSettings(a.deps)
		a.screen = screenSettings
		return a, a.settings.Init()
	case "e":
		// Autopilot step editor (global recipe order/enable).
		a.steps = NewSteps(a.deps)
		a.screen = screenSteps
		return a, a.steps.Init()
	case "t":
		// Strategy templates (named presets: base strategy + steps + params).
		if a.deps.Templates == nil {
			a.fleet.notice = errStyle.Render("read-only — templates unavailable")
			return a, nil
		}
		a.tmpls = NewTemplates(a.deps)
		a.screen = screenTemplates
		return a, a.tmpls.Init()
	case "c":
		// Daily check-in for the whole fleet (gas only; skips jailed / already-done).
		if a.deps.Manager == nil {
			a.fleet.notice = errStyle.Render("read-only — no signer configured for check-in")
			return a, nil
		}
		a.fleet.notice = statusBarStyle.Render("checking in…")
		return a, checkInAllCmd(a.deps, a.fleet.snaps)
	case "s":
		// Cycle the selected dealer's autopilot strategy (farm→pve→pvp→manual).
		if a.deps.Strategies == nil {
			a.fleet.notice = errStyle.Render("read-only — autopilot strategy unavailable")
			return a, nil
		}
		tid := a.fleet.SelectedTokenID()
		if tid == 0 {
			return a, nil
		}
		next, err := a.deps.Strategies.Cycle(tid)
		if err != nil {
			a.fleet.notice = errStyle.Render("strategy save failed: " + err.Error())
			return a, nil
		}
		a.fleet.notice = okStyle.Render(fmt.Sprintf("#%d strategy → %s", tid, next))
		return a, nil
	case "A":
		// Toggle the autopilot (capital A to avoid an accidental spend).
		if a.deps.ToggleAutopilot != nil {
			on := a.deps.ToggleAutopilot()
			a.fleet.notice = autopilotNotice(on)
		}
		return a, nil
	case "enter":
		tid := a.fleet.SelectedTokenID()
		if tid == 0 {
			return a, nil
		}
		snap, _ := a.fleet.SnapshotFor(tid)
		a.detail = NewDetail(a.deps, tid, snap)
		a.screen = screenDetail
		return a, a.detail.Init()
	default:
		var cmd tea.Cmd
		a.fleet, cmd = a.fleet.Update(msg)
		return a, cmd
	}
}

func (a App) View() string {
	netStyle := networkTestnet
	if a.deps.Net == "mainnet" {
		netStyle = networkMainnet
	}
	header := lipgloss.JoinHorizontal(lipgloss.Center,
		appTitle(),
		"  ", netStyle.Render(a.deps.Net),
		"  ", statusBarStyle.Render(shortAddr(a.deps.Owner)),
		"  ", statusBarStyle.Render(a.walletLine()),
		"  ", a.autopilotChip(),
	)

	parts := []string{header}
	if bar := a.updateBar(); bar != "" {
		parts = append(parts, bar)
	}
	if bar := a.alertBar(); bar != "" {
		parts = append(parts, bar)
	}

	var body string
	switch a.screen {
	case screenFleet:
		body = a.fleet.View()
	case screenMarket:
		body = a.market.View()
	case screenAllies:
		body = a.allies.View()
	case screenMissions:
		body = a.missions.View()
	case screenSettings:
		body = a.settings.View()
	case screenSteps:
		body = a.steps.View()
	case screenTemplates:
		body = a.tmpls.View()
	default:
		body = a.detail.View()
	}
	parts = append(parts, "", body)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// walletLine shows the owner ETH balance and cumulative session spend.
func (a App) walletLine() string {
	s := ""
	if a.balance != nil {
		s = "bal " + dealer.EthStr(a.balance)
	}
	if a.deps.SpentFn != nil {
		if spent := a.deps.SpentFn(); spent != nil && spent.Sign() > 0 {
			s += "  spent " + dealer.EthStr(spent)
		}
	}
	return s
}

// updateBar renders the "new version available" notice once the startup GitHub
// check finds a newer release. The version is an OSC 8 hyperlink to the release
// page (clickable in supporting terminals). Empty until/unless an update exists.
func (a App) updateBar() string {
	if a.update.latest == "" {
		return ""
	}
	ver := a.update.latest
	if a.update.url != "" {
		ver = osc8(a.update.url, a.update.latest)
	}
	line := updateNoticeStyle.Render(" ⬆ update available: " + ver + " ")
	if a.deps.Version != "" {
		line += helpStyle.Render(" (you have " + a.deps.Version + ")")
	}
	return line
}

// alertBar renders the persistent alerts overlay (FR10). Empty when all clear.
func (a App) alertBar() string {
	if len(a.alerts) == 0 {
		return ""
	}
	var chips []string
	for _, al := range a.alerts {
		st := alertWarnStyle
		if al.Level == dealer.AlertCrit {
			st = alertCritStyle
		}
		chips = append(chips, st.Render(" "+al.Text+" "))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, chips...)
}

// autopilotChip renders the AUTO ON/OFF indicator (empty in read-only).
func (a App) autopilotChip() string {
	if a.deps.AutopilotOn == nil {
		return ""
	}
	tag := a.deps.strategyChip()
	if a.deps.AutopilotOn() {
		s := " AUTO ON "
		if tag != "" {
			s = " AUTO:" + tag + " ON "
		}
		return alertCritStyle.Render(s)
	}
	if tag != "" {
		return statusBarStyle.Render("auto:" + tag + " off")
	}
	return statusBarStyle.Render("auto off")
}

func autopilotNotice(on bool) string {
	if on {
		return alertCritStyle.Render(" AUTOPILOT ON ") + errStyle.Render(" — acting on its own, spending energy/cash")
	}
	return okStyle.Render("autopilot off")
}

// dezeterURL is the author's Abstract portal profile, linked from the title.
const dezeterURL = "https://portal.abs.xyz/profile/0xEd4234a5f233B5E642D47caff292bdc0591D5656"

// appTitle renders the always-visible header badge "Dealers Manager by Dezeter",
// where "Dezeter" is an OSC 8 terminal hyperlink (clickable in Windows Terminal,
// iTerm2, WezTerm, etc.; plain underlined text in terminals without support).
func appTitle() string {
	return titleStyle.Render("Dealers Manager by " + osc8(dezeterURL, "Dezeter"))
}

// osc8 wraps text in an OSC 8 hyperlink escape sequence.
func osc8(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func shortAddr(x common.Address) string {
	h := x.Hex()
	if len(h) < 12 {
		return h
	}
	return h[:6] + "…" + h[len(h)-4:]
}
