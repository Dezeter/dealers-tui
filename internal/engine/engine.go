// Package engine wires the commit-reveal runtime together: one block watcher
// (ADR-2) feeding one scheduler that drives the DealerManager's resolver. On
// start it does a catch-up pass at the current height so any rounds persisted by
// a previous process resume immediately (FR8).
package engine

import (
	"context"
	"log"
	"time"

	"dealers/internal/chain"
	"dealers/internal/chain/bindings"
	"dealers/internal/dealer"
	"dealers/internal/scheduler"
	"dealers/internal/store"
)

// Engine owns the background runtime for one network + wallet.
type Engine struct {
	cl      *chain.Client
	watcher *chain.BlockWatcher
	sched   *scheduler.Scheduler
	mgr     *dealer.Manager
	auto    *Autopilot
	logger  *log.Logger
}

// New builds the engine. sender is required (resolves send transactions); pass
// a *chain.Sender. ids is the managed dealer set (for the autopilot); strategy
// is the autopilot policy (starts disabled regardless).
func New(cl *chain.Client, st *store.Store, sender dealer.TxSender, ids []uint64, strategy dealer.Strategy, logger *log.Logger) *Engine {
	if logger == nil {
		logger = log.Default()
	}
	reader := bindings.NewReader(cl)
	mgr := dealer.NewManager(cl.Net, sender, reader, st, logger)
	if strategy == nil {
		strategy = dealer.ManualStrategy{}
	}
	return &Engine{
		cl:      cl,
		watcher: chain.NewBlockWatcher(cl.Net),
		sched:   scheduler.New(st, mgr, logger),
		mgr:     mgr,
		auto:    NewAutopilot(mgr, st, reader, strategy, ids, logger),
		logger:  logger,
	}
}

// Manager exposes the DealerManager for submitting actions.
func (e *Engine) Manager() *dealer.Manager { return e.mgr }

// Autopilot exposes the autopilot for the UI toggle.
func (e *Engine) Autopilot() *Autopilot { return e.auto }

// Run starts the watcher, scheduler, and autopilot and blocks until ctx is
// cancelled. It performs an immediate resume pass so committed rounds don't wait
// for the next block tick.
func (e *Engine) Run(ctx context.Context) {
	go e.watcher.Run(ctx)
	go e.auto.Run(ctx, 12*time.Second)

	if bn, err := e.cl.BlockNumber(ctx); err == nil {
		e.logger.Printf("engine: resume pass at block %d", bn)
		e.sched.OnBlock(ctx, bn)
	} else {
		e.logger.Printf("engine: resume pass skipped (block number: %v)", err)
	}

	e.sched.Run(ctx, e.watcher.Blocks)
}
