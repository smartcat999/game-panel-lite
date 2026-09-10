package nodeexecution

import (
	"context"
	"errors"
	"sync"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
)

type AssignmentStore interface {
	PollAssignments(context.Context, contract.NodeID, int, time.Time) []regionexecution.WorkAssignment
	ClaimAssignment(context.Context, contract.WorkAssignmentID, contract.NodeID, time.Time, time.Time) (regionexecution.WorkAssignment, error)
	CompleteAssignment(context.Context, contract.WorkAssignmentID, contract.NodeID, int64, bool, time.Time) (bool, error)
}

type ReconcileFunc func(context.Context, regionexecution.WorkAssignment) error

type Metrics struct {
	Polls                  int
	Claimed                int
	Succeeded              int
	Failed                 int
	ReconciliationFailures int
	LastTaskLatency        time.Duration
}

type Agent struct {
	NodeID      contract.NodeID
	BatchSize   int
	ClaimTTL    time.Duration
	BackoffBase time.Duration
	BackoffMax  time.Duration
	Store       AssignmentStore
	Reconcile   ReconcileFunc
	mu          sync.RWMutex
	metrics     Metrics
}

func (a *Agent) RunOnce(ctx context.Context, now time.Time) (int, error) {
	batchSize := a.BatchSize
	if batchSize < 1 {
		batchSize = 1
	}
	if batchSize > 100 {
		batchSize = 100
	}
	a.mu.Lock()
	a.metrics.Polls++
	a.mu.Unlock()
	assignments := a.Store.PollAssignments(ctx, a.NodeID, batchSize, now)
	claimTTL := a.ClaimTTL
	if claimTTL <= 0 {
		claimTTL = 30 * time.Second
	}
	var failures []error
	processed := 0
	for _, candidate := range assignments {
		claimed, err := a.Store.ClaimAssignment(ctx, candidate.ID, a.NodeID, now.Add(claimTTL), now)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		a.mu.Lock()
		a.metrics.Claimed++
		a.mu.Unlock()
		started := time.Now()
		reconcileErr := a.Reconcile(ctx, claimed)
		latency := time.Since(started)
		completed, completeErr := a.Store.CompleteAssignment(ctx, claimed.ID, a.NodeID, claimed.FencingToken, reconcileErr == nil, now.Add(latency))
		a.mu.Lock()
		a.metrics.LastTaskLatency = latency
		if reconcileErr != nil {
			a.metrics.ReconciliationFailures++
		}
		if completed && reconcileErr == nil {
			a.metrics.Succeeded++
		}
		if completed && reconcileErr != nil {
			a.metrics.Failed++
		}
		a.mu.Unlock()
		if reconcileErr != nil {
			failures = append(failures, reconcileErr)
		}
		if completeErr != nil {
			failures = append(failures, completeErr)
		}
		if completed {
			processed++
		}
	}
	return processed, errors.Join(failures...)
}

func (a *Agent) Run(ctx context.Context, clock func() time.Time) error {
	attempt := 0
	for {
		processed, err := a.RunOnce(ctx, clock())
		if err == nil && processed > 0 {
			attempt = 0
		} else {
			attempt++
		}
		timer := time.NewTimer(a.Backoff(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (a *Agent) Backoff(attempt int) time.Duration {
	base, maximum := a.BackoffBase, a.BackoffMax
	if base <= 0 {
		base = 250 * time.Millisecond
	}
	if maximum < base {
		maximum = 10 * time.Second
	}
	delay := base
	for step := 0; step < attempt && delay < maximum; step++ {
		delay *= 2
		if delay > maximum {
			delay = maximum
		}
	}
	return delay
}

func (a *Agent) Metrics() Metrics { a.mu.RLock(); defer a.mu.RUnlock(); return a.metrics }
