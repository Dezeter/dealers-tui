package dealer

import (
	"context"
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
	"dealers/internal/template"
)

func progFor(steps ...ProgStep) func(uint64) []ProgStep {
	return func(uint64) []ProgStep { return steps }
}

func TestProgramTradeDailyCap(t *testing.T) {
	// Count is a per-day cap: a trade step with Count=2 acts twice, then yields to
	// the lower-priority clear_stars step (which has heat to clear).
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionTrade, BuyArea: manhattan, SellArea: amsterdam, Count: 2},
		ProgStep{Action: template.ActionClearStars},
	), nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0)
	st.HeatLevel = 5 // so clear_stars has work once trade is capped
	d := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}
	r := stakeReader()

	// ticks 1 & 2: trade wins (a PvE buy each), counting toward the cap.
	for i := 1; i <= 2; i++ {
		if a, ok := p.Next(context.Background(), r, d); !ok || a.Kind != ActionPVE {
			t.Fatalf("tick %d = %+v ok=%v, want a PvE buy", i, a, ok)
		}
	}
	// tick 3: trade has hit its daily cap → clear_stars takes the tick.
	if a, ok := p.Next(context.Background(), r, d); !ok || a.Kind != ActionClearHeat {
		t.Fatalf("after the trade cap, clear_stars should run, got %+v ok=%v", a, ok)
	}
}

func TestProgramBreakoutReachedWhenJailed(t *testing.T) {
	// Breakout is the LAST step; a jailed dealer's earlier steps are non-actionable,
	// so the priority scan skips them and reaches breakout the same tick.
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionTrade, BuyArea: manhattan, SellArea: amsterdam},
		ProgStep{Action: template.ActionBreakout},
	), nil, nil, manhattan)
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
	// clear_stars first but no heat → it's non-actionable, so trade takes the tick.
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionClearStars},
		ProgStep{Action: template.ActionTrade, BuyArea: manhattan, SellArea: amsterdam},
	), nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0) // heat 0
	d := Decision{Snap: Snapshot{TokenID: 4, State: st}, Area: weedMarket(100, 90)}
	if a, ok := p.Next(context.Background(), stakeReader(), d); !ok || a.Kind != ActionPVE {
		t.Fatalf("clear_stars with no heat should skip to trade, got %+v ok=%v", a, ok)
	}
}

func TestProgramEmptyIdles(t *testing.T) {
	p := NewProgram(progFor(), nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0)
	if _, ok := p.Next(context.Background(), stakeReader(), Decision{Snap: Snapshot{TokenID: 3, State: st}}); ok {
		t.Error("an empty program should idle")
	}
}

func TestProgramClearStarsThreshold(t *testing.T) {
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionClearStars, HeatAt: 4}, // only at 4★+
		ProgStep{Action: template.ActionTrade, BuyArea: manhattan, SellArea: amsterdam},
	), nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0)
	st.HeatLevel = 3 // below the 4★ threshold → clear_stars skips, trade runs
	d := Decision{Snap: Snapshot{TokenID: 5, State: st}, Area: weedMarket(100, 90)}
	if a, ok := p.Next(context.Background(), stakeReader(), d); !ok || a.Kind != ActionPVE {
		t.Fatalf("heat 3 < threshold 4 should skip to trade, got %+v ok=%v", a, ok)
	}
	st.HeatLevel = 4 // now at the threshold → clear_stars wins (it's first)
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
	p := NewProgram(progFor(ProgStep{Action: template.ActionBreakout, PayBail: true}), nil, nil, manhattan)
	if a, ok := p.Next(context.Background(), stakeReader(), d); !ok || a.Kind != ActionPayBail {
		t.Fatalf("breakout with PayBail should pay bail, got %+v ok=%v", a, ok)
	}
	// Without PayBail and no global → no bail (idle, waits for tomorrow's free try).
	p2 := NewProgram(progFor(ProgStep{Action: template.ActionBreakout}), nil, nil, manhattan)
	if _, ok := p2.Next(context.Background(), stakeReader(), d); ok {
		t.Error("breakout without PayBail should not pay bail")
	}
}

func TestProgramPvPEscapesBlackMarket(t *testing.T) {
	// A raider parked in the black market with energy but no targets there must
	// leave (travel home) rather than sit idle burning nothing.
	p := NewProgram(progFor(ProgStep{Action: template.ActionPvP}), nil, nil, manhattan)
	st := pveState(bindings.BlackMarketArea, 5, 100000, 0) // in the BM, has energy, no loot
	d := Decision{Snap: Snapshot{TokenID: 7, State: st}, Area: weedMarket(100, 90)}
	a, ok := p.Next(context.Background(), stakeReader(), d) // stakeReader has no targets
	if !ok || a.Kind != ActionTravel || a.DestArea != manhattan {
		t.Fatalf("stranded raider with energy should leave the black market, got %+v ok=%v", a, ok)
	}
}

func TestProgramPvPTradesWhenNoTarget(t *testing.T) {
	// A raider with energy but no target must deal drugs (stay productive) rather
	// than idle. Those fallback trades DON'T count toward the raid Count cap — so
	// even with Count=3 the step keeps trading well past 3 ticks.
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionPvP, BuyArea: manhattan, SellArea: amsterdam, Count: 3},
	), nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0) // on Manhattan, energy, no loot
	d := Decision{Snap: Snapshot{TokenID: 10, State: st}, Area: weedMarket(100, 90)}
	for i := 1; i <= 5; i++ { // more than Count — proves fallback trades don't cap
		a, ok := p.Next(context.Background(), stakeReader(), d) // no targets
		if !ok || a.Kind != ActionPVE || a.Hustle != bindings.HustleBuy {
			t.Fatalf("tick %d: raider with no target should trade (buy weed), got %+v ok=%v", i, a, ok)
		}
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
	), nil, nil, manhattan)
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

func TestProgramPriorityMissionsBeforeGreedyTrade(t *testing.T) {
	// The regression this fixes: a greedy trade step must NOT starve an earlier
	// maintenance step. missions_accept sits above an uncapped trade; while a
	// mission needs accepting it wins every tick, even though trade is always ready.
	r := stakeReader() // supports trading (stake params) so trade can take over
	r.missions = []bindings.MissionStatus{{
		TemplateID: big.NewInt(1),
		Mission:    bindings.MissionTemplate{Cadence: bindings.CadenceDaily, Enabled: true, Target: 5},
		CheckedIn:  false,
	}}
	p := NewProgram(progFor(
		ProgStep{Action: template.ActionMissionsAccept},
		ProgStep{Action: template.ActionTrade, BuyArea: manhattan, SellArea: amsterdam}, // greedy, uncapped
	), nil, nil, manhattan)
	st := pveState(manhattan, 5, 100000, 0)
	d := Decision{Snap: Snapshot{TokenID: 11, State: st}, Area: weedMarket(100, 90)}

	if a, ok := p.Next(context.Background(), r, d); !ok || a.Kind != ActionMissionCheckIn {
		t.Fatalf("accept must win over the greedy trade below it, got %+v ok=%v", a, ok)
	}
	// Once accepted, the accept step idles and trade takes over.
	r.missions[0].CheckedIn = true
	if a, ok := p.Next(context.Background(), r, d); !ok || a.Kind != ActionPVE {
		t.Fatalf("with nothing to accept, trade should run, got %+v ok=%v", a, ok)
	}
}
