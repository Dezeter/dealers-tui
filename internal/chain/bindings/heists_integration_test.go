//go:build integration

// Validates the heist reads (activeHeist + getHeist DailyHeist tuple) live. Run:
//
//	DEALERS_OWNER=0x... DEALERS_NET=mainnet go test ./internal/chain/bindings/ -tags integration -run TestLiveHeistRead -v -timeout 60s
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

func TestLiveHeistRead(t *testing.T) {
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
	r := NewReader(cl)

	// Decode a few historical heists to exercise the DailyHeist tuple.
	for id := uint64(1); id <= 4; id++ {
		h, err := r.GetHeist(ctx, id)
		if err != nil {
			t.Logf("getHeist(%d): %v (may not exist)", id, err)
			continue
		}
		t.Logf("heist #%d: family=%s difficulty=%d stage=%d status=%s pot=%s stake=%s jackpot=%v token=%s",
			id, HeistFamily(h.Family), h.Difficulty, h.CurrentStage, HeistStatus(h.Status),
			h.CurrentPot, h.EntryStake, h.EthJackpot, h.TokenID)
		if h.Status > uint8(HeistSetback) {
			t.Errorf("heist %d status=%d out of enum range — tuple likely mis-decoded", id, h.Status)
		}
		if h.CurrentStage > 5 {
			t.Errorf("heist %d stage=%d > 5 — tuple likely mis-decoded", id, h.CurrentStage)
		}
	}

	if owner := os.Getenv("DEALERS_OWNER"); owner != "" {
		ids, err := r.TokensOfOwner(ctx, common.HexToAddress(owner))
		if err != nil {
			t.Fatalf("tokensOfOwner: %v", err)
		}
		for _, id := range ids {
			active, err := r.ActiveHeist(ctx, id)
			if err != nil {
				t.Errorf("activeHeist(%d): %v", id, err)
				continue
			}
			t.Logf("dealer #%d activeHeist = %d", id, active)
		}
	}
}
