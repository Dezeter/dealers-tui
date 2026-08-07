// Package bindings holds hand-written ABI fragments and typed decoders for the
// read path (Phase 0). Structs and ABI are transcribed verbatim from
// dealers-contracts@main source — see docs/CHAIN_REFERENCE.md §1.1/§5.1 for the
// field-by-field provenance. Every struct field carries an explicit `abi:` tag
// so decoding never depends on go-ethereum's name-capitalisation heuristic.
package bindings

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"dealers/internal/chain"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// DrugBalance mirrors DealersMulticall.DrugBalance (CHAIN_REFERENCE §1.1).
type DrugBalance struct {
	DrugID  *big.Int `abi:"drugId"`
	Name    string   `abi:"name"`
	Balance *big.Int `abi:"balance"`
	Rarity  uint8    `abi:"rarity"`
}

// FullDealerState mirrors DealersMulticall.FullDealerState — 34 fields in the
// exact ABI tuple order. Fields 18-22 (boost block) are zero unless BoostActive.
type FullDealerState struct {
	Reputation             *big.Int      `abi:"reputation"`
	StashBonusRep          *big.Int      `abi:"stashBonusRep"`
	CurrentArea            uint8         `abi:"currentArea"`
	PreviousArea           uint8         `abi:"previousArea"`
	HeatLevel              uint8         `abi:"heatLevel"`
	DailyAttemptsRemaining uint8         `abi:"dailyAttemptsRemaining"`
	MaxAttempts            uint8         `abi:"maxAttempts"`
	Threat                 uint8         `abi:"threat"`
	Armor                  uint8         `abi:"armor"`
	IsInitialized          bool          `abi:"isInitialized"`
	IsJailed               bool          `abi:"isJailed"`
	IsInSafeHouse          bool          `abi:"isInSafeHouse"`
	JailChance             uint16        `abi:"jailChance"`
	ReputationTitle        string        `abi:"reputationTitle"`
	CashBalance            *big.Int      `abi:"cashBalance"`
	DrugBalances           []DrugBalance `abi:"drugBalances"`
	BoostActive            bool          `abi:"boostActive"`
	BoostExpiry            uint64        `abi:"boostExpiry"`
	DrugMultiplier         uint8         `abi:"drugMultiplier"`
	CashMultiplier         uint8         `abi:"cashMultiplier"`
	RepMultiplier          uint8         `abi:"repMultiplier"`
	FreeAreaMovement       bool          `abi:"freeAreaMovement"`
	PveWins                uint32        `abi:"pveWins"`
	PveLosses              uint32        `abi:"pveLosses"`
	PveTies                uint32        `abi:"pveTies"`
	PvpAttackWins          uint32        `abi:"pvpAttackWins"`
	PvpAttackLosses        uint32        `abi:"pvpAttackLosses"`
	PvpDefendWins          uint32        `abi:"pvpDefendWins"`
	PvpDefendLosses        uint32        `abi:"pvpDefendLosses"`
	LastBreakoutAttempt    uint32        `abi:"lastBreakoutAttempt"`
	CanBreakoutToday       bool          `abi:"canBreakoutToday"`
	AttacksReceivedToday   uint8         `abi:"attacksReceivedToday"`
	MaxAttacksPerDay       uint8         `abi:"maxAttacksPerDay"`
	Infamy                 *big.Int      `abi:"infamy"`
}

// ABI fragments — only the read methods Phase 0 needs.
const multicallABIJSON = `[{
  "type":"function","name":"getFullDealerState","stateMutability":"view",
  "inputs":[{"name":"tokenId","type":"uint256"}],
  "outputs":[{"name":"","type":"tuple","components":[
    {"name":"reputation","type":"uint256"},
    {"name":"stashBonusRep","type":"uint256"},
    {"name":"currentArea","type":"uint8"},
    {"name":"previousArea","type":"uint8"},
    {"name":"heatLevel","type":"uint8"},
    {"name":"dailyAttemptsRemaining","type":"uint8"},
    {"name":"maxAttempts","type":"uint8"},
    {"name":"threat","type":"uint8"},
    {"name":"armor","type":"uint8"},
    {"name":"isInitialized","type":"bool"},
    {"name":"isJailed","type":"bool"},
    {"name":"isInSafeHouse","type":"bool"},
    {"name":"jailChance","type":"uint16"},
    {"name":"reputationTitle","type":"string"},
    {"name":"cashBalance","type":"uint256"},
    {"name":"drugBalances","type":"tuple[]","components":[
      {"name":"drugId","type":"uint256"},
      {"name":"name","type":"string"},
      {"name":"balance","type":"uint256"},
      {"name":"rarity","type":"uint8"}
    ]},
    {"name":"boostActive","type":"bool"},
    {"name":"boostExpiry","type":"uint64"},
    {"name":"drugMultiplier","type":"uint8"},
    {"name":"cashMultiplier","type":"uint8"},
    {"name":"repMultiplier","type":"uint8"},
    {"name":"freeAreaMovement","type":"bool"},
    {"name":"pveWins","type":"uint32"},
    {"name":"pveLosses","type":"uint32"},
    {"name":"pveTies","type":"uint32"},
    {"name":"pvpAttackWins","type":"uint32"},
    {"name":"pvpAttackLosses","type":"uint32"},
    {"name":"pvpDefendWins","type":"uint32"},
    {"name":"pvpDefendLosses","type":"uint32"},
    {"name":"lastBreakoutAttempt","type":"uint32"},
    {"name":"canBreakoutToday","type":"bool"},
    {"name":"attacksReceivedToday","type":"uint8"},
    {"name":"maxAttacksPerDay","type":"uint8"},
    {"name":"infamy","type":"uint256"}
  ]}]
}]`

// DealersNFT is ERC721Enumerable; tokensOfOwner is the convenience enumerator
// (CHAIN_REFERENCE §5.1) — resolves the FR2/TODO-5 auto-discovery path.
const nftABIJSON = `[
  {"type":"function","name":"tokensOfOwner","stateMutability":"view",
   "inputs":[{"name":"owner","type":"address"}],
   "outputs":[{"name":"","type":"uint256[]"}]},
  {"type":"function","name":"balanceOf","stateMutability":"view",
   "inputs":[{"name":"owner","type":"address"}],
   "outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"totalSupply","stateMutability":"view",
   "inputs":[],"outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"tokenByIndex","stateMutability":"view",
   "inputs":[{"name":"index","type":"uint256"}],
   "outputs":[{"name":"","type":"uint256"}]}
]`

var (
	multicallABI = mustParseABI(multicallABIJSON)
	nftABI       = mustParseABI(nftABIJSON)
)

func mustParseABI(s string) abi.ABI {
	a, err := abi.JSON(strings.NewReader(s))
	if err != nil {
		panic("bindings: bad ABI literal: " + err.Error())
	}
	return a
}

// abiConvert copies an abi-decoded anonymous tuple into a named struct T by
// field name (the abigen pattern) — used for single-tuple returns.
func abiConvert[T any](v any) *T {
	return abi.ConvertType(v, new(T)).(*T)
}

// Reader issues read-only calls against a network's contracts.
type Reader struct {
	cl *chain.Client

	// Check-in status and the active season change ≤ once/day, but the fleet
	// polls fast (1s under autopilot). These caches keep those reads off the hot
	// path so we don't burn the RPC rate limit on values that rarely move.
	ciMu       sync.Mutex
	seasonAt   time.Time
	seasonVal  uint64
	seasonErr  error
	checkinTTL map[uint64]checkinEntry

	// Mission status per dealer (progress moves as they act, so a short TTL keeps
	// the fast fleet poll off the RPC while staying fresh enough).
	misMu       sync.Mutex
	missionsTTL map[uint64]missionEntry
}

type checkinEntry struct {
	at   time.Time
	done bool
}

type missionEntry struct {
	at  time.Time
	val []MissionStatus
}

// Cache TTLs for the rarely-changing bank-heist reads and mission status.
const (
	seasonCacheTTL  = 5 * time.Minute
	checkinCacheTTL = 45 * time.Second
	missionCacheTTL = 20 * time.Second
)

// NewReader binds a Reader to a live chain client.
func NewReader(cl *chain.Client) *Reader {
	return &Reader{cl: cl, checkinTTL: map[uint64]checkinEntry{}, missionsTTL: map[uint64]missionEntry{}}
}

// call packs, eth_calls, and returns raw return bytes.
func (r *Reader) call(ctx context.Context, a abi.ABI, to common.Address, method string, args ...any) ([]byte, error) {
	data, err := a.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", method, err)
	}
	out, err := r.cl.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data})
	if err != nil {
		return nil, fmt.Errorf("eth_call %s: %w", method, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s returned no data (contract missing or reverted)", method)
	}
	return out, nil
}

// GetFullDealerState reads the dashboard snapshot for one dealer. Reverts
// on-chain with DealerNotInitialized(tokenId) surface as an eth_call error.
func (r *Reader) GetFullDealerState(ctx context.Context, tokenID uint64) (*FullDealerState, error) {
	to := r.cl.Net.Contracts.DealersMulticall
	out, err := r.call(ctx, multicallABI, to, "getFullDealerState", new(big.Int).SetUint64(tokenID))
	if err != nil {
		return nil, err
	}
	// Single-tuple return: go-ethereum can't UnpackIntoInterface straight into
	// the struct, so decode to the anonymous tuple then ConvertType into ours
	// (the abigen pattern; matches fields by capitalised component name).
	vals, err := multicallABI.Unpack("getFullDealerState", out)
	if err != nil {
		return nil, fmt.Errorf("decode getFullDealerState(%d): %w", tokenID, err)
	}
	st := abi.ConvertType(vals[0], new(FullDealerState)).(*FullDealerState)
	return st, nil
}

// TokensOfOwner returns the dealer token ids held by owner (FR2 auto-discovery).
func (r *Reader) TokensOfOwner(ctx context.Context, owner common.Address) ([]uint64, error) {
	to := r.cl.Net.Contracts.DealersNFT
	out, err := r.call(ctx, nftABI, to, "tokensOfOwner", owner)
	if err != nil {
		return nil, err
	}
	var ids []*big.Int
	if err := nftABI.UnpackIntoInterface(&ids, "tokensOfOwner", out); err != nil {
		return nil, fmt.Errorf("decode tokensOfOwner: %w", err)
	}
	res := make([]uint64, len(ids))
	for i, id := range ids {
		res[i] = id.Uint64()
	}
	return res, nil
}

// TotalSupply returns the number of minted dealer NFTs (ERC721Enumerable).
func (r *Reader) TotalSupply(ctx context.Context) (uint64, error) {
	out, err := r.call(ctx, nftABI, r.cl.Net.Contracts.DealersNFT, "totalSupply")
	if err != nil {
		return 0, err
	}
	var n *big.Int
	if err := nftABI.UnpackIntoInterface(&n, "totalSupply", out); err != nil {
		return 0, fmt.Errorf("decode totalSupply: %w", err)
	}
	return n.Uint64(), nil
}

// TokenByIndex returns the token id at a global enumeration index.
func (r *Reader) TokenByIndex(ctx context.Context, index uint64) (uint64, error) {
	out, err := r.call(ctx, nftABI, r.cl.Net.Contracts.DealersNFT, "tokenByIndex", new(big.Int).SetUint64(index))
	if err != nil {
		return 0, err
	}
	var id *big.Int
	if err := nftABI.UnpackIntoInterface(&id, "tokenByIndex", out); err != nil {
		return 0, fmt.Errorf("decode tokenByIndex: %w", err)
	}
	return id.Uint64(), nil
}
