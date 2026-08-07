//go:build integration

// Validates the new write-path read bindings against live chain: the CoreConfig
// fee schedule (12-field tuple), canPlay, and movement fees. Run:
//
//	DEALERS_NET=mainnet go test ./internal/chain/bindings/ -tags integration -run TestLiveConfigAndFees -v -timeout 60s
package bindings

import (
	"context"
	"os"
	"testing"
	"time"

	"dealers/internal/chain"
	"dealers/internal/config"

	"github.com/ethereum/go-ethereum/common"
)

func TestLiveConfigAndFees(t *testing.T) {
	netName := os.Getenv("DEALERS_NET")
	if netName == "" {
		netName = "mainnet"
	}
	net, ok := config.Profile(netName)
	if !ok {
		t.Fatalf("unknown network %q", netName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cl, err := chain.Dial(ctx, net)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()
	r := NewReader(cl)

	cfg, err := r.Config(ctx)
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	t.Logf("fees: attemptReset=%s wei  bribeCop=%s wei  cashTopup=%s wei",
		cfg.AttemptResetFee, cfg.BribeCopFee, cfg.CashTopupPrice)
	t.Logf("misc: cashTopupAmount=%s starterCash=%s jailChancePerHeat=%d",
		cfg.CashTopupAmount, cfg.StarterCash, cfg.JailChancePerHeat)

	// Sanity: fees are small non-huge numbers (a decode misalignment would yield
	// absurd 10^70 values). Default is 0.001 ETH = 1e15 wei.
	if cfg.BribeCopFee == nil || cfg.BribeCopFee.Sign() < 0 {
		t.Errorf("bribeCopFee decoded wrong: %v", cfg.BribeCopFee)
	}
	if cfg.JailChancePerHeat == 0 || cfg.JailChancePerHeat > 1000 {
		t.Errorf("jailChancePerHeat=%d looks mis-decoded (expect ~5)", cfg.JailChancePerHeat)
	}
	if cfg.StarterCash == nil || cfg.StarterCash.Sign() <= 0 {
		t.Errorf("starterCash decoded wrong: %v", cfg.StarterCash)
	}

	// Movement fees for a couple of areas.
	for _, a := range []uint8{1, 2, 3, 254, 255} {
		fee, err := r.MovementFee(ctx, a)
		if err != nil {
			t.Errorf("MovementFee(%d): %v", a, err)
			continue
		}
		t.Logf("movementFee(area %d) = %s wei", a, fee)
	}

	// Area economy (nested AreaDrug[] decode) — what's tradeable where.
	for _, area := range []uint8{1, 2} {
		econ, err := r.AreaEconomy(ctx, area)
		if err != nil {
			t.Errorf("AreaEconomy(%d): %v", area, err)
			continue
		}
		t.Logf("area %d %q: %d drugs", area, econ.AreaName, len(econ.Drugs))
		for _, d := range econ.Drugs {
			t.Logf("   #%s %-8s buy=%s sell=%s avail=%v", d.DrugID, d.Name, d.BuyPrice, d.SellPrice, d.IsAvailable)
		}
	}

	// Area names (getTotalAreas + getAreaInfo tuple decode).
	names, err := r.AreaNames(ctx)
	if err != nil {
		t.Errorf("AreaNames: %v", err)
	} else {
		t.Logf("areas: %d named", len(names))
		for id := uint8(0); id <= 6; id++ {
			if n, ok := names[id]; ok {
				t.Logf("  area %d = %q", id, n)
			}
		}
		if n, ok := names[255]; ok {
			t.Logf("  area 255 = %q (jail)", n)
		}
	}

	// canPlay for the owner's dealers, if provided.
	if owner := os.Getenv("DEALERS_OWNER"); owner != "" {
		ids, err := r.TokensOfOwner(ctx, common.HexToAddress(owner))
		if err != nil {
			t.Fatalf("tokensOfOwner: %v", err)
		}
		for _, id := range ids {
			ok, reason, err := r.CanPlay(ctx, id)
			if err != nil {
				t.Errorf("canPlay(%d): %v", id, err)
				continue
			}
			t.Logf("canPlay(token %d) = %v (%s)", id, ok, CanPlayReason(reason))
		}
	}
}
