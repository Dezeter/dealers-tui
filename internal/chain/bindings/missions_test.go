package bindings

import (
	"math/big"
	"testing"
)

func status(id uint64, cadence uint8, enabled, checkedIn, claimable, claimed bool) MissionStatus {
	return MissionStatus{
		TemplateID: new(big.Int).SetUint64(id),
		Mission:    MissionTemplate{Cadence: cadence, Enabled: enabled},
		CheckedIn:  checkedIn, Claimable: claimable, Claimed: claimed,
	}
}

func TestFirstClaimablePrefersDaily(t *testing.T) {
	ms := []MissionStatus{
		status(2, CadenceWeekly, true, true, true, false),
		status(1, CadenceDaily, true, true, true, false),
	}
	id, ok := FirstClaimable(ms)
	if !ok || id != 1 {
		t.Fatalf("FirstClaimable = %d ok=%v, want daily #1", id, ok)
	}
	// Already-claimed and non-claimable are ignored.
	none := []MissionStatus{
		status(3, CadenceDaily, true, true, true, true),   // claimed
		status(4, CadenceWeekly, true, true, false, false), // not claimable
	}
	if _, ok := FirstClaimable(none); ok {
		t.Error("no claimable-unclaimed mission should report ok=false")
	}
}

func TestNeedsCheckIn(t *testing.T) {
	if !NeedsCheckIn([]MissionStatus{status(1, CadenceDaily, true, false, false, false)}) {
		t.Error("an enabled, not-checked-in mission needs check-in")
	}
	if NeedsCheckIn([]MissionStatus{status(1, CadenceDaily, true, true, false, false)}) {
		t.Error("already checked in → no check-in needed")
	}
	// Disabled missions don't force a check-in.
	if NeedsCheckIn([]MissionStatus{status(1, CadenceDaily, false, false, false, false)}) {
		t.Error("disabled mission should not require check-in")
	}
}

func TestGetMissionStatusABIRoundTrip(t *testing.T) {
	// The nested tuple[] ABI must decode into []MissionStatus without error.
	want := []MissionStatus{{
		TemplateID: big.NewInt(1),
		Mission: MissionTemplate{
			Metric: 2, Cadence: CadenceDaily, Enabled: true, Target: 3,
			RepReward: 50, InfamyReward: 0, CashReward: big.NewInt(1000), DrugID: 0, DrugAmount: 0,
		},
		Epoch: big.NewInt(42), EpochEndsAt: 1790000000, Progress: 2, CheckedIn: true, Claimable: false, Claimed: false,
	}}
	packed, err := missionsABI.Methods["getMissionStatus"].Outputs.Pack(want)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	vals, err := missionsABI.Unpack("getMissionStatus", packed)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	got := *abiConvert[[]MissionStatus](vals[0])
	if len(got) != 1 || got[0].Mission.Target != 3 || got[0].Progress != 2 || !got[0].CheckedIn {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
