package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dealers/internal/dealer"
	"dealers/internal/i18n"

	tea "github.com/charmbracelet/bubbletea"
)

// Batch daily check-in (DealersBankHeist.checkIn): one keypress checks in every
// eligible dealer in the fleet. Jailed / uninitialized dealers and those that
// already checked in today are skipped so no gas is wasted on a guaranteed
// revert. Check-in is gas-only — it stakes no $CASH or ETH.

// checkInStatus is the per-dealer outcome of a batch check-in.
type checkInStatus string

const (
	ciDone    checkInStatus = "checked in"
	ciAlready checkInStatus = "already today"
	ciJailed  checkInStatus = "jailed"
	ciUninit  checkInStatus = "uninitialized"
	ciSkipped checkInStatus = "not eligible"
	ciError   checkInStatus = "error"
)

// checkInResult is one dealer's outcome; err is set only for infra failures.
type checkInResult struct {
	tokenID uint64
	status  checkInStatus
	err     error
}

// checkInDoneMsg carries a completed batch check-in back to the UI.
type checkInDoneMsg struct {
	results []checkInResult
}

// checkInAllCmd runs the batch off the UI goroutine over the given snapshots.
func checkInAllCmd(deps Deps, snaps []dealer.Snapshot) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		return checkInDoneMsg{results: runCheckInAll(ctx, deps, snaps, time.Now().UTC().Unix())}
	}
}

// runCheckInAll checks in every eligible dealer sequentially (the sender
// serializes sends anyway, so there is no concurrency win — and sequential keeps
// the reporting order stable). Best-effort season/focus reads only ever cause a
// redundant attempt, never a skipped check-in.
func runCheckInAll(ctx context.Context, deps Deps, snaps []dealer.Snapshot, nowUnix int64) []checkInResult {
	out := make([]checkInResult, 0, len(snaps))
	// Read the season fresh: right after a rollover a cached (old) season would
	// mis-skip dealers as "already checked in" and route enter()/checkIn to the
	// wrong season.
	deps.Reader.InvalidateCheckins()
	season, seasonErr := deps.Reader.ActiveSeason(ctx)
	for _, s := range snaps {
		r := checkInResult{tokenID: s.TokenID}
		switch {
		case s.State == nil || !s.State.IsInitialized:
			r.status = ciUninit
		case s.State.IsJailed:
			r.status = ciJailed
		default:
			if seasonErr == nil {
				if done, err := deps.Reader.CheckedInToday(ctx, season, s.TokenID, nowUnix); err == nil && done {
					r.status = ciAlready
					out = append(out, r)
					continue
				}
			}
			if err := deps.Manager.CheckIn(ctx, s.TokenID); err != nil {
				// An on-chain revert (mined-and-reverted OR a gas-estimate revert) on
				// check-in is benign: the dealer already checked in today or isn't
				// eligible yet. Treat it as a skip, not an error, so a routine
				// already-done state doesn't show up as a scary error count.
				msg := err.Error()
				if strings.Contains(msg, "reverted on chain") || strings.Contains(msg, "execution reverted") {
					r.status = ciSkipped // already checked in today, or season not open
				} else {
					r.status, r.err = ciError, err
				}
			} else {
				r.status = ciDone
			}
		}
		out = append(out, r)
	}
	return out
}

// checkInSummary renders a one-line notice from the batch results, e.g.
// "check-in: 3 done · 1 already · 1 jailed".
func checkInSummary(results []checkInResult) string {
	if len(results) == 0 {
		return statusBarStyle.Render(i18n.T("checkin.none"))
	}
	counts := map[checkInStatus]int{}
	var firstErr error
	for _, r := range results {
		counts[r.status]++
		if r.status == ciError && firstErr == nil {
			firstErr = r.err
		}
	}
	// Stable, human-ordered summary.
	order := []struct {
		st  checkInStatus
		key string
	}{
		{ciDone, "checkin.done"},
		{ciAlready, "checkin.already"},
		{ciSkipped, "checkin.not_eligible"},
		{ciJailed, "checkin.jailed"},
		{ciUninit, "checkin.uninit"},
		{ciError, "checkin.errors"},
	}
	var parts []string
	for _, o := range order {
		if n := counts[o.st]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, i18n.T(o.key)))
		}
	}
	line := okStyle.Render(i18n.T("checkin.prefix")) + strings.Join(parts, " · ")
	if firstErr != nil {
		line += "  " + errStyle.Render("⚠ "+firstErr.Error())
	}
	return line
}
