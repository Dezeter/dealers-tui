//go:build integration

// Validates the PVP read decode (getPotentialTargets nested tuple[] + canAttack)
// against live chain for the owner's dealers. Run:
//
//	DEALERS_OWNER=0x... DEALERS_NET=mainnet go test ./internal/chain/bindings/ -tags integration -run TestLivePVPScan -v -timeout 60s
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

func TestLivePVPScan(t *testing.T) {
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
	r := NewReader(cl)

	ids, err := r.TokensOfOwner(ctx, common.HexToAddress(owner))
	if err != nil {
		t.Fatalf("tokensOfOwner: %v", err)
	}

	for _, id := range ids {
		targets, total, err := r.PotentialTargets(ctx, id, 0, 10)
		if err != nil {
			t.Errorf("token %d: PotentialTargets: %v", id, err)
			continue
		}
		t.Logf("attacker #%d: %d targets (%d in area)", id, len(targets), total)
		for _, tgt := range targets {
			// A decode misalignment would surface as absurd win chances / ids.
			if tgt.WinChance != nil && tgt.WinChance.Uint64() > 100 {
				t.Errorf("winChance=%s out of range — PVPTarget likely mis-decoded", tgt.WinChance)
			}
			t.Logf("   target #%s rep=%s win=%s%% infamy=%s canAttackNow=%v",
				tgt.TokenID, tgt.Reputation, tgt.WinChance, tgt.Infamy, tgt.CanAttackNow)
		}
	}
}
