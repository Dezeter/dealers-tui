package tui

import (
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"dealers/internal/allies"
	"dealers/internal/chain/bindings"
	"dealers/internal/config"
	"dealers/internal/dealer"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func drugCatalog() []bindings.DrugBalance {
	return []bindings.DrugBalance{
		{DrugID: big.NewInt(0), Name: "Weed", Balance: big.NewInt(12), Rarity: 0},
		{DrugID: big.NewInt(1), Name: "Coke", Balance: big.NewInt(0), Rarity: 2},
		{DrugID: big.NewInt(2), Name: "Meth", Balance: big.NewInt(3), Rarity: 1},
	}
}

func areaDrugCatalog() []bindings.AreaDrug {
	return []bindings.AreaDrug{
		{DrugID: big.NewInt(0), Name: "Weed", BuyPrice: big.NewInt(1), SellPrice: big.NewInt(1), IsAvailable: true},
		{DrugID: big.NewInt(1), Name: "Coke", BuyPrice: big.NewInt(120), SellPrice: big.NewInt(100), IsAvailable: true},
		{DrugID: big.NewInt(2), Name: "Meth", BuyPrice: big.NewInt(50), SellPrice: big.NewInt(40), IsAvailable: true},
	}
}

func seededDetail() DetailModel {
	snap := dealer.Snapshot{
		TokenID: 1,
		State: &bindings.FullDealerState{
			IsInitialized:          true,
			ReputationTitle:        "Soldier",
			Reputation:             big.NewInt(814),
			HeatLevel:              0,
			CashBalance:            big.NewInt(40432),
			CurrentArea:            2,
			DailyAttemptsRemaining: 5,
			MaxAttempts:            5,
			Infamy:                 big.NewInt(0),
			DrugBalances:           drugCatalog(),
		},
	}
	return NewDetail(Deps{Net: "mainnet"}, 1, snap) // Manager nil → read-only
}

func TestDetailViewRenders(t *testing.T) {
	v := seededDetail().View()
	for _, want := range []string{"Dealer #1", "Soldier", "40432", "Stash", "Weed", "Pending", "read-only"} {
		if !strings.Contains(v, want) {
			t.Errorf("detail view missing %q", want)
		}
	}
}

func TestOpenFormReadOnlyBlocked(t *testing.T) {
	m := seededDetail()
	m, _ = m.openForm(bindings.HustleBuy)
	if m.formOpen {
		t.Error("form opened despite nil Manager (read-only)")
	}
	if !strings.Contains(m.notice, "read-only") {
		t.Errorf("expected read-only notice, got %q", m.notice)
	}
}

func TestConfirmReadOnlyBlockedAndCancel(t *testing.T) {
	m := seededDetail() // Manager nil → read-only

	// Read-only: reset-attempts must not arm a confirm; shows a notice.
	m, _ = m.askConfirm("resetattempts", "Reset daily attempts")
	if m.confirm != "" {
		t.Error("armed confirm despite read-only")
	}
	if !strings.Contains(m.notice, "read-only") {
		t.Errorf("expected read-only notice, got %q", m.notice)
	}

	// State machine: an armed confirm is cancelled by n and by esc.
	m.confirm = "resetattempts"
	m, _ = m.updateConfirm(key("n"))
	if m.confirm != "" {
		t.Error("n did not cancel confirm")
	}
	m.confirm = "resetattempts"
	m, _ = m.updateConfirm(key("esc"))
	if m.confirm != "" {
		t.Error("esc did not cancel confirm")
	}
}

func TestClearHeatReadOnlyBlocked(t *testing.T) {
	m := seededDetail() // Manager nil
	m, _ = m.clearHeat()
	if m.submitting {
		t.Error("clearHeat proceeded despite read-only")
	}
	if !strings.Contains(m.notice, "read-only") {
		t.Errorf("expected read-only notice, got %q", m.notice)
	}
}

func TestPVPBrowser(t *testing.T) {
	m := seededDetail() // Manager nil → read-only

	// Read-only blocks opening the browser.
	m, _ = m.openPVP()
	if m.pvpOpen {
		t.Fatal("opened PVP browser in read-only mode")
	}
	if !strings.Contains(m.notice, "read-only") {
		t.Errorf("expected read-only notice, got %q", m.notice)
	}

	// Drive the browser directly with a target list.
	m.pvpOpen = true
	m.targets = []bindings.PVPTarget{
		{TokenID: big.NewInt(10), Reputation: big.NewInt(232), WinChance: big.NewInt(50), Infamy: big.NewInt(0), CanAttackNow: true},
		{TokenID: big.NewInt(42), Reputation: big.NewInt(233), WinChance: big.NewInt(50), Infamy: big.NewInt(0), CanAttackNow: false},
	}

	m, _ = m.updatePVP(key("down"))
	if m.targetIdx != 1 {
		t.Errorf("down → idx %d, want 1", m.targetIdx)
	}
	m, _ = m.updatePVP(key("down")) // wrap to 0
	if m.targetIdx != 0 {
		t.Errorf("down wrap → idx %d, want 0", m.targetIdx)
	}
	m, _ = m.updatePVP(key("up")) // up from 0 wraps to last
	if m.targetIdx != 1 {
		t.Errorf("up wrap → idx %d, want 1", m.targetIdx)
	}

	// Enter on the unavailable target must not submit.
	m, _ = m.updatePVP(key("enter"))
	if m.submitting {
		t.Error("submitted attack on an unavailable target")
	}
	if !strings.Contains(m.notice, "not attackable") {
		t.Errorf("expected not-attackable notice, got %q", m.notice)
	}

	// View renders the browser.
	v := m.pvpView()
	if !strings.Contains(v, "PVP TARGETS") || !strings.Contains(v, "#10") {
		t.Errorf("pvpView missing content:\n%s", v)
	}

	// esc closes the browser.
	m, _ = m.updatePVP(key("esc"))
	if m.pvpOpen {
		t.Error("esc did not close PVP browser")
	}
}

func TestPVPHidesAllies(t *testing.T) {
	m := seededDetail()
	m.deps.Allies = allies.Load(filepath.Join(t.TempDir(), "a.json"), []uint64{42}) // #42 fixed ally → hidden

	updated, _ := m.Update(pvpTargetsMsg{targets: []bindings.PVPTarget{
		{TokenID: big.NewInt(10), WinChance: big.NewInt(50), Reputation: big.NewInt(230), Infamy: big.NewInt(0), CanAttackNow: true},
		{TokenID: big.NewInt(42), WinChance: big.NewInt(50), Reputation: big.NewInt(233), Infamy: big.NewInt(0), CanAttackNow: true},
		{TokenID: big.NewInt(58), WinChance: big.NewInt(50), Reputation: big.NewInt(214), Infamy: big.NewInt(0), CanAttackNow: true},
	}})
	dm := updated

	if len(dm.targets) != 2 {
		t.Fatalf("targets = %d, want 2 (ally hidden): %+v", len(dm.targets), dm.targets)
	}
	for _, tg := range dm.targets {
		if tg.TokenID.Uint64() == 42 {
			t.Error("ally #42 was not hidden from PVP targets")
		}
	}
	if dm.alliesHidden != 1 {
		t.Errorf("alliesHidden = %d, want 1", dm.alliesHidden)
	}
	// The hidden count surfaces in the view.
	dm.pvpOpen = true
	if v := dm.pvpView(); !strings.Contains(v, "ally target(s) hidden") {
		t.Errorf("pvpView missing ally-hidden note:\n%s", v)
	}
}

func TestPVPTargetsMsg(t *testing.T) {
	m := seededDetail()
	m.pvpLoading = true
	updated, _ := m.Update(pvpTargetsMsg{targets: []bindings.PVPTarget{
		{TokenID: big.NewInt(7), Reputation: big.NewInt(300), WinChance: big.NewInt(50), Infamy: big.NewInt(1), CanAttackNow: true},
	}})
	dm := updated
	if dm.pvpLoading {
		t.Error("pvpLoading still true after targets msg")
	}
	if len(dm.targets) != 1 || dm.targets[0].TokenID.Uint64() != 7 {
		t.Errorf("targets not stored: %+v", dm.targets)
	}
}

func TestHeistStartFormNavigation(t *testing.T) {
	m := seededDetail() // Manager nil

	// Read-only blocks opening.
	m, _ = m.openHeistStart()
	if m.hsOpen {
		t.Fatal("opened heist form in read-only mode")
	}

	// Drive the form directly.
	m.hsOpen = true
	m.hsFamily = bindings.FamilyCash
	m.hsDiff = 0
	m.hsJackpot = false

	// Field 0 = family: toggle CASH→SUPPLY.
	m, _ = m.updateHeistStart(key("right"))
	if m.hsFamily != bindings.FamilySupply {
		t.Errorf("family toggle failed: %v", m.hsFamily)
	}
	// Move to field 1 (difficulty) and cycle 0→1→2→0.
	m, _ = m.updateHeistStart(key("down"))
	if m.hsField != 1 {
		t.Fatalf("field = %d, want 1", m.hsField)
	}
	m, _ = m.updateHeistStart(key("right"))
	m, _ = m.updateHeistStart(key("right"))
	if m.hsDiff != 2 {
		t.Errorf("difficulty = %d, want 2", m.hsDiff)
	}
	// Field 2 (jackpot) toggle.
	m, _ = m.updateHeistStart(key("down"))
	m, _ = m.updateHeistStart(key(" "))
	if !m.hsJackpot {
		t.Error("jackpot toggle failed")
	}
	// View renders the form.
	if v := m.heistStartView(); !strings.Contains(v, "Family") || !strings.Contains(v, "hard") {
		t.Errorf("heistStartView missing content:\n%s", v)
	}
	// esc closes.
	m, _ = m.updateHeistStart(key("esc"))
	if m.hsOpen {
		t.Error("esc did not close heist form")
	}
}

func TestHeistCashOutGating(t *testing.T) {
	m := seededDetail()

	m.heist = &bindings.DailyHeist{Status: uint8(bindings.HeistRevealedWin), CurrentStage: 3}
	if !m.canCashOut() {
		t.Error("REVEALED_WIN stage 3 should allow cash out")
	}
	m.heist = &bindings.DailyHeist{Status: uint8(bindings.HeistRevealedWin), CurrentStage: 1}
	if m.canCashOut() {
		t.Error("stage 1 should NOT allow cash out (min stage 2)")
	}
	m.heist = &bindings.DailyHeist{Status: uint8(bindings.HeistPreStage)}
	if m.canCashOut() {
		t.Error("PRE_STAGE should not allow cash out")
	}

	// Action hints per status.
	if got := heistActionsHint(&bindings.DailyHeist{Status: uint8(bindings.HeistPreStage)}); !strings.Contains(got, "abandon") {
		t.Errorf("pre-stage hint: %q", got)
	}
	if got := heistActionsHint(&bindings.DailyHeist{Status: uint8(bindings.HeistRevealedWin), CurrentStage: 3}); !strings.Contains(got, "cash out") {
		t.Errorf("revealed-win hint: %q", got)
	}
}

func TestBlackMarketFormRouting(t *testing.T) {
	m := seededDetail()
	m.deps.Manager = dealer.NewManager(config.Network{}, nil, nil, nil, nil) // non-nil = not read-only
	m.snap.State.CurrentArea = bindings.BlackMarketArea

	if !m.inBlackMarket() {
		t.Fatal("inBlackMarket() false at area 254")
	}
	// Buying is blocked in the black market.
	m, _ = m.openForm(bindings.HustleBuy)
	if m.formOpen {
		t.Error("buy form opened in black market")
	}
	if !strings.Contains(m.notice, "can't buy") {
		t.Errorf("expected no-buy notice, got %q", m.notice)
	}
	// The footer hint reflects the black market.
	if v := m.View(); !strings.Contains(v, "BLACK MARKET") || !strings.Contains(v, "sell loot") {
		t.Errorf("black-market hint missing:\n%s", v)
	}
}

func TestTradeableFiltersByArea(t *testing.T) {
	m := seededDetail()
	// Area sells Weed & Coke; Heroin is present but unavailable here.
	m.areaDrugs = []bindings.AreaDrug{
		{DrugID: big.NewInt(0), Name: "Weed", BuyPrice: big.NewInt(1), SellPrice: big.NewInt(1), IsAvailable: true},
		{DrugID: big.NewInt(1), Name: "Coke", BuyPrice: big.NewInt(120), SellPrice: big.NewInt(100), IsAvailable: true},
		{DrugID: big.NewInt(8), Name: "Heroin", BuyPrice: big.NewInt(0), SellPrice: big.NewInt(0), IsAvailable: false},
	}
	// Dealer holds Weed (12) and Meth (3) — Meth isn't traded in this area.
	m.snap.State.DrugBalances = []bindings.DrugBalance{
		{DrugID: big.NewInt(0), Name: "Weed", Balance: big.NewInt(12)},
		{DrugID: big.NewInt(2), Name: "Meth", Balance: big.NewInt(3)},
	}

	// BUY: only the two available drugs; Heroin excluded (would revert).
	buy := m.tradeable(bindings.HustleBuy)
	if len(buy) != 2 {
		t.Fatalf("buy list = %d, want 2 (Weed, Coke): %+v", len(buy), buy)
	}
	for _, d := range buy {
		if d.DrugID.Uint64() == 8 {
			t.Error("Heroin (unavailable) offered for buy — the bug")
		}
	}

	// SELL: only drugs held AND traded here → Weed only (Meth not sold here,
	// Coke not held).
	sell := m.tradeable(bindings.HustleSell)
	if len(sell) != 1 || sell[0].DrugID.Uint64() != 0 {
		t.Errorf("sell list wrong, want [Weed]: %+v", sell)
	}
}

func TestTravelPicker(t *testing.T) {
	m := seededDetail() // state: CurrentArea 2, Reputation 814
	m.deps.AreaNames = map[uint8]string{1: "Manhattan", 2: "Amsterdam", 3: "Colombia", 255: "Jail"}

	areas := []bindings.AreaEconomy{
		{AreaID: 0, IsActive: true, IsSafeHouse: true}, // safe house → excluded (unreachable)
		{AreaID: 1, IsActive: true, MinReputation: big.NewInt(0), MovementFee: big.NewInt(0)},
		{AreaID: 2, IsActive: true, MinReputation: big.NewInt(250)},                                // current area → excluded
		{AreaID: 3, IsActive: true, MinReputation: big.NewInt(500), MovementFee: big.NewInt(1e15)}, // reachable (814≥500)
		{AreaID: 9, IsActive: true, MinReputation: big.NewInt(5500)},                               // locked (814<5500)
		{AreaID: 255, IsActive: true, IsJail: true},                                                // jail → excluded
	}
	dests := m.filterDestinations(areas)
	if len(dests) != 3 { // 1, 3, 9 (0 safehouse, 2 current, 255 jail excluded)
		t.Fatalf("dests = %d, want 3: %+v", len(dests), dests)
	}
	for _, d := range dests {
		if d.AreaID == 0 || d.AreaID == 2 || d.AreaID == 255 {
			t.Errorf("destination %d should be excluded", d.AreaID)
		}
	}

	// Reachability gate.
	var colombia, dubai bindings.AreaEconomy
	for _, d := range dests {
		if d.AreaID == 3 {
			colombia = d
		}
		if d.AreaID == 9 {
			dubai = d
		}
	}
	if ok, _ := m.canEnter(colombia); !ok {
		t.Error("Colombia (rep 500) should be reachable at rep 814")
	}
	if ok, _ := m.canEnter(dubai); ok {
		t.Error("rep-5500 area should be locked at rep 814")
	}

	// Black market gates on INFAMY, not reputation.
	bm := bindings.AreaEconomy{AreaID: bindings.BlackMarketArea, IsActive: true, MinReputation: big.NewInt(0)}
	m.snap.State.Infamy = big.NewInt(0)
	if ok, why := m.canEnter(bm); ok || !strings.Contains(why, "infamy") {
		t.Errorf("black market with infamy 0 should be locked on infamy, got ok=%v why=%q", ok, why)
	}
	m.snap.State.Infamy = big.NewInt(10)
	if ok, _ := m.canEnter(bm); !ok {
		t.Error("black market should be enterable at infamy 10")
	}

	// Navigation + view.
	m.tvOpen = true
	m.tvDests = dests
	m, _ = m.updateTravel(key("down"))
	if m.tvIdx != 1 {
		t.Errorf("down → idx %d, want 1", m.tvIdx)
	}
	if v := m.travelView(); !strings.Contains(v, "TRAVEL") {
		t.Errorf("travelView missing header:\n%s", v)
	}

	// esc closes.
	m, _ = m.updateTravel(key("esc"))
	if m.tvOpen {
		t.Error("esc did not close travel picker")
	}
}

func TestBreakoutGating(t *testing.T) {
	// Not jailed → breakout refuses with a clear message.
	m := seededDetail() // Manager nil, not jailed
	m, _ = m.breakout()
	if m.submitting {
		t.Error("breakout proceeded in read-only")
	}
	if !strings.Contains(m.notice, "read-only") {
		t.Errorf("expected read-only notice, got %q", m.notice)
	}

	// A jailed dealer's footer surfaces the jail actions (needs a signer/Manager).
	mj := seededDetail()
	mj.snap.State.IsJailed = true
	mj.deps.Manager = dealer.NewManager(config.Network{}, nil, nil, nil, nil) // non-nil = not read-only
	if v := mj.View(); !strings.Contains(v, "JAILED") || !strings.Contains(v, "breakout") || !strings.Contains(v, "bail") {
		t.Errorf("jailed hint missing:\n%s", v)
	}
}

func TestTravelReadOnlyBlocked(t *testing.T) {
	m := seededDetail() // Manager nil
	m, _ = m.openTravel()
	if m.tvOpen {
		t.Error("opened travel in read-only mode")
	}
	if !strings.Contains(m.notice, "read-only") {
		t.Errorf("expected read-only notice, got %q", m.notice)
	}
}

func TestParseAmount(t *testing.T) {
	m := seededDetail()

	// Empty defaults to 1.
	if a, err := m.parseAmount(); err != nil || a != 1 {
		t.Errorf("default amount: a=%d err=%v", a, err)
	}
	m.amount.SetValue("5")
	if a, err := m.parseAmount(); err != nil || a != 5 {
		t.Errorf("amount 5: a=%d err=%v", a, err)
	}
	m.amount.SetValue("0")
	if _, err := m.parseAmount(); err == nil {
		t.Error("amount 0 should be rejected")
	}
	m.amount.SetValue("xx")
	if _, err := m.parseAmount(); err == nil {
		t.Error("non-numeric amount should be rejected")
	}
}

func TestDrugSelectorCyclesAndRemembers(t *testing.T) {
	ui := &UIState{}
	m := NewDetail(Deps{Net: "mainnet", UI: ui}, 1, dealer.Snapshot{})
	m.drugs = areaDrugCatalog()
	m.seedDrugIdx() // no memory yet → index 0

	if m.selectedDrugID() != 0 {
		t.Fatalf("initial drug = %d, want 0", m.selectedDrugID())
	}

	// Down twice → Meth (id 2); wrap on a third.
	m.applyDrugDelta(1)
	m.applyDrugDelta(1)
	if m.selectedDrugID() != 2 {
		t.Errorf("after 2×down drug = %d, want 2", m.selectedDrugID())
	}
	if ui.LastDrugID != 2 {
		t.Errorf("UI did not remember: LastDrugID = %d, want 2", ui.LastDrugID)
	}
	m.applyDrugDelta(1) // wrap to index 0
	if m.selectedDrugID() != 0 {
		t.Errorf("wrap failed: drug = %d, want 0", m.selectedDrugID())
	}

	// Up from index 0 wraps to the last (Meth, id 2).
	m.applyDrugDelta(-1)
	if m.selectedDrugID() != 2 {
		t.Errorf("up-wrap drug = %d, want 2", m.selectedDrugID())
	}

	// A fresh detail sharing the same UI restores the remembered drug.
	ui.LastDrugID = 1
	m2 := NewDetail(Deps{Net: "mainnet", UI: ui}, 9, dealer.Snapshot{})
	m2.drugs = areaDrugCatalog()
	m2.seedDrugIdx()
	if m2.selectedDrugID() != 1 {
		t.Errorf("remembered drug not restored: got %d, want 1", m2.selectedDrugID())
	}
}
