package dealer

import (
	"context"
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
)

func TestDefaultStepOrderMatchesCatalog(t *testing.T) {
	got := DefaultStepOrder()
	if len(got) != len(StepCatalog) {
		t.Fatalf("order len %d ≠ catalog len %d", len(got), len(StepCatalog))
	}
	for i, id := range got {
		if id != StepCatalog[i].ID {
			t.Errorf("order[%d]=%q, catalog=%q", i, id, StepCatalog[i].ID)
		}
	}
}

func TestRecipeReordersCoreBeforeHeists(t *testing.T) {
	// A pve dealer with an incomplete heist mission AND a stocked bank.
	r := &fakeReader{
		gs:               &bindings.GameState{RepCap: 100, RepTieBonus: 1, TotalReputation: big.NewInt(0)},
		sp:               &bindings.PVEStakeParams{RepStakeDivisor: 10, SlopeBps: 0, HeadroomBps: 10000},
		missions:         []bindings.MissionStatus{weeklyHeistMission(bindings.MetricHeistStages, 0, 5)},
		needHeistCheckIn: false,
	}
	st := richState(12000, 100000, 5) // rep clears the heist gate; on Manhattan, has energy
	dec := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}

	// Default order: heists run BEFORE the core → start a heist.
	def := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	if a, _ := def.Next(context.Background(), r, dec); a.Kind != ActionStartHeist {
		t.Fatalf("default order should run the heist mission first, got %+v", a)
	}

	// Custom order with "core" BEFORE "heists" → trade wins, heists never reached.
	custom := func() []string {
		return []string{StepHeistCheckIn, StepClearStars, StepMissions, StepFollowMissions, StepCore, StepHeists}
	}
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, custom)
	if a, ok := s.Next(context.Background(), r, dec); !ok || a.Kind != ActionPVE || a.Hustle != bindings.HustleBuy {
		t.Fatalf("core-before-heists should trade instead of heisting, got %+v ok=%v", a, ok)
	}
}

func TestHeistCheckInStepBoundedPerDay(t *testing.T) {
	r := &fakeReader{needHeistCheckIn: true}
	lim := newDailyLimiter()
	fired := 0
	for i := 0; i < heistCheckInAttemptsPerDay+3; i++ {
		if _, ok := heistCheckInStep(context.Background(), r, 42, lim); ok {
			fired++
		}
	}
	if fired != heistCheckInAttemptsPerDay {
		t.Fatalf("check-in fired %d times, want cap %d", fired, heistCheckInAttemptsPerDay)
	}
	// A different dealer gets its own daily budget.
	if _, ok := heistCheckInStep(context.Background(), r, 99, lim); !ok {
		t.Fatal("a different dealer should still get its first attempt")
	}
	// A nil limiter never blocks (defensive).
	if _, ok := heistCheckInStep(context.Background(), r, 42, nil); !ok {
		t.Fatal("nil limiter should not block the check-in")
	}
}

func TestHeistCheckInDoesNotStarvePipeline(t *testing.T) {
	// A dealer that perpetually needs check-in (e.g. too poor to pay the season
	// enter() fee) must not spin on the first step forever — after the daily cap it
	// falls through so stars/missions/trading still run. Regression for the "one NFT
	// does nothing in auto" bug.
	r := &fakeReader{
		gs:               &bindings.GameState{RepCap: 100, RepTieBonus: 1, TotalReputation: big.NewInt(0)},
		sp:               &bindings.PVEStakeParams{RepStakeDivisor: 10, SlopeBps: 0, HeadroomBps: 10000},
		needHeistCheckIn: true,
	}
	st := pveState(manhattan, 5, 100000, 0)
	dec := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)

	for i := 0; i < heistCheckInAttemptsPerDay; i++ {
		if a, ok := s.Next(context.Background(), r, dec); !ok || a.Kind != ActionHeistCheckIn {
			t.Fatalf("tick %d: want a check-in attempt, got %+v ok=%v", i, a, ok)
		}
	}
	// Cap reached → the dealer trades instead of spinning on the check-in.
	if a, ok := s.Next(context.Background(), r, dec); !ok || a.Kind != ActionPVE {
		t.Fatalf("after the check-in cap, dealer should trade, got %+v ok=%v", a, ok)
	}
}

func TestRecipeDisableStep(t *testing.T) {
	// Heat 4★ would normally clear stars; disable that step → it trades instead.
	r := &fakeReader{
		gs: &bindings.GameState{RepCap: 100, RepTieBonus: 1, TotalReputation: big.NewInt(0)},
		sp: &bindings.PVEStakeParams{RepStakeDivisor: 10, SlopeBps: 0, HeadroomBps: 10000},
	}
	st := pveState(manhattan, 5, 100000, 0)
	st.HeatLevel = 4
	dec := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}
	// Recipe without clear_stars.
	noStars := func() []string { return []string{StepMissions, StepCore} }
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, noStars)
	if a, ok := s.Next(context.Background(), r, dec); !ok || a.Kind != ActionPVE {
		t.Fatalf("with clear_stars disabled, 4★ dealer should trade, got %+v ok=%v", a, ok)
	}
}
