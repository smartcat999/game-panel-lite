package regional

import (
	"context"
	"errors"
	"time"
)

var ErrSchedulingClaimLost = errors.New("regional scheduling claim is no longer current")

type SchedulingClaim struct {
	Token      string
	Deployment Deployment
	Snapshot   RevisionSnapshot
}

type SchedulingTasks interface {
	ClaimScheduling(context.Context, time.Duration) (*SchedulingClaim, error)
	RetryScheduling(context.Context, SchedulingClaim, time.Duration) error
	CompleteScheduling(context.Context, SchedulingClaim, Allocation) error
}

// SchedulingWorker persists retry/completion around one bounded scheduling run.
// Scope resolution must verify current authorization; no node set is inferred
// from inventory or from untrusted user input here.
type SchedulingWorker struct {
	Tasks     SchedulingTasks
	Scheduler Scheduler
	Scopes    interface {
		SchedulingScope(context.Context, RevisionSnapshot) (SchedulingScope, error)
	}
	Lease, Timeout, RetryDelay time.Duration
}

func (w SchedulingWorker) RunOnce(ctx context.Context) (bool, error) {
	if w.Tasks == nil || w.Scopes == nil || w.Timeout <= 0 || w.Lease <= w.Timeout || w.Lease > time.Hour || w.RetryDelay < time.Millisecond || w.RetryDelay > 24*time.Hour {
		return false, errors.New("invalid scheduling worker settings")
	}
	claim, err := w.Tasks.ClaimScheduling(ctx, w.Lease)
	if err != nil || claim == nil {
		return false, err
	}
	workCtx, cancel := context.WithTimeout(ctx, w.Timeout)
	defer cancel()
	scope, err := w.Scopes.SchedulingScope(workCtx, claim.Snapshot)
	var allocation Allocation
	if err == nil {
		allocation, err = w.Scheduler.Schedule(workCtx, claim.Deployment, claim.Snapshot, scope)
	}
	if err == nil {
		err = workCtx.Err()
	}
	if err != nil {
		return false, errors.Join(errors.New("regional scheduling unsuccessful"), w.Tasks.RetryScheduling(ctx, *claim, w.RetryDelay))
	}
	// A reservation may already exist when this commit loses its claim or reply.
	// The next claimant recovers that receipt rather than freeing or duplicating it.
	if err := w.Tasks.CompleteScheduling(workCtx, *claim, allocation); err != nil {
		return false, err
	}
	return true, nil
}
