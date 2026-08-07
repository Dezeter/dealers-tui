//go:build integration

// Live testnet WS smoke test for the block watcher. Run with:
//
//	go test ./internal/chain/ -tags integration -run TestLiveWatcher -v -timeout 40s
package chain

import (
	"context"
	"testing"
	"time"

	"dealers/internal/config"
)

func TestLiveWatcherReceivesBlocks(t *testing.T) {
	net, ok := config.Profile("testnet")
	if !ok {
		t.Fatal("testnet profile missing")
	}
	w := NewBlockWatcher(net)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go w.Run(ctx)

	// Abstract blocks are ~0.25s; expect several within a few seconds.
	seen := 0
	var last uint64
	deadline := time.After(20 * time.Second)
	for seen < 3 {
		select {
		case bn := <-w.Blocks:
			if bn < last {
				t.Errorf("block number went backwards: %d < %d", bn, last)
			}
			last = bn
			seen++
			t.Logf("block %d", bn)
		case <-deadline:
			t.Fatalf("only saw %d blocks in 20s (WS endpoint %s may be down)", seen, net.WS)
		}
	}
	if w.Latest() == 0 {
		t.Error("Latest() still 0 after receiving blocks")
	}
}
