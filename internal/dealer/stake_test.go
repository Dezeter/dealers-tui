package dealer

import (
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
)

func TestMaxStake(t *testing.T) {
	p := &bindings.PVEStakeParams{RepStakeDivisor: 50, SlopeBps: 2500, HeadroomBps: 10000}

	// Outsider tier (repCap 120, tie 60) at 0 rep:
	//   divisor = 50 + 0 = 50
	//   maxStake = 120*50*10000 / (60*10000) = 6000/60 = 100
	gs := &bindings.GameState{RepCap: 120, RepTieBonus: 60, TotalReputation: big.NewInt(0)}
	if got := MaxStake(gs, p); got == nil || got.Int64() != 100 {
		t.Errorf("MaxStake(0 rep) = %v, want 100", got)
	}

	// Same tier at 1000 rep: divisor = 50 + 1000*2500/10000 = 300;
	//   maxStake = 120*300 / 60 = 600.
	gs.TotalReputation = big.NewInt(1000)
	if got := MaxStake(gs, p); got == nil || got.Int64() != 600 {
		t.Errorf("MaxStake(1000 rep) = %v, want 600", got)
	}

	// No cap when headroom 0 / repCap 0 / tieBonus 0.
	if MaxStake(&bindings.GameState{RepCap: 120, RepTieBonus: 60}, &bindings.PVEStakeParams{HeadroomBps: 0}) != nil {
		t.Error("headroom 0 should be no cap")
	}
	if MaxStake(&bindings.GameState{RepCap: 0, RepTieBonus: 60, TotalReputation: big.NewInt(0)}, p) != nil {
		t.Error("repCap 0 should be no cap")
	}
}

func TestMaxUnitsAtPrice(t *testing.T) {
	// maxStake 194, Weed @1 → 194; Cocaine @120 → 1.
	if u := MaxUnitsAtPrice(big.NewInt(194), big.NewInt(1)); u != 194 {
		t.Errorf("Weed units = %d, want 194", u)
	}
	if u := MaxUnitsAtPrice(big.NewInt(194), big.NewInt(120)); u != 1 {
		t.Errorf("Cocaine units = %d, want 1", u)
	}
	// nil maxStake (no cap) → 0.
	if u := MaxUnitsAtPrice(nil, big.NewInt(1)); u != 0 {
		t.Errorf("no-cap units = %d, want 0", u)
	}
}
