package tui

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"dealers/internal/dealer"
	"dealers/internal/store"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// fleetLogLines is how many fleet-wide activity entries show under the roster.
const fleetLogLines = 6

// card geometry: 4 content lines + rounded border = 6 rows.
const (
	cardContentRows = 4
	cardHeight      = cardContentRows + 2
	cardMaxWidth    = 68
	cardMinWidth    = 40
	colGap          = 2
	maxCols         = 4
)

// FleetModel is the fleet dashboard (FR3), rendered as a responsive grid of
// lipgloss dealer cards (columns adapt to the terminal width). It is a sub-model
// of App, which owns the refresh ticker and screen switching.
type FleetModel struct {
	deps     Deps
	snaps    []dealer.Snapshot
	fleetLog []store.FleetLogRow
	cursor   int // selected snapshot index
	topRow   int // first visible grid row (scroll offset)
	width    int
	vpRows   int // visible grid rows (from window height)

	loading    bool
	refreshing bool // a Refresh is in flight (guards fast-poll overlap)
	lastAt     time.Time
	lastErr    string
	notice     string
}

// snapshotsMsg carries a completed fleet refresh.
type snapshotsMsg struct {
	snaps    []dealer.Snapshot
	balance  *big.Int
	at       time.Time
	fleetLog []store.FleetLogRow
}

// NewFleet builds the fleet sub-model.
func NewFleet(deps Deps) FleetModel {
	return FleetModel{deps: deps, loading: true, vpRows: 2}
}

// Refresh fetches all dealer states (and the owner balance for alerts) off the
// UI goroutine.
func (m FleetModel) Refresh() tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		now := time.Now()
		msg := snapshotsMsg{snaps: dealer.FetchAll(ctx, deps.Reader, deps.IDs, now), at: now}
		if deps.BalanceFn != nil {
			msg.balance, _ = deps.BalanceFn(ctx)
		}
		if deps.Store != nil {
			msg.fleetLog, _ = deps.Store.RecentFleetActions(fleetLogLines)
		}
		return msg
	}
}

// cols/cardWidth compute the responsive column count and per-card width from the
// current terminal width.
func (m FleetModel) cols() int {
	if m.width <= 0 {
		return 1
	}
	c := (m.width + colGap) / (cardMinWidth + colGap)
	if c < 1 {
		c = 1
	}
	if c > maxCols {
		c = maxCols
	}
	return c
}

func (m FleetModel) cardWidth() int {
	c := m.cols()
	w := (m.width - (c-1)*colGap) / c
	if w > cardMaxWidth {
		w = cardMaxWidth
	}
	if w < cardMinWidth {
		w = cardMinWidth
	}
	return w
}

// SelectedTokenID returns the token id under the cursor (0 if none).
func (m FleetModel) SelectedTokenID() uint64 {
	if m.cursor < 0 || m.cursor >= len(m.snaps) {
		return 0
	}
	return m.snaps[m.cursor].TokenID
}

// SnapshotFor returns the cached snapshot for a token id, if loaded.
func (m FleetModel) SnapshotFor(tokenID uint64) (dealer.Snapshot, bool) {
	for _, s := range m.snaps {
		if s.TokenID == tokenID {
			return s, true
		}
	}
	return dealer.Snapshot{}, false
}

func (m FleetModel) Update(msg tea.Msg) (FleetModel, tea.Cmd) {
	switch msg := msg.(type) {
	case snapshotsMsg:
		m.loading = false
		m.refreshing = false
		m.snaps = msg.snaps
		m.fleetLog = msg.fleetLog
		m.lastAt = msg.at
		m.lastErr = firstErr(msg.snaps)
		m.clampCursor()
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width - 1
		rowH := cardHeight + 1 // card + inter-row gap
		avail := msg.Height - 7 - (fleetLogLines + 2)
		m.vpRows = avail / rowH
		if m.vpRows < 1 {
			m.vpRows = 1
		}
		m.ensureVisible()
	case tea.KeyMsg:
		switch msg.String() {
		case "down", "j":
			m.move(m.cols())
		case "up", "k":
			m.move(-m.cols())
		case "right", "l":
			m.move(1)
		case "left", "h":
			m.move(-1)
		case "pgdown":
			m.move(m.cols() * m.vpRows)
		case "pgup":
			m.move(-m.cols() * m.vpRows)
		case "home", "g":
			m.move(-len(m.snaps))
		case "end", "G":
			m.move(len(m.snaps))
		}
	}
	return m, nil
}

func (m *FleetModel) move(delta int) {
	m.cursor += delta
	m.clampCursor()
	m.ensureVisible()
}

func (m *FleetModel) clampCursor() {
	if m.cursor >= len(m.snaps) {
		m.cursor = len(m.snaps) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *FleetModel) ensureVisible() {
	c := m.cols()
	row := m.cursor / c
	if row < m.topRow {
		m.topRow = row
	}
	if row >= m.topRow+m.vpRows {
		m.topRow = row - m.vpRows + 1
	}
	if m.topRow < 0 {
		m.topRow = 0
	}
}

func (m FleetModel) View() string {
	status := fmt.Sprintf("%d dealers", len(m.deps.IDs))
	if m.loading {
		status += " · refreshing…"
	} else if !m.lastAt.IsZero() {
		status += " · updated " + m.lastAt.Format("15:04:05")
	}
	statusLine := statusBarStyle.Render(status)
	if m.lastErr != "" {
		statusLine += "  " + errStyle.Render("⚠ "+m.lastErr)
	}

	hint := "↑↓←→ · enter · n missions · c check-in · s strategy · e steps · m market · f allies · o settings · r refresh · q quit"
	if m.deps.Manager == nil {
		hint = "read-only · ↑↓←→ · enter · n missions · e steps · m market · f allies · o settings · r refresh · q quit"
	} else if m.deps.ToggleAutopilot != nil {
		hint = "↑↓←→ · enter · n missions · c check-in · s strategy · e steps · A auto · m market · f allies · o settings · r refresh · q"
	}

	lines := []string{m.gridView(), statusLine}
	if m.notice != "" {
		lines = append(lines, m.notice)
	}
	if log := m.fleetLogView(); log != "" {
		lines = append(lines, log)
	}
	lines = append(lines, helpStyle.Render(hint))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// gridView lays the cards out in the responsive column grid, windowed to the
// visible rows.
func (m FleetModel) gridView() string {
	if len(m.snaps) == 0 {
		if m.loading {
			return helpStyle.Render("loading…")
		}
		return helpStyle.Render("no dealers")
	}
	c := m.cols()
	cw := m.cardWidth()
	totalRows := (len(m.snaps) + c - 1) / c
	end := m.topRow + m.vpRows
	if end > totalRows {
		end = totalRows
	}

	var rows []string
	if m.topRow > 0 {
		rows = append(rows, helpStyle.Render(fmt.Sprintf("  ↑ %d more", m.topRow*c)))
	}
	for r := m.topRow; r < end; r++ {
		var cards []string
		for col := 0; col < c; col++ {
			i := r*c + col
			if i >= len(m.snaps) {
				break
			}
			card := renderCard(m.deps, m.snaps[i], i == m.cursor, cw)
			if col < c-1 {
				card = lipgloss.NewStyle().MarginRight(colGap).Render(card)
			}
			cards = append(cards, card)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	}
	if end < totalRows {
		rows = append(rows, helpStyle.Render(fmt.Sprintf("  ↓ %d more", len(m.snaps)-end*c)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// fleetLogView renders the recent fleet-wide activity feed.
func (m FleetModel) fleetLogView() string {
	if len(m.fleetLog) == 0 {
		return ""
	}
	lines := []string{sectionStyle.Render("Activity")}
	for _, l := range m.fleetLog {
		lines = append(lines, fmt.Sprintf("  %s %s",
			helpStyle.Render(fmt.Sprintf("#%d", l.TokenID)), colorizeLog(l.Summary)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

var (
	cardBorderIdle = lipgloss.Color("240")
	cardBorderSel  = lipgloss.Color("62")
	labelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// renderCard draws one dealer as a bordered lipgloss card. The selected card gets
// a bright border and a bold, accented title.
func renderCard(deps Deps, s dealer.Snapshot, selected bool, width int) string {
	// Card total width = `width`: border adds 2, padding adds 2, leaving the text
	// area `inner`. Content lines are padded to `inner`; the box Width is set to
	// inner+padding so lines fill exactly without wrapping.
	inner := width - 4
	if inner < 24 {
		inner = 24
	}
	pve, pvp := deps.Ranks(s.TokenID)

	// Title line: #id · title · <status chip> (chip right-aligned).
	title := "—"
	if s.State != nil {
		title = s.State.ReputationTitle
	}
	idPart := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")).Render("#" + u64(s.TokenID))
	left := idPart + labelStyle.Render(" · ") + truncate(title, 16)
	chip := statusChip(s)
	titleLine := fitBetween(left, chip, inner)

	var body []string
	if s.Err != nil || s.State == nil {
		body = []string{
			errStyle.Render(truncate("read error: "+errText(s), inner)),
			labelStyle.Render("no data"),
			"",
		}
	} else {
		st := s.State
		body = []string{
			fmt.Sprintf("%s %s %s   %s %s %s   %s",
				labelStyle.Render("REP"), valStyle(bigStr(st.Reputation)), rankTag(pve),
				labelStyle.Render("INF"), valStyle(bigStr(st.Infamy)), rankTag(pvp),
				labelStyle.Render("$")+valStyle(bigStr(st.CashBalance))),
			fmt.Sprintf("%s %s   %s %s   %s",
				labelStyle.Render("Energy"), meter(int(st.DailyAttemptsRemaining), int(st.MaxAttempts), lipgloss.Color("42"))+" "+dimNum(st.DailyAttemptsRemaining, st.MaxAttempts),
				labelStyle.Render("Heat"), meter(int(st.HeatLevel), 5, heatMeterColor(st.HeatLevel))+fmt.Sprintf(" %d/5", st.HeatLevel),
				labelStyle.Render("@")+deps.AreaName(st.CurrentArea)),
			fmt.Sprintf("%s %s   %s %s   %s %s",
				labelStyle.Render("check-in"), chkColor(s).Render(checkInCell(s)),
				labelStyle.Render("missions"), missColor(s).Render(missionLabel(s)),
				labelStyle.Render("auto"), autoStyle.Render(deps.StrategyTag(s.TokenID))),
		}
	}
	// Pad body to a fixed number of content rows so every card is the same height.
	for len(body) < cardContentRows-1 {
		body = append(body, "")
	}
	// Hard-clip each line to the inner width so nothing wraps — a wrapped line
	// would make the card taller than cardHeight and desync the grid layout.
	lines := append([]string{titleLine}, body...)
	for i := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(inner).Render(lines[i])
	}
	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	border := cardBorderIdle
	if selected {
		border = cardBorderSel
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(inner + 2). // content area = text (inner) + horizontal padding (2)
		Render(content)
}

// --- small render helpers ---

var (
	valStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render
	autoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
)

// statusChip renders a coloured status badge.
func statusChip(s dealer.Snapshot) string {
	txt := s.Status()
	st := lipgloss.NewStyle().Padding(0, 1).Bold(true)
	switch txt {
	case "JAILED", "ERR":
		st = st.Background(lipgloss.Color("124")).Foreground(lipgloss.Color("231"))
	case "SAFEHOUSE", "UNINIT":
		st = st.Background(lipgloss.Color("94")).Foreground(lipgloss.Color("231"))
	default:
		st = st.Background(lipgloss.Color("22")).Foreground(lipgloss.Color("231"))
	}
	return st.Render(txt)
}

// fitBetween places left and right on one line inner cells wide, right-aligned.
func fitBetween(left, right string, inner int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	gap := inner - lw - rw
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// meter renders a fixed-width bar (▰ filled / ▱ empty).
func meter(filled, total int, on lipgloss.Color) string {
	const w = 5
	if total <= 0 {
		total = 1
	}
	n := filled * w / total
	if n > w {
		n = w
	}
	if n < 0 {
		n = 0
	}
	return lipgloss.NewStyle().Foreground(on).Render(strings.Repeat("▰", n)) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("▱", w-n))
}

func heatMeterColor(level uint8) lipgloss.Color {
	switch {
	case level >= 5:
		return lipgloss.Color("203")
	case level >= 3:
		return lipgloss.Color("214")
	default:
		return lipgloss.Color("42")
	}
}

func dimNum(a, b uint8) string { return fmt.Sprintf("%d/%d", a, b) }

func rankTag(pos int) string {
	if pos <= 0 {
		return labelStyle.Render("#-")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(fmt.Sprintf("#%d", pos))
}

func errText(s dealer.Snapshot) string {
	if s.Err != nil {
		return s.Err.Error()
	}
	return "uninitialised"
}

// checkInCell renders the daily check-in status glyph.
func checkInCell(s dealer.Snapshot) string {
	if s.CheckedIn == nil {
		return "-"
	}
	if *s.CheckedIn {
		return "✓"
	}
	return "○"
}

// missionCell / missionLabel render the mission indicator.
func missionCell(s dealer.Snapshot) string {
	if !s.MissionsKnown {
		return "-"
	}
	if s.MissionsClaimable > 0 {
		return fmt.Sprintf("★%d", s.MissionsClaimable)
	}
	if s.MissionsNeedCheckIn {
		return "○"
	}
	return "✓"
}

func missionLabel(s dealer.Snapshot) string {
	switch {
	case !s.MissionsKnown:
		return "-"
	case s.MissionsClaimable > 0:
		return fmt.Sprintf("★%d claimable", s.MissionsClaimable)
	case s.MissionsNeedCheckIn:
		return "○ accept"
	default:
		return "✓ up to date"
	}
}

func chkColor(s dealer.Snapshot) lipgloss.Style {
	if s.CheckedIn != nil && *s.CheckedIn {
		return posStyle
	}
	if s.CheckedIn != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	}
	return helpStyle
}

func missColor(s dealer.Snapshot) lipgloss.Style {
	switch {
	case !s.MissionsKnown:
		return helpStyle
	case s.MissionsClaimable > 0:
		return posStyle
	case s.MissionsNeedCheckIn:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	default:
		return helpStyle
	}
}

func firstErr(snaps []dealer.Snapshot) string {
	for _, s := range snaps {
		if s.Err != nil {
			return fmt.Sprintf("dealer %d: %v", s.TokenID, s.Err)
		}
	}
	return ""
}

func bigStr(v *big.Int) string {
	if v == nil {
		return "-"
	}
	return v.String()
}

func u64(v uint64) string { return fmt.Sprintf("%d", v) }

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
