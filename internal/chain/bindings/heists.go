package bindings

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Heist enums (CHAIN_REFERENCE §7).
type HeistFamily uint8

const (
	FamilySupply HeistFamily = 0
	FamilyCash   HeistFamily = 1
)

func (f HeistFamily) String() string {
	if f == FamilyCash {
		return "CASH"
	}
	return "SUPPLY"
}

type HeistStatus uint8

const (
	HeistNone        HeistStatus = 0
	HeistPreStage    HeistStatus = 1
	HeistCommitted   HeistStatus = 2
	HeistRevealedWin HeistStatus = 3
	HeistBusted      HeistStatus = 4
	HeistCashedOut   HeistStatus = 5
	HeistAbandoned   HeistStatus = 6
	HeistSetback     HeistStatus = 7
)

func (s HeistStatus) String() string {
	switch s {
	case HeistNone:
		return "NONE"
	case HeistPreStage:
		return "PRE_STAGE"
	case HeistCommitted:
		return "COMMITTED"
	case HeistRevealedWin:
		return "REVEALED_WIN"
	case HeistBusted:
		return "BUSTED"
	case HeistCashedOut:
		return "CASHED_OUT"
	case HeistAbandoned:
		return "ABANDONED"
	case HeistSetback:
		return "SETBACK"
	default:
		return fmt.Sprintf("Status(%d)", uint8(s))
	}
}

// DailyHeist mirrors IDealersHeists.DailyHeist (CHAIN_REFERENCE §1.4).
type DailyHeist struct {
	Family          uint8    `abi:"family"`
	Difficulty      uint8    `abi:"difficulty"`
	CurrentStage    uint8    `abi:"currentStage"`
	Status          uint8    `abi:"status"`
	EthJackpot      bool     `abi:"ethJackpot"`
	JackpotFired    bool     `abi:"jackpotFired"`
	EntryStake      *big.Int `abi:"entryStake"`
	CurrentPot      *big.Int `abi:"currentPot"`
	CommitSeq       uint64   `abi:"commitSeq"`
	CommitTimestamp uint64   `abi:"commitTimestamp"`
	LastActionTime  uint64   `abi:"lastActionTime"`
	TokenID         *big.Int `abi:"tokenId"`
}

const heistsABIJSON = `[
  {"type":"function","name":"activeHeist","stateMutability":"view",
   "inputs":[{"name":"tokenId","type":"uint256"}],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"getHeist","stateMutability":"view",
   "inputs":[{"name":"heistId","type":"uint256"}],
   "outputs":[{"name":"","type":"tuple","components":[
     {"name":"family","type":"uint8"},
     {"name":"difficulty","type":"uint8"},
     {"name":"currentStage","type":"uint8"},
     {"name":"status","type":"uint8"},
     {"name":"ethJackpot","type":"bool"},
     {"name":"jackpotFired","type":"bool"},
     {"name":"entryStake","type":"uint96"},
     {"name":"currentPot","type":"uint96"},
     {"name":"commitSeq","type":"uint64"},
     {"name":"commitTimestamp","type":"uint64"},
     {"name":"lastActionTime","type":"uint64"},
     {"name":"tokenId","type":"uint256"}
   ]}]},
  {"type":"function","name":"startHeist","stateMutability":"payable",
   "inputs":[{"name":"tokenId","type":"uint256"},{"name":"family","type":"uint8"},{"name":"difficulty","type":"uint8"},{"name":"ethJackpot","type":"bool"}],
   "outputs":[{"name":"heistId","type":"uint256"}]},
  {"type":"function","name":"commitStage","stateMutability":"nonpayable",
   "inputs":[{"name":"heistId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"resolveStage","stateMutability":"nonpayable",
   "inputs":[{"name":"seq","type":"uint64"}],"outputs":[]},
  {"type":"function","name":"cashOut","stateMutability":"nonpayable",
   "inputs":[{"name":"heistId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"abandonHeist","stateMutability":"nonpayable",
   "inputs":[{"name":"heistId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"claimJackpot","stateMutability":"nonpayable",
   "inputs":[{"name":"tokenId","type":"uint256"}],"outputs":[]},
  {"type":"event","name":"HeistStarted","anonymous":false,"inputs":[
    {"name":"heistId","type":"uint256","indexed":true},
    {"name":"tokenId","type":"uint256","indexed":true},
    {"name":"player","type":"address","indexed":true},
    {"name":"family","type":"uint8"},{"name":"difficulty","type":"uint8"},
    {"name":"ethJackpot","type":"bool"},{"name":"cashStake","type":"uint96"}]},
  {"type":"event","name":"StageCommitted","anonymous":false,"inputs":[
    {"name":"heistId","type":"uint256","indexed":true},
    {"name":"seq","type":"uint64","indexed":true},
    {"name":"tokenId","type":"uint256","indexed":true},
    {"name":"stage","type":"uint8"}]},
  {"type":"event","name":"StageWon","anonymous":false,"inputs":[
    {"name":"heistId","type":"uint256","indexed":true},
    {"name":"tokenId","type":"uint256","indexed":true},
    {"name":"stage","type":"uint8"},{"name":"pot","type":"uint96"}]},
  {"type":"event","name":"HeistSetback","anonymous":false,"inputs":[
    {"name":"heistId","type":"uint256","indexed":true},
    {"name":"tokenId","type":"uint256","indexed":true},
    {"name":"stage","type":"uint8"},{"name":"partialPot","type":"uint96"}]},
  {"type":"event","name":"HeistBusted","anonymous":false,"inputs":[
    {"name":"heistId","type":"uint256","indexed":true},
    {"name":"tokenId","type":"uint256","indexed":true},{"name":"stage","type":"uint8"}]},
  {"type":"event","name":"HeistArrest","anonymous":false,"inputs":[
    {"name":"heistId","type":"uint256","indexed":true},
    {"name":"tokenId","type":"uint256","indexed":true}]},
  {"type":"event","name":"HeistCashedOut","anonymous":false,"inputs":[
    {"name":"heistId","type":"uint256","indexed":true},
    {"name":"tokenId","type":"uint256","indexed":true},{"name":"pot","type":"uint96"}]}
]`

var (
	heistsABI = mustParseABI(heistsABIJSON)

	EventHeistStarted   = heistsABI.Events["HeistStarted"].ID
	EventStageCommitted = heistsABI.Events["StageCommitted"].ID
	EventStageWon       = heistsABI.Events["StageWon"].ID
	EventHeistSetback   = heistsABI.Events["HeistSetback"].ID
	EventHeistBusted    = heistsABI.Events["HeistBusted"].ID
	EventHeistArrest    = heistsABI.Events["HeistArrest"].ID
	EventHeistCashedOut = heistsABI.Events["HeistCashedOut"].ID
)

// HeistsAddr returns the DealersHeists address.
func (r *Reader) HeistsAddr() common.Address { return r.cl.Net.Contracts.DealersHeists }

// ActiveHeist returns the dealer's active heist id (0 if none).
func (r *Reader) ActiveHeist(ctx context.Context, tokenID uint64) (uint64, error) {
	out, err := r.call(ctx, heistsABI, r.cl.Net.Contracts.DealersHeists, "activeHeist", new(big.Int).SetUint64(tokenID))
	if err != nil {
		return 0, err
	}
	var id *big.Int
	if err := heistsABI.UnpackIntoInterface(&id, "activeHeist", out); err != nil {
		return 0, fmt.Errorf("decode activeHeist: %w", err)
	}
	return id.Uint64(), nil
}

// GetHeist reads a heist run's full state.
func (r *Reader) GetHeist(ctx context.Context, heistID uint64) (*DailyHeist, error) {
	out, err := r.call(ctx, heistsABI, r.cl.Net.Contracts.DealersHeists, "getHeist", new(big.Int).SetUint64(heistID))
	if err != nil {
		return nil, err
	}
	vals, err := heistsABI.Unpack("getHeist", out)
	if err != nil {
		return nil, fmt.Errorf("decode getHeist: %w", err)
	}
	return abiConvert[DailyHeist](vals[0]), nil
}

// Calldata packers.
func PackStartHeist(tokenID uint64, family HeistFamily, difficulty uint8, ethJackpot bool) ([]byte, error) {
	return heistsABI.Pack("startHeist", new(big.Int).SetUint64(tokenID), uint8(family), difficulty, ethJackpot)
}
func PackCommitStage(heistID uint64) ([]byte, error) {
	return heistsABI.Pack("commitStage", new(big.Int).SetUint64(heistID))
}
func PackResolveStage(seq uint64) ([]byte, error) {
	return heistsABI.Pack("resolveStage", seq)
}
func PackCashOut(heistID uint64) ([]byte, error) {
	return heistsABI.Pack("cashOut", new(big.Int).SetUint64(heistID))
}
func PackAbandonHeist(heistID uint64) ([]byte, error) {
	return heistsABI.Pack("abandonHeist", new(big.Int).SetUint64(heistID))
}
func PackClaimJackpot(tokenID uint64) ([]byte, error) {
	return heistsABI.Pack("claimJackpot", new(big.Int).SetUint64(tokenID))
}

// Test-data packers for the non-indexed portion of heist outcome logs.
func PackStageWonData(stage uint8, pot *big.Int) ([]byte, error) {
	return heistsABI.Events["StageWon"].Inputs.NonIndexed().Pack(stage, pot)
}
func PackHeistBustedData(stage uint8) ([]byte, error) {
	return heistsABI.Events["HeistBusted"].Inputs.NonIndexed().Pack(stage)
}

// ParseHeistID reads the new heist id from a startHeist receipt's HeistStarted
// log (first indexed topic).
func ParseHeistID(logs []*types.Log, heistsAddr common.Address) (uint64, error) {
	for _, lg := range logs {
		if lg.Address == heistsAddr && len(lg.Topics) >= 2 && lg.Topics[0] == EventHeistStarted {
			return new(big.Int).SetBytes(lg.Topics[1].Bytes()).Uint64(), nil
		}
	}
	return 0, fmt.Errorf("no HeistStarted log in receipt")
}

// ParseStageSeq reads the randomness seq from a commitStage receipt. NOTE:
// StageCommitted has three indexed fields (heistId, seq, tokenId) so seq is
// topic[2] (commitStage itself returns void — CHAIN_REFERENCE §2).
func ParseStageSeq(logs []*types.Log, heistsAddr common.Address) (uint64, error) {
	for _, lg := range logs {
		if lg.Address == heistsAddr && len(lg.Topics) >= 3 && lg.Topics[0] == EventStageCommitted {
			return new(big.Int).SetBytes(lg.Topics[2].Bytes()).Uint64(), nil
		}
	}
	return 0, fmt.Errorf("no StageCommitted log in receipt")
}

// StageResult is the decoded outcome of a resolveStage receipt. Exactly one of
// Clean/Setback/Busted is the primary outcome.
type StageResult struct {
	Clean     bool // StageWon — advanced to REVEALED_WIN (or auto-cashed on final stage)
	Setback   bool // HeistSetback — run ended, partial pot paid
	Busted    bool // HeistBusted — lost the stake
	Arrested  bool // HeistArrest — bust escalated to jail
	CashedOut bool // HeistCashedOut — final-stage auto cash-out
	Stage     uint8
	Pot       *big.Int
}

// ParseStageResult decodes a resolveStage receipt.
func ParseStageResult(logs []*types.Log, heistsAddr common.Address) (StageResult, error) {
	var res StageResult
	for _, lg := range logs {
		if lg.Address != heistsAddr || len(lg.Topics) == 0 {
			continue
		}
		switch lg.Topics[0] {
		case EventStageWon:
			var d struct {
				Stage uint8    `abi:"stage"`
				Pot   *big.Int `abi:"pot"`
			}
			if err := heistsABI.UnpackIntoInterface(&d, "StageWon", lg.Data); err != nil {
				return res, fmt.Errorf("decode StageWon: %w", err)
			}
			res.Clean = true
			res.Stage = d.Stage
			res.Pot = d.Pot
		case EventHeistSetback:
			var d struct {
				Stage      uint8    `abi:"stage"`
				PartialPot *big.Int `abi:"partialPot"`
			}
			if err := heistsABI.UnpackIntoInterface(&d, "HeistSetback", lg.Data); err != nil {
				return res, fmt.Errorf("decode HeistSetback: %w", err)
			}
			res.Setback = true
			res.Stage = d.Stage
			res.Pot = d.PartialPot
		case EventHeistBusted:
			var d struct {
				Stage uint8 `abi:"stage"`
			}
			if err := heistsABI.UnpackIntoInterface(&d, "HeistBusted", lg.Data); err != nil {
				return res, fmt.Errorf("decode HeistBusted: %w", err)
			}
			res.Busted = true
			res.Stage = d.Stage
		case EventHeistArrest:
			res.Arrested = true
		case EventHeistCashedOut:
			res.CashedOut = true
		}
	}
	if !res.Clean && !res.Setback && !res.Busted && !res.CashedOut {
		return res, fmt.Errorf("no heist stage outcome log in resolve receipt")
	}
	return res, nil
}
