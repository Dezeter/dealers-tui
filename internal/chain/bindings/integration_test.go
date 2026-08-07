//go:build integration

// Live testnet smoke test for the read path. Run with:
//
//	go test ./internal/chain/bindings/ -tags integration -v -timeout 60s
//
// It hits Abstract testnet (chain 11124) with no keys — read-only. Proves the
// hand-written ABI decodes a real on-chain FullDealerState (the 34-field tuple
// most at risk from a transcription error).
package bindings

import (
	"context"
	"testing"
	"time"

	"dealers/internal/chain"
	"dealers/internal/config"
)

func testnetReader(t *testing.T) (*Reader, context.Context, func()) {
	t.Helper()
	net, ok := config.Profile("testnet")
	if !ok {
		t.Fatal("testnet profile missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	cl, err := chain.Dial(ctx, net)
	if err != nil {
		cancel()
		t.Fatalf("dial testnet: %v", err)
	}
	return NewReader(cl), ctx, func() { cl.Close(); cancel() }
}

func TestLiveTestnetFullDealerState(t *testing.T) {
	r, ctx, done := testnetReader(t)
	defer done()

	total, err := r.TotalSupply(ctx)
	if err != nil {
		t.Fatalf("totalSupply: %v", err)
	}
	t.Logf("testnet totalSupply = %d", total)
	if total == 0 {
		t.Skip("no dealers minted on testnet; cannot validate getFullDealerState decode")
	}

	tokenID, err := r.TokenByIndex(ctx, 0)
	if err != nil {
		t.Fatalf("tokenByIndex(0): %v", err)
	}

	st, err := r.GetFullDealerState(ctx, tokenID)
	if err != nil {
		t.Fatalf("getFullDealerState(%d): %v", tokenID, err)
	}
	t.Logf("dealer %d: title=%q rep=%s heat=%d cash=%s area=%d attempts=%d/%d infamy=%s jailed=%v drugs=%d",
		tokenID, st.ReputationTitle, st.Reputation, st.HeatLevel, st.CashBalance,
		st.CurrentArea, st.DailyAttemptsRemaining, st.MaxAttempts, st.Infamy, st.IsJailed, len(st.DrugBalances))

	// Sanity checks that would fail loudly on a mis-decoded tuple.
	if !st.IsInitialized {
		t.Errorf("token %d from tokenByIndex(0) decoded IsInitialized=false", tokenID)
	}
	if st.ReputationTitle == "" {
		t.Errorf("empty reputationTitle — string field likely mis-aligned in the tuple")
	}
	if st.HeatLevel > 5 {
		t.Errorf("heatLevel=%d out of range 0..5 — tuple likely mis-aligned", st.HeatLevel)
	}
	if st.MaxAttempts == 0 {
		t.Errorf("maxAttempts=0 — tuple likely mis-aligned")
	}
	for i, d := range st.DrugBalances {
		if d.Name == "" {
			t.Errorf("drugBalances[%d] empty name — nested tuple mis-aligned", i)
		}
	}
}
