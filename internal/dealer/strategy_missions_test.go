package dealer

import (
	"context"
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
)

func TestClassifyMetric(t *testing.T) {
	cases := map[uint8]metricClass{
		bindings.MetricPVEWins:        classPVE,
		bindings.MetricPVEGames:       classPVE,
		bindings.MetricPVPAttackWins:  classPVP,
		bindings.MetricPVPGames:       classPVP,
		bindings.MetricAnyGames:       classAny,
		bindings.MetricPVPDefendWins:  classPassive,
		bindings.MetricHeistRuns:      classPassive,
		bindings.MetricRepGain:        classPassive,
		bindings.MetricInfamyGain:     classPassive,
		bindings.MetricMissionsClaimed: classPassive,
	}
	for metric, want := range cases {
		if got := classifyMetric(metric); got != want {
			t.Errorf("classifyMetric(%d) = %d, want %d", metric, got, want)
		}
	}
}

// dailyMission builds a checked-in, in-progress daily mission of a given metric.
func dailyMission(metric uint8, progress, target uint32) bindings.MissionStatus {
	return bindings.MissionStatus{
		TemplateID: big.NewInt(int64(metric) + 1),
		Mission:    bindings.MissionTemplate{Metric: metric, Cadence: bindings.CadenceDaily, Enabled: true, Target: target},
		CheckedIn:  true, Progress: progress,
	}
}

func steerState() *bindings.FullDealerState {
	return &bindings.FullDealerState{
		IsInitialized: true, CurrentArea: manhattan, DailyAttemptsRemaining: 5,
		CashBalance: big.NewInt(100000), Reputation: big.NewInt(500),
	}
}

func TestMissionSteerPvEStrategyOverriddenByPvPMission(t *testing.T) {
	// A PvE-strategy dealer with a PvP daily mission → steer to a PvP attack.
	r := &fakeReader{
		missions: []bindings.MissionStatus{dailyMission(bindings.MetricPVPGames, 0, 3)},
		targets:  []bindings.PVPTarget{{TokenID: big.NewInt(99), CanAttackNow: true}},
	}
	d := Decision{Snap: Snapshot{TokenID: 1, State: steerState()}, Area: weedMarket(100, 90)}
	a, ok := missionSteer(context.Background(), r, d, classPVE, nil, "")
	if !ok || a.Kind != ActionPVP || a.DefenderID != 99 {
		t.Fatalf("PvE dealer w/ PvP mission should attack, got %+v ok=%v", a, ok)
	}
}

func TestMissionSteerLeavesMatchingMissionToStrategy(t *testing.T) {
	// A PvE-strategy dealer with a PvE daily mission → no steer (strategy handles it).
	r := &fakeReader{missions: []bindings.MissionStatus{dailyMission(bindings.MetricPVEGames, 0, 5)}}
	d := Decision{Snap: Snapshot{TokenID: 1, State: steerState()}, Area: weedMarket(100, 90)}
	if _, ok := missionSteer(context.Background(), r, d, classPVE, nil, ""); ok {
		t.Error("matching-class mission should be left to the strategy")
	}
	// ANY_GAMES is covered by any strategy → never steered.
	r = &fakeReader{missions: []bindings.MissionStatus{dailyMission(bindings.MetricAnyGames, 0, 5)}}
	if _, ok := missionSteer(context.Background(), r, d, classPVE, nil, ""); ok {
		t.Error("ANY_GAMES is covered by every strategy → no steer")
	}
}

func TestMissionSteerCompletedMissionReturnsToStrategy(t *testing.T) {
	// PvP mission already at target on a PvE dealer → no steer (back to strategy).
	r := &fakeReader{missions: []bindings.MissionStatus{dailyMission(bindings.MetricPVPGames, 3, 3)}}
	d := Decision{Snap: Snapshot{TokenID: 1, State: steerState()}, Area: weedMarket(100, 90)}
	if _, ok := missionSteer(context.Background(), r, d, classPVE, nil, ""); ok {
		t.Error("completed mission should stop steering")
	}
}

func TestMissionSteerPvPStrategyOverriddenByPvEMission(t *testing.T) {
	// A PvP-strategy dealer with a PvE daily mission → steer to a PvE deal.
	r := &fakeReader{missions: []bindings.MissionStatus{dailyMission(bindings.MetricPVEWins, 0, 3)}}
	d := Decision{Snap: Snapshot{TokenID: 1, State: steerState()}, Area: weedMarket(100, 90)}
	a, ok := missionSteer(context.Background(), r, d, classPVP, nil, "")
	if !ok || a.Kind != ActionPVE || a.Hustle != bindings.HustleBuy {
		t.Fatalf("PvP dealer w/ PvE mission should do a PvE deal, got %+v ok=%v", a, ok)
	}
}

// weeklyMission builds a checked-in, in-progress weekly mission of a given metric.
func weeklyMission(metric uint8, progress, target uint32) bindings.MissionStatus {
	m := dailyMission(metric, progress, target)
	m.Mission.Cadence = bindings.CadenceWeekly
	m.TemplateID = big.NewInt(int64(metric) + 100)
	return m
}

func TestMissionSteerWeeklyWhenNoDailyNeeds(t *testing.T) {
	// PvE dealer: daily mission is a matching PvE one (no steer), weekly needs PvP →
	// steer to the weekly PvP action (weekly still beats the strategy).
	r := &fakeReader{
		missions: []bindings.MissionStatus{
			dailyMission(bindings.MetricPVEGames, 0, 5),  // covered by strategy
			weeklyMission(bindings.MetricPVPGames, 0, 3), // needs steering
		},
		targets: []bindings.PVPTarget{{TokenID: big.NewInt(77), CanAttackNow: true}},
	}
	d := Decision{Snap: Snapshot{TokenID: 1, State: steerState()}, Area: weedMarket(100, 90)}
	a, ok := missionSteer(context.Background(), r, d, classPVE, nil, "")
	if !ok || a.Kind != ActionPVP || a.DefenderID != 77 {
		t.Fatalf("weekly PvP mission should steer, got %+v ok=%v", a, ok)
	}
}

func TestMissionSteerDailyBeatsWeekly(t *testing.T) {
	// Both a daily and a weekly mission need steering → daily wins.
	r := &fakeReader{
		missions: []bindings.MissionStatus{
			weeklyMission(bindings.MetricPVPGames, 0, 3), // weekly PvP (listed first)
			dailyMission(bindings.MetricPVEGames, 0, 5),  // daily PvE
		},
		targets: []bindings.PVPTarget{{TokenID: big.NewInt(88), CanAttackNow: true}},
	}
	// PvP-strategy dealer: daily PvE mission mismatches → should do the DAILY PvE
	// deal, not the weekly PvP one, even though weekly is listed first.
	d := Decision{Snap: Snapshot{TokenID: 1, State: steerState()}, Area: weedMarket(100, 90)}
	a, ok := missionSteer(context.Background(), r, d, classPVP, nil, "")
	if !ok || a.Kind != ActionPVE {
		t.Fatalf("daily mission must take priority over weekly, got %+v ok=%v", a, ok)
	}
}

func TestStarsClearedBeforeMissions(t *testing.T) {
	// Heat 3★ AND a claimable mission → clear stars FIRST, then (next tick) claim.
	s := NewPvEArbitrage(manhattan, amsterdam, nil, nil, nil)
	st := pveState(manhattan, 5, 100000, 0)
	st.HeatLevel = 3
	r := &fakeReader{missions: []bindings.MissionStatus{
		{TemplateID: big.NewInt(5), Mission: bindings.MissionTemplate{Cadence: bindings.CadenceDaily, Enabled: true}, CheckedIn: true, Claimable: true},
	}}
	dec := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}
	a, ok := s.Next(context.Background(), r, dec)
	if !ok || a.Kind != ActionClearHeat {
		t.Fatalf("stars should be cleared before claiming, got %+v ok=%v", a, ok)
	}
	// Heat cleared → next tick claims the mission.
	st.HeatLevel = 0
	a2, ok := s.Next(context.Background(), r, dec)
	if !ok || a2.Kind != ActionMissionClaim {
		t.Fatalf("after clearing stars, should claim the mission, got %+v ok=%v", a2, ok)
	}
}

func TestMissionSteerRespectsAlliesAndEnergy(t *testing.T) {
	// Only target is an ally → no attack (yield to strategy).
	r := &fakeReader{
		missions: []bindings.MissionStatus{dailyMission(bindings.MetricPVPGames, 0, 3)},
		targets:  []bindings.PVPTarget{{TokenID: big.NewInt(50), CanAttackNow: true}},
	}
	d := Decision{Snap: Snapshot{TokenID: 1, State: steerState()}, Area: weedMarket(100, 90)}
	if _, ok := missionSteer(context.Background(), r, d, classPVE, func(id uint64) bool { return id == 50 }, ""); ok {
		t.Error("should not attack an ally to complete a mission")
	}
	// Out of energy → no steer (games cost an attempt).
	st := steerState()
	st.DailyAttemptsRemaining = 0
	d2 := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}
	if _, ok := missionSteer(context.Background(), r, d2, classPVE, nil, ""); ok {
		t.Error("no energy → no steering")
	}
}
