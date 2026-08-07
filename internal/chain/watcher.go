package chain

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"dealers/internal/config"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// BlockWatcher maintains a single WebSocket subscription to new block heads and
// republishes block numbers on Blocks. One watcher serves the whole fleet
// (ADR-2). It self-heals: on any dial/subscription/read error it backs off and
// reconnects (NFR1). Latest() exposes the highest seen block for pollers.
type BlockWatcher struct {
	net    config.Network
	Blocks chan uint64
	latest atomic.Uint64
}

// NewBlockWatcher builds a watcher for the given network. Blocks is buffered so
// a slow consumer coalesces rather than blocking the reader; consumers should
// treat each value as "at least this height".
func NewBlockWatcher(net config.Network) *BlockWatcher {
	return &BlockWatcher{net: net, Blocks: make(chan uint64, 16)}
}

// Latest returns the highest block number observed so far (0 until the first).
func (w *BlockWatcher) Latest() uint64 { return w.latest.Load() }

// Run blocks until ctx is cancelled, keeping the subscription alive across
// disconnects. Each connection attempt: dial WS, seed the current height via
// HTTP-less BlockNumber over the same WS client, then stream heads.
func (w *BlockWatcher) Run(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		if err := w.connectAndStream(ctx); err != nil && ctx.Err() == nil {
			log.Printf("block watcher: %v — reconnecting in %s", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < maxBackoff {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second // clean exit path (ctx cancelled) or reset after success
	}
}

func (w *BlockWatcher) connectAndStream(ctx context.Context) error {
	cl, err := ethclient.DialContext(ctx, w.net.WS)
	if err != nil {
		return err
	}
	defer cl.Close()

	// Seed current height immediately so consumers don't wait a full block.
	if bn, err := cl.BlockNumber(ctx); err == nil {
		w.publish(bn)
	}

	heads := make(chan *types.Header, 8)
	sub, err := cl.SubscribeNewHead(ctx, heads)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-sub.Err():
			return err // triggers reconnect
		case h := <-heads:
			if h != nil && h.Number != nil {
				w.publish(h.Number.Uint64())
			}
		}
	}
}

// publish records the latest height and best-effort forwards it; if the channel
// buffer is full the value is dropped (the next block supersedes it anyway).
func (w *BlockWatcher) publish(bn uint64) {
	for {
		cur := w.latest.Load()
		if bn <= cur {
			break
		}
		if w.latest.CompareAndSwap(cur, bn) {
			break
		}
	}
	select {
	case w.Blocks <- bn:
	default:
	}
}
