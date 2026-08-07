package dealer

import (
	"context"
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
)

// fakeReader is a scripted StrategyReader for strategy tests.
type fakeReader struct {
	gs       *bindings.GameState
	sp       *bindings.PVEStakeParams
	targets          []bindings.PVPTarget
	missions         []bindings.MissionStatus
	needHeistCheckIn bool
	activeHeist      uint64
	heist            *bindings.DailyHeist
	gsErr            error
	tgErr            error
}

func (f *fakeReader) GameState(context.Context, uint64) (*bindings.GameState, error) {
	return f.gs, f.gsErr
}
func (f *fakeReader) StakeParams(context.Context) (*bindings.PVEStakeParams, error) {
	return f.sp, nil
}
func (f *fakeReader) PotentialTargets(context.Context, uint64, uint64, uint64) ([]bindings.PVPTarget, uint64, error) {
	return f.targets, uint64(len(f.targets)), f.tgErr
}
func (f *fakeReader) MissionStatus(context.Context, uint64) ([]bindings.MissionStatus, error) {
	return f.missions, nil
}
func (f *fakeReader) NeedsHeistCheckIn(context.Context, uint64) (bool, error) {
	return f.needHeistCheckIn, nil
}
func (f *fakeReader) ActiveHeist(context.Context, uint64) (uint64, error) {
	return f.activeHeist, nil
}
func (f *fakeReader) GetHeist(context.Context, uint64) (*bindings.DailyHeist, error) {
	return f.heist, nil
}

// stakeReader yields a stake cap of 1000 $CASH (repCap 100 × divisor 10 × 1.0
// headroom / 1). With a unit price of 100 that is a per-hustle cap of 10 units.
func stakeReader() *fakeReader {
	return &fakeReader{
		gs: &bindings.GameState{RepCap: 100, RepTieBonus: 1, TotalReputation: big.NewInt(0)},
		sp: &bindings.PVEStakeParams{RepStakeDivisor: 10, SlopeBps: 0, HeadroomBps: 10000},
	}
}

const (
	manhattan uint8 = 1
	amsterdam uint8 = 7
)

func weedMarket(buy, sell int64) []bindings.AreaDrug {
	return []bindings.AreaDrug{
		{DrugID: big.NewInt(4), Name: "Weed", BuyPrice: big.NewInt(buy), SellPrice: big.NewInt(sell), IsAvailable: true},
	}
}

func pveState(area uint8, attempts uint8, cash int64, weedHeld int64) *bindings.FullDealerState {
	st := &bindings.FullDealerState{
		IsInitialized: true, CurrentArea: area, DailyAttemptsRemaining: attempts,
		CashBalance: big.NewInt(cash), Reputation: big.NewInt(300),
	}
	if weedHeld > 0 {
		st.DrugBalances = []bindings.DrugBalance{{DrugID: big.NewInt(4), Name: "Weed", Balance: big.NewInt(weedHeld)}}
	}
	return st
}

func TestPvEBuysWeedOnManhattan(t *testing.T) {
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	st := pveState(manhattan, 5, 100000, 0)
	a, ok := s.Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)})
	if !ok || a.Kind != ActionPVE || a.Hustle != bindings.HustleBuy || a.DrugID != 4 {
		t.Fatalf("want weed buy, got %+v ok=%v", a, ok)
	}
	if a.Amount != 10 { // per-hustle cap = 1000/100
		t.Errorf("buy amount = %d, want 10 (stake cap)", a.Amount)
	}
}

func TestPvEHaulsToAmsterdamWhenStocked(t *testing.T) {
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	// Holdings already at the ×3 stockpile target (30) → travel to sell.
	st := pveState(manhattan, 5, 100000, 30)
	a, ok := s.Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)})
	if !ok || a.Kind != ActionTravel || a.DestArea != amsterdam {
		t.Fatalf("want travel to Amsterdam, got %+v ok=%v", a, ok)
	}
}

func TestPvESellsInAmsterdam(t *testing.T) {
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	st := pveState(amsterdam, 5, 100000, 30)
	a, ok := s.Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 100)})
	if !ok || a.Kind != ActionPVE || a.Hustle != bindings.HustleSell {
		t.Fatalf("want weed sell, got %+v ok=%v", a, ok)
	}
	if a.Amount != 10 { // capped at per-hustle stake limit (1000/100)
		t.Errorf("sell amount = %d, want 10", a.Amount)
	}
}

func TestPvEReturnsToManhattanWhenEmpty(t *testing.T) {
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	st := pveState(amsterdam, 5, 100000, 0) // nothing left to sell
	a, ok := s.Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 100)})
	if !ok || a.Kind != ActionTravel || a.DestArea != manhattan {
		t.Fatalf("want travel back to Manhattan, got %+v ok=%v", a, ok)
	}
}

func TestPvERetreatsToBlackMarketOutOfEnergy(t *testing.T) {
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	st := pveState(amsterdam, 0, 100000, 5)
	a, ok := s.Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 100)})
	if !ok || a.Kind != ActionTravel || a.DestArea != bindings.BlackMarketArea {
		t.Fatalf("want travel to black market, got %+v ok=%v", a, ok)
	}
	// Already in the black market with no energy → idle.
	st.CurrentArea = bindings.BlackMarketArea
	if _, ok := s.Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 1, State: st}}); ok {
		t.Error("parked in black market should be idle")
	}
}

func TestPvEPosterFirstOncePerDay(t *testing.T) {
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	st := pveState(manhattan, 5, 100000, 0)
	st.HeatLevel = 4
	dec := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}
	a, ok := s.Next(context.Background(), stakeReader(), dec)
	if !ok || a.Kind != ActionClearHeat {
		t.Fatalf("first action at 4★ should clear heat, got %+v ok=%v", a, ok)
	}
	// Same day, heat still high → does NOT clear again, proceeds to buy.
	a2, ok := s.Next(context.Background(), stakeReader(), dec)
	if !ok || a2.Kind != ActionPVE {
		t.Fatalf("second call should proceed to buy, got %+v ok=%v", a2, ok)
	}
}
