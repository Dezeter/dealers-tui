package dealer

import (
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
)

func TestFleetAlerts(t *testing.T) {
	snaps := []Snapshot{
		{TokenID: 1, State: &bindings.FullDealerState{IsInitialized: true, HeatLevel: 2}},
		{TokenID: 2, State: &bindings.FullDealerState{IsInitialized: true, HeatLevel: 5}}, // warn
		{TokenID: 3, State: &bindings.FullDealerState{IsInitialized: true, IsJailed: true}}, // crit
		{TokenID: 4, Err: errNil()},                                                         // ignored
	}
	// Balance below runway → crit.
	alerts := FleetAlerts(snaps, big.NewInt(1e15), big.NewInt(5e15))

	if len(alerts) != 3 {
		t.Fatalf("got %d alerts, want 3: %+v", len(alerts), alerts)
	}
	// Crits sort first.
	if alerts[0].Level != AlertCrit || alerts[len(alerts)-1].Level != AlertWarn {
		t.Errorf("alerts not sorted crit-first: %+v", alerts)
	}
	// The heat-5 dealer produces a warn; the healthy heat-2 one does not.
	var haveHeat, haveJail, haveEth bool
	for _, a := range alerts {
		switch {
		case a.Text == "#2 heat 5/5":
			haveHeat = true
		case a.Text == "#3 JAILED":
			haveJail = true
		}
		if a.Level == AlertCrit && len(a.Text) > 3 && a.Text[:3] == "ETH" {
			haveEth = true
		}
	}
	if !haveHeat || !haveJail || !haveEth {
		t.Errorf("missing expected alerts: heat=%v jail=%v eth=%v (%+v)", haveHeat, haveJail, haveEth, alerts)
	}
}

func TestFleetAlertsHealthy(t *testing.T) {
	snaps := []Snapshot{{TokenID: 1, State: &bindings.FullDealerState{IsInitialized: true, HeatLevel: 1}}}
	if a := FleetAlerts(snaps, big.NewInt(1e18), big.NewInt(5e15)); len(a) != 0 {
		t.Errorf("expected no alerts for a healthy fleet, got %+v", a)
	}
}

func errNil() error { return nil }
