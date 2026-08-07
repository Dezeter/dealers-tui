package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dealers/internal/chain/bindings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ethereum/go-ethereum/common"
)

// missionBar is the shared progress-bar renderer (ViewAs is a value receiver, so
// one configured model renders every mission's bar).
var missionBar = progress.New(
	progress.WithWidth(22),
	progress.WithoutPercentage(),
	progress.WithGradient("#3a7d44", "#5ee06e"),
)

// MissionsModel is the per-dealer missions screen: it shows daily/weekly
// progress and offers the manual accept (check-in) and claim actions.
type MissionsModel struct {
	deps     Deps
	tokenID  uint64
	missions []bindings.MissionStatus
	loading  bool
	busy     bool
	notice   string
	err      string
}

func NewMissions(deps Deps, tokenID uint64) MissionsModel {
	return MissionsModel{deps: deps, tokenID: tokenID, loading: true}
}

type missionsLoadedMsg struct {
	tokenID  uint64
	missions []bindings.MissionStatus
	err      error
}

type missionActionDoneMsg struct {
	notice string
	err    error
}

func (m MissionsModel) Init() tea.Cmd { return m.load() }

// load fetches the dealer's mission status fresh (bypassing the cache so manual
// refresh always reflects the chain).
func (m MissionsModel) load() tea.Cmd {
	deps, id := m.deps, m.tokenID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if deps.Reader != nil {
			deps.Reader.InvalidateMissions(id)
		}
		var ms []bindings.MissionStatus
		var err error
		if deps.Reader != nil {
			ms, err = deps.Reader.MissionStatus(ctx, id)
			if err == nil {
				bindings.SortMissions(ms)
			}
		}
		return missionsLoadedMsg{tokenID: id, missions: ms, err: err}
	}
}

func (m MissionsModel) Update(msg tea.Msg) (MissionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case missionsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.err = ""
			m.missions = msg.missions
		}
		return m, nil
	case missionActionDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.notice = errStyle.Render(msg.err.Error())
		} else {
			m.notice = okStyle.Render(msg.notice)
		}
		return m, m.load()
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return backToFleetMsg{} }
		case "r":
			m.loading = true
			return m, m.load()
		case "a":
			return m.accept()
		case "c":
			return m.claimAll()
		}
	}
	return m, nil
}

// accept runs the mission check-in (snapshots the epoch baseline).
func (m MissionsModel) accept() (MissionsModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render("read-only — no signer")
		return m, nil
	}
	if m.busy {
		return m, nil
	}
	m.busy = true
	m.notice = statusBarStyle.Render("accepting…")
	deps, id := m.deps, m.tokenID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := deps.Manager.MissionCheckIn(ctx, id); err != nil {
			return missionActionDoneMsg{err: err}
		}
		return missionActionDoneMsg{notice: "checked in — today's missions accepted"}
	}
}

// claimAll claims every claimable mission, daily before weekly.
func (m MissionsModel) claimAll() (MissionsModel, tea.Cmd) {
	if m.deps.Manager == nil {
		m.notice = errStyle.Render("read-only — no signer")
		return m, nil
	}
	if m.busy {
		return m, nil
	}
	if _, ok := bindings.FirstClaimable(m.missions); !ok {
		m.notice = helpStyle.Render("nothing to claim")
		return m, nil
	}
	m.busy = true
	m.notice = statusBarStyle.Render("claiming…")
	deps, id := m.deps, m.tokenID
	claimable := append([]bindings.MissionStatus(nil), m.missions...)
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		n := 0
		// Re-scan FirstClaimable each pass so priority (daily→weekly) holds.
		for {
			tpl, ok := bindings.FirstClaimable(claimable)
			if !ok {
				break
			}
			if err := deps.Manager.ClaimMission(ctx, id, tpl); err != nil {
				if n > 0 {
					return missionActionDoneMsg{notice: fmt.Sprintf("claimed %d, then: %v", n, err)}
				}
				return missionActionDoneMsg{err: err}
			}
			n++
			markClaimed(claimable, tpl)
		}
		return missionActionDoneMsg{notice: fmt.Sprintf("claimed %d mission reward(s)", n)}
	}
}

// markClaimed flips a just-claimed mission so the loop advances to the next.
func markClaimed(ms []bindings.MissionStatus, tpl uint64) {
	for i := range ms {
		if ms[i].TemplateID != nil && ms[i].TemplateID.Uint64() == tpl {
			ms[i].Claimable = false
			ms[i].Claimed = true
			return
		}
	}
}

func (m MissionsModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", titleStyle.Render(fmt.Sprintf("MISSIONS — dealer #%d", m.tokenID)))

	switch {
	case m.deps.Reader != nil && m.deps.Reader.MissionsAddr() == (common.Address{}):
		return b.String() + helpStyle.Render("missions contract not deployed on this network")
	case m.loading:
		b.WriteString(helpStyle.Render("loading…\n"))
	case m.err != "":
		b.WriteString(errStyle.Render("⚠ "+m.err) + "\n")
	case len(m.missions) == 0:
		b.WriteString(helpStyle.Render("no active missions\n"))
	default:
		b.WriteString(renderMissionGroup("DAILY", m.missions, bindings.CadenceDaily))
		b.WriteString(renderMissionGroup("WEEKLY", m.missions, bindings.CadenceWeekly))
	}

	if m.notice != "" {
		b.WriteString("\n" + m.notice + "\n")
	}
	hint := "a accept (check-in) · c claim all · r refresh · esc back"
	if m.deps.Manager == nil {
		hint = "read-only · r refresh · esc back"
	}
	b.WriteString("\n" + helpStyle.Render(hint))
	return b.String()
}

// renderMissionGroup renders all missions of one cadence.
func renderMissionGroup(title string, ms []bindings.MissionStatus, cadence uint8) string {
	var rows []string
	for i := range ms {
		if ms[i].Mission.Cadence == cadence {
			rows = append(rows, "  "+renderMission(ms[i]))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	return sectionStyle.Render(title) + "\n" + strings.Join(rows, "\n") + "\n\n"
}

// renderMission renders one mission line: progress bar, reward, and state.
func renderMission(s bindings.MissionStatus) string {
	m := s.Mission
	pct := 0.0
	if m.Target > 0 {
		pct = float64(s.Progress) / float64(m.Target)
		if pct > 1 {
			pct = 1
		}
	}
	bar := missionBar.ViewAs(pct)
	state := statusBarStyle.Render("in progress")
	switch {
	case s.Claimed:
		state = helpStyle.Render("claimed ✓")
	case s.Claimable:
		state = okStyle.Render("CLAIMABLE ★")
	case !s.CheckedIn:
		state = negStyle.Render("not accepted — press a")
	}
	return fmt.Sprintf("%s %s  %s  %s", bar,
		fmt.Sprintf("%d/%d", s.Progress, m.Target), missionReward(m), state)
}

// missionReward renders a template's reward bundle compactly.
func missionReward(m bindings.MissionTemplate) string {
	var parts []string
	if m.RepReward > 0 {
		parts = append(parts, fmt.Sprintf("+%d rep", m.RepReward))
	}
	if m.InfamyReward > 0 {
		parts = append(parts, fmt.Sprintf("+%d inf", m.InfamyReward))
	}
	if m.CashReward != nil && m.CashReward.Sign() > 0 {
		parts = append(parts, "+"+m.CashReward.String()+" cash")
	}
	if m.DrugAmount > 0 {
		parts = append(parts, fmt.Sprintf("+%d drug#%d", m.DrugAmount, m.DrugID))
	}
	if len(parts) == 0 {
		return helpStyle.Render("(no reward)")
	}
	return posStyle.Render(strings.Join(parts, ", "))
}
