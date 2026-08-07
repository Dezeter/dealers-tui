// Package chain wraps RPC access to the Abstract network. For the read-only
// Phase 0 surface it uses go-ethereum's ethclient over HTTP (EVM-compatible for
// eth_call/eth_getCode/eth_getBalance). The zksync2-go signer path is added in
// Phase 1 when we start sending transactions (ADR-1, one BlockWatcher for all
// dealers per ADR-2).
package chain

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"dealers/internal/config"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// RPC pacing: the public Abstract endpoints rate-limit (HTTP 429). Every read
// goes through one limiter that spaces requests, and a 429 is retried with
// exponential backoff so a transient limit self-heals instead of surfacing as a
// scary error mid-autopilot.
const (
	rpcMinInterval = 60 * time.Millisecond // ≈16 req/s ceiling across all reads
	rpcMaxRetries  = 5
	rpcRetryBase   = 300 * time.Millisecond
	rpcRetryMax    = 5 * time.Second
)

// Client is a thin RPC handle bound to one resolved network profile.
type Client struct {
	Net     config.Network
	RPC     *ethclient.Client
	ChainID *big.Int

	lim *limiter          // paces all read calls (shared ceiling)
	ws  *ethclient.Client // lazily dialed for block subscriptions (Phase 1)
}

// limiter spaces calls by a minimum interval (a simple leaky bucket). It never
// drops calls — it delays them — so bursts from the fleet fetch / autopilot /
// leaderboard queue behind one another rather than hammering the endpoint.
type limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	wait := l.next.Sub(now)
	l.next = l.next.Add(l.interval)
	l.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// isRateLimited reports whether err is an HTTP 429 / rate-limit response.
func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "429") || strings.Contains(s, "too many requests")
}

// rpcCall runs fn under the client's limiter, retrying on 429 with exponential
// backoff. Non-rate-limit errors return immediately.
func rpcCall[T any](ctx context.Context, c *Client, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	backoff := rpcRetryBase
	for attempt := 0; ; attempt++ {
		if c.lim != nil {
			if err := c.lim.wait(ctx); err != nil {
				return zero, err
			}
		}
		v, err := fn(ctx)
		if err == nil || attempt >= rpcMaxRetries || !isRateLimited(err) {
			return v, err
		}
		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return zero, ctx.Err()
		case <-t.C:
		}
		if backoff < rpcRetryMax {
			backoff *= 2
		}
	}
}

// Dial opens the HTTP RPC connection and reads the chain id. It does NOT verify
// the chain id matches the profile — that belongs in preflight so the failure
// is reported with full context (FR1).
func Dial(ctx context.Context, net config.Network) (*Client, error) {
	rpc, err := ethclient.DialContext(ctx, net.RPC)
	if err != nil {
		return nil, fmt.Errorf("dial rpc %s: %w", net.RPC, err)
	}
	id, err := rpc.ChainID(ctx)
	if err != nil {
		rpc.Close()
		return nil, fmt.Errorf("eth_chainId on %s: %w", net.RPC, err)
	}
	return &Client{Net: net, RPC: rpc, ChainID: id, lim: &limiter{interval: rpcMinInterval}}, nil
}

// WS returns the WebSocket client, dialing it on first use. Used by the block
// watcher (Phase 1); read-only Phase 0 never calls this.
func (c *Client) WS(ctx context.Context) (*ethclient.Client, error) {
	if c.ws != nil {
		return c.ws, nil
	}
	ws, err := ethclient.DialContext(ctx, c.Net.WS)
	if err != nil {
		return nil, fmt.Errorf("dial ws %s: %w", c.Net.WS, err)
	}
	c.ws = ws
	return ws, nil
}

// Close releases both connections.
func (c *Client) Close() {
	if c.RPC != nil {
		c.RPC.Close()
	}
	if c.ws != nil {
		c.ws.Close()
	}
}

// CodeAt returns the deployed bytecode at addr (empty slice = no contract).
func (c *Client) CodeAt(ctx context.Context, addr common.Address) ([]byte, error) {
	return rpcCall(ctx, c, func(ctx context.Context) ([]byte, error) {
		return c.RPC.CodeAt(ctx, addr, nil)
	})
}

// BalanceAt returns the wei balance of addr at the latest block.
func (c *Client) BalanceAt(ctx context.Context, addr common.Address) (*big.Int, error) {
	return rpcCall(ctx, c, func(ctx context.Context) (*big.Int, error) {
		return c.RPC.BalanceAt(ctx, addr, nil)
	})
}

// CallContract performs a read-only eth_call at the latest block.
func (c *Client) CallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	return rpcCall(ctx, c, func(ctx context.Context) ([]byte, error) {
		return c.RPC.CallContract(ctx, msg, nil)
	})
}

// BlockNumber returns the latest block height.
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	return rpcCall(ctx, c, func(ctx context.Context) (uint64, error) {
		return c.RPC.BlockNumber(ctx)
	})
}
