//go:build integration

// Diagnostic: read the live heat for every dealer owned by DEALERS_OWNER on the
// chosen network, via BOTH the multicall bundle and the direct core getter, to
// establish ground truth vs the game UI. Run:
//
//	DEALERS_OWNER=0x... DEALERS_NET=mainnet go test ./internal/chain/bindings/ -tags integration -run TestLiveHeatCheck -v -timeout 60s
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

func TestLiveHeatCheck(t *testing.T) {
	ownerHex := os.Getenv("DEALERS_OWNER")
	if ownerHex == "" {
		t.Skip("set DEALERS_OWNER=0x... to run")
	}
	netName := os.Getenv("DEALERS_NET")
	if netName == "" {
		netName = "mainnet"
	}
	net, ok := config.Profile(netName)
	if !ok {
		t.Fatalf("unknown network %q", netName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	cl, err := chain.Dial(ctx, net)
	if err != nil {
		t.Fatalf("dial %s: %v", netName, err)
	}
	defer cl.Close()
	r := NewReader(cl)

	owner := common.HexToAddress(ownerHex)
	ids, err := r.TokensOfOwner(ctx, owner)
	if err != nil {
		t.Fatalf("tokensOfOwner: %v", err)
	}
	t.Logf("owner %s holds %d dealers on %s", owner.Hex(), len(ids), netName)

	for _, id := range ids {
		st, err := r.GetFullDealerState(ctx, id)
		if err != nil {
			t.Errorf("token %d: getFullDealerState: %v", id, err)
			continue
		}
		direct, err := r.EffectiveHeat(ctx, id)
		if err != nil {
			t.Errorf("token %d: EffectiveHeat: %v", id, err)
			continue
		}
		match := "OK"
		if direct != st.HeatLevel {
			match = "MISMATCH(multicall vs core!)"
		}
		t.Logf("token %-4d heat=%d(%s) rep=%s cash=%s attempts=%d/%d | PVE W/T/L=%d/%d/%d",
			id, st.HeatLevel, match, st.Reputation, st.CashBalance,
			st.DailyAttemptsRemaining, st.MaxAttempts,
			st.PveWins, st.PveTies, st.PveLosses)
		// Non-empty stash lines.
		for _, d := range st.DrugBalances {
			if d.Balance != nil && d.Balance.Sign() > 0 {
				t.Logf("        stash #%s %s ×%s", d.DrugID, d.Name, d.Balance)
			}
		}
	}
}
