package regional

import (
	"context"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

var ErrRevisionClaimLost = errors.New("regional revision claim is no longer current")

type RevisionClaim struct {
	Event instances.RevisionAvailable
	Token string
}

type RevisionTasks interface {
	ClaimRevision(context.Context, time.Duration) (*RevisionClaim, error)
	SaveRevision(context.Context, RevisionClaim, RevisionSnapshot) error
	RetryRevision(context.Context, RevisionClaim, time.Duration) error
}

type RevisionSource interface {
	GetRevision(context.Context, instances.RevisionAvailable) (RevisionSnapshot, error)
}

type Fetcher struct {
	Tasks                      RevisionTasks
	Source                     RevisionSource
	Lease, Timeout, RetryDelay time.Duration
}

// RunOnce fetches one durable notification outside the database transaction.
// Failed reads remain retryable: a 404 is not proof of revoked execution or safe release.
func (f Fetcher) RunOnce(ctx context.Context) (bool, error) {
	if f.Tasks == nil || f.Source == nil || f.Timeout <= 0 || f.Lease <= f.Timeout || f.Lease > time.Hour || f.RetryDelay < time.Millisecond || f.RetryDelay > 24*time.Hour {
		return false, errors.New("invalid regional fetch settings")
	}
	claim, err := f.Tasks.ClaimRevision(ctx, f.Lease)
	if err != nil || claim == nil {
		return false, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, f.Timeout)
	snapshot, err := f.Source.GetRevision(fetchCtx, claim.Event)
	cancel()
	if err == nil {
		err = snapshot.ValidateFor(claim.Event)
	}
	if err != nil {
		return false, errors.Join(errors.New("regional revision fetch unsuccessful"), f.Tasks.RetryRevision(ctx, *claim, f.RetryDelay))
	}
	if err := f.Tasks.SaveRevision(ctx, *claim, snapshot); err != nil {
		return false, err
	}
	return true, nil
}
