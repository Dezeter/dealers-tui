package dealer

import (
	"math/big"
	"sort"

	"dealers/internal/chain/bindings"
)

// ArbPair is a buy-low/sell-high opportunity for one drug: buy it cheapest in
// BuyArea, carry it, sell it dearest in SellArea. Profit is the per-unit spread
// (before the PVE gamble and travel fee — it's the theoretical edge).
type ArbPair struct {
	DrugID     uint64
	DrugName   string
	BuyArea    uint8
	BuyPrice   *big.Int
	SellArea   uint8
	SellPrice  *big.Int
	Profit     *big.Int // SellPrice - BuyPrice, per unit (= expected cash profit;
	// the PVE gamble is EV-neutral, win/loss cancel, tie is a normal trade)
	SellMinRep  *big.Int // reputation gate to reach the sell area
	BuyMoveFee  *big.Int // ETH (wei) to enter the buy area
	SellMoveFee *big.Int // ETH (wei) to enter the sell area
}

// TravelWei is the total ETH (wei) to enter both the buy and sell areas — the
// real per-trip cost, amortised over the amount carried.
func (p ArbPair) TravelWei() *big.Int {
	t := new(big.Int)
	if p.BuyMoveFee != nil {
		t.Add(t, p.BuyMoveFee)
	}
	if p.SellMoveFee != nil {
		t.Add(t, p.SellMoveFee)
	}
	return t
}

// Arbitrage finds, per drug, the cheapest place to buy and the dearest place to
// sell across all active trading areas, and returns the profitable pairs sorted
// by per-unit profit (descending, tie-broken by drug id). Jail and safe-house
// "areas" are skipped.
func Arbitrage(areas []bindings.AreaEconomy) []ArbPair {
	type agg struct {
		name                     string
		buyArea, sellArea        uint8
		buyPrice, sellPrice      *big.Int
		sellMinRep               *big.Int
		buyMoveFee, sellMoveFee  *big.Int
	}
	best := map[uint64]*agg{}

	for _, a := range areas {
		if !a.IsActive || a.IsJail || a.IsSafeHouse {
			continue
		}
		for _, d := range a.Drugs {
			if !d.IsAvailable || d.DrugID == nil {
				continue
			}
			id := d.DrugID.Uint64()
			e := best[id]
			if e == nil {
				e = &agg{name: d.Name}
				best[id] = e
			}
			if d.BuyPrice != nil && d.BuyPrice.Sign() > 0 && (e.buyPrice == nil || d.BuyPrice.Cmp(e.buyPrice) < 0) {
				e.buyPrice = d.BuyPrice
				e.buyArea = a.AreaID
				e.buyMoveFee = a.MovementFee
			}
			if d.SellPrice != nil && d.SellPrice.Sign() > 0 && (e.sellPrice == nil || d.SellPrice.Cmp(e.sellPrice) > 0) {
				e.sellPrice = d.SellPrice
				e.sellArea = a.AreaID
				e.sellMinRep = a.MinReputation
				e.sellMoveFee = a.MovementFee
			}
		}
	}

	var pairs []ArbPair
	for id, e := range best {
		if e.buyPrice == nil || e.sellPrice == nil {
			continue
		}
		// Only real travel arbitrage: buy and sell must be different areas.
		// A same-area "spread" (e.g. Black-Market exotic loot, which you sell but
		// can't buy) isn't a where-to-buy/where-to-sell route.
		if e.buyArea == e.sellArea {
			continue
		}
		profit := new(big.Int).Sub(e.sellPrice, e.buyPrice)
		if profit.Sign() <= 0 {
			continue
		}
		pairs = append(pairs, ArbPair{
			DrugID: id, DrugName: e.name,
			BuyArea: e.buyArea, BuyPrice: e.buyPrice,
			SellArea: e.sellArea, SellPrice: e.sellPrice,
			Profit: profit, SellMinRep: e.sellMinRep,
			BuyMoveFee: e.buyMoveFee, SellMoveFee: e.sellMoveFee,
		})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if c := pairs[i].Profit.Cmp(pairs[j].Profit); c != 0 {
			return c > 0
		}
		return pairs[i].DrugID < pairs[j].DrugID
	})
	return pairs
}
