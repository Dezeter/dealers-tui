package chain

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIsRateLimited(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("429 Too Many Requests: <!doctype html>"), true},
		{errors.New("Too Many Requests"), true},
		{errors.New("HTTP 429"), true},
		{errors.New("connection refused"), false},
		{errors.New("execution reverted"), false},
	}
	for _, c := range cases {
		if got := isRateLimited(c.err); got != c.want {
			t.Errorf("isRateLimited(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestRpcCallRetriesOn429ThenSucceeds(t *testing.T) {
	c := &Client{lim: &limiter{interval: time.Millisecond}}
	calls := 0
	v, err := rpcCall(context.Background(), c, func(context.Context) (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("429 Too Many Requests")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if v != 42 || calls != 3 {
		t.Errorf("v=%d calls=%d, want 42 and 3 calls", v, calls)
	}
}

func TestRpcCallReturnsNonRateLimitErrorImmediately(t *testing.T) {
	c := &Client{lim: &limiter{interval: time.Millisecond}}
	calls := 0
	_, err := rpcCall(context.Background(), c, func(context.Context) (int, error) {
		calls++
		return 0, errors.New("execution reverted")
	})
	if err == nil || calls != 1 {
		t.Errorf("non-429 error should not retry: calls=%d err=%v", calls, err)
	}
}

func TestLimiterSpacesCalls(t *testing.T) {
	l := &limiter{interval: 20 * time.Millisecond}
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 4; i++ {
		if err := l.wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// 4 calls at 20ms spacing → the 4th is ~60ms after the first (3 intervals).
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("limiter did not space calls: elapsed=%v, want ≥50ms", elapsed)
	}
}

func TestLimiterRespectsContextCancel(t *testing.T) {
	l := &limiter{interval: time.Hour} // force a long wait
	l.wait(context.Background())        // consume the first (immediate) slot
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.wait(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("wait should return context error, got %v", err)
	}
}
