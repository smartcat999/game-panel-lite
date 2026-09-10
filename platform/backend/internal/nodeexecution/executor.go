package nodeexecution

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
)

type BackupResultStore interface {
	RecordBackupResult(context.Context, regionexecution.WorkAssignment, regionexecution.BackupResult) (bool, error)
}

type WorkloadResultStore interface {
	RecordWorkloadResult(context.Context, regionexecution.WorkAssignment, string, string, time.Time) (bool, error)
}

type Executor struct {
	Root            ScopedRoot
	Transfer        ObjectTransfer
	Results         BackupResultStore
	WorkloadResults WorkloadResultStore
	Games           map[string]GameProvider
	Runtime         RuntimeProvider
	Clock           func() time.Time
}

type WorkloadIntent struct {
	LogicalInstanceID string
	GameKey           string
	GameVersion       string
	DesiredState      string
	DataDir           string
	CPUUnits          int
	MemoryMegabytes   int
	Configuration     map[string]any
	FencingToken      int64
}

type WorkloadSpec struct {
	LogicalInstanceID string
	DesiredState      string
	Image             string
	DataDir           string
	Port              int
	HostPort          int
	Protocol          string
	CPUUnits          int
	MemoryMegabytes   int
	Environment       []string
	Command           []string
	Files             map[string]string
	DataMounts        map[string]string
	FencingToken      int64
}

type WorkloadObservation struct {
	RuntimeID string
	State     string
}

// GameProvider translates logical game intent into a provider-neutral workload.
type GameProvider interface {
	Key() string
	Build(WorkloadIntent) (WorkloadSpec, error)
}

// RuntimeProvider is the only Node call site allowed to mutate workload runtime state.
type RuntimeProvider interface {
	Reconcile(context.Context, WorkloadSpec) (WorkloadObservation, error)
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
		return e.reconcileWorkload(ctx, assignment)
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

func (e Executor) reconcileWorkload(ctx context.Context, assignment regionexecution.WorkAssignment) error {
	if e.Runtime == nil || len(e.Games) == 0 {
		return nil
	}
	gameKey := assignment.Payload["gameKey"]
	provider, ok := e.Games[gameKey]
	if !ok || provider.Key() != gameKey {
		return errors.New("unsupported game provider")
	}
	dataDir, err := e.Root.Resolve(assignment.Payload["dataRelative"])
	if err != nil {
		return err
	}
	cpuUnits, err := strconv.Atoi(assignment.Payload["cpuUnits"])
	if err != nil {
		return errors.New("invalid workload CPU units")
	}
	memoryMegabytes, err := strconv.Atoi(assignment.Payload["memoryMegabytes"])
	if err != nil {
		return errors.New("invalid workload memory")
	}
	configuration := make(map[string]any)
	rawConfiguration := assignment.Payload["configuration"]
	if rawConfiguration == "" {
		rawConfiguration = "{}"
	}
	if err := json.Unmarshal([]byte(rawConfiguration), &configuration); err != nil {
		return errors.New("invalid workload configuration")
	}
	spec, err := provider.Build(WorkloadIntent{LogicalInstanceID: assignment.Payload["logicalInstanceId"], GameKey: gameKey, GameVersion: assignment.Payload["gameVersion"], DesiredState: assignment.Payload["desiredState"], DataDir: dataDir, CPUUnits: cpuUnits, MemoryMegabytes: memoryMegabytes, Configuration: configuration, FencingToken: assignment.FencingToken})
	if err != nil {
		return err
	}
	observation, err := e.Runtime.Reconcile(ctx, spec)
	if err != nil {
		return err
	}
	if e.WorkloadResults != nil {
		now := time.Now().UTC()
		if e.Clock != nil {
			now = e.Clock()
		}
		_, err = e.WorkloadResults.RecordWorkloadResult(ctx, assignment, observation.State, observation.RuntimeID, now)
	}
	return err
}
