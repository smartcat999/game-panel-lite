package backup

import (
	"context"
	"errors"
	"time"
)

var ErrPreparationClaimLost = errors.New("backup preparation claim is no longer current")

// PreparationClaim reserves orchestration work only. It is not a Node execution
// grant and does not authorize reading files or stopping a game server.
type PreparationClaim struct {
	Request Requested
	Token   string
}

type PreparationTasks interface {
	ClaimBackupPreparation(context.Context, time.Duration) (*PreparationClaim, error)
	RetryBackupPreparation(context.Context, PreparationClaim, time.Duration) error
	PrepareArchiveUpload(context.Context, PreparationClaim, UploadPlan) error
}
