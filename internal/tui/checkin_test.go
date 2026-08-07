package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckInSummaryCounts(t *testing.T) {
	results := []checkInResult{
		{tokenID: 1, status: ciDone},
		{tokenID: 2, status: ciDone},
		{tokenID: 3, status: ciAlready},
		{tokenID: 4, status: ciJailed},
	}
	got := stripANSI(checkInSummary(results))
	for _, want := range []string{"2 done", "1 already", "1 jailed"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "uninit") || strings.Contains(got, "not eligible") {
		t.Errorf("summary listed zero-count buckets: %q", got)
	}
}

func TestCheckInSummaryEmpty(t *testing.T) {
	if got := stripANSI(checkInSummary(nil)); !strings.Contains(got, "no dealers") {
		t.Errorf("empty summary = %q, want 'no dealers'", got)
	}
}

func TestCheckInSummarySurfacesError(t *testing.T) {
	results := []checkInResult{
		{tokenID: 1, status: ciDone},
		{tokenID: 2, status: ciError, err: errors.New("rpc timeout")},
	}
	got := stripANSI(checkInSummary(results))
	if !strings.Contains(got, "1 errors") || !strings.Contains(got, "rpc timeout") {
		t.Errorf("summary %q should report the error count and first message", got)
	}
}

// stripANSI removes lipgloss color escapes so assertions match on plain text.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true
		case inEsc && r == 'm':
			inEsc = false
		case !inEsc:
			b.WriteRune(r)
		}
	}
	return b.String()
}
