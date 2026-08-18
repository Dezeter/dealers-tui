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

// Batch season-reward claim (DealersBankHeist.claim): one keypress collects every
// ended-season reward due to any dealer in the fleet. There is no getter for
// pending rewards, so each (season, dealer) pair is dry-run with
// Reader.CanClaimSeason and only the ones that actually pay out are claimed — no
// gas is wasted on a not-yet-due or already-claimed season. Claiming is gas-only
// on our side; the ETH reward lands in the owner AGW.

// maxSeasonLookback bounds how many past seasons a claim sweep probes per dealer,
// so a long-lived game with many seasons can't blow up the eth_call count. Unclaimed
// rewards older than this are unlikely, and the user can re-run to reach further back.
const maxSeasonLookback = 24

// claimStatus is the per-dealer outcome of a batch claim.
type claimStatus string

const (
	clDone    claimStatus = "claimed"
	clNothing claimStatus = "nothing"
	clUninit  claimStatus = "uninitialized"
	clError   claimStatus = "error"
)

// claimResult is one dealer's outcome; claimed counts the seasons actually banked.
type claimResult struct {
	tokenID uint64
	claimed int
	status  claimStatus
	err     error
}

// claimDoneMsg carries a completed batch claim back to the UI.
type claimDoneMsg struct {
	results []claimResult
}

// claimAllCmd runs the batch off the UI goroutine over the given snapshots.
func claimAllCmd(deps Deps, snaps []dealer.Snapshot) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		return claimDoneMsg{results: runClaimAll(ctx, deps, snaps)}
	}
}

// runClaimAll claims every due season reward across the fleet sequentially (the
// sender serializes sends anyway). For each initialized dealer it probes the last
// maxSeasonLookback seasons up to the active one and claims the ones that dry-run
// clean; an inconclusive read just skips that season this run (never a blind spend).
func runClaimAll(ctx context.Context, deps Deps, snaps []dealer.Snapshot) []claimResult {
	out := make([]claimResult, 0, len(snaps))
	// Read the season fresh so a just-ended season is seen right at the rollover.
	deps.Reader.InvalidateCheckins()
	active, seasonErr := deps.Reader.ActiveSeason(ctx)

	low := uint64(0)
	if active >= maxSeasonLookback {
		low = active - maxSeasonLookback + 1
	}

	for _, s := range snaps {
		r := claimResult{tokenID: s.TokenID}
		switch {
		case s.State == nil || !s.State.IsInitialized:
			r.status = clUninit
		case seasonErr != nil:
			r.status, r.err = clError, seasonErr
		default:
			// Sweep seasons newest-first so the just-ended one is claimed first.
			for season := int64(active); season >= int64(low); season-- {
				can, err := deps.Reader.CanClaimSeason(ctx, deps.Owner, uint64(season), s.TokenID)
				if err != nil {
					if r.err == nil {
						r.err = err // inconclusive read — remember but don't spend
					}
					continue
				}
				if !can {
					continue // nothing due for this (season, dealer)
				}
				if err := deps.Manager.ClaimSeason(ctx, s.TokenID, uint64(season)); err != nil {
					if strings.Contains(err.Error(), "reverted on chain") {
						continue // raced to unclaimable — harmless, skip
					}
					if r.err == nil {
						r.err = err
					}
					continue
				}
				r.claimed++
			}
			switch {
			case r.claimed > 0:
				r.status = clDone
			case r.err != nil:
				r.status = clError
			default:
				r.status = clNothing
			}
		}
		out = append(out, r)
	}
	return out
}

// claimSummary renders a one-line notice from the batch results, e.g.
// "rewards: claimed 4 seasons (2) · 5 nothing".
func claimSummary(results []claimResult) string {
	if len(results) == 0 {
		return statusBarStyle.Render(i18n.T("claim.none"))
	}
	var (
		dealersClaimed, seasonsClaimed, nothing, uninit, errs int
		firstErr                                              error
	)
	for _, r := range results {
		switch r.status {
		case clDone:
			dealersClaimed++
			seasonsClaimed += r.claimed
		case clNothing:
			nothing++
		case clUninit:
			uninit++
		case clError:
			errs++
			if firstErr == nil {
				firstErr = r.err
			}
		}
	}
	var parts []string
	if seasonsClaimed > 0 {
		parts = append(parts, fmt.Sprintf("%d %s (%d)", seasonsClaimed, i18n.T("claim.seasons"), dealersClaimed))
	}
	if nothing > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", nothing, i18n.T("claim.nothing")))
	}
	if uninit > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", uninit, i18n.T("claim.uninit")))
	}
	if errs > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", errs, i18n.T("claim.errors")))
	}
	line := okStyle.Render(i18n.T("claim.prefix")) + strings.Join(parts, " · ")
	if firstErr != nil {
		line += "  " + errStyle.Render("⚠ "+firstErr.Error())
	}
	return line
}
