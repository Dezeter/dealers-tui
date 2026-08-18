package dealer

import (
	"context"
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
	"dealers/internal/template"
)

// memState is an in-memory ProgState for tests.
type memState struct{ m map[uint64][2]int }

func newMemState() *memState                 { return &memState{m: map[uint64][2]int{}} }
func (s *memState) Get(id uint64) (int, int) { v := s.m[id]; return v[0], v[1] }
func (s *memState) Set(id uint64, step, reps int) error {
	s.m[id] = [2]int{step, reps}
	return nil
}

func progFor(steps ...ProgStep) func(uint64) []ProgStep {
	return func(uint64) []ProgStep { return steps }
}

func TestProgramTradeCountThenAdvance(t *testing.T) {
	state := newMemState()
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionTrade, BuyArea: manhattan, SellArea: amsterdam, Count: 2},
		ProgStep{Action: template.ActionClearStars},
	), state, nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0)
	st.HeatLevel = 5 // so clear_stars has work once we reach it
	d := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}
	r := stakeReader()

	// tick 1: first trade (rep 1), still on the trade step.
	a, ok := p.Next(context.Background(), r, d)
	if !ok || a.Kind != ActionPVE {
		t.Fatalf("t1 = %+v ok=%v, want a PvE buy", a, ok)
	}
	if step, reps := state.Get(1); step != 0 || reps != 1 {
		t.Fatalf("after t1 pos = (%d,%d), want (0,1)", step, reps)
	}
	// tick 2: second trade completes the count → advance to clear_stars.
	a, _ = p.Next(context.Background(), r, d)
	if a.Kind != ActionPVE {
		t.Fatalf("t2 = %+v, want a PvE buy", a)
	}
	if step, reps := state.Get(1); step != 1 || reps != 0 {
		t.Fatalf("after t2 pos = (%d,%d), want (1,0) — advanced", step, reps)
	}
	// tick 3: now on clear_stars, heat is 5 → clear heat.
	a, ok = p.Next(context.Background(), r, d)
	if !ok || a.Kind != ActionClearHeat {
		t.Fatalf("t3 = %+v ok=%v, want ClearHeat", a, ok)
	}
}

func TestProgramBreakoutReachedWhenJailed(t *testing.T) {
	// Breakout is the LAST step; a jailed dealer's earlier steps are non-actionable,
	// so the tick scan advances past them and reaches breakout the same tick.
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionTrade, BuyArea: manhattan, SellArea: amsterdam},
		ProgStep{Action: template.ActionBreakout},
	), newMemState(), nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0)
	st.IsJailed = true
	st.CanBreakoutToday = true
	d := Decision{Snap: Snapshot{TokenID: 2, State: st}, Area: weedMarket(100, 90)}
	a, ok := p.Next(context.Background(), stakeReader(), d)
	if !ok || a.Kind != ActionBreakout {
		t.Fatalf("jailed dealer should reach the breakout step, got %+v ok=%v", a, ok)
	}
}

func TestProgramClearStarsSkipsWhenNoHeat(t *testing.T) {
	state := newMemState()
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionClearStars}, // heat 0 → nothing to do
		ProgStep{Action: template.ActionTrade, BuyArea: manhattan, SellArea: amsterdam},
	), state, nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0) // heat 0
	d := Decision{Snap: Snapshot{TokenID: 4, State: st}, Area: weedMarket(100, 90)}
	a, ok := p.Next(context.Background(), stakeReader(), d)
	if !ok || a.Kind != ActionPVE {
		t.Fatalf("clear_stars with no heat should skip to trade, got %+v ok=%v", a, ok)
	}
	if step, _ := state.Get(4); step != 1 {
		t.Errorf("position should be the trade step (1), got %d", step)
	}
}

func TestProgramEmptyIdles(t *testing.T) {
	p := NewProgram(progFor(), newMemState(), nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0)
	if _, ok := p.Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 3, State: st}}); ok {
		t.Error("an empty program should idle")
	}
}

func TestProgramClearStarsThreshold(t *testing.T) {
	state := newMemState()
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionClearStars, HeatAt: 4}, // only at 4★+
		ProgStep{Action: template.ActionTrade, BuyArea: manhattan, SellArea: amsterdam},
	), state, nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0)
	st.HeatLevel = 3 // below the 4★ threshold → skip clear_stars
	d := Decision{Snap: Snapshot{TokenID: 5, State: st}, Area: weedMarket(100, 90)}
	if a, ok := p.Next(context.Background(), stakeReader(), d); !ok || a.Kind != ActionPVE {
		t.Fatalf("heat 3 < threshold 4 should skip to trade, got %+v ok=%v", a, ok)
	}
	st.HeatLevel = 4       // now at the threshold → clear
	_ = state.Set(5, 0, 0) // rewind to step 0
	if a, ok := p.Next(context.Background(), stakeReader(), d); !ok || a.Kind != ActionClearHeat {
		t.Fatalf("heat 4 ≥ threshold should clear, got %+v ok=%v", a, ok)
	}
}

func TestProgramBreakoutPayBailPerStep(t *testing.T) {
	st := pveState(manhattan, 5, 100000, 0)
	st.IsJailed = true
	st.CanBreakoutToday = false // free attempt already used today
	d := Decision{Snap: Snapshot{TokenID: 6, State: st}, Area: weedMarket(100, 90)}

	// PayBail opted in on the step → pay bail even with no global setting.
	p := NewProgram(progFor(ProgStep{Action: template.ActionBreakout, PayBail: true}), newMemState(), nil, nil, manhattan)
	if a, ok := p.Next(context.Background(), stakeReader(), d); !ok || a.Kind != ActionPayBail {
		t.Fatalf("breakout with PayBail should pay bail, got %+v ok=%v", a, ok)
	}
	// Without PayBail and no global → no bail (idle, waits for tomorrow's free try).
	p2 := NewProgram(progFor(ProgStep{Action: template.ActionBreakout}), newMemState(), nil, nil, manhattan)
	if _, ok := p2.Next(context.Background(), stakeReader(), d); ok {
		t.Error("breakout without PayBail should not pay bail")
	}
}

func TestProgramPvPEscapesBlackMarket(t *testing.T) {
	// A raider parked in the black market with energy but no targets there must
	// leave (travel home) rather than sit idle burning nothing.
	p := NewProgram(progFor(ProgStep{Action: template.ActionPvP}), newMemState(), nil, nil, manhattan)
	st := pveState(bindings.BlackMarketArea, 5, 100000, 0) // in the BM, has energy, no loot
	d := Decision{Snap: Snapshot{TokenID: 7, State: st}, Area: weedMarket(100, 90)}
	a, ok := p.Next(context.Background(), stakeReader(), d) // stakeReader has no targets
	if !ok || a.Kind != ActionTravel || a.DestArea != manhattan {
		t.Fatalf("stranded raider with energy should leave the black market, got %+v ok=%v", a, ok)
	}
}

func TestProgramPvPTradesWhenNoTarget(t *testing.T) {
	// A raider with energy but no target must deal drugs (stay productive) rather
	// than idle — so a pvp program with no trade step still spends the dealer's
	// energy. The fallback trade must not count toward the raid Count or advance.
	state := newMemState()
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionPvP, BuyArea: manhattan, SellArea: amsterdam, Count: 3},
	), state, nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0) // on Manhattan, energy, no loot
	d := Decision{Snap: Snapshot{TokenID: 10, State: st}, Area: weedMarket(100, 90)}
	a, ok := p.Next(context.Background(), stakeReader(), d) // stakeReader has no targets
	if !ok || a.Kind != ActionPVE || a.Hustle != bindings.HustleBuy {
		t.Fatalf("raider with energy and no target should trade (buy weed), got %+v ok=%v", a, ok)
	}
	if step, reps := state.Get(10); step != 0 || reps != 0 {
		t.Fatalf("a fallback trade shouldn't advance or count: pos=(%d,%d), want (0,0)", step, reps)
	}
}

func TestProgramMissionAcceptThenClaim(t *testing.T) {
	// The accept step checks in a not-yet-accepted mission; once accepted and
	// finished, the claim step claims it — the two concepts are independent steps.
	r := &fakeReader{missions: []bindings.MissionStatus{{
		TemplateID: big.NewInt(1),
		Mission:    bindings.MissionTemplate{Cadence: bindings.CadenceDaily, Enabled: true, Target: 5},
		CheckedIn:  false,
	}}}
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionMissionsAccept},
		ProgStep{Action: template.ActionMissionsClaim},
	), newMemState(), nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0)
	d := Decision{Snap: Snapshot{TokenID: 8, State: st}, Area: weedMarket(100, 90)}

	if a, ok := p.Next(context.Background(), r, d); !ok || a.Kind != ActionMissionCheckIn {
		t.Fatalf("accept step should check in, got %+v ok=%v", a, ok)
	}
	// Accepted + claimable now → accept idles, claim step claims.
	r.missions = []bindings.MissionStatus{{
		TemplateID: big.NewInt(1),
		Mission:    bindings.MissionTemplate{Cadence: bindings.CadenceDaily, Enabled: true, Target: 5},
		CheckedIn:  true, Claimable: true,
	}}
	if a, ok := p.Next(context.Background(), r, d); !ok || a.Kind != ActionMissionClaim {
		t.Fatalf("claim step should claim the finished mission, got %+v ok=%v", a, ok)
	}
}

func TestProgramClampsStaleIndex(t *testing.T) {
	// A persisted index past the (now shorter) program restarts at 0.
	state := newMemState()
	_ = state.Set(9, 7, 3)
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionTrade, BuyArea: manhattan, SellArea: amsterdam},
	), state, nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0)
	d := Decision{Snap: Snapshot{TokenID: 9, State: st}, Area: weedMarket(100, 90)}
	if a, ok := p.Next(context.Background(), stakeReader(), d); !ok || a.Kind != ActionPVE {
		t.Fatalf("stale index should clamp to 0 and trade, got %+v ok=%v", a, ok)
	}
}
