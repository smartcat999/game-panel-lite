package backup

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
)

// ArchiveFiles opens an immutable, verified local archive by exact asset version.
// The returned file must support ReaderAt and remains owned by this worker.
// assetfiles.Store supplies this port without accepting host paths from a task.
type ArchiveFiles interface {
	Open(context.Context, assets.PublishedVersion) (io.ReadCloser, error)
}

type UploadWorker struct {
	Tasks                      UploadTasks
	Files                      ArchiveFiles
	Archives                   ArchiveStore
	StorageID                  string // must name the injected Archives adapter in this Region
	Lease, Timeout, RetryDelay time.Duration
}

// RunOnce uploads one already prepared archive. It never generates snapshots,
// stops a game, or treats a transport claim as Node execution authority.
func (w UploadWorker) RunOnce(ctx context.Context) (bool, error) {
	if w.Tasks == nil || w.Files == nil || w.Archives == nil || !backupID(w.StorageID) || w.Timeout <= 0 || w.Lease <= w.Timeout || w.Lease > time.Hour || w.RetryDelay < time.Millisecond || w.RetryDelay > 24*time.Hour {
		return false, errors.New("invalid archive upload worker settings")
	}
	workCtx, cancel := context.WithTimeout(ctx, w.Timeout)
	defer cancel()
	claim, err := w.Tasks.ClaimArchiveUpload(workCtx, w.Lease)
	if err != nil || claim == nil {
		return false, err
	}
	var receipt StoredArchive
	if claim.Token == "" || claim.Plan.Validate() != nil || claim.Plan.StorageID != w.StorageID {
		err = ErrUploadPlan
	} else {
		receipt, err = w.transfer(workCtx, claim.Plan)
	}
	if err == nil {
		err = workCtx.Err()
	}
	if err != nil {
		// Provider failures may contain credentials; keep only the durable retry error.
		return false, errors.Join(errors.New("archive upload unsuccessful"), ctx.Err(), w.Tasks.RetryArchiveUpload(ctx, *claim, w.RetryDelay))
	}
	if err := w.Tasks.CompleteArchiveUpload(ctx, *claim, receipt); err != nil {
		return false, err
	}
	return true, nil
}

func (w UploadWorker) transfer(ctx context.Context, plan UploadPlan) (StoredArchive, error) {
	// Recover a completed upload whose response or database completion was lost.
	receipt, err := w.Archives.ResolveUpload(ctx, plan.ObjectKey, plan.Asset)
	if err != nil {
		if ctx.Err() != nil {
			return StoredArchive{}, ctx.Err()
		}
		file, err := w.Files.Open(ctx, plan.Asset)
		if err != nil {
			return StoredArchive{}, err
		}
		source, ok := file.(io.ReaderAt)
		if !ok {
			_ = file.Close()
			return StoredArchive{}, ErrUploadPlan
		}
		receipt, err = w.Archives.Upload(ctx, plan.ObjectKey, plan.Asset, source)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return StoredArchive{}, errors.Join(err, closeErr)
		}
	}
	if err := receipt.ValidateFor(plan); err != nil {
		return StoredArchive{}, err
	}
	return receipt, nil
}
