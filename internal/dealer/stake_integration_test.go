//go:build integration

// Validates GameState + StakeParams decode and the max-stake formula live. Run:
//
//	DEALERS_OWNER=0x... DEALERS_NET=mainnet go test ./internal/dealer/ -tags integration -run TestLiveMaxStake -v -timeout 60s
package dealer

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"dealers/internal/chain"
	"dealers/internal/chain/bindings"
	"dealers/internal/config"

	"github.com/ethereum/go-ethereum/common"
)

func TestLiveMaxStake(t *testing.T) {
	owner := os.Getenv("DEALERS_OWNER")
	if owner == "" {
		t.Skip("set DEALERS_OWNER=0x... to run")
	}
	netName := os.Getenv("DEALERS_NET")
	if netName == "" {
		netName = "mainnet"
	}
	net, _ := config.Profile(netName)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	cl, err := chain.Dial(ctx, net)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()
	r := bindings.NewReader(cl)

	sp, err := r.StakeParams(ctx)
	if err != nil {
		t.Fatalf("StakeParams: %v", err)
	}
	t.Logf("stake params: divisor=%d slope=%d headroom=%d", sp.RepStakeDivisor, sp.SlopeBps, sp.HeadroomBps)

	ids, err := r.TokensOfOwner(ctx, common.HexToAddress(owner))
	if err != nil {
		t.Fatalf("tokensOfOwner: %v", err)
	}
	for _, id := range ids {
		gs, err := r.GameState(ctx, id)
		if err != nil {
			t.Errorf("GameState(%d): %v", id, err)
			continue
		}
		ms := MaxStake(gs, sp)
		weed := MaxUnitsAtPrice(ms, big.NewInt(1))
		coke := MaxUnitsAtPrice(ms, big.NewInt(120))
		t.Logf("#%-4d rep=%s repCap=%d tie=%d → maxStake=%v · max Weed@1=%d · max Coke@120=%d",
			id, gs.TotalReputation, gs.RepCap, gs.RepTieBonus, ms, weed, coke)
		if gs.RepCap <= 0 {
			t.Errorf("#%d repCap=%d — GameState likely mis-decoded", id, gs.RepCap)
		}
	}
}
