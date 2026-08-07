package bindings

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// PVE choice / hustle / outcome enums (CHAIN_REFERENCE §7).
type Choice uint8

const (
	ChoiceDeal     Choice = 0
	ChoiceThreaten Choice = 1
	ChoiceBail     Choice = 2
)

type HustleType uint8

const (
	HustleBuy  HustleType = 0
	HustleSell HustleType = 1
)

type Outcome uint8

const (
	OutcomeWin  Outcome = 0
	OutcomeTie  Outcome = 1
	OutcomeLoss Outcome = 2
)

func (o Outcome) String() string {
	switch o {
	case OutcomeWin:
		return "WIN"
	case OutcomeTie:
		return "TIE"
	case OutcomeLoss:
		return "LOSS"
	default:
		return fmt.Sprintf("Outcome(%d)", uint8(o))
	}
}

// pveABIJSON — DealersPVE commit/resolve + the events we decode from receipts.
// Signatures verbatim from CHAIN_REFERENCE §2/§2.1. Note commitGame's real arg
// order (tokenId, choice, hustleType, drugId, amount) — differs from TZ §B.6.
const pveABIJSON = `[
  {"type":"function","name":"commitGame","stateMutability":"nonpayable",
   "inputs":[
     {"name":"tokenId","type":"uint256"},
     {"name":"choice","type":"uint8"},
     {"name":"hustleType","type":"uint8"},
     {"name":"drugId","type":"uint256"},
     {"name":"amount","type":"uint256"}],
   "outputs":[{"name":"seq","type":"uint64"}]},
  {"type":"function","name":"resolveGame","stateMutability":"nonpayable",
   "inputs":[{"name":"seq","type":"uint64"}],"outputs":[]},
  {"type":"event","name":"GameCommitted","anonymous":false,"inputs":[
    {"name":"seq","type":"uint64","indexed":true},
    {"name":"tokenId","type":"uint256","indexed":true},
    {"name":"player","type":"address","indexed":true},
    {"name":"choice","type":"uint8"},
    {"name":"hustleType","type":"uint8"},
    {"name":"drugId","type":"uint256"},
    {"name":"amount","type":"uint256"},
    {"name":"price","type":"uint256"},
    {"name":"cashDelta","type":"int256"},
    {"name":"drugDelta","type":"int256"}]},
  {"type":"event","name":"GamePlayed","anonymous":false,"inputs":[
    {"name":"tokenId","type":"uint256","indexed":true},
    {"name":"player","type":"address","indexed":true},
    {"name":"playerChoice","type":"uint8"},
    {"name":"houseChoice","type":"uint8"},
    {"name":"outcome","type":"uint8"},
    {"name":"hustleType","type":"uint8"},
    {"name":"drugId","type":"uint256"},
    {"name":"drugAmount","type":"uint256"},
    {"name":"cashChange","type":"int256"},
    {"name":"reputationChange","type":"int256"},
    {"name":"drugBalanceChange","type":"int256"},
    {"name":"newHeatLevel","type":"uint8"},
    {"name":"stakedCash","type":"uint256"},
    {"name":"stakedDrug","type":"uint256"}]},
  {"type":"event","name":"GameExpired","anonymous":false,"inputs":[
    {"name":"seq","type":"uint64","indexed":true},
    {"name":"tokenId","type":"uint256","indexed":true}]},
  {"type":"event","name":"DealerArrested","anonymous":false,"inputs":[
    {"name":"tokenId","type":"uint256","indexed":true},
    {"name":"player","type":"address","indexed":true},
    {"name":"jailChance","type":"uint16"}]}
]`

var pveABI = mustParseABI(pveABIJSON)

// Exported PVE event topic-0 IDs — useful for log subscriptions/filtering and
// for synthesizing logs in tests/simulations.
var (
	EventGameCommitted  = pveABI.Events["GameCommitted"].ID
	EventGamePlayed     = pveABI.Events["GamePlayed"].ID
	EventGameExpired    = pveABI.Events["GameExpired"].ID
	EventDealerArrested = pveABI.Events["DealerArrested"].ID
)

// PackGamePlayedData ABI-encodes the non-indexed portion of a GamePlayed log.
// Exported so callers can synthesize resolve receipts for tests/simulation.
func PackGamePlayedData(playerChoice, houseChoice uint8, outcome Outcome, hustle uint8,
	drugID, drugAmount, cashChange, repChange, drugChange *big.Int, newHeat uint8, stakedCash, stakedDrug *big.Int) ([]byte, error) {
	return pveABI.Events["GamePlayed"].Inputs.NonIndexed().Pack(
		playerChoice, houseChoice, uint8(outcome), hustle,
		drugID, drugAmount, cashChange, repChange, drugChange, newHeat, stakedCash, stakedDrug)
}

// PackCommitGame builds calldata for DealersPVE.commitGame.
func PackCommitGame(tokenID uint64, choice Choice, hustle HustleType, drugID, amount uint64) ([]byte, error) {
	return pveABI.Pack("commitGame",
		new(big.Int).SetUint64(tokenID),
		uint8(choice),
		uint8(hustle),
		new(big.Int).SetUint64(drugID),
		new(big.Int).SetUint64(amount),
	)
}

// PackResolveGame builds calldata for DealersPVE.resolveGame.
func PackResolveGame(seq uint64) ([]byte, error) {
	return pveABI.Pack("resolveGame", seq)
}

// ParseCommitSeq extracts the commit-reveal seq from a commit receipt's
// GameCommitted log (seq is the first indexed topic). pveAddr scopes the search
// to the DealersPVE contract.
func ParseCommitSeq(logs []*types.Log, pveAddr common.Address) (uint64, error) {
	id := pveABI.Events["GameCommitted"].ID
	for _, lg := range logs {
		if lg.Address == pveAddr && len(lg.Topics) >= 2 && lg.Topics[0] == id {
			return new(big.Int).SetBytes(lg.Topics[1].Bytes()).Uint64(), nil
		}
	}
	return 0, fmt.Errorf("no GameCommitted log found in commit receipt")
}

// GameResult is the decoded outcome of a resolveGame receipt. Exactly one of
// Played / Arrested / Expired is the dominant signal; deltas are set on Played.
type GameResult struct {
	Played      bool
	Arrested    bool
	Expired     bool
	Outcome     Outcome
	HouseChoice uint8
	NewHeat     uint8
	CashChange  *big.Int
	RepChange   *big.Int
	DrugChange  *big.Int
	HustleType  uint8    // 0 buy, 1 sell (from GamePlayed)
	DrugID      *big.Int // what was traded
	DrugAmount  *big.Int // how many units
}

// gamePlayedData mirrors GamePlayed's non-indexed fields for UnpackIntoInterface.
type gamePlayedData struct {
	PlayerChoice      uint8    `abi:"playerChoice"`
	HouseChoice       uint8    `abi:"houseChoice"`
	Outcome           uint8    `abi:"outcome"`
	HustleType        uint8    `abi:"hustleType"`
	DrugId            *big.Int `abi:"drugId"`
	DrugAmount        *big.Int `abi:"drugAmount"`
	CashChange        *big.Int `abi:"cashChange"`
	ReputationChange  *big.Int `abi:"reputationChange"`
	DrugBalanceChange *big.Int `abi:"drugBalanceChange"`
	NewHeatLevel      uint8    `abi:"newHeatLevel"`
	StakedCash        *big.Int `abi:"stakedCash"`
	StakedDrug        *big.Int `abi:"stakedDrug"`
}

// ParseGameResult decodes a resolveGame receipt. A resolve emits GamePlayed on a
// normal outcome, DealerArrested if the jail roll hit (outcome skipped), or
// GameExpired if the reveal window lapsed (treated as a loss on chain).
func ParseGameResult(logs []*types.Log, pveAddr common.Address) (GameResult, error) {
	var res GameResult
	played := pveABI.Events["GamePlayed"].ID
	expired := pveABI.Events["GameExpired"].ID
	arrested := pveABI.Events["DealerArrested"].ID

	for _, lg := range logs {
		if lg.Address != pveAddr || len(lg.Topics) == 0 {
			continue
		}
		switch lg.Topics[0] {
		case played:
			var d gamePlayedData
			if err := pveABI.UnpackIntoInterface(&d, "GamePlayed", lg.Data); err != nil {
				return res, fmt.Errorf("decode GamePlayed: %w", err)
			}
			res.Played = true
			res.Outcome = Outcome(d.Outcome)
			res.HouseChoice = d.HouseChoice
			res.NewHeat = d.NewHeatLevel
			res.CashChange = d.CashChange
			res.RepChange = d.ReputationChange
			res.DrugChange = d.DrugBalanceChange
			res.HustleType = d.HustleType
			res.DrugID = d.DrugId
			res.DrugAmount = d.DrugAmount
		case arrested:
			res.Arrested = true
		case expired:
			res.Expired = true
		}
	}
	if !res.Played && !res.Arrested && !res.Expired {
		return res, fmt.Errorf("no PVE outcome log (GamePlayed/DealerArrested/GameExpired) in resolve receipt")
	}
	return res, nil
}

// PVEAddr returns the DealersPVE address for the reader's network — convenience
// for callers assembling commit/resolve transactions.
func (r *Reader) PVEAddr() common.Address { return r.cl.Net.Contracts.DealersPVE }
