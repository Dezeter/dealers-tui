package dealer

import (
	"context"
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
)

func snap(st *bindings.FullDealerState) Snapshot { return Snapshot{TokenID: 1, State: st} }

func market() []bindings.AreaDrug {
	return []bindings.AreaDrug{
		{DrugID: big.NewInt(6), Name: "Cocaine", BuyPrice: big.NewInt(120), IsAvailable: true},
		{DrugID: big.NewInt(4), Name: "Weed", BuyPrice: big.NewInt(1), IsAvailable: true},
		{DrugID: big.NewInt(8), Name: "Heroin", BuyPrice: big.NewInt(0), IsAvailable: false}, // not sold here
	}
}

func TestManualStrategyNeverActs(t *testing.T) {
	if _, ok := (ManualStrategy{}).Next(context.Background(), nil, Decision{Snap: snap(&bindings.FullDealerState{IsInitialized: true, DailyAttemptsRemaining: 5, CashBalance: big.NewInt(1000)}), Area: market()}); ok {
		t.Error("ManualStrategy should never act")
	}
}

func TestMissionStepPriority(t *testing.T) {
	daily := bindings.MissionStatus{TemplateID: big.NewInt(1), Mission: bindings.MissionTemplate{Cadence: bindings.CadenceDaily, Enabled: true}, CheckedIn: true, Claimable: true}
	// Claimable mission → claim it first.
	r := &fakeReader{missions: []bindings.MissionStatus{daily}}
	a, ok := missionStep(context.Background(), r, 1)
	if !ok || a.Kind != ActionMissionClaim || a.TemplateID != 1 {
		t.Fatalf("want claim of daily #1, got %+v ok=%v", a, ok)
	}
	// Nothing claimable but not checked in → check in.
	r = &fakeReader{missions: []bindings.MissionStatus{{TemplateID: big.NewInt(9), Mission: bindings.MissionTemplate{Cadence: bindings.CadenceDaily, Enabled: true}, CheckedIn: false}}}
	a, ok = missionStep(context.Background(), r, 1)
	if !ok || a.Kind != ActionMissionCheckIn {
		t.Fatalf("want mission check-in, got %+v ok=%v", a, ok)
	}
	// All done → no mission action (strategy proceeds).
	r = &fakeReader{missions: []bindings.MissionStatus{{TemplateID: big.NewInt(9), Mission: bindings.MissionTemplate{Cadence: bindings.CadenceDaily, Enabled: true}, CheckedIn: true}}}
	if _, ok := missionStep(context.Background(), r, 1); ok {
		t.Error("all checked-in and nothing claimable → missionStep should yield")
	}
}

func TestPvEDoesMissionsBeforeTrading(t *testing.T) {
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	r := raidReader(nil) // has stake data for PvE
	r.missions = []bindings.MissionStatus{{TemplateID: big.NewInt(5), Mission: bindings.MissionTemplate{Cadence: bindings.CadenceDaily, Enabled: true}, CheckedIn: true, Claimable: true}}
	st := pveState(manhattan, 5, 100000, 0)
	a, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)})
	if !ok || a.Kind != ActionMissionClaim || a.TemplateID != 5 {
		t.Fatalf("PvE should claim the mission before buying, got %+v ok=%v", a, ok)
	}
}

func TestHeistCheckInIsFirstAction(t *testing.T) {
	// Heist check-in needed AND heat 4★ AND a claimable mission → check-in wins
	// (it runs before stars and missions).
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	r := stakeReader()
	r.needHeistCheckIn = true
	r.missions = []bindings.MissionStatus{{TemplateID: big.NewInt(5), Mission: bindings.MissionTemplate{Cadence: bindings.CadenceDaily, Enabled: true}, CheckedIn: true, Claimable: true}}
	st := pveState(manhattan, 5, 100000, 0)
	st.HeatLevel = 4
	a, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)})
	if !ok || a.Kind != ActionHeistCheckIn {
		t.Fatalf("heist check-in should be the first action, got %+v ok=%v", a, ok)
	}
}

func TestHeistCheckInWaitsForJailRelease(t *testing.T) {
	// Jailed dealer that also needs a heist check-in → breakout first (check-in
	// happens on a later tick, once released).
	s := NewPvPRaider(manhattan, amsterdam, nil, nil, nil)
	r := stakeReader()
	r.needHeistCheckIn = true
	st := &bindings.FullDealerState{IsInitialized: true, IsJailed: true, CanBreakoutToday: true}
	a, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}})
	if !ok || a.Kind != ActionBreakout {
		t.Fatalf("jailed dealer should break out before checking in, got %+v ok=%v", a, ok)
	}
}

func TestNoHeistCheckInWhenNotNeeded(t *testing.T) {
	// No active season / already checked in → step yields, strategy proceeds.
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	r := stakeReader() // needHeistCheckIn defaults false
	st := pveState(manhattan, 5, 100000, 0)
	a, ok := s.Next(context.Background(), r, Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)})
	if !ok || a.Kind == ActionHeistCheckIn {
		t.Fatalf("no check-in needed → should trade, got %+v ok=%v", a, ok)
	}
}

func TestJailbreakFirst(t *testing.T) {
	jailed := &bindings.FullDealerState{IsInitialized: true, IsJailed: true, CanBreakoutToday: true}
	for name, s := range map[string]Strategy{
		"pve": NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil),
		"pvp": NewPvPRaider(manhattan, amsterdam, nil, nil, nil),
	} {
		a, ok := s.Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 1, State: jailed}})
		if !ok || a.Kind != ActionBreakout {
			t.Errorf("%s: jailed dealer should attempt breakout, got %+v ok=%v", name, a, ok)
		}
	}
	// Jailed but the free daily attempt is used → idle when bail is OFF.
	used := &bindings.FullDealerState{IsInitialized: true, IsJailed: true, CanBreakoutToday: false}
	if _, ok := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil).Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 1, State: used}}); ok {
		t.Error("no breakout attempt left + bail off → should idle")
	}
	// Same state, but with the pay-bail setting ON → pay bail.
	payOn := NewPvEArbitrage(manhattan, amsterdam, nil, func() bool { return true }, nil)
	a, ok := payOn.Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 1, State: used}})
	if !ok || a.Kind != ActionPayBail {
		t.Errorf("free attempt used + bail on → should pay bail, got %+v ok=%v", a, ok)
	}
}

func TestJailbreakFirstUnit(t *testing.T) {
	free := &bindings.FullDealerState{IsInitialized: true, IsJailed: true, CanBreakoutToday: true}
	used := &bindings.FullDealerState{IsInitialized: true, IsJailed: true, CanBreakoutToday: false}
	free2 := &bindings.FullDealerState{IsInitialized: true, IsJailed: false}

	// Free attempt available → breakout, regardless of the bail setting.
	if a, ok := jailbreakFirst(free, false); !ok || a.Kind != ActionBreakout {
		t.Errorf("free attempt → breakout, got %+v ok=%v", a, ok)
	}
	if a, ok := jailbreakFirst(free, true); !ok || a.Kind != ActionBreakout {
		t.Errorf("free attempt (bail on) → still breakout first, got %+v ok=%v", a, ok)
	}
	// Used up: bail off → idle; bail on → pay bail.
	if _, ok := jailbreakFirst(used, false); ok {
		t.Error("used + bail off → idle")
	}
	if a, ok := jailbreakFirst(used, true); !ok || a.Kind != ActionPayBail {
		t.Errorf("used + bail on → pay bail, got %+v ok=%v", a, ok)
	}
	// Not jailed → nothing.
	if _, ok := jailbreakFirst(free2, true); ok {
		t.Error("not jailed → no jail action")
	}
}

// tagStrategy is a stub that reports which dealer it was asked about.
type tagStrategy struct{ tag uint64 }

func (t tagStrategy) Next(context.Context, StrategyReader, Decision) (Action, bool) {
	return Action{Kind: ActionPVE, DrugID: t.tag}, true
}

func TestMultiStrategyRoutesByTokenID(t *testing.T) {
	assigned := map[uint64]Strategy{7: tagStrategy{tag: 700}}
	def := tagStrategy{tag: 999}
	m := MultiStrategy{Resolve: func(id uint64) Strategy {
		if s, ok := assigned[id]; ok {
			return s
		}
		return def
	}}
	// Assigned dealer → its strategy.
	a, _ := m.Next(context.Background(), nil, Decision{Snap: Snapshot{TokenID: 7}})
	if a.DrugID != 700 {
		t.Errorf("dealer 7 routed wrong: DrugID=%d, want 700", a.DrugID)
	}
	// Unassigned dealer → default.
	a, _ = m.Next(context.Background(), nil, Decision{Snap: Snapshot{TokenID: 3}})
	if a.DrugID != 999 {
		t.Errorf("dealer 3 should hit default: DrugID=%d, want 999", a.DrugID)
	}
	// Resolve returning nil (or a nil Resolve) → idle, never panics.
	if _, ok := (MultiStrategy{}).Next(context.Background(), nil, Decision{Snap: Snapshot{TokenID: 1}}); ok {
		t.Error("empty MultiStrategy should idle")
	}
}
