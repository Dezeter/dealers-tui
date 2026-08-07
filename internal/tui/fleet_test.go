package tui

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"dealers/internal/chain/bindings"
	"dealers/internal/dealer"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testDeps() Deps {
	return Deps{AreaNames: map[uint8]string{2: "Manhattan"}}
}

func testSnaps() []dealer.Snapshot {
	yes := true
	return []dealer.Snapshot{
		{
			TokenID: 1,
			State: &bindings.FullDealerState{
				IsInitialized: true, ReputationTitle: "Soldier", Reputation: big.NewInt(814),
				HeatLevel: 2, CashBalance: big.NewInt(40432), CurrentArea: 2,
				DailyAttemptsRemaining: 4, MaxAttempts: 5, Infamy: big.NewInt(0),
			},
			CheckedIn: &yes, MissionsKnown: true, MissionsClaimable: 2,
		},
		{TokenID: 2, Err: errors.New("boom")},
	}
}

func TestRenderCardHealthyAndErrored(t *testing.T) {
	snaps := testSnaps()
	card := stripANSI(renderCard(testDeps(), snaps[0], true, cardMaxWidth))
	for _, want := range []string{"#1", "Soldier", "IDLE", "814", "40432", "Heat", "Manhattan", "★2"} {
		if !strings.Contains(card, want) {
			t.Errorf("healthy card missing %q in:\n%s", want, card)
		}
	}
	// Errored card still renders with its id and ERR status.
	ecard := stripANSI(renderCard(testDeps(), snaps[1], false, cardMaxWidth))
	if !strings.Contains(ecard, "#2") || !strings.Contains(ecard, "ERR") {
		t.Errorf("errored card wrong:\n%s", ecard)
	}
}

func TestRenderCardFixedHeight(t *testing.T) {
	// Every card must be exactly cardHeight rows at ANY width (narrow widths must
	// clip, not wrap) so the grid layout stays aligned.
	longArea := Deps{AreaNames: map[uint8]string{2: "Very Long District Name"}}
	for _, w := range []int{cardMinWidth, 50, cardMaxWidth} {
		for _, s := range testSnaps() {
			h := lipgloss.Height(renderCard(longArea, s, false, w))
			if h != cardHeight {
				t.Errorf("card #%d at width %d: height %d, want %d", s.TokenID, w, h, cardHeight)
			}
		}
	}
}

func TestFleetSelection(t *testing.T) {
	m := NewFleet(testDeps())
	m, _ = m.Update(snapshotsMsg{snaps: testSnaps()})
	if id := m.SelectedTokenID(); id != 1 {
		t.Fatalf("initial selection = %d, want 1", id)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if id := m.SelectedTokenID(); id != 2 {
		t.Errorf("after ↓ selection = %d, want 2", id)
	}
}

func TestResponsiveColumns(t *testing.T) {
	m := NewFleet(testDeps())
	cases := []struct {
		width, wantCols int
	}{
		{0, 1},    // unknown width
		{50, 1},   // one card fits
		{100, 2},  // two columns
		{160, 3},  // three
		{400, 4},  // capped at maxCols
	}
	for _, c := range cases {
		m.width = c.width
		if got := m.cols(); got != c.wantCols {
			t.Errorf("width %d → cols %d, want %d", c.width, got, c.wantCols)
		}
	}
}

func TestGridNavigation(t *testing.T) {
	m := NewFleet(testDeps())
	m.width = 100 // 2 columns
	m.vpRows = 10
	four := make([]dealer.Snapshot, 4)
	for i := range four {
		four[i] = dealer.Snapshot{TokenID: uint64(10 + i)}
	}
	m, _ = m.Update(snapshotsMsg{snaps: four}) // #10 #11 / #12 #13
	// right → #11
	m.move(1)
	if id := m.SelectedTokenID(); id != 11 {
		t.Errorf("right → %d, want 11", id)
	}
	// down (by cols=2) from index 1 → index 3 → #13
	m.move(m.cols())
	if id := m.SelectedTokenID(); id != 13 {
		t.Errorf("down → %d, want 13", id)
	}
	// up → back to #11
	m.move(-m.cols())
	if id := m.SelectedTokenID(); id != 11 {
		t.Errorf("up → %d, want 11", id)
	}
}

func TestMissionCell(t *testing.T) {
	cases := []struct {
		name string
		snap dealer.Snapshot
		want string
	}{
		{"claimable", dealer.Snapshot{MissionsKnown: true, MissionsClaimable: 2}, "★2"},
		{"need-accept", dealer.Snapshot{MissionsKnown: true, MissionsNeedCheckIn: true}, "○"},
		{"all-good", dealer.Snapshot{MissionsKnown: true}, "✓"},
		{"unknown", dealer.Snapshot{}, "-"},
	}
	for _, c := range cases {
		if got := missionCell(c.snap); got != c.want {
			t.Errorf("%s: missionCell = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCheckInCell(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name string
		snap dealer.Snapshot
		want string
	}{
		{"done", dealer.Snapshot{CheckedIn: &yes}, "✓"},
		{"pending", dealer.Snapshot{CheckedIn: &no}, "○"},
		{"unknown", dealer.Snapshot{}, "-"},
	}
	for _, c := range cases {
		if got := checkInCell(c.snap); got != c.want {
			t.Errorf("%s: checkInCell = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestStatusDerivation(t *testing.T) {
	cases := []struct {
		name string
		s    dealer.Snapshot
		want string
	}{
		{"jailed", dealer.Snapshot{State: &bindings.FullDealerState{IsInitialized: true, IsJailed: true}}, "JAILED"},
		{"safehouse", dealer.Snapshot{State: &bindings.FullDealerState{IsInitialized: true, IsInSafeHouse: true}}, "SAFEHOUSE"},
		{"idle", dealer.Snapshot{State: &bindings.FullDealerState{IsInitialized: true}}, "IDLE"},
		{"uninit", dealer.Snapshot{State: &bindings.FullDealerState{}}, "UNINIT"},
		{"err", dealer.Snapshot{Err: errors.New("x")}, "ERR"},
	}
	for _, c := range cases {
		if got := c.s.Status(); got != c.want {
			t.Errorf("%s: Status()=%q want %q", c.name, got, c.want)
		}
	}
}
