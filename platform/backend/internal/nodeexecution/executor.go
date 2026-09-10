package nodeexecution

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
)

type BackupResultStore interface {
	RecordBackupResult(context.Context, regionexecution.WorkAssignment, regionexecution.BackupResult) (bool, error)
}

type Executor struct {
	Root     ScopedRoot
	Transfer ObjectTransfer
	Results  BackupResultStore
	Clock    func() time.Time
}

type HTTPObjectTransfer struct {
	Client *http.Client
}

func (t HTTPObjectTransfer) Upload(ctx context.Context, signedURL string, body io.Reader) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, signedURL, body)
	if err != nil {
		return err
	}
	response, err := t.client().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("object upload rejected")
	}
	return nil
}

func (t HTTPObjectTransfer) Download(ctx context.Context, signedURL string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := t.client().Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, errors.New("object download rejected")
	}
	return response.Body, nil
}

func (t HTTPObjectTransfer) client() *http.Client {
	if t.Client != nil {
		return t.Client
	}
	return &http.Client{Timeout: 30 * time.Minute}
}

func (e Executor) Reconcile(ctx context.Context, assignment regionexecution.WorkAssignment) error {
	if assignment.Action == "reconcile_workload" {
		return nil
	}
	if assignment.Action != "backup" && assignment.Action != "restore" {
		return errors.New("unsupported work assignment action")
	}
	now := time.Now().UTC()
	if e.Clock != nil {
		now = e.Clock()
	}
	result := regionexecution.BackupResult{MessageID: contract.EventID("evt_" + string(assignment.ID)), BackupRequestID: contract.BackupRequestID(assignment.Payload["backupRequestId"]), Sequence: 1, Status: "completed", ObjectKey: assignment.Payload["objectKey"], ObservedAt: now}
	var executionErr error
	if assignment.Action == "backup" {
		var transferResult TransferResult
		transferResult, executionErr = e.Root.Backup(ctx, e.Transfer, BackupJob{SourceRelative: assignment.Payload["relativePath"], SignedUploadURL: assignment.Payload["transferUrl"]})
		result.SizeBytes, result.Checksum = transferResult.SizeBytes, transferResult.Checksum
	} else {
		executionErr = e.Root.Restore(ctx, e.Transfer, RestoreJob{TargetRelative: assignment.Payload["relativePath"], SignedDownloadURL: assignment.Payload["transferUrl"]})
	}
	if executionErr != nil {
		result.Status = "failed"
	}
	_, recordErr := e.Results.RecordBackupResult(ctx, assignment, result)
	return errors.Join(executionErr, recordErr)
}
