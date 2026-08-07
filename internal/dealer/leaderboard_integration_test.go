//go:build integration

// Computes real leaderboard positions for the owner's dealers. Run:
//
//	DEALERS_OWNER=0x... DEALERS_NET=mainnet go test ./internal/dealer/ -tags integration -run TestLiveLeaderboard -v -timeout 90s
package dealer

import (
	"context"
	"os"
	"testing"
	"time"

	"dealers/internal/chain"
	"dealers/internal/chain/bindings"
	"dealers/internal/config"

	"github.com/ethereum/go-ethereum/common"
)

func TestLiveLeaderboard(t *testing.T) {
	owner := os.Getenv("DEALERS_OWNER")
	if owner == "" {
		t.Skip("set DEALERS_OWNER=0x... to run")
	}
	netName := os.Getenv("DEALERS_NET")
	if netName == "" {
		netName = "mainnet"
	}
	net, _ := config.Profile(netName)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	cl, err := chain.Dial(ctx, net)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()
	r := bindings.NewReader(cl)

	lb := NewLeaderboardCache()
	if err := lb.Refresh(ctx, r); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	total, _ := r.TotalSupply(ctx)
	t.Logf("ranked over %d dealers, computed at %s", total, lb.ComputedAt().Format("15:04:05"))

	ids, err := r.TokensOfOwner(ctx, common.HexToAddress(owner))
	if err != nil {
		t.Fatalf("tokensOfOwner: %v", err)
	}
	for _, id := range ids {
		rk, ok := lb.Get(id)
		if !ok {
			t.Errorf("no rank for owned dealer #%d", id)
			continue
		}
		t.Logf("dealer #%-4d  PvE #%-3d  PvP #%-3d", id, rk.Pve, rk.Pvp)
		if rk.Pve < 1 || rk.Pve > int(total) || rk.Pvp < 1 || rk.Pvp > int(total) {
			t.Errorf("dealer #%d ranks out of range: %+v (total %d)", id, rk, total)
		}
	}
}
