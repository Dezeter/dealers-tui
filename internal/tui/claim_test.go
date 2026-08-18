package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestClaimSummaryCounts(t *testing.T) {
	results := []claimResult{
		{tokenID: 1, status: clDone, claimed: 2},
		{tokenID: 2, status: clDone, claimed: 1},
		{tokenID: 3, status: clNothing},
		{tokenID: 4, status: clUninit},
	}
	got := stripANSI(claimSummary(results))
	// 3 seasons claimed across 2 dealers, plus the zero-reward buckets.
	for _, want := range []string{"3 seasons (2)", "1 nothing", "1 uninit"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "errors") {
		t.Errorf("summary listed a zero-count error bucket: %q", got)
	}
}

func TestClaimSummaryEmpty(t *testing.T) {
	if got := stripANSI(claimSummary(nil)); !strings.Contains(got, "no dealers") {
		t.Errorf("empty summary = %q, want 'no dealers'", got)
	}
}

func TestClaimSummarySurfacesError(t *testing.T) {
	results := []claimResult{
		{tokenID: 1, status: clDone, claimed: 1},
		{tokenID: 2, status: clError, err: errors.New("rpc timeout")},
	}
	got := stripANSI(claimSummary(results))
	if !strings.Contains(got, "1 errors") || !strings.Contains(got, "rpc timeout") {
		t.Errorf("summary %q should report the error count and first message", got)
	}
}
