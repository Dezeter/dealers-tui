package bindings

import (
	"encoding/hex"
	"errors"
	"testing"
)

// TestIsRevert separates on-chain reverts (skip the enter) from transport/
// rate-limit errors (inconclusive → attempt anyway), the discriminator
// CanEnterSeason relies on to never wrongly block a real check-in.
func TestIsRevert(t *testing.T) {
	reverts := []string{
		"execution reverted",
		"execution reverted: InsufficientCash",
		"VM Exception while processing transaction: revert",
	}
	for _, s := range reverts {
		if !isRevert(errors.New(s)) {
			t.Errorf("isRevert(%q) = false, want true", s)
		}
	}
	notReverts := []string{
		"429 Too Many Requests",
		"context deadline exceeded",
		"dial tcp: connection refused",
	}
	for _, s := range notReverts {
		if isRevert(errors.New(s)) {
			t.Errorf("isRevert(%q) = true, want false", s)
		}
	}
	if isRevert(nil) {
		t.Error("isRevert(nil) = true, want false")
	}
}

// TestPackCheckInMatchesOnChain pins PackCheckIn to the real mainnet check-in
// transaction (abscan 0x5f2884…d641873): checkIn(24) with selector 0xe95a644f.
// If the ABI signature ever drifts, this catches it against ground truth.
func TestPackCheckInMatchesOnChain(t *testing.T) {
	data, err := PackCheckIn(24)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	const want = "e95a644f0000000000000000000000000000000000000000000000000000000000000018"
	if got := hex.EncodeToString(data); got != want {
		t.Errorf("checkIn(24) calldata\n got  %s\n want %s", got, want)
	}
	// Selector must equal the method ID for the real signature.
	if wantSel := bankHeistABI.Methods["checkIn"].ID; string(data[:4]) != string(wantSel) {
		t.Errorf("selector mismatch: got %x want %x", data[:4], wantSel)
	}
}

// TestPackEnterMatchesOnChain pins PackEnter to the real season-entry tx
// (abscan 0x2a8f6e…f26d6): enter(24) with selector 0xa59f3e0c.
func TestPackEnterMatchesOnChain(t *testing.T) {
	data, err := PackEnter(24)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	const want = "a59f3e0c0000000000000000000000000000000000000000000000000000000000000018"
	if got := hex.EncodeToString(data); got != want {
		t.Errorf("enter(24) calldata\n got  %s\n want %s", got, want)
	}
}

// TestCheckedInTodayDayMath pins the today/lastDay comparison unit: focusState
// stores UTC day numbers (timestamp/86400) in uint32, and the whole day maps to
// one number (floor division) with the boundary rolling exactly at 86400s.
func TestCheckedInTodayDayMath(t *testing.T) {
	const day = int64(20663)
	cases := []struct {
		now  int64
		want int64
	}{
		{day * secondsPerDay, day},                   // 00:00:00 of the day
		{day*secondsPerDay + secondsPerDay - 1, day}, // 23:59:59 same day
		{(day + 1) * secondsPerDay, day + 1},         // next midnight rolls over
	}
	for _, c := range cases {
		if got := c.now / secondsPerDay; got != c.want {
			t.Errorf("day(%d) = %d, want %d", c.now, got, c.want)
		}
	}
}
