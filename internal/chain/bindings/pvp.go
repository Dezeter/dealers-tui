package bindings

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// PVPTarget mirrors DealersMulticall.PVPTarget (CHAIN_REFERENCE §1.6).
type PVPTarget struct {
	TokenID           *big.Int `abi:"tokenId"`
	Reputation        *big.Int `abi:"reputation"`
	Threat            uint8    `abi:"threat"`
	Armor             uint8    `abi:"armor"`
	AttemptsRemaining uint8    `abi:"attemptsRemaining"`
	WinChance         *big.Int `abi:"winChance"`
	LossChance        *big.Int `abi:"lossChance"`
	CanAttackNow      bool     `abi:"canAttackNow"`
	Infamy            *big.Int `abi:"infamy"`
}

// Read side lives on DealersMulticall.
const pvpMulticallABIJSON = `[
  {"type":"function","name":"getPotentialTargets","stateMutability":"view",
   "inputs":[{"name":"attackerId","type":"uint256"},{"name":"offset","type":"uint256"},{"name":"limit","type":"uint256"}],
   "outputs":[
     {"name":"targets","type":"tuple[]","components":[
       {"name":"tokenId","type":"uint256"},
       {"name":"reputation","type":"uint256"},
       {"name":"threat","type":"uint8"},
       {"name":"armor","type":"uint8"},
       {"name":"attemptsRemaining","type":"uint8"},
       {"name":"winChance","type":"uint256"},
       {"name":"lossChance","type":"uint256"},
       {"name":"canAttackNow","type":"bool"},
       {"name":"infamy","type":"uint256"}
     ]},
     {"name":"totalInArea","type":"uint256"}
   ]},
  {"type":"function","name":"canAttack","stateMutability":"view",
   "inputs":[{"name":"attackerId","type":"uint256"},{"name":"defenderId","type":"uint256"}],
   "outputs":[{"name":"canFight","type":"bool"},{"name":"reason","type":"uint8"}]}
]`

// Write side + events live on DealersPVP.
const pvpABIJSON = `[
  {"type":"function","name":"commitAttack","stateMutability":"nonpayable",
   "inputs":[{"name":"attackerId","type":"uint256"},{"name":"defenderId","type":"uint256"}],
   "outputs":[{"name":"seq","type":"uint64"}]},
  {"type":"function","name":"resolveAttack","stateMutability":"nonpayable",
   "inputs":[{"name":"seq","type":"uint64"}],"outputs":[]},
  {"type":"event","name":"PvpCommitted","anonymous":false,"inputs":[
    {"name":"seq","type":"uint64","indexed":true},
    {"name":"attackerId","type":"uint256","indexed":true},
    {"name":"defenderId","type":"uint256","indexed":true},
    {"name":"attackerThreat","type":"uint8"},
    {"name":"defenderArmor","type":"uint8"},
    {"name":"winChancePct","type":"uint16"},
    {"name":"attackerJailChance","type":"uint16"}]},
  {"type":"event","name":"PVPBattleResult","anonymous":false,"inputs":[
    {"name":"attacker","type":"uint256","indexed":true},
    {"name":"defender","type":"uint256","indexed":true},
    {"name":"attackerWon","type":"bool"},
    {"name":"drugIdStolen","type":"uint256"},
    {"name":"drugsStolen","type":"uint256"},
    {"name":"cashStolen","type":"uint256"},
    {"name":"attackerRepChange","type":"int16"},
    {"name":"defenderRepChange","type":"int16"},
    {"name":"attackerInfamyChange","type":"int16"},
    {"name":"winChancePct","type":"uint16"},
    {"name":"newHeatLevelAttacker","type":"uint8"}]},
  {"type":"event","name":"PvpExpired","anonymous":false,"inputs":[
    {"name":"seq","type":"uint64","indexed":true},
    {"name":"attackerId","type":"uint256","indexed":true}]}
]`

var (
	pvpMulticallABI = mustParseABI(pvpMulticallABIJSON)
	pvpABI          = mustParseABI(pvpABIJSON)

	EventPvpCommitted   = pvpABI.Events["PvpCommitted"].ID
	EventPVPBattleResult = pvpABI.Events["PVPBattleResult"].ID
	EventPvpExpired     = pvpABI.Events["PvpExpired"].ID
)

// PVPAddr returns the DealersPVP address.
func (r *Reader) PVPAddr() common.Address { return r.cl.Net.Contracts.DealersPVP }

// PotentialTargets lists attackable dealers in the attacker's current area.
func (r *Reader) PotentialTargets(ctx context.Context, attackerID uint64, offset, limit uint64) ([]PVPTarget, uint64, error) {
	out, err := r.call(ctx, pvpMulticallABI, r.cl.Net.Contracts.DealersMulticall, "getPotentialTargets",
		new(big.Int).SetUint64(attackerID), new(big.Int).SetUint64(offset), new(big.Int).SetUint64(limit))
	if err != nil {
		return nil, 0, err
	}
	vals, err := pvpMulticallABI.Unpack("getPotentialTargets", out)
	if err != nil {
		return nil, 0, fmt.Errorf("decode getPotentialTargets: %w", err)
	}
	targets := *abiConvert[[]PVPTarget](vals[0])
	total, _ := vals[1].(*big.Int)
	return targets, total.Uint64(), nil
}

// CanAttack checks whether attacker may hit defender right now.
func (r *Reader) CanAttack(ctx context.Context, attackerID, defenderID uint64) (bool, uint8, error) {
	out, err := r.call(ctx, pvpMulticallABI, r.cl.Net.Contracts.DealersMulticall, "canAttack",
		new(big.Int).SetUint64(attackerID), new(big.Int).SetUint64(defenderID))
	if err != nil {
		return false, 0, err
	}
	vals, err := pvpMulticallABI.Unpack("canAttack", out)
	if err != nil {
		return false, 0, fmt.Errorf("decode canAttack: %w", err)
	}
	ok, _ := vals[0].(bool)
	reason, _ := vals[1].(uint8)
	return ok, reason, nil
}

// CanAttackReason renders a canAttack reason code (DealersMulticall.canAttack).
func CanAttackReason(code uint8) string {
	switch code {
	case 0:
		return "ok"
	case 1:
		return "cannot attack yourself"
	case 2:
		return "attacker not initialized"
	case 3:
		return "target not initialized"
	case 4:
		return "attacker is jailed"
	case 5:
		return "attacker is in the safe house"
	case 6:
		return "target is jailed"
	case 7:
		return "target is in the safe house"
	case 8:
		return "target is in a different area"
	case 9:
		return "no daily attempts left"
	case 10:
		return "target has hit its daily defend limit"
	case 11:
		return "target outside your reputation range"
	case 12:
		return "reputation below the PVP minimum (200)"
	default:
		return fmt.Sprintf("cannot attack (reason %d)", code)
	}
}

// PackCommitAttack / PackResolveAttack build the commit-reveal calldata.
func PackCommitAttack(attackerID, defenderID uint64) ([]byte, error) {
	return pvpABI.Pack("commitAttack", new(big.Int).SetUint64(attackerID), new(big.Int).SetUint64(defenderID))
}

func PackResolveAttack(seq uint64) ([]byte, error) {
	return pvpABI.Pack("resolveAttack", seq)
}

// PackPVPBattleResultData ABI-encodes the non-indexed part of a PVPBattleResult
// log (for tests/simulation).
func PackPVPBattleResultData(won bool, drugID, drugs, cash *big.Int, repDelta, defRepDelta, infamyDelta int16, winPct uint16, newHeat uint8) ([]byte, error) {
	return pvpABI.Events["PVPBattleResult"].Inputs.NonIndexed().Pack(
		won, drugID, drugs, cash, repDelta, defRepDelta, infamyDelta, winPct, newHeat)
}

// ParsePvpSeq extracts the seq from a commit receipt's PvpCommitted log.
func ParsePvpSeq(logs []*types.Log, pvpAddr common.Address) (uint64, error) {
	for _, lg := range logs {
		if lg.Address == pvpAddr && len(lg.Topics) >= 2 && lg.Topics[0] == EventPvpCommitted {
			return new(big.Int).SetBytes(lg.Topics[1].Bytes()).Uint64(), nil
		}
	}
	return 0, fmt.Errorf("no PvpCommitted log in commit receipt")
}

// PVPResult is the decoded outcome of a resolveAttack receipt.
type PVPResult struct {
	Fought       bool
	Expired      bool
	Won          bool
	CashStolen   *big.Int
	DrugIDStolen *big.Int
	DrugsStolen  *big.Int
	RepChange    int16
	InfamyChange int16
	NewHeat      uint8
}

type pvpBattleData struct {
	AttackerWon          bool     `abi:"attackerWon"`
	DrugIDStolen         *big.Int `abi:"drugIdStolen"`
	DrugsStolen          *big.Int `abi:"drugsStolen"`
	CashStolen           *big.Int `abi:"cashStolen"`
	AttackerRepChange    int16    `abi:"attackerRepChange"`
	DefenderRepChange    int16    `abi:"defenderRepChange"`
	AttackerInfamyChange int16    `abi:"attackerInfamyChange"`
	WinChancePct         uint16   `abi:"winChancePct"`
	NewHeatLevelAttacker uint8    `abi:"newHeatLevelAttacker"`
}

// ParsePVPResult decodes a resolveAttack receipt (PVPBattleResult, or PvpExpired
// if the reveal window lapsed).
func ParsePVPResult(logs []*types.Log, pvpAddr common.Address) (PVPResult, error) {
	var res PVPResult
	for _, lg := range logs {
		if lg.Address != pvpAddr || len(lg.Topics) == 0 {
			continue
		}
		switch lg.Topics[0] {
		case EventPVPBattleResult:
			var d pvpBattleData
			if err := pvpABI.UnpackIntoInterface(&d, "PVPBattleResult", lg.Data); err != nil {
				return res, fmt.Errorf("decode PVPBattleResult: %w", err)
			}
			res.Fought = true
			res.Won = d.AttackerWon
			res.CashStolen = d.CashStolen
			res.DrugIDStolen = d.DrugIDStolen
			res.DrugsStolen = d.DrugsStolen
			res.RepChange = d.AttackerRepChange
			res.InfamyChange = d.AttackerInfamyChange
			res.NewHeat = d.NewHeatLevelAttacker
		case EventPvpExpired:
			res.Expired = true
		}
	}
	if !res.Fought && !res.Expired {
		return res, fmt.Errorf("no PVP outcome log (PVPBattleResult/PvpExpired) in resolve receipt")
	}
	return res, nil
}
