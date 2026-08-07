package dealer

import (
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
)

func drug(id int64, name string, buy, sell int64, avail bool) bindings.AreaDrug {
	return bindings.AreaDrug{DrugID: big.NewInt(id), Name: name, BuyPrice: big.NewInt(buy), SellPrice: big.NewInt(sell), IsAvailable: avail}
}

func TestArbitrage(t *testing.T) {
	areas := []bindings.AreaEconomy{
		{AreaID: 1, AreaName: "Manhattan", IsActive: true, MinReputation: big.NewInt(0), Drugs: []bindings.AreaDrug{
			drug(4, "Weed", 1, 1, true),
			drug(6, "Coke", 120, 100, true),
		}},
		{AreaID: 2, AreaName: "Amsterdam", IsActive: true, MinReputation: big.NewInt(250), MovementFee: big.NewInt(1_000_000_000_000_000), Drugs: []bindings.AreaDrug{
			drug(4, "Weed", 3, 2, true),   // sell Weed dearest here (2)
			drug(6, "Coke", 150, 130, true), // sell Coke dearest here (130)
		}},
		{AreaID: 255, AreaName: "Jail", IsJail: true, IsActive: true, Drugs: []bindings.AreaDrug{drug(4, "Weed", 999, 999, true)}}, // skipped
	}

	pairs := Arbitrage(areas)
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2: %+v", len(pairs), pairs)
	}

	// Sorted by profit desc → Coke (130-120=10) before Weed (2-1=1).
	if pairs[0].DrugName != "Coke" || pairs[0].Profit.Int64() != 10 {
		t.Errorf("pair0 wrong: %+v", pairs[0])
	}
	if pairs[0].BuyArea != 1 || pairs[0].SellArea != 2 {
		t.Errorf("Coke route wrong: buy@%d sell@%d, want 1→2", pairs[0].BuyArea, pairs[0].SellArea)
	}
	if pairs[1].DrugName != "Weed" || pairs[1].Profit.Int64() != 1 {
		t.Errorf("pair1 wrong: %+v", pairs[1])
	}
	// Sell-area rep gate carried through (Amsterdam 250).
	if pairs[0].SellMinRep.Int64() != 250 {
		t.Errorf("sell min rep = %s, want 250", pairs[0].SellMinRep)
	}
	// Travel fee = buy-area (Manhattan, 0) + sell-area (Amsterdam, 0.001 ETH).
	if pairs[0].TravelWei().Cmp(big.NewInt(1_000_000_000_000_000)) != 0 {
		t.Errorf("travel wei = %s, want 1e15", pairs[0].TravelWei())
	}
	// The jail area's absurd 999 price must not leak in.
	if pairs[1].SellPrice.Int64() == 999 {
		t.Error("jail area was not skipped")
	}
}

func TestArbitrageNoProfit(t *testing.T) {
	// A single area: sell < buy everywhere → no profitable pair.
	areas := []bindings.AreaEconomy{
		{AreaID: 1, IsActive: true, MinReputation: big.NewInt(0), Drugs: []bindings.AreaDrug{drug(4, "Weed", 10, 5, true)}},
	}
	if p := Arbitrage(areas); len(p) != 0 {
		t.Errorf("expected no pairs, got %+v", p)
	}
}
