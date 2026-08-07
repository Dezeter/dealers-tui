package bindings

import (
	"context"
	"fmt"
	"math/big"
)

// Minimal DealersCore reads used to cross-check the multicall and for
// pre-action state checks. getEffectiveHeat is the canonical heat getter that
// DealersMulticall.getFullDealerState also uses (CHAIN_REFERENCE §1.5).
const coreABIJSON = `[
  {"type":"function","name":"getEffectiveHeat","stateMutability":"view",
   "inputs":[{"name":"tokenId","type":"uint256"}],
   "outputs":[{"name":"","type":"uint8"}]},
  {"type":"function","name":"getInfamy","stateMutability":"view",
   "inputs":[{"name":"tokenId","type":"uint256"}],
   "outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"getCashBalance","stateMutability":"view",
   "inputs":[{"name":"tokenId","type":"uint256"}],
   "outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"config","stateMutability":"view","inputs":[],
   "outputs":[{"name":"","type":"tuple","components":[
     {"name":"attemptResetFee","type":"uint256"},
     {"name":"bribeCopFee","type":"uint256"},
     {"name":"cashTopupPrice","type":"uint256"},
     {"name":"cashTopupAmount","type":"uint256"},
     {"name":"cashPurchaseThreshold","type":"uint256"},
     {"name":"jailRepPenaltyPercent","type":"uint8"},
     {"name":"jailRepPenaltyCap","type":"uint256"},
     {"name":"wantedPosterSuccessChance","type":"uint8"},
     {"name":"breakoutSuccessChance","type":"uint8"},
     {"name":"jailDrugConfiscationPercent","type":"uint8"},
     {"name":"starterCash","type":"uint256"},
     {"name":"jailChancePerHeat","type":"uint16"}
   ]}]}
]`

var coreABI = mustParseABI(coreABIJSON)

// CoreConfig mirrors DealersCore.CoreConfig (CHAIN_REFERENCE §8.1). Only the fee
// fields are used today; the rest are decoded for completeness.
type CoreConfig struct {
	AttemptResetFee            *big.Int `abi:"attemptResetFee"`
	BribeCopFee                *big.Int `abi:"bribeCopFee"`
	CashTopupPrice             *big.Int `abi:"cashTopupPrice"`
	CashTopupAmount            *big.Int `abi:"cashTopupAmount"`
	CashPurchaseThreshold      *big.Int `abi:"cashPurchaseThreshold"`
	JailRepPenaltyPercent      uint8    `abi:"jailRepPenaltyPercent"`
	JailRepPenaltyCap          *big.Int `abi:"jailRepPenaltyCap"`
	WantedPosterSuccessChance  uint8    `abi:"wantedPosterSuccessChance"`
	BreakoutSuccessChance      uint8    `abi:"breakoutSuccessChance"`
	JailDrugConfiscationPercent uint8   `abi:"jailDrugConfiscationPercent"`
	StarterCash                *big.Int `abi:"starterCash"`
	JailChancePerHeat          uint16   `abi:"jailChancePerHeat"`
}

// GameState mirrors IDealersCore.GameState (CHAIN_REFERENCE §1.5). Carries the
// per-rank stake-limit inputs (RepCap, RepTieBonus) that FullDealerState omits.
type GameState struct {
	CurrentArea            uint8    `abi:"currentArea"`
	PreviousArea           uint8    `abi:"previousArea"`
	HeatLevel              uint8    `abi:"heatLevel"`
	DailyAttemptsRemaining uint8    `abi:"dailyAttemptsRemaining"`
	Reputation             *big.Int `abi:"reputation"`
	TotalReputation        *big.Int `abi:"totalReputation"`
	IsInitialized          bool     `abi:"isInitialized"`
	IsJailed               bool     `abi:"isJailed"`
	IsInSafeHouse          bool     `abi:"isInSafeHouse"`
	CashBalance            *big.Int `abi:"cashBalance"`
	BoostActive            bool     `abi:"boostActive"`
	BoostExpiresAt         uint64   `abi:"boostExpiresAt"`
	FreeAreaMovement       bool     `abi:"freeAreaMovement"`
	DrugMultiplier         uint8    `abi:"drugMultiplier"`
	RepMultiplier          uint8    `abi:"repMultiplier"`
	CashMultiplier         uint8    `abi:"cashMultiplier"`
	ExtraAttempts          uint8    `abi:"extraAttempts"`
	JailChance             uint16   `abi:"jailChance"`
	RepWinBonus            int16    `abi:"repWinBonus"`
	RepTieBonus            int16    `abi:"repTieBonus"`
	RepLossPenalty         int16    `abi:"repLossPenalty"`
	RepCap                 int16    `abi:"repCap"`
	Threat                 uint8    `abi:"threat"`
	Armor                  uint8    `abi:"armor"`
	LastBreakoutAttempt    uint32   `abi:"lastBreakoutAttempt"`
	Infamy                 *big.Int `abi:"infamy"`
}

const gameStateABIJSON = `[{"type":"function","name":"getGameState","stateMutability":"view",
  "inputs":[{"name":"tokenId","type":"uint256"}],
  "outputs":[{"name":"","type":"tuple","components":[
    {"name":"currentArea","type":"uint8"},
    {"name":"previousArea","type":"uint8"},
    {"name":"heatLevel","type":"uint8"},
    {"name":"dailyAttemptsRemaining","type":"uint8"},
    {"name":"reputation","type":"uint256"},
    {"name":"totalReputation","type":"uint256"},
    {"name":"isInitialized","type":"bool"},
    {"name":"isJailed","type":"bool"},
    {"name":"isInSafeHouse","type":"bool"},
    {"name":"cashBalance","type":"uint256"},
    {"name":"boostActive","type":"bool"},
    {"name":"boostExpiresAt","type":"uint64"},
    {"name":"freeAreaMovement","type":"bool"},
    {"name":"drugMultiplier","type":"uint8"},
    {"name":"repMultiplier","type":"uint8"},
    {"name":"cashMultiplier","type":"uint8"},
    {"name":"extraAttempts","type":"uint8"},
    {"name":"jailChance","type":"uint16"},
    {"name":"repWinBonus","type":"int16"},
    {"name":"repTieBonus","type":"int16"},
    {"name":"repLossPenalty","type":"int16"},
    {"name":"repCap","type":"int16"},
    {"name":"threat","type":"uint8"},
    {"name":"armor","type":"uint8"},
    {"name":"lastBreakoutAttempt","type":"uint32"},
    {"name":"infamy","type":"uint256"}
  ]}]}]`

var gameStateABI = mustParseABI(gameStateABIJSON)

// GameState reads DealersCore.getGameState — used for the per-action stake cap.
func (r *Reader) GameState(ctx context.Context, tokenID uint64) (*GameState, error) {
	out, err := r.call(ctx, gameStateABI, r.cl.Net.Contracts.DealersCore, "getGameState", new(big.Int).SetUint64(tokenID))
	if err != nil {
		return nil, err
	}
	vals, err := gameStateABI.Unpack("getGameState", out)
	if err != nil {
		return nil, fmt.Errorf("decode getGameState: %w", err)
	}
	return abiConvert[GameState](vals[0]), nil
}

// PVEStakeParams are the DealersPVE public vars feeding the max-stake formula.
type PVEStakeParams struct {
	RepStakeDivisor uint64
	SlopeBps        uint64
	HeadroomBps     uint64
}

const pveStakeABIJSON = `[
  {"type":"function","name":"repStakeDivisor","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"stakeDivisorSlopeBps","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint16"}]},
  {"type":"function","name":"stakeHeadroomBps","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint16"}]}
]`

var pveStakeABI = mustParseABI(pveStakeABIJSON)

// StakeParams reads the three DealersPVE stake-config vars (static-ish; fetch
// once at startup).
func (r *Reader) StakeParams(ctx context.Context) (*PVEStakeParams, error) {
	pve := r.cl.Net.Contracts.DealersPVE
	readU := func(name string) (uint64, error) {
		out, err := r.call(ctx, pveStakeABI, pve, name)
		if err != nil {
			return 0, err
		}
		vals, err := pveStakeABI.Unpack(name, out)
		if err != nil || len(vals) == 0 {
			return 0, fmt.Errorf("decode %s: %w", name, err)
		}
		switch x := vals[0].(type) {
		case *big.Int:
			return x.Uint64(), nil
		case uint64:
			return x, nil
		case uint16:
			return uint64(x), nil
		default:
			return 0, fmt.Errorf("%s: unexpected type %T", name, vals[0])
		}
	}
	div, err := readU("repStakeDivisor")
	if err != nil {
		return nil, err
	}
	slope, err := readU("stakeDivisorSlopeBps")
	if err != nil {
		return nil, err
	}
	head, err := readU("stakeHeadroomBps")
	if err != nil {
		return nil, err
	}
	return &PVEStakeParams{RepStakeDivisor: div, SlopeBps: slope, HeadroomBps: head}, nil
}

// Config reads DealersCore.config() — the on-chain fee schedule.
func (r *Reader) Config(ctx context.Context) (*CoreConfig, error) {
	out, err := r.call(ctx, coreABI, r.cl.Net.Contracts.DealersCore, "config")
	if err != nil {
		return nil, err
	}
	vals, err := coreABI.Unpack("config", out)
	if err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return abiConvert[CoreConfig](vals[0]), nil
}

// EffectiveHeat reads DealersCore.getEffectiveHeat — the live lazy-decayed heat.
// Independent of the multicall path, so comparing the two isolates any decode
// drift.
func (r *Reader) EffectiveHeat(ctx context.Context, tokenID uint64) (uint8, error) {
	out, err := r.call(ctx, coreABI, r.cl.Net.Contracts.DealersCore, "getEffectiveHeat", new(big.Int).SetUint64(tokenID))
	if err != nil {
		return 0, err
	}
	var h uint8
	if err := coreABI.UnpackIntoInterface(&h, "getEffectiveHeat", out); err != nil {
		return 0, fmt.Errorf("decode getEffectiveHeat: %w", err)
	}
	return h, nil
}
