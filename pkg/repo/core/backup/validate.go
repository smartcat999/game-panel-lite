package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

var (
	ErrNotFound = errors.New("backup command not found")
	ErrConflict = errors.New("backup command conflict")
)

type (
	BackupGateRecord    = corev1.BackupGateRecord
	BackupCommandRecord = corev1.BackupCommandRecord
	StorageBinding      = s3.Binding
)

type Phase string

const (
	PendingQuota Phase = corev1.BackupPhasePendingQuota
	Ready        Phase = corev1.BackupPhaseReady
	Uploading    Phase = corev1.BackupPhaseUploading
	Unknown      Phase = corev1.BackupPhaseUnknown
	Completed    Phase = corev1.BackupPhaseCompleted
	Deleting     Phase = corev1.BackupPhaseDeleting
	Deleted      Phase = corev1.BackupPhaseDeleted
)

type Command struct {
	OperationID       string         `json:"operationID"`
	BackupUID         string         `json:"backupUID"`
	AllocationID      string         `json:"allocationID"`
	Tenant            string         `json:"tenant"`
	Owner             pgutil.Owner   `json:"owner"`
	ObjectKey         string         `json:"objectKey"`
	RequestPath       string         `json:"requestPath,omitempty"`
	MaxBytes          int64          `json:"maxBytes"`
	RequestedMaxBytes int64          `json:"requestedMaxBytes"`
	RateLimit         int64          `json:"rateLimit"`
	NodeName          string         `json:"nodeName"`
	Binding           pgutil.Binding `json:"binding"`
	ApprovedRevision  int64          `json:"approvedRevision"`
	Storage           StorageBinding `json:"storage"`
	DirectUpload      bool           `json:"directUpload,omitempty"`
}

type Result struct {
	SizeBytes      int64     `json:"sizeBytes"`
	SHA256         string    `json:"sha256,omitempty"`
	CompletedAt    time.Time `json:"completedAt"`
	StagingCleaned bool      `json:"stagingCleaned,omitempty"`
}

type Record struct {
	Command         Command    `json:"command"`
	Phase           Phase      `json:"phase"`
	Version         int64      `json:"version"`
	Result          *Result    `json:"result,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UploadExpiresAt *time.Time `json:"uploadExpiresAt,omitempty"`
}

type Update struct {
	Phase              Phase
	Result             *Result
	UploadExpiresAt    *time.Time
	MarkStagingCleaned bool
}

func (c Command) fingerprint() (string, error) {
	for _, field := range []struct{ name, value string }{
		{"operationID", c.OperationID}, {"backupUID", c.BackupUID}, {"allocationID", c.AllocationID},
		{"tenant", c.Tenant}, {"nodeName", c.NodeName},
	} {
		if err := corev1.ValidateIdentifier(field.name, field.value); err != nil {
			return "", err
		}
	}
	if err := c.Owner.Validate(); err != nil {
		return "", err
	}
	if err := c.Binding.Validate(); err != nil {
		return "", err
	}
	if c.Storage.Endpoint == "" || c.Storage.Bucket == "" || c.RequestedMaxBytes < 0 {
		return "", fmt.Errorf("%w: immutable storage target and valid request budget are required", ErrConflict)
	}
	if c.MaxBytes < 1 || c.RateLimit < 0 || c.ApprovedRevision < 1 {
		return "", fmt.Errorf("%w: positive budget/revision and non-negative rate limit required", ErrConflict)
	}
	if c.ObjectKey == "" || len(c.ObjectKey) > 1024 || strings.ContainsAny(c.ObjectKey, "\\\x00\r\n") {
		return "", fmt.Errorf("%w: invalid object key", ErrConflict)
	}
	for _, segment := range strings.Split(c.ObjectKey, "/") {
		if segment == ".." || segment == "." || segment == "" {
			return "", fmt.Errorf("%w: invalid object key", ErrConflict)
		}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func match(current Record, c Command) error {
	a, err := current.Command.fingerprint()
	if err != nil {
		return err
	}
	b, err := c.fingerprint()
	if err == nil && a != b {
		err = fmt.Errorf("%w: operation identity or immutable command differs", ErrConflict)
	}
	return err
}

func applyStagingCleanedMark(current Record, u Update) (Record, error) {
	if current.Phase != Completed || u.Phase != Completed || current.Result == nil || u.Result != nil || u.UploadExpiresAt != nil {
		return current, fmt.Errorf("%w: staging cleanup can only be marked on a completed backup", ErrConflict)
	}
	r := *current.Result
	r.StagingCleaned = true
	current.Result = &r
	current.Version++
	return current, nil
}

func apply(current Record, expected int64, u Update) (Record, error) {
	if current.Version != expected || current.Version == math.MaxInt64 {
		return current, ErrConflict
	}
	if u.MarkStagingCleaned {
		return applyStagingCleanedMark(current, u)
	}
	allowed := u.Phase == Deleting && current.Phase != Deleted
	switch current.Phase {
	case PendingQuota:
		allowed = allowed || u.Phase == Ready
	case Ready:
		allowed = allowed || u.Phase == Uploading
	case Uploading:
		allowed = allowed || u.Phase == Unknown || u.Phase == Completed
	case Unknown:
		allowed = allowed || u.Phase == Completed
	case Deleting:
		allowed = u.Phase == Deleted || u.Phase == Deleting && u.Result != nil && current.Result == nil
	}
	if !allowed {
		return current, fmt.Errorf("%w: cannot transition %s to %s", ErrConflict, current.Phase, u.Phase)
	}
	if u.Phase == Uploading {
		if current.UploadExpiresAt != nil || u.UploadExpiresAt == nil || u.UploadExpiresAt.IsZero() {
			return current, fmt.Errorf("%w: transfer deadline must be frozen once", ErrConflict)
		}
		t := u.UploadExpiresAt.UTC()
		current.UploadExpiresAt = &t
	} else if u.UploadExpiresAt != nil {
		return current, fmt.Errorf("%w: transfer deadline is immutable", ErrConflict)
	}
	if u.Phase == Completed || u.Phase == Deleting && u.Result != nil {
		if u.Result == nil || u.Result.SizeBytes < 0 || u.Result.SizeBytes > current.Command.MaxBytes || u.Result.CompletedAt.IsZero() || current.Result != nil {
			return current, fmt.Errorf("%w: invalid transfer result", ErrConflict)
		}
		r := *u.Result
		current.Result = &r
	} else if u.Result != nil {
		return current, fmt.Errorf("%w: result is immutable", ErrConflict)
	}
	current.Phase = u.Phase
	current.Version++
	return current, nil
}
