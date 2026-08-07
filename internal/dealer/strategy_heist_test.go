package dealer

import (
	"context"
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
)

func weeklyHeistMission(metric uint8, progress, target uint32) bindings.MissionStatus {
	return bindings.MissionStatus{
		TemplateID: big.NewInt(int64(metric) + 200),
		Mission:    bindings.MissionTemplate{Metric: metric, Cadence: bindings.CadenceWeekly, Enabled: true, Target: target},
		CheckedIn:  true, Progress: progress,
	}
}

func richState(rep, cash int64, attempts uint8) *bindings.FullDealerState {
	return &bindings.FullDealerState{
		IsInitialized: true, CurrentArea: manhattan, DailyAttemptsRemaining: attempts,
		Reputation: big.NewInt(rep), CashBalance: big.NewInt(cash),
	}
}

func TestMaxHeistDifficulty(t *testing.T) {
	cases := []struct {
		rep, cash int64
		wantDiff  uint8
		wantOK    bool
	}{
		{12000, 100000, 2, true}, // meets max gate
		{2000, 5000, 1, true},    // meets d1 (1500/4000), not d2
		{800, 1000, 0, true},     // only d0 (600/600)
		{500, 1000, 0, false},    // below even d0 rep gate
		{800, 100, 0, false},     // can't afford d0 stake
	}
	for _, c := range cases {
		d, ok := maxHeistDifficulty(richState(c.rep, c.cash, 5))
		if ok != c.wantOK || (ok && d != c.wantDiff) {
			t.Errorf("rep %d cash %d → (%d,%v), want (%d,%v)", c.rep, c.cash, d, ok, c.wantDiff, c.wantOK)
		}
	}
}

func TestHeistStartsAtMaxDifficulty(t *testing.T) {
	r := &fakeReader{missions: []bindings.MissionStatus{weeklyHeistMission(bindings.MetricHeistStages, 0, 5)}}
	d := Decision{Snap: Snapshot{TokenID: 1, State: richState(12000, 100000, 5)}}
	a, ok := heistMissionStep(context.Background(), r, d, newDailyLimiter())
	if !ok || a.Kind != ActionStartHeist || a.HeistDifficulty != 2 || a.HeistFamily != bindings.FamilyCash {
		t.Fatalf("want start heist at max difficulty, got %+v ok=%v", a, ok)
	}
}

func TestHeistCommitsUntilCashable(t *testing.T) {
	// Below the cashable stage → commit one more stage to reach it.
	r := &fakeReader{
		missions:    []bindings.MissionStatus{weeklyHeistMission(bindings.MetricHeistStages, 1, 5)},
		activeHeist: 500,
		heist:       &bindings.DailyHeist{Status: uint8(bindings.HeistRevealedWin), CurrentStage: 1},
	}
	d := Decision{Snap: Snapshot{TokenID: 1, State: richState(12000, 100000, 5)}}
	a, ok := heistMissionStep(context.Background(), r, d, newDailyLimiter())
	if !ok || a.Kind != ActionHeistStage || a.HeistID != 500 {
		t.Fatalf("stage 1 (below cashable) → commit next stage, got %+v ok=%v", a, ok)
	}
	// Pre-stage → commit the first stage too.
	r.heist = &bindings.DailyHeist{Status: uint8(bindings.HeistPreStage)}
	a, ok = heistMissionStep(context.Background(), r, d, newDailyLimiter())
	if !ok || a.Kind != ActionHeistStage {
		t.Fatalf("pre-stage → commit stage, got %+v ok=%v", a, ok)
	}
}

func TestHeistBanksAtCashableStage(t *testing.T) {
	// Revealed-win at stage ≥ 2 → cash out (bank & end the run), NOT push further —
	// this is the "hung at stage 2" fix.
	for _, done := range []bool{false, true} { // whether the mission is already complete
		prog := uint32(1)
		if done {
			prog = 5
		}
		r := &fakeReader{
			missions:    []bindings.MissionStatus{weeklyHeistMission(bindings.MetricHeistStages, prog, 5)},
			activeHeist: 500,
			heist:       &bindings.DailyHeist{Status: uint8(bindings.HeistRevealedWin), CurrentStage: 2},
		}
		d := Decision{Snap: Snapshot{TokenID: 1, State: richState(12000, 100000, 5)}}
		a, ok := heistMissionStep(context.Background(), r, d, newDailyLimiter())
		if !ok || a.Kind != ActionHeistCashOut || a.HeistID != 500 {
			t.Fatalf("done=%v: cashable stage → cash out, got %+v ok=%v", done, a, ok)
		}
	}
}

func TestHeistNoNewRunWhenMissionDone(t *testing.T) {
	// Mission complete, no active heist → do NOT launch anymore.
	r := &fakeReader{missions: []bindings.MissionStatus{weeklyHeistMission(bindings.MetricHeistRuns, 3, 3)}}
	d := Decision{Snap: Snapshot{TokenID: 1, State: richState(12000, 100000, 5)}}
	if _, ok := heistMissionStep(context.Background(), r, d, newDailyLimiter()); ok {
		t.Error("completed heist mission should not start new runs")
	}
}

func TestHeistSkippedWithoutHeistMission(t *testing.T) {
	// Only a PvE weekly mission on the board → no heist activity (and no heist reads).
	r := &fakeReader{
		missions:    []bindings.MissionStatus{weeklyMission(bindings.MetricPVEGames, 0, 5)},
		activeHeist: 999, // would be used if the step didn't short-circuit
	}
	d := Decision{Snap: Snapshot{TokenID: 1, State: richState(12000, 100000, 5)}}
	if _, ok := heistMissionStep(context.Background(), r, d, newDailyLimiter()); ok {
		t.Error("no heist mission on the board → heist step must yield")
	}
}

func TestHeistDailyRunCap(t *testing.T) {
	// After heistRunsPerDay starts, the step yields so deals get the energy.
	r := &fakeReader{missions: []bindings.MissionStatus{weeklyHeistMission(bindings.MetricHeistRuns, 0, 99)}}
	d := Decision{Snap: Snapshot{TokenID: 1, State: richState(12000, 100000, 5)}}
	limiter := newDailyLimiter()
	for i := 0; i < heistRunsPerDay; i++ {
		if _, ok := heistMissionStep(context.Background(), r, d, limiter); !ok {
			t.Fatalf("start %d should be allowed (under cap)", i+1)
		}
	}
	if _, ok := heistMissionStep(context.Background(), r, d, limiter); ok {
		t.Errorf("start beyond the daily cap should yield to the strategy")
	}
	// A different dealer is unaffected by another's cap.
	d2 := Decision{Snap: Snapshot{TokenID: 2, State: richState(12000, 100000, 5)}}
	if _, ok := heistMissionStep(context.Background(), r, d2, limiter); !ok {
		t.Error("the cap is per-dealer, not global")
	}
}

func TestHeistNoStartWithoutEnergy(t *testing.T) {
	r := &fakeReader{missions: []bindings.MissionStatus{weeklyHeistMission(bindings.MetricHeistRuns, 0, 3)}}
	d := Decision{Snap: Snapshot{TokenID: 1, State: richState(12000, 100000, 0)}} // 0 attempts
	if _, ok := heistMissionStep(context.Background(), r, d, newDailyLimiter()); ok {
		t.Error("no daily attempts → can't start a heist")
	}
}
