package dealer

import (
	"context"
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
)

// raidReader gives PvP targets plus the stake data the PvE fallback needs.
func raidReader(targets []bindings.PVPTarget) *fakeReader {
	r := stakeReader()
	r.targets = targets
	return r
}

func raidState(area uint8, attempts uint8) *bindings.FullDealerState {
	return &bindings.FullDealerState{
		IsInitialized: true, CurrentArea: area, DailyAttemptsRemaining: attempts,
		Reputation: big.NewInt(500), CashBalance: big.NewInt(100000), // above PvP unlock, funded for PvE
	}
}

func target(id uint64, attackable bool) bindings.PVPTarget {
	return bindings.PVPTarget{TokenID: new(big.Int).SetUint64(id), CanAttackNow: attackable}
}

func TestPvPAttacksFirstNonAlly(t *testing.T) {
	ally := map[uint64]bool{20: true}
	s := NewPvPRaider(manhattan, amsterdam, func(id uint64) bool { return ally[id] }, nil, nil)
	r := raidReader([]bindings.PVPTarget{target(20, true), target(21, true)})
	st := raidState(manhattan, 5)
	a, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)})
	if !ok || a.Kind != ActionPVP || a.DefenderID != 21 {
		t.Fatalf("want attack on #21 (20 is ally), got %+v ok=%v", a, ok)
	}
}

func TestPvPFallsBackToPvEWhenNoTarget(t *testing.T) {
	s := NewPvPRaider(manhattan, amsterdam, nil, nil, nil)
	r := raidReader(nil) // no targets anywhere
	st := raidState(manhattan, 5)
	a, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)})
	if !ok || a.Kind != ActionPVE || a.Hustle != bindings.HustleBuy {
		t.Fatalf("no targets → PvE weed buy, got %+v ok=%v", a, ok)
	}
}

func TestPvPUnattackableFallsBackToPvE(t *testing.T) {
	s := NewPvPRaider(manhattan, amsterdam, nil, nil, nil)
	r := raidReader([]bindings.PVPTarget{target(30, false)}) // present but not attackable now
	st := raidState(manhattan, 5)
	a, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)})
	if !ok || a.Kind != ActionPVE {
		t.Fatalf("no attackable target → PvE, got %+v ok=%v", a, ok)
	}
}

func TestPvPBelowUnlockDoesPvE(t *testing.T) {
	s := NewPvPRaider(manhattan, amsterdam, nil, nil, nil)
	r := raidReader([]bindings.PVPTarget{target(40, true)})
	st := raidState(manhattan, 5)
	st.Reputation = big.NewInt(50) // below 200 — can't raid, so trade instead of idling
	a, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)})
	if !ok || a.Kind == ActionPVP {
		t.Fatalf("below unlock should NOT attack, should PvE; got %+v ok=%v", a, ok)
	}
}

func TestPvPPosterFirstThenActs(t *testing.T) {
	s := NewPvPRaider(manhattan, amsterdam, nil, nil, nil)
	r := raidReader([]bindings.PVPTarget{target(50, true)})
	st := raidState(manhattan, 5)
	st.HeatLevel = 3
	dec := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}
	a, ok := s.Next(context.Background(), r, dec)
	if !ok || a.Kind != ActionClearHeat {
		t.Fatalf("first action at 3★ should clear heat, got %+v ok=%v", a, ok)
	}
	// Heat cleared → now attacks the target.
	st.HeatLevel = 0
	a2, ok := s.Next(context.Background(), r, dec)
	if !ok || a2.Kind != ActionPVP || a2.DefenderID != 50 {
		t.Fatalf("second call should attack #50, got %+v ok=%v", a2, ok)
	}
}

func TestPvPSellsLootInBlackMarket(t *testing.T) {
	s := NewPvPRaider(manhattan, amsterdam, nil, nil, nil)
	r := &fakeReader{}
	st := raidState(bindings.BlackMarketArea, 0) // out of energy, in black market
	st.DrugBalances = []bindings.DrugBalance{
		{DrugID: big.NewInt(9), Name: "Fentanyl", Balance: big.NewInt(3)},
	}
	a, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}})
	if !ok || a.Kind != ActionSellDrop || a.DrugID != 9 || a.Amount != 3 {
		t.Fatalf("want sellDrop of looted drug, got %+v ok=%v", a, ok)
	}
	// Same drug already attempted today → no repeat (no spin), now idle.
	if _, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}}); ok {
		t.Error("loot already had its sell attempt today → should idle")
	}
}

func TestPvPTravelsToBlackMarketOutOfEnergy(t *testing.T) {
	s := NewPvPRaider(manhattan, amsterdam, nil, nil, nil)
	r := &fakeReader{}
	st := raidState(manhattan, 0) // out of energy, not yet in black market
	a, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}})
	if !ok || a.Kind != ActionTravel || a.DestArea != bindings.BlackMarketArea {
		t.Fatalf("want travel to black market, got %+v ok=%v", a, ok)
	}
}
