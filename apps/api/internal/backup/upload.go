package backup

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
)

var (
	ErrUploadPlan      = errors.New("invalid or conflicting archive upload plan")
	ErrUploadClaimLost = errors.New("archive upload claim is no longer current")
)

// UploadPlan describes an already prepared immutable snapshot. A trusted
// regional coordinator must verify its source binding before registration.
// Registering or claiming this transport task grants no Node execution rights.
type UploadPlan struct {
	OperationID    string                  `json:"operationId"`
	RequestEventID string                  `json:"requestEventId"`
	ID             string                  `json:"id"`
	RegionID       string                  `json:"regionId"`
	ServerID       string                  `json:"serverId"`
	DeploymentID   string                  `json:"deploymentId"`
	NodeID         string                  `json:"nodeId"`
	SnapshotID     string                  `json:"snapshotId"`
	PlacementEpoch int64                   `json:"placementEpoch"`
	StorageID      string                  `json:"storageId"`
	ObjectKey      string                  `json:"objectKey"`
	Asset          assets.PublishedVersion `json:"asset"`
}

func (p UploadPlan) Validate() error {
	for _, id := range []string{p.OperationID, p.RequestEventID, p.ID, p.RegionID, p.ServerID, p.DeploymentID, p.NodeID, p.SnapshotID, p.StorageID, p.ObjectKey} {
		if id == "" || len(id) > 128 || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\x00\r\n") {
			return ErrUploadPlan
		}
	}
	if p.PlacementEpoch < 1 || p.Asset.Validate() != nil {
		return ErrUploadPlan
	}
	return nil
}

type UploadClaim struct {
	Token string
	Plan  UploadPlan
}

type UploadTasks interface {
	ClaimArchiveUpload(context.Context, time.Duration) (*UploadClaim, error)
	CompleteArchiveUpload(context.Context, UploadClaim, StoredArchive) error
	RetryArchiveUpload(context.Context, UploadClaim, time.Duration) error
}

// ArchiveUploaded is a regional execution result. Global must authenticate the
// producing Region and reconcile its own operation before publishing a backup.
type ArchiveUploaded struct {
	SchemaVersion int           `json:"schemaVersion"`
	EventID       string        `json:"eventId"`
	Plan          UploadPlan    `json:"plan"`
	Receipt       StoredArchive `json:"receipt"`
}

// Validate checks internal consistency only. It does not authenticate the Region
// or prove that this snapshot was authorized by a global backup request.
func (e ArchiveUploaded) Validate() error {
	if e.SchemaVersion != 1 || !backupID(e.EventID) || e.Plan.Validate() != nil {
		return ErrUploadPlan
	}
	return e.Receipt.ValidateFor(e.Plan)
}
