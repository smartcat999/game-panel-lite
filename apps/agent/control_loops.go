package main

import (
	"context"
	"time"
)

// runAgentControlLoops keeps liveness independent of workload execution. Each
// lane is serial: slow work coalesces ticks instead of spawning more workers.
// Callbacks must honor ctx; both lanes finish before the runtime can be closed.
func runAgentControlLoops(ctx context.Context, interval time.Duration, heartbeat, work func(context.Context)) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		runAgentPeriodic(ctx, interval, work)
	}()
	defer func() { <-done }()
	runAgentPeriodic(ctx, interval, heartbeat)
}

func runAgentPeriodic(ctx context.Context, interval time.Duration, action func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			action(ctx)
		}
	}
}
