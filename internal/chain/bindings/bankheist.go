package bindings

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

// DealersBankHeist is the seasonal competition contract. Beyond the heist entry
// flow it hosts the daily check-in: checkIn(tokenId) builds a per-dealer "focus"
// streak for the active season (emits CheckedIn(seasonId, tokenId, focus)).
// A dealer can check in once per UTC day and only while not jailed.
//
// We only bind the surface the client needs: the checkIn write, seasonCount /
// getSeason to locate the live season, and focusState to tell whether a dealer
// has already checked in today (so a batch check-in can skip them without
// burning gas on a guaranteed revert).
const bankHeistABIJSON = `[
  {"type":"function","name":"checkIn","stateMutability":"nonpayable",
   "inputs":[{"name":"tokenId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"enter","stateMutability":"payable",
   "inputs":[{"name":"tokenId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"getSeason","stateMutability":"view",
   "inputs":[{"name":"seasonId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"claim","stateMutability":"nonpayable",
   "inputs":[{"name":"seasonId","type":"uint256"},{"name":"tokenId","type":"uint256"}],"outputs":[]},
  {"type":"function","name":"entered","stateMutability":"view",
   "inputs":[{"name":"seasonId","type":"uint256"},{"name":"tokenId","type":"uint256"}],
   "outputs":[{"name":"","type":"bool"}]},
  {"type":"function","name":"seasonCount","stateMutability":"view","inputs":[],
   "outputs":[{"name":"","type":"uint256"}]},
  {"type":"function","name":"focusState","stateMutability":"view",
   "inputs":[{"name":"seasonId","type":"uint256"},{"name":"tokenId","type":"uint256"}],
   "outputs":[{"name":"count","type":"uint32"},{"name":"lastDay","type":"uint32"},{"name":"entryDay","type":"uint32"}]},
  {"type":"event","name":"CheckedIn","anonymous":false,
   "inputs":[{"name":"seasonId","type":"uint256","indexed":true},{"name":"tokenId","type":"uint256","indexed":true},{"name":"focus","type":"uint32","indexed":false}]}
]`

var bankHeistABI = mustParseABI(bankHeistABIJSON)

// secondsPerDay is the check-in day length. The contract stores focusState days
// as block.timestamp/1 day (UTC day number) in uint32, so we compare against the
// same unit. If this ever mismatches the contract, the only cost is a redundant
// (reverting) check-in tx — never a missed check-in (see CheckedInToday).
const secondsPerDay int64 = 86400

// PackCheckIn builds the daily check-in calldata for one dealer.
func PackCheckIn(tokenID uint64) ([]byte, error) {
	return bankHeistABI.Pack("checkIn", new(big.Int).SetUint64(tokenID))
}

// PackEnter builds the season-entry calldata. A dealer must enter() a season once
// before checkIn works for it. In Bank Heist V2 enter() is payable and requires an
// ETH entry fee (read live via SeasonEntryFee) — pass that as the tx value.
func PackEnter(tokenID uint64) ([]byte, error) {
	return bankHeistABI.Pack("enter", new(big.Int).SetUint64(tokenID))
}

// seasonEntryFeeWord is the index of the entry-fee field (a uint256, in wei) within
// the getSeason(seasonId) return tuple — the 2nd word (offset 32). Verified live on
// mainnet V2 (season 0 fee = 0.001 ETH sits at word[1]).
const seasonEntryFeeWord = 1

// SeasonEntryFee reads the ETH entry fee (wei) for a season from getSeason(seasonId).
// We only need one field of the season struct, so we call raw and slice out the
// fee word rather than binding the whole (unstable) tuple. Returns an error if the
// contract isn't deployed or the response is too short to hold the field.
func (r *Reader) SeasonEntryFee(ctx context.Context, seasonID uint64) (*big.Int, error) {
	addr := r.cl.Net.Contracts.DealersBankHeist
	if addr == (common.Address{}) {
		return nil, fmt.Errorf("bank heist contract not deployed")
	}
	data, err := bankHeistABI.Pack("getSeason", new(big.Int).SetUint64(seasonID))
	if err != nil {
		return nil, fmt.Errorf("pack getSeason: %w", err)
	}
	out, err := r.cl.CallContract(ctx, ethereum.CallMsg{To: &addr, Data: data})
	if err != nil {
		return nil, err
	}
	lo := seasonEntryFeeWord * 32
	if len(out) < lo+32 {
		return nil, fmt.Errorf("getSeason returned %d bytes, too short for the fee field", len(out))
	}
	return new(big.Int).SetBytes(out[lo : lo+32]), nil
}

// PackClaim builds the season-reward claim calldata for one dealer. Once a season
// has ended, claim(seasonId, tokenId) pays that dealer's ETH reward to the owner
// (emits Claimed(seasonId, tokenId, to, amount)); it's gas-only on our side.
func PackClaim(seasonID, tokenID uint64) ([]byte, error) {
	return bankHeistABI.Pack("claim", new(big.Int).SetUint64(seasonID), new(big.Int).SetUint64(tokenID))
}

// NeedsHeistCheckIn reports whether a dealer should run today's bank-heist
// check-in: true only when a season is active (contract deployed, season exists)
// and the dealer hasn't checked in yet today. Any error (no season / RPC hiccup)
// yields false so the autopilot simply skips it this tick — never a wasted tx.
func (r *Reader) NeedsHeistCheckIn(ctx context.Context, tokenID uint64) (bool, error) {
	if r.cl.Net.Contracts.DealersBankHeist == (common.Address{}) {
		return false, nil
	}
	season, err := r.ActiveSeason(ctx)
	if err != nil {
		return false, err // no active season (or read failed) → skip
	}
	done, err := r.CheckedInToday(ctx, season, tokenID, time.Now().UTC().Unix())
	if err != nil {
		return false, err
	}
	return !done, nil
}

// Entered reports whether a dealer has already entered the given season. Not
// cached — it's read only on the (infrequent) check-in path, and must be fresh
// right at a season rollover.
func (r *Reader) Entered(ctx context.Context, seasonID, tokenID uint64) (bool, error) {
	out, err := r.call(ctx, bankHeistABI, r.cl.Net.Contracts.DealersBankHeist, "entered",
		new(big.Int).SetUint64(seasonID), new(big.Int).SetUint64(tokenID))
	if err != nil {
		return false, err
	}
	var joined bool
	if err := bankHeistABI.UnpackIntoInterface(&joined, "entered", out); err != nil {
		return false, fmt.Errorf("decode entered: %w", err)
	}
	return joined, nil
}

// BankHeistAddr returns the DealersBankHeist address for the reader's network
// (zero if not deployed there).
func (r *Reader) BankHeistAddr() common.Address { return r.cl.Net.Contracts.DealersBankHeist }

// CanEnterSeason simulates enter(tokenId) as a read-only eth_call from owner (the
// on-chain msg.sender for the dealer's AGW actions), carrying the same ETH value the
// real tx would send, to tell whether the one-time season entry would go through —
// e.g. whether the dealer meets the rep gate and the owner can cover the ETH entry
// fee. Because the AGW is the msg.sender on both the real tx and this From=owner
// simulation, a simulated revert reliably predicts a real one. Returns:
//
//	(true,  nil) enter would succeed
//	(false, nil) enter would REVERT (rep gate / insufficient value) — skip, save gas
//	(false, err) inconclusive (RPC/transport error) — caller should attempt as usual
//
// so an ambiguous read never wrongly blocks a check-in.
func (r *Reader) CanEnterSeason(ctx context.Context, owner common.Address, tokenID uint64, value *big.Int) (bool, error) {
	addr := r.cl.Net.Contracts.DealersBankHeist
	if addr == (common.Address{}) {
		return false, fmt.Errorf("bank heist contract not deployed")
	}
	data, err := PackEnter(tokenID)
	if err != nil {
		return false, err
	}
	// enter() returns nothing, so an empty result with no error means "would pass";
	// call the client directly (r.call treats empty output as an error).
	if _, err := r.cl.CallContract(ctx, ethereum.CallMsg{From: owner, To: &addr, Data: data, Value: value}); err != nil {
		if isRevert(err) {
			return false, nil // definitive: the entry would revert
		}
		return false, err // inconclusive — let the caller proceed
	}
	return true, nil
}

// CanClaimSeason simulates claim(seasonId, tokenId) as a read-only eth_call from
// owner (the AGW that is msg.sender for the dealer's real actions) to tell whether
// a season reward is actually claimable right now — there's no public getter for
// pending rewards, so we ask the contract by dry-running the claim. Because the AGW
// is msg.sender on both the real tx and this From=owner simulation, a simulated
// revert reliably predicts a real one. Returns:
//
//	(true,  nil) claim would succeed → a reward is due
//	(false, nil) claim would REVERT (nothing to claim / already claimed / season not
//	             ended yet) — skip it, save the gas
//	(false, err) inconclusive (RPC/transport error) — caller decides
func (r *Reader) CanClaimSeason(ctx context.Context, owner common.Address, seasonID, tokenID uint64) (bool, error) {
	addr := r.cl.Net.Contracts.DealersBankHeist
	if addr == (common.Address{}) {
		return false, fmt.Errorf("bank heist contract not deployed")
	}
	data, err := PackClaim(seasonID, tokenID)
	if err != nil {
		return false, err
	}
	// claim() may return nothing, so an empty result with no error means "would
	// pass"; call the client directly (r.call treats empty output as an error).
	if _, err := r.cl.CallContract(ctx, ethereum.CallMsg{From: owner, To: &addr, Data: data}); err != nil {
		if isRevert(err) {
			return false, nil // definitive: nothing claimable for this (season, dealer)
		}
		return false, err // inconclusive — let the caller decide
	}
	return true, nil
}

// isRevert reports whether an eth_call error is an on-chain revert (as opposed to
// a transport/rate-limit error). geth and the zkSync/Abstract nodes both surface a
// revert with an "execution reverted" style message; rate-limit errors are already
// retried inside the client and never reach here as a revert.
func isRevert(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "revert")
}

// ActiveSeason returns the current season id (seasonCount-1), cached for
// seasonCacheTTL — seasons roll over rarely, so the fast fleet poll shouldn't
// re-read seasonCount every second. Errors are cached too (short-circuits the
// per-dealer check-in reads on networks without the contract).
func (r *Reader) ActiveSeason(ctx context.Context) (uint64, error) {
	r.ciMu.Lock()
	if !r.seasonAt.IsZero() && time.Since(r.seasonAt) < seasonCacheTTL {
		v, err := r.seasonVal, r.seasonErr
		r.ciMu.Unlock()
		return v, err
	}
	r.ciMu.Unlock()

	v, err := r.activeSeasonUncached(ctx)

	r.ciMu.Lock()
	r.seasonAt, r.seasonVal, r.seasonErr = time.Now(), v, err
	r.ciMu.Unlock()
	return v, err
}

// InvalidateCheckins clears the season + per-dealer check-in caches so the next
// read hits the chain. Call it right after a check-in action so the fleet's Chk
// column flips without waiting for the TTL.
func (r *Reader) InvalidateCheckins() {
	r.ciMu.Lock()
	r.seasonAt = time.Time{}
	for k := range r.checkinTTL {
		delete(r.checkinTTL, k)
	}
	r.ciMu.Unlock()
}

func (r *Reader) activeSeasonUncached(ctx context.Context) (uint64, error) {
	addr := r.cl.Net.Contracts.DealersBankHeist
	if addr == (common.Address{}) {
		return 0, fmt.Errorf("bank heist (check-in) contract not deployed on this network")
	}
	out, err := r.call(ctx, bankHeistABI, addr, "seasonCount")
	if err != nil {
		return 0, err
	}
	var count *big.Int
	if err := bankHeistABI.UnpackIntoInterface(&count, "seasonCount", out); err != nil {
		return 0, fmt.Errorf("decode seasonCount: %w", err)
	}
	if count == nil || count.Sign() == 0 {
		return 0, fmt.Errorf("no active season")
	}
	return count.Uint64() - 1, nil
}

// CheckedInToday reports whether a dealer has already checked in for the given
// season on the UTC day containing nowUnix. Conservative by construction: it
// only returns true when the stored lastDay matches today AND a check-in exists,
// so an unexpected day encoding can at worst make us re-attempt (a cheap revert),
// never skip a dealer that still needs to check in.
func (r *Reader) CheckedInToday(ctx context.Context, seasonID, tokenID uint64, nowUnix int64) (bool, error) {
	// Serve from cache within the TTL — check-in flips at most once/day, so the
	// per-second fleet poll must not re-read focusState for every dealer.
	r.ciMu.Lock()
	if e, ok := r.checkinTTL[tokenID]; ok && time.Since(e.at) < checkinCacheTTL {
		r.ciMu.Unlock()
		return e.done, nil
	}
	r.ciMu.Unlock()

	done, err := r.checkedInTodayUncached(ctx, seasonID, tokenID, nowUnix)
	if err == nil {
		r.ciMu.Lock()
		r.checkinTTL[tokenID] = checkinEntry{at: time.Now(), done: done}
		r.ciMu.Unlock()
	}
	return done, err
}

func (r *Reader) checkedInTodayUncached(ctx context.Context, seasonID, tokenID uint64, nowUnix int64) (bool, error) {
	out, err := r.call(ctx, bankHeistABI, r.cl.Net.Contracts.DealersBankHeist, "focusState",
		new(big.Int).SetUint64(seasonID), new(big.Int).SetUint64(tokenID))
	if err != nil {
		return false, err
	}
	vals, err := bankHeistABI.Unpack("focusState", out)
	if err != nil {
		return false, fmt.Errorf("decode focusState: %w", err)
	}
	count, _ := vals[0].(uint32)
	lastDay, _ := vals[1].(uint32)
	today := uint32(nowUnix / secondsPerDay)
	return count > 0 && lastDay == today, nil
}
