package engine

import (
	"testing"
	"time"

	"dealers/internal/dealer"
)

func TestAutopilotDefaultOffAndToggle(t *testing.T) {
	ap := NewAutopilot(nil, nil, nil, dealer.ManualStrategy{}, nil, nil)
	if ap.Enabled() {
		t.Fatal("autopilot must start DISABLED")
	}
	ap.SetEnabled(true)
	if !ap.Enabled() {
		t.Error("SetEnabled(true) did not enable")
	}
	ap.SetEnabled(false)
	if ap.Enabled() {
		t.Error("SetEnabled(false) did not disable")
	}
}

func TestSettleGatePreventsDoubleMove(t *testing.T) {
	ap := NewAutopilot(nil, nil, nil, dealer.ManualStrategy{}, nil, nil)
	now := time.Now()

	// No pending move → free to act.
	if !ap.clearedToAct(1, 1, now) {
		t.Fatal("with no pending move the dealer should be free to act")
	}

	// Issue a travel to area 7; while the state still shows the old area (1), the
	// dealer must NOT act again (this is what stopped the double-move).
	ap.settling[1] = settleMove{area: 7, deadline: now.Add(settleTimeout)}
	if ap.clearedToAct(1, 1, now) {
		t.Error("still in transit (area 1 ≠ dest 7) → should be gated")
	}
	// Gate persists across ticks until arrival.
	if ap.clearedToAct(1, 1, now.Add(time.Second)) {
		t.Error("gate must hold until arrival or timeout")
	}
	// State now reflects arrival → gate clears and the dealer acts.
	if !ap.clearedToAct(1, 7, now.Add(2*time.Second)) {
		t.Error("arrived at dest → should be cleared to act")
	}
	if _, still := ap.settling[1]; still {
		t.Error("gate should be removed after arrival")
	}

	// Timeout path: never arrives, but the deadline lapses → we act anyway.
	ap.settling[2] = settleMove{area: 9, deadline: now}
	if !ap.clearedToAct(2, 3, now.Add(settleTimeout+time.Second)) {
		t.Error("past the deadline the gate should give up and allow acting")
	}
}

func TestTravelGateArmsOnAttempt(t *testing.T) {
	now := time.Now()
	// A real travel (dest ≠ current) arms the gate — regardless of the send result.
	if mv, ok := travelGate(dealer.Action{Kind: dealer.ActionTravel, DestArea: 7}, 1, now); !ok || mv.area != 7 {
		t.Errorf("travel to a new area should arm the gate, got %+v ok=%v", mv, ok)
	}
	// Travelling to the area we're already in is a no-op → no gate.
	if _, ok := travelGate(dealer.Action{Kind: dealer.ActionTravel, DestArea: 1}, 1, now); ok {
		t.Error("travel to the current area should not gate")
	}
	// Non-travel actions never gate.
	if _, ok := travelGate(dealer.Action{Kind: dealer.ActionPVE}, 1, now); ok {
		t.Error("non-travel action should not gate")
	}
}
