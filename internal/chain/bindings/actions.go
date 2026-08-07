package bindings

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// DealersActions single-tx calls (CHAIN_REFERENCE §4) + the pre-action
// playability check and the movement-fee getter.
// bribeCop (the ETH heat clear) is deliberately omitted — we only clear heat via
// the wanted poster (attempt-based), never by paying ETH.
const actionsABIJSON = `[
  {"type":"function","name":"purchaseAttemptReset","stateMutability":"payable",
   "inputs":[{"name":"tokenId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"travel","stateMutability":"payable",
   "inputs":[{"name":"tokenId","type":"uint256"},{"name":"destinationArea","type":"uint8"}],"outputs":[]},
  {"type":"function","name":"payBail","stateMutability":"payable",
   "inputs":[{"name":"tokenId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"sellDrop","stateMutability":"nonpayable",
   "inputs":[{"name":"tokenId","type":"uint256"},{"name":"drugId","type":"uint256"},{"name":"amount","type":"uint256"}],"outputs":[]}
]`

// Reserved area ids (DEAreaRegistry constants, CHAIN_REFERENCE §1.3).
const (
	JailArea        uint8 = 255 // bail cost is getMovementFee(JailArea)
	BlackMarketArea uint8 = 254

	// BlackMarketMinInfamy gates entry to the black market (DealersActions
	// constant) — reputation does not; infamy does.
	BlackMarketMinInfamy int64 = 10
)

const canPlayABIJSON = `[
  {"type":"function","name":"canPlay","stateMutability":"view",
   "inputs":[{"name":"tokenId","type":"uint256"}],
   "outputs":[{"name":"isPlayable","type":"bool"},{"name":"reason","type":"uint8"}]}
]`

const areaRegistryABIJSON = `[
  {"type":"function","name":"getMovementFee","stateMutability":"view",
   "inputs":[{"name":"areaId","type":"uint8"}],
   "outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"getTotalAreas","stateMutability":"view","inputs":[],
   "outputs":[{"name":"","type":"uint8"}]},
  {"type":"function","name":"getAreaInfo","stateMutability":"view",
   "inputs":[{"name":"areaId","type":"uint8"}],
   "outputs":[{"name":"","type":"tuple","components":[
     {"name":"name","type":"string"},
     {"name":"movementFee","type":"uint256"},
     {"name":"minReputation","type":"uint256"},
     {"name":"isActive","type":"bool"},
     {"name":"isSafeHouse","type":"bool"},
     {"name":"isJail","type":"bool"}
   ]}]}
]`

// Wanted poster is the ETH-free heat clear: commitWantedPoster spends 1 attempt
// and, on a ~50% roll at resolve, fully clears heat (DealersActions, §2.1). This
// is the preferred heat clear — bribeCop (ETH) is intentionally not used.
const wantedPosterABIJSON = `[
  {"type":"function","name":"commitWantedPoster","stateMutability":"nonpayable",
   "inputs":[{"name":"tokenId","type":"uint256"}],"outputs":[{"name":"seq","type":"uint64"}]},
  {"type":"function","name":"resolveWantedPoster","stateMutability":"nonpayable",
   "inputs":[{"name":"seq","type":"uint64"}],"outputs":[]},
  {"type":"event","name":"WantedPosterCommitted","anonymous":false,
   "inputs":[{"name":"seq","type":"uint64","indexed":true},{"name":"tokenId","type":"uint256","indexed":true}]},
  {"type":"event","name":"WantedPosterRemoved","anonymous":false,
   "inputs":[{"name":"tokenId","type":"uint256","indexed":true},{"name":"success","type":"bool"}]},
  {"type":"event","name":"WantedPosterExpired","anonymous":false,
   "inputs":[{"name":"seq","type":"uint64","indexed":true},{"name":"tokenId","type":"uint256","indexed":true}]}
]`

var (
	actionsABI      = mustParseABI(actionsABIJSON)
	canPlayABI      = mustParseABI(canPlayABIJSON)
	areaRegistryABI = mustParseABI(areaRegistryABIJSON)
	wantedPosterABI = mustParseABI(wantedPosterABIJSON)
)

// Exported wanted-poster event IDs (for filtering / synthesizing logs in tests).
var (
	EventWantedPosterCommitted = wantedPosterABI.Events["WantedPosterCommitted"].ID
	EventWantedPosterRemoved   = wantedPosterABI.Events["WantedPosterRemoved"].ID
	EventWantedPosterExpired   = wantedPosterABI.Events["WantedPosterExpired"].ID
)

// PackCommitWantedPoster / PackResolveWantedPoster build the commit-reveal
// calldata for the ETH-free heat clear.
func PackCommitWantedPoster(tokenID uint64) ([]byte, error) {
	return wantedPosterABI.Pack("commitWantedPoster", new(big.Int).SetUint64(tokenID))
}

func PackResolveWantedPoster(seq uint64) ([]byte, error) {
	return wantedPosterABI.Pack("resolveWantedPoster", seq)
}

// PackWantedPosterRemovedData ABI-encodes the non-indexed part of a
// WantedPosterRemoved log (for tests/simulation).
func PackWantedPosterRemovedData(success bool) ([]byte, error) {
	return wantedPosterABI.Events["WantedPosterRemoved"].Inputs.NonIndexed().Pack(success)
}

// Jailbreak is the free (no ETH, no attempt) commit-reveal escape: once per UTC
// day, ~50% success (DealersActions, CHAIN_REFERENCE §2.1).
const breakoutABIJSON = `[
  {"type":"function","name":"commitBreakout","stateMutability":"nonpayable",
   "inputs":[{"name":"tokenId","type":"uint256"}],"outputs":[{"name":"seq","type":"uint64"}]},
  {"type":"function","name":"resolveBreakout","stateMutability":"nonpayable",
   "inputs":[{"name":"seq","type":"uint64"}],"outputs":[]},
  {"type":"event","name":"BreakoutCommitted","anonymous":false,
   "inputs":[{"name":"seq","type":"uint64","indexed":true},{"name":"tokenId","type":"uint256","indexed":true}]},
  {"type":"event","name":"BreakoutAttempted","anonymous":false,
   "inputs":[{"name":"tokenId","type":"uint256","indexed":true},{"name":"success","type":"bool"},{"name":"exitArea","type":"uint8"}]},
  {"type":"event","name":"BreakoutExpired","anonymous":false,
   "inputs":[{"name":"seq","type":"uint64","indexed":true},{"name":"tokenId","type":"uint256","indexed":true}]}
]`

var breakoutABI = mustParseABI(breakoutABIJSON)

var (
	EventBreakoutCommitted = breakoutABI.Events["BreakoutCommitted"].ID
	EventBreakoutAttempted = breakoutABI.Events["BreakoutAttempted"].ID
	EventBreakoutExpired   = breakoutABI.Events["BreakoutExpired"].ID
)

func PackCommitBreakout(tokenID uint64) ([]byte, error) {
	return breakoutABI.Pack("commitBreakout", new(big.Int).SetUint64(tokenID))
}
func PackResolveBreakout(seq uint64) ([]byte, error) {
	return breakoutABI.Pack("resolveBreakout", seq)
}

// PackBreakoutAttemptedData ABI-encodes the non-indexed part of a
// BreakoutAttempted log (for tests).
func PackBreakoutAttemptedData(success bool, exitArea uint8) ([]byte, error) {
	return breakoutABI.Events["BreakoutAttempted"].Inputs.NonIndexed().Pack(success, exitArea)
}

// ParseBreakoutSeq reads the seq from a commitBreakout receipt.
func ParseBreakoutSeq(logs []*types.Log, actionsAddr common.Address) (uint64, error) {
	for _, lg := range logs {
		if lg.Address == actionsAddr && len(lg.Topics) >= 2 && lg.Topics[0] == EventBreakoutCommitted {
			return new(big.Int).SetBytes(lg.Topics[1].Bytes()).Uint64(), nil
		}
	}
	return 0, fmt.Errorf("no BreakoutCommitted log in commit receipt")
}

// BreakoutResult is the decoded outcome of a resolveBreakout receipt.
type BreakoutResult struct {
	Attempted bool // BreakoutAttempted emitted
	Success   bool // escaped
	Expired   bool // reveal window lapsed
}

// ParseBreakoutResult decodes a resolveBreakout receipt.
func ParseBreakoutResult(logs []*types.Log, actionsAddr common.Address) (BreakoutResult, error) {
	var res BreakoutResult
	for _, lg := range logs {
		if lg.Address != actionsAddr || len(lg.Topics) == 0 {
			continue
		}
		switch lg.Topics[0] {
		case EventBreakoutAttempted:
			var d struct {
				Success  bool  `abi:"success"`
				ExitArea uint8 `abi:"exitArea"`
			}
			if err := breakoutABI.UnpackIntoInterface(&d, "BreakoutAttempted", lg.Data); err != nil {
				return res, fmt.Errorf("decode BreakoutAttempted: %w", err)
			}
			res.Attempted = true
			res.Success = d.Success
		case EventBreakoutExpired:
			res.Expired = true
		}
	}
	if !res.Attempted && !res.Expired {
		return res, fmt.Errorf("no breakout outcome log in resolve receipt")
	}
	return res, nil
}

// ParseWantedPosterSeq reads the seq from a commit receipt's
// WantedPosterCommitted log (first indexed topic).
func ParseWantedPosterSeq(logs []*types.Log, actionsAddr common.Address) (uint64, error) {
	id := wantedPosterABI.Events["WantedPosterCommitted"].ID
	for _, lg := range logs {
		if lg.Address == actionsAddr && len(lg.Topics) >= 2 && lg.Topics[0] == id {
			return new(big.Int).SetBytes(lg.Topics[1].Bytes()).Uint64(), nil
		}
	}
	return 0, fmt.Errorf("no WantedPosterCommitted log in commit receipt")
}

// WantedPosterResult is the decoded outcome of a resolveWantedPoster receipt.
type WantedPosterResult struct {
	Removed bool // WantedPosterRemoved emitted
	Success bool // heat actually cleared (the ~50% roll hit)
	Expired bool // reveal window lapsed
}

// ParseWantedPosterResult decodes a resolveWantedPoster receipt.
func ParseWantedPosterResult(logs []*types.Log, actionsAddr common.Address) (WantedPosterResult, error) {
	var res WantedPosterResult
	removed := wantedPosterABI.Events["WantedPosterRemoved"].ID
	expired := wantedPosterABI.Events["WantedPosterExpired"].ID
	for _, lg := range logs {
		if lg.Address != actionsAddr || len(lg.Topics) == 0 {
			continue
		}
		switch lg.Topics[0] {
		case removed:
			var d struct {
				Success bool `abi:"success"`
			}
			if err := wantedPosterABI.UnpackIntoInterface(&d, "WantedPosterRemoved", lg.Data); err != nil {
				return res, fmt.Errorf("decode WantedPosterRemoved: %w", err)
			}
			res.Removed = true
			res.Success = d.Success
		case expired:
			res.Expired = true
		}
	}
	if !res.Removed && !res.Expired {
		return res, fmt.Errorf("no wanted-poster outcome log in resolve receipt")
	}
	return res, nil
}

// PackPurchaseAttemptReset / PackBribeCop / PackTravel build calldata for the
// DealersActions single-tx calls.
func PackPurchaseAttemptReset(tokenID uint64) ([]byte, error) {
	return actionsABI.Pack("purchaseAttemptReset", new(big.Int).SetUint64(tokenID))
}

func PackTravel(tokenID uint64, areaID uint8) ([]byte, error) {
	return actionsABI.Pack("travel", new(big.Int).SetUint64(tokenID), areaID)
}

func PackPayBail(tokenID uint64) ([]byte, error) {
	return actionsABI.Pack("payBail", new(big.Int).SetUint64(tokenID))
}

// PackSellDrop — sell exotic loot in the black market (guaranteed, no energy).
func PackSellDrop(tokenID, drugID, amount uint64) ([]byte, error) {
	return actionsABI.Pack("sellDrop", new(big.Int).SetUint64(tokenID), new(big.Int).SetUint64(drugID), new(big.Int).SetUint64(amount))
}

// ActionsAddr returns the DealersActions address for the reader's network.
func (r *Reader) ActionsAddr() common.Address { return r.cl.Net.Contracts.DealersActions }

// CanPlay reports whether a dealer can currently take a PVE action. reason:
// 0=ok, 1=not initialized, 2=jailed, 3=safe house, 4=no attempts
// (DealersMulticall.canPlay, CHAIN_REFERENCE §1.6).
func (r *Reader) CanPlay(ctx context.Context, tokenID uint64) (bool, uint8, error) {
	out, err := r.call(ctx, canPlayABI, r.cl.Net.Contracts.DealersMulticall, "canPlay", new(big.Int).SetUint64(tokenID))
	if err != nil {
		return false, 0, err
	}
	vals, err := canPlayABI.Unpack("canPlay", out)
	if err != nil {
		return false, 0, fmt.Errorf("decode canPlay: %w", err)
	}
	ok, _ := vals[0].(bool)
	reason, _ := vals[1].(uint8)
	return ok, reason, nil
}

// CanPlayReason renders a canPlay reason code as a human message.
func CanPlayReason(code uint8) string {
	switch code {
	case 0:
		return "ok"
	case 1:
		return "dealer not initialized"
	case 2:
		return "dealer is jailed"
	case 3:
		return "dealer is in the safe house"
	case 4:
		return "no daily attempts left (resets 00:00 UTC, or use Reset Attempts)"
	default:
		return fmt.Sprintf("not playable (reason %d)", code)
	}
}

// MovementFee reads DEAreaRegistry.getMovementFee(areaId) — the ETH cost to
// travel to an area (0 for free routes).
func (r *Reader) MovementFee(ctx context.Context, areaID uint8) (*big.Int, error) {
	out, err := r.call(ctx, areaRegistryABI, r.cl.Net.Contracts.DEAreaRegistry, "getMovementFee", areaID)
	if err != nil {
		return nil, err
	}
	var fee *big.Int
	if err := areaRegistryABI.UnpackIntoInterface(&fee, "getMovementFee", out); err != nil {
		return nil, fmt.Errorf("decode getMovementFee: %w", err)
	}
	return fee, nil
}

// areaInfo is the getAreaInfo return we care about.
type areaInfo struct {
	Name          string   `abi:"name"`
	MovementFee   *big.Int `abi:"movementFee"`
	MinReputation *big.Int `abi:"minReputation"`
	IsActive      bool     `abi:"isActive"`
	IsSafeHouse   bool     `abi:"isSafeHouse"`
	IsJail        bool     `abi:"isJail"`
}

func (r *Reader) areaName(ctx context.Context, id uint8) (string, error) {
	out, err := r.call(ctx, areaRegistryABI, r.cl.Net.Contracts.DEAreaRegistry, "getAreaInfo", id)
	if err != nil {
		return "", err
	}
	vals, err := areaRegistryABI.Unpack("getAreaInfo", out)
	if err != nil {
		return "", err
	}
	return abiConvert[areaInfo](vals[0]).Name, nil
}

// AreaNames loads the id→name map for all areas plus the black-market (254) and
// jail (255) specials (FR12 cache). Names are effectively static, so callers
// fetch this once at startup.
func (r *Reader) AreaNames(ctx context.Context) (map[uint8]string, error) {
	out, err := r.call(ctx, areaRegistryABI, r.cl.Net.Contracts.DEAreaRegistry, "getTotalAreas")
	if err != nil {
		return nil, err
	}
	var total uint8
	if err := areaRegistryABI.UnpackIntoInterface(&total, "getTotalAreas", out); err != nil {
		return nil, fmt.Errorf("decode getTotalAreas: %w", err)
	}
	names := make(map[uint8]string)
	ids := make([]uint8, 0, int(total)+3)
	for i := uint8(0); i <= total; i++ {
		ids = append(ids, i)
	}
	ids = append(ids, 254, 255) // BLACK_MARKET, JAIL
	for _, id := range ids {
		if n, err := r.areaName(ctx, id); err == nil && n != "" {
			names[id] = n
		}
	}
	return names, nil
}
