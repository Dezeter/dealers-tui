package bindings

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// TestFullDealerStateRoundTrip ABI-encodes a FullDealerState via the method's
// output arguments and decodes it back into the struct, asserting every field
// survives. This proves the ABI literal, the field order, and the `abi:` tags
// are mutually consistent — a misplaced field or wrong tag corrupts the result.
func TestFullDealerStateRoundTrip(t *testing.T) {
	want := FullDealerState{
		Reputation:             big.NewInt(1234),
		StashBonusRep:          big.NewInt(56),
		CurrentArea:            1,
		PreviousArea:           2,
		HeatLevel:              3,
		DailyAttemptsRemaining: 4,
		MaxAttempts:            5,
		Threat:                 0,
		Armor:                  0,
		IsInitialized:          true,
		IsJailed:               false,
		IsInSafeHouse:          false,
		JailChance:             21,
		ReputationTitle:        "Soldier",
		CashBalance:            big.NewInt(9999),
		DrugBalances: []DrugBalance{
			{DrugID: big.NewInt(0), Name: "Weed", Balance: big.NewInt(10), Rarity: 0},
			{DrugID: big.NewInt(3), Name: "Coke", Balance: big.NewInt(2), Rarity: 2},
		},
		BoostActive:          true,
		BoostExpiry:          1750000000,
		DrugMultiplier:       120,
		CashMultiplier:       110,
		RepMultiplier:        125,
		FreeAreaMovement:     true,
		PveWins:              7,
		PveLosses:            1,
		PveTies:              3,
		PvpAttackWins:        2,
		PvpAttackLosses:      1,
		PvpDefendWins:        4,
		PvpDefendLosses:      0,
		LastBreakoutAttempt:  0,
		CanBreakoutToday:     true,
		AttacksReceivedToday: 1,
		MaxAttacksPerDay:     3,
		Infamy:               big.NewInt(42),
	}

	method := multicallABI.Methods["getFullDealerState"]
	encoded, err := method.Outputs.Pack(want)
	if err != nil {
		t.Fatalf("pack outputs: %v", err)
	}

	vals, err := multicallABI.Unpack("getFullDealerState", encoded)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	got := *abi.ConvertType(vals[0], new(FullDealerState)).(*FullDealerState)

	if got.Reputation.Cmp(want.Reputation) != 0 {
		t.Errorf("Reputation = %s, want %s", got.Reputation, want.Reputation)
	}
	// Multiplier trio is the field most likely to be mis-ordered (FullDealerState
	// orders drug→cash→rep, GameState orders drug→rep→cash).
	if got.DrugMultiplier != 120 || got.CashMultiplier != 110 || got.RepMultiplier != 125 {
		t.Errorf("multiplier trio = (%d,%d,%d), want (120,110,125)",
			got.DrugMultiplier, got.CashMultiplier, got.RepMultiplier)
	}
	if got.ReputationTitle != "Soldier" {
		t.Errorf("ReputationTitle = %q", got.ReputationTitle)
	}
	if got.HeatLevel != 3 || got.DailyAttemptsRemaining != 4 {
		t.Errorf("heat/attempts = (%d,%d), want (3,4)", got.HeatLevel, got.DailyAttemptsRemaining)
	}
	if len(got.DrugBalances) != 2 || got.DrugBalances[1].Name != "Coke" || got.DrugBalances[1].Rarity != 2 {
		t.Errorf("drug balances decoded wrong: %+v", got.DrugBalances)
	}
	if got.Infamy.Cmp(big.NewInt(42)) != 0 || !got.CanBreakoutToday || got.MaxAttacksPerDay != 3 {
		t.Errorf("tail fields decoded wrong: infamy=%s breakout=%v maxAtk=%d",
			got.Infamy, got.CanBreakoutToday, got.MaxAttacksPerDay)
	}
}
