package bindings

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// DealersMissions hosts daily/weekly missions. checkIn snapshots the per-epoch
// baseline (progress is measured as the delta since check-in), and claim
// redeems a completed mission's reward. getMissionStatus returns everything the
// UI and autopilot need in a single call.
//
// Cadence enum (assumed order Daily=0, Weekly=1 — the priority the autopilot
// follows). MetricType is an opaque uint8 here; we render missions from their
// concrete target/reward/progress, which need no enum knowledge.
const (
	CadenceDaily  uint8 = 0
	CadenceWeekly uint8 = 1
)

// MetricType values (IDealersMissions.MetricType, in enum order). Action metrics
// count games/runs; REP_GAIN/INFAMY_GAIN measure Core score deltas from any
// source; MISSIONS_CLAIMED is an epoch claim-count capstone; ANY_GAMES sums
// PVE games + PVP attacks + heist runs (each costs one daily attempt).
const (
	MetricPVEWins        uint8 = 0
	MetricPVEGames       uint8 = 1
	MetricPVPAttackWins  uint8 = 2
	MetricPVPDefendWins  uint8 = 3
	MetricPVPGames       uint8 = 4
	MetricHeistRuns      uint8 = 5
	MetricHeistStages    uint8 = 6
	MetricHeistCashouts  uint8 = 7
	MetricRepGain        uint8 = 8
	MetricInfamyGain     uint8 = 9
	MetricMissionsClaimed uint8 = 10
	MetricAnyGames       uint8 = 11
)

// MissionTemplate mirrors IDealersMissions.MissionTemplate.
type MissionTemplate struct {
	Metric       uint8    `abi:"metric"`
	Cadence      uint8    `abi:"cadence"`
	Enabled      bool     `abi:"enabled"`
	Target       uint32   `abi:"target"`
	RepReward    uint32   `abi:"repReward"`
	InfamyReward uint32   `abi:"infamyReward"`
	CashReward   *big.Int `abi:"cashReward"`
	DrugID       uint32   `abi:"drugId"`
	DrugAmount   uint32   `abi:"drugAmount"`
}

// MissionStatus mirrors IDealersMissions.MissionStatus — one active mission for
// a dealer, with live progress and claim state.
type MissionStatus struct {
	TemplateID  *big.Int        `abi:"templateId"`
	Mission     MissionTemplate `abi:"mission"`
	Epoch       *big.Int        `abi:"epoch"`
	EpochEndsAt uint64          `abi:"epochEndsAt"`
	Progress    uint32          `abi:"progress"`
	CheckedIn   bool            `abi:"checkedIn"`
	Claimable   bool            `abi:"claimable"`
	Claimed     bool            `abi:"claimed"`
}

const missionsABIJSON = `[
  {"type":"function","name":"checkIn","stateMutability":"nonpayable",
   "inputs":[{"name":"tokenId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"claim","stateMutability":"nonpayable",
   "inputs":[{"name":"tokenId","type":"uint256"},{"name":"templateId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"getMissionStatus","stateMutability":"view",
   "inputs":[{"name":"tokenId","type":"uint256"}],
   "outputs":[{"name":"","type":"tuple[]","components":[
     {"name":"templateId","type":"uint256"},
     {"name":"mission","type":"tuple","components":[
       {"name":"metric","type":"uint8"},
       {"name":"cadence","type":"uint8"},
       {"name":"enabled","type":"bool"},
       {"name":"target","type":"uint32"},
       {"name":"repReward","type":"uint32"},
       {"name":"infamyReward","type":"uint32"},
       {"name":"cashReward","type":"uint96"},
       {"name":"drugId","type":"uint32"},
       {"name":"drugAmount","type":"uint32"}
     ]},
     {"name":"epoch","type":"uint256"},
     {"name":"epochEndsAt","type":"uint64"},
     {"name":"progress","type":"uint32"},
     {"name":"checkedIn","type":"bool"},
     {"name":"claimable","type":"bool"},
     {"name":"claimed","type":"bool"}
   ]}]}
]`

var missionsABI = mustParseABI(missionsABIJSON)

// MissionsAddr returns the DealersMissions address (zero if not deployed).
func (r *Reader) MissionsAddr() common.Address { return r.cl.Net.Contracts.DealersMissions }

// PackMissionCheckIn / PackMissionClaim build the mission calldata.
func PackMissionCheckIn(tokenID uint64) ([]byte, error) {
	return missionsABI.Pack("checkIn", new(big.Int).SetUint64(tokenID))
}

func PackMissionClaim(tokenID, templateID uint64) ([]byte, error) {
	return missionsABI.Pack("claim", new(big.Int).SetUint64(tokenID), new(big.Int).SetUint64(templateID))
}

// MissionStatus reads a dealer's active missions (cached for missionCacheTTL to
// keep the fast fleet poll off the RPC). Returns nil (no error) when the
// contract isn't deployed on this network.
func (r *Reader) MissionStatus(ctx context.Context, tokenID uint64) ([]MissionStatus, error) {
	if r.cl.Net.Contracts.DealersMissions == (common.Address{}) {
		return nil, nil
	}
	r.misMu.Lock()
	if e, ok := r.missionsTTL[tokenID]; ok && time.Since(e.at) < missionCacheTTL {
		r.misMu.Unlock()
		return e.val, nil
	}
	r.misMu.Unlock()

	v, err := r.missionStatusUncached(ctx, tokenID)
	if err == nil {
		r.misMu.Lock()
		r.missionsTTL[tokenID] = missionEntry{at: time.Now(), val: v}
		r.misMu.Unlock()
	}
	return v, err
}

// InvalidateMissions drops a dealer's cached mission status so the next read
// hits the chain (call after a check-in / claim so the UI updates immediately).
func (r *Reader) InvalidateMissions(tokenID uint64) {
	r.misMu.Lock()
	delete(r.missionsTTL, tokenID)
	r.misMu.Unlock()
}

func (r *Reader) missionStatusUncached(ctx context.Context, tokenID uint64) ([]MissionStatus, error) {
	out, err := r.call(ctx, missionsABI, r.cl.Net.Contracts.DealersMissions, "getMissionStatus", new(big.Int).SetUint64(tokenID))
	if err != nil {
		return nil, err
	}
	vals, err := missionsABI.Unpack("getMissionStatus", out)
	if err != nil {
		return nil, fmt.Errorf("decode getMissionStatus: %w", err)
	}
	return *abiConvert[[]MissionStatus](vals[0]), nil
}

// --- decision helpers over a mission-status slice ---

// FirstClaimable returns the template id of the first claimable, unclaimed
// mission, preferring daily over weekly (the autopilot's priority). ok=false
// when nothing is claimable.
func FirstClaimable(ms []MissionStatus) (templateID uint64, ok bool) {
	best := -1
	for i := range ms {
		m := &ms[i]
		if !m.Claimable || m.Claimed || m.TemplateID == nil {
			continue
		}
		if best == -1 || m.Mission.Cadence < ms[best].Mission.Cadence {
			best = i
		}
	}
	if best == -1 {
		return 0, false
	}
	return ms[best].TemplateID.Uint64(), true
}

// NeedsCheckIn reports whether any enabled mission hasn't been checked in this
// epoch (one checkIn snapshots every cadence at once).
func NeedsCheckIn(ms []MissionStatus) bool {
	for i := range ms {
		if ms[i].Mission.Enabled && !ms[i].CheckedIn {
			return true
		}
	}
	return false
}

// SortMissions orders missions daily-first, then by template id, for stable
// display.
func SortMissions(ms []MissionStatus) {
	sort.SliceStable(ms, func(a, b int) bool {
		if ms[a].Mission.Cadence != ms[b].Mission.Cadence {
			return ms[a].Mission.Cadence < ms[b].Mission.Cadence
		}
		return idOf(ms[a].TemplateID) < idOf(ms[b].TemplateID)
	})
}

func idOf(v *big.Int) uint64 {
	if v == nil {
		return 0
	}
	return v.Uint64()
}

// CadenceLabel renders a cadence value.
func CadenceLabel(c uint8) string {
	switch c {
	case CadenceDaily:
		return "daily"
	case CadenceWeekly:
		return "weekly"
	default:
		return fmt.Sprintf("cadence%d", c)
	}
}
