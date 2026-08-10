package dealer

import (
	"context"
	"math/big"
	"testing"

	"dealers/internal/chain/bindings"
)

// heroinMarket is a one-drug market for a non-weed trade-route test.
func heroinMarket(buy, sell int64) []bindings.AreaDrug {
	return []bindings.AreaDrug{
		{DrugID: big.NewInt(9), Name: "Heroin", BuyPrice: big.NewInt(buy), SellPrice: big.NewInt(sell), IsAvailable: true},
	}
}

func TestTemplateTradeRouteAndDrug(t *testing.T) {
	// Custom template: buy HEROIN in what's normally the sell zone (amsterdam) and
	// sell in manhattan — proves the drug + route params drive the core.
	s := NewPvEArbitrageCfg(StrategyConfig{
		BuyArea: amsterdam, SellArea: manhattan, Drug: "heroin", HeistDifficulty: -1,
	})
	st := pveState(amsterdam, 5, 1_000_000, 0) // sitting in the (custom) buy zone, no stock
	dec := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: heroinMarket(100, 200)}
	a, ok := s.Next(context.Background(), stakeReader(), dec)
	if !ok || a.Kind != ActionPVE || a.Hustle != bindings.HustleBuy || a.DrugID != 9 {
		t.Fatalf("want a heroin buy in the custom buy zone, got %+v ok=%v", a, ok)
	}
}

func TestCoreStepDailyCap(t *testing.T) {
	// Cap the core (trade) at 2 deals/day → after 2 buys it yields; with no step
	// after core, the dealer idles the rest of the day.
	s := NewPvEArbitrageCfg(StrategyConfig{
		BuyArea: manhattan, SellArea: amsterdam, HeistDifficulty: -1,
		StepMax: func(id string) int {
			if id == StepCore {
				return 2
			}
			return 0
		},
	})
	st := pveState(manhattan, 5, 100000, 0)
	dec := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}
	for i := 0; i < 2; i++ {
		if a, ok := s.Next(context.Background(), stakeReader(), dec); !ok || a.Kind != ActionPVE {
			t.Fatalf("deal %d should be a buy, got %+v ok=%v", i+1, a, ok)
		}
	}
	if a, ok := s.Next(context.Background(), stakeReader(), dec); ok {
		t.Fatalf("after the core cap the dealer should idle, got %+v ok=%v", a, ok)
	}
}

func TestCoreCapCountsPrimaryNotTravel(t *testing.T) {
	// Travel is plumbing and must NOT count against a core cap: a stocked dealer on
	// Manhattan travels to sell without spending its deal budget.
	s := NewPvEArbitrageCfg(StrategyConfig{
		BuyArea: manhattan, SellArea: amsterdam, HeistDifficulty: -1,
		StepMax: func(id string) int {
			if id == StepCore {
				return 1
			}
			return 0
		},
	})
	st := pveState(manhattan, 5, 100000, 30) // at the ×3 stockpile target → haul, don't buy
	dec := Decision{Snap: Snapshot{TokenID: 1, State: st}, Area: weedMarket(100, 90)}
	a, ok := s.Next(context.Background(), stakeReader(), dec)
	if !ok || a.Kind != ActionTravel {
		t.Fatalf("stocked dealer should travel to sell, got %+v ok=%v", a, ok)
	}
	// The travel didn't consume the 1-deal budget: a fresh (unstocked) dealer still buys.
	st2 := pveState(manhattan, 5, 100000, 0)
	dec2 := Decision{Snap: Snapshot{TokenID: 1, State: st2}, Area: weedMarket(100, 90)}
	if a, ok := s.Next(context.Background(), stakeReader(), dec2); !ok || a.Kind != ActionPVE {
		t.Fatalf("deal budget should be intact after a travel, got %+v ok=%v", a, ok)
	}
}
