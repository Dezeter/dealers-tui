// Package preflight runs the FR1 bootstrap safety checks before the app does
// anything on chain: chain id matches the active profile, every contract
// address actually has bytecode, and the wallet has some ETH. Any failure
// returns a specific reason — never a silent abort.
package preflight

import (
	"context"
	"fmt"
	"math/big"

	"dealers/internal/chain"
	"dealers/internal/config"

	"github.com/ethereum/go-ethereum/common"
)

// Result summarizes a preflight run for display in the UI/log.
type Result struct {
	Network     string
	ChainID     int64
	BlockNumber uint64
	WalletAddr  common.Address
	WalletWei   *big.Int
	MissingCode []string // contract fields with no bytecode at their address
	Warnings    []string // non-fatal issues (e.g. low ETH runway)
}

// Check verifies the chain, contract bytecode, and wallet balance. cfg selects
// the profile; walletAddr is the resolved owner EOA.
func Check(ctx context.Context, cl *chain.Client, cfg *config.Config, walletAddr common.Address) (*Result, error) {
	net := cfg.Network()

	// 1. Chain id must match the compiled-in profile — guards against pointing
	//    a mainnet key at testnet RPC or vice versa.
	if cl.ChainID == nil || cl.ChainID.Int64() != net.ChainID {
		got := "unknown"
		if cl.ChainID != nil {
			got = cl.ChainID.String()
		}
		return nil, fmt.Errorf("chain id mismatch: profile %s expects %d, RPC %s reports %s",
			net.Name, net.ChainID, net.RPC, got)
	}

	res := &Result{Network: net.Name, ChainID: net.ChainID, WalletAddr: walletAddr}

	bn, err := cl.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("read block number: %w", err)
	}
	res.BlockNumber = bn

	// 2. Every contract we depend on must have bytecode. A blank here usually
	//    means a redeployed address or the wrong network (TZ risk §3.1).
	for name, addr := range contractAddresses(net.Contracts) {
		code, err := cl.CodeAt(ctx, addr)
		if err != nil {
			return nil, fmt.Errorf("eth_getCode %s (%s): %w", name, addr.Hex(), err)
		}
		if len(code) == 0 {
			res.MissingCode = append(res.MissingCode, fmt.Sprintf("%s (%s)", name, addr.Hex()))
		}
	}
	if len(res.MissingCode) > 0 {
		return res, fmt.Errorf("no bytecode at %d contract address(es): %v", len(res.MissingCode), res.MissingCode)
	}

	// 3. Wallet balance. Low balance is a WARNING, not a fatal error: it only
	//    gates write/batch actions (FR11), which Phase 0 doesn't perform. The
	//    enforcement lives at the point a batch is submitted (Phase 2+).
	bal, err := cl.BalanceAt(ctx, walletAddr)
	if err != nil {
		return nil, fmt.Errorf("read wallet balance: %w", err)
	}
	res.WalletWei = bal

	if min, ok := new(big.Int).SetString(cfg.MinETHRunwayWei, 10); ok && cfg.MinETHRunwayWei != "" {
		if bal.Cmp(min) < 0 {
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"ETH runway low: balance %s wei < configured %s wei — enough to view the fleet, but batch/write actions will be gated",
				bal.String(), min.String()))
		}
	}
	return res, nil
}

// contractAddresses maps the contracts we require bytecode for. The renderer
// contracts are display-only and excluded from the hard check.
func contractAddresses(c config.Contracts) map[string]common.Address {
	return map[string]common.Address{
		"DealersNFT":            c.DealersNFT,
		"DealersCore":           c.DealersCore,
		"DealersActions":        c.DealersActions,
		"DealersPVE":            c.DealersPVE,
		"DealersPVP":            c.DealersPVP,
		"DealersHeists":         c.DealersHeists,
		"DealersBoosts":         c.DealersBoosts,
		"DEDrugRegistry":        c.DEDrugRegistry,
		"DEAreaRegistry":        c.DEAreaRegistry,
		"DealersClaims":         c.DealersClaims,
		"DealersMulticall":      c.DealersMulticall,
		"DealersPaymentHandler": c.DealersPaymentHandler,
		"DealersRandomness":     c.DealersRandomness,
	}
}
