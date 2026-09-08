package regional

import (
	"context"
	"errors"
	"time"
)

var ErrAssetClaimLost = errors.New("regional asset preparation claim is no longer current")

type AssetClaim struct {
	Token    string
	Snapshot RevisionSnapshot
}
type AssetTasks interface {
	ClaimAssets(context.Context, time.Duration) (*AssetClaim, error)
	CompleteAssets(context.Context, AssetClaim) error
	RetryAssets(context.Context, AssetClaim, time.Duration) error
}
type AssetPreparation interface {
	Prepare(context.Context, RevisionSnapshot) error
}
type AssetWorker struct {
	Tasks                      AssetTasks
	Preparation                AssetPreparation
	Lease, Timeout, RetryDelay time.Duration
}

func (w AssetWorker) RunOnce(ctx context.Context) (bool, error) {
	if w.Tasks == nil || w.Preparation == nil || w.Timeout <= 0 || w.Lease <= w.Timeout || w.Lease > time.Hour || w.RetryDelay < time.Millisecond || w.RetryDelay > 24*time.Hour {
		return false, errors.New("invalid asset worker settings")
	}
	claim, err := w.Tasks.ClaimAssets(ctx, w.Lease)
	if err != nil || claim == nil {
		return false, err
	}
	workCtx, cancel := context.WithTimeout(ctx, w.Timeout)
	err = w.Preparation.Prepare(workCtx, claim.Snapshot)
	if err == nil {
		err = workCtx.Err()
	}
	cancel()
	if err != nil {
		return false, errors.Join(errors.New("regional asset preparation unsuccessful"), w.Tasks.RetryAssets(ctx, *claim, w.RetryDelay))
	}
	if err := w.Tasks.CompleteAssets(ctx, *claim); err != nil {
		return false, err
	}
	return true, nil
}
