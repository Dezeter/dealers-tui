package bindings

import (
	"context"
	"fmt"
	"math/big"
)

// AreaDrug mirrors DealersMulticall.AreaDrug (CHAIN_REFERENCE §1.2) — a drug as
// traded in a specific area, with its local buy/sell price and availability.
type AreaDrug struct {
	DrugID      *big.Int `abi:"drugId"`
	Name        string   `abi:"name"`
	Rarity      uint8    `abi:"rarity"`
	BuyPrice    *big.Int `abi:"buyPrice"`
	SellPrice   *big.Int `abi:"sellPrice"`
	IsAvailable bool     `abi:"isAvailable"`
}

// AreaEconomy mirrors DealersMulticall.AreaEconomy.
type AreaEconomy struct {
	AreaID        uint8      `abi:"areaId"`
	AreaName      string     `abi:"areaName"`
	MovementFee   *big.Int   `abi:"movementFee"`
	MinReputation *big.Int   `abi:"minReputation"`
	IsActive      bool       `abi:"isActive"`
	IsSafeHouse   bool       `abi:"isSafeHouse"`
	IsJail        bool       `abi:"isJail"`
	DealerCount   *big.Int   `abi:"dealerCount"`
	Drugs         []AreaDrug `abi:"drugs"`
}

// areaEconomyComponents is the shared AreaEconomy tuple layout (reused by both
// getAreaEconomy and getAllAreas).
const areaEconomyComponents = `[
  {"name":"areaId","type":"uint8"},
  {"name":"areaName","type":"string"},
  {"name":"movementFee","type":"uint256"},
  {"name":"minReputation","type":"uint256"},
  {"name":"isActive","type":"bool"},
  {"name":"isSafeHouse","type":"bool"},
  {"name":"isJail","type":"bool"},
  {"name":"dealerCount","type":"uint256"},
  {"name":"drugs","type":"tuple[]","components":[
    {"name":"drugId","type":"uint256"},
    {"name":"name","type":"string"},
    {"name":"rarity","type":"uint8"},
    {"name":"buyPrice","type":"uint256"},
    {"name":"sellPrice","type":"uint256"},
    {"name":"isAvailable","type":"bool"}
  ]}
]`

const areaEconomyABIJSON = `[
  {"type":"function","name":"getAreaEconomy","stateMutability":"view",
   "inputs":[{"name":"areaId","type":"uint8"}],
   "outputs":[{"name":"","type":"tuple","components":` + areaEconomyComponents + `}]},
  {"type":"function","name":"getAllAreas","stateMutability":"view",
   "inputs":[],
   "outputs":[{"name":"","type":"tuple[]","components":` + areaEconomyComponents + `}]}
]`

var areaEconomyABI = mustParseABI(areaEconomyABIJSON)

// AreaEconomy reads the drugs (with local prices/availability) tradeable in an
// area — the source of truth for what a dealer can buy/sell there.
func (r *Reader) AreaEconomy(ctx context.Context, areaID uint8) (*AreaEconomy, error) {
	out, err := r.call(ctx, areaEconomyABI, r.cl.Net.Contracts.DealersMulticall, "getAreaEconomy", areaID)
	if err != nil {
		return nil, err
	}
	vals, err := areaEconomyABI.Unpack("getAreaEconomy", out)
	if err != nil {
		return nil, fmt.Errorf("decode getAreaEconomy: %w", err)
	}
	return abiConvert[AreaEconomy](vals[0]), nil
}

// AllAreas reads every area's economy in one call — the basis for the price /
// arbitrage view (FR12).
func (r *Reader) AllAreas(ctx context.Context) ([]AreaEconomy, error) {
	out, err := r.call(ctx, areaEconomyABI, r.cl.Net.Contracts.DealersMulticall, "getAllAreas")
	if err != nil {
		return nil, err
	}
	vals, err := areaEconomyABI.Unpack("getAllAreas", out)
	if err != nil {
		return nil, fmt.Errorf("decode getAllAreas: %w", err)
	}
	return *abiConvert[[]AreaEconomy](vals[0]), nil
}
