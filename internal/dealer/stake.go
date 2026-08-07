package dealer

import (
	"math/big"

	"dealers/internal/chain/bindings"
)

// MaxStake computes the per-action stake cap (in $CASH) from a dealer's game
// state and the PVE stake params, mirroring DealersPVE._checkMaxStake:
//
//	maxStake = repCap × (repStakeDivisor + totalRep×slopeBps/10000) × headroomBps
//	           / (repTieBonus × 10000)
//
// Returns nil when there is effectively no cap (headroom 0, or repCap/repTieBonus
// non-positive) — matching the contract's early return.
func MaxStake(gs *bindings.GameState, p *bindings.PVEStakeParams) *big.Int {
	if gs == nil || p == nil || p.HeadroomBps == 0 || gs.RepTieBonus <= 0 || gs.RepCap <= 0 {
		return nil
	}
	totalRep := gs.TotalReputation
	if totalRep == nil {
		totalRep = big.NewInt(0)
	}
	// divisor = repStakeDivisor + totalRep*slopeBps/10000
	divisor := new(big.Int).Div(new(big.Int).Mul(totalRep, big.NewInt(int64(p.SlopeBps))), big.NewInt(10000))
	divisor.Add(divisor, new(big.Int).SetUint64(p.RepStakeDivisor))

	num := new(big.Int).Mul(big.NewInt(int64(gs.RepCap)), divisor)
	num.Mul(num, new(big.Int).SetUint64(p.HeadroomBps))
	den := new(big.Int).Mul(big.NewInt(int64(gs.RepTieBonus)), big.NewInt(10000))
	if den.Sign() == 0 {
		return nil
	}
	return num.Div(num, den)
}

// MaxUnitsAtPrice returns floor(maxStake / price) — the largest amount tradeable
// under the stake cap at a given unit price. Returns 0 if price is invalid; a
// nil maxStake (no cap) yields 0 too, so callers treat 0 as "no stake limit".
func MaxUnitsAtPrice(maxStake, price *big.Int) uint64 {
	if maxStake == nil || price == nil || price.Sign() <= 0 {
		return 0
	}
	return new(big.Int).Div(maxStake, price).Uint64()
}
