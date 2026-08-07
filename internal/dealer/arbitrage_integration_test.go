//go:build integration

// Shows the real cross-area arbitrage board. Run:
//
//	DEALERS_NET=mainnet go test ./internal/dealer/ -tags integration -run TestLiveArbitrage -v -timeout 60s
package dealer

import (
	"context"
	"os"
	"testing"
	"time"

	"dealers/internal/chain"
	"dealers/internal/chain/bindings"
	"dealers/internal/config"
)

func TestLiveArbitrage(t *testing.T) {
	netName := os.Getenv("DEALERS_NET")
	if netName == "" {
		netName = "mainnet"
	}
	net, _ := config.Profile(netName)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cl, err := chain.Dial(ctx, net)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.Close()
	r := bindings.NewReader(cl)

	areas, err := r.AllAreas(ctx)
	if err != nil {
		t.Fatalf("AllAreas: %v", err)
	}
	t.Logf("scanned %d areas", len(areas))

	pairs := Arbitrage(areas)
	names, _ := r.AreaNames(ctx)
	nm := func(id uint8) string {
		if n, ok := names[id]; ok {
			return n
		}
		return "?"
	}
	for _, p := range pairs {
		t.Logf("%-9s buy %s @%-10s → sell %s @%-10s  = +%s/u", p.DrugName, p.BuyPrice, nm(p.BuyArea), p.SellPrice, nm(p.SellArea), p.Profit)
	}
	if len(pairs) == 0 {
		t.Log("no profitable cross-area spreads currently")
	}
}
