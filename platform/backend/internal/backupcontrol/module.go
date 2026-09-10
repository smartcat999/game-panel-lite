package backupcontrol

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
)

var (
	ErrInvalidRequest     = errors.New("invalid backup request")
	ErrBackupMissing      = errors.New("backup request not found")
	ErrCrossRegionRestore = errors.New("restore source must belong to the same instance and region")
)

type Kind string

const (
	KindBackup  Kind = "backup"
	KindRestore Kind = "restore"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Request struct {
	ID                contract.BackupRequestID   `json:"id"`
	WorkspaceID       contract.WorkspaceID       `json:"workspaceId"`
	LogicalInstanceID contract.LogicalInstanceID `json:"logicalInstanceId"`
	RegionID          contract.RegionID          `json:"regionId"`
	Kind              Kind                       `json:"kind"`
	Status            Status                     `json:"status"`
	Sequence          int64                      `json:"sequence"`
	ObjectKey         string                     `json:"objectKey,omitempty"`
	SizeBytes         int64                      `json:"sizeBytes,omitempty"`
	Checksum          string                     `json:"checksum,omitempty"`
	CreatedAt         time.Time                  `json:"createdAt"`
	UpdatedAt         time.Time                  `json:"updatedAt"`
}

type CreateCommand struct {
	Identity              contract.CommandIdentity
	WorkspaceID           contract.WorkspaceID
	LogicalInstanceID     contract.LogicalInstanceID
	RegionID              contract.RegionID
	Kind                  Kind
	SourceBackupRequestID contract.BackupRequestID
}

type Observation struct {
	MessageID       contract.EventID
	BackupRequestID contract.BackupRequestID
	Sequence        int64
	Status          Status
	ObjectKey       string
	SizeBytes       int64
	Checksum        string
	ObservedAt      time.Time
}

type Module struct {
	mu       sync.RWMutex
	messages *messaging.Module
	requests map[contract.BackupRequestID]Request
	results  map[contract.IdempotencyKey]contract.BackupRequestID
	inbox    map[contract.EventID]bool
	nextID   int
}

func New(messages *messaging.Module) *Module {
	return &Module{messages: messages, requests: make(map[contract.BackupRequestID]Request), results: make(map[contract.IdempotencyKey]contract.BackupRequestID), inbox: make(map[contract.EventID]bool)}
}

func (m *Module) Create(_ context.Context, command CreateCommand, now time.Time) (Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if previousID, ok := m.results[command.Identity.IdempotencyKey]; ok {
		return m.requests[previousID], nil
	}
	if command.Identity.IdempotencyKey == "" || command.WorkspaceID == "" || command.LogicalInstanceID == "" || command.RegionID == "" || (command.Kind != KindBackup && command.Kind != KindRestore) {
		return Request{}, ErrInvalidRequest
	}
	objectKey := fmt.Sprintf("regions/%s/instances/%s/backups/%s.tar.gz", command.RegionID, command.LogicalInstanceID, command.Identity.IdempotencyKey)
	if command.Kind == KindRestore {
		source, ok := m.requests[command.SourceBackupRequestID]
		if !ok || source.Kind != KindBackup || source.Status != StatusCompleted || source.WorkspaceID != command.WorkspaceID || source.LogicalInstanceID != command.LogicalInstanceID || source.RegionID != command.RegionID {
			return Request{}, ErrCrossRegionRestore
		}
		objectKey = source.ObjectKey
	}
	m.nextID++
	request := Request{ID: contract.BackupRequestID(fmt.Sprintf("bkr_%06d", m.nextID)), WorkspaceID: command.WorkspaceID, LogicalInstanceID: command.LogicalInstanceID, RegionID: command.RegionID, Kind: command.Kind, Status: StatusQueued, ObjectKey: objectKey, CreatedAt: now, UpdatedAt: now}
	m.requests[request.ID] = request
	m.results[command.Identity.IdempotencyKey] = request.ID
	m.messages.Publish("backup.requested.v1", command.Identity.IdempotencyKey, requestedPayload(request), now)
	return request, nil
}

func (m *Module) ApplyObservation(_ context.Context, observation Observation) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inbox[observation.MessageID] {
		return false, nil
	}
	request, ok := m.requests[observation.BackupRequestID]
	if !ok {
		return false, ErrBackupMissing
	}
	if observation.Sequence <= request.Sequence {
		m.inbox[observation.MessageID] = true
		return false, nil
	}
	request.Sequence, request.Status, request.UpdatedAt = observation.Sequence, observation.Status, observation.ObservedAt
	if observation.ObjectKey != "" {
		request.ObjectKey = observation.ObjectKey
	}
	request.SizeBytes, request.Checksum = observation.SizeBytes, observation.Checksum
	m.requests[request.ID] = request
	m.inbox[observation.MessageID] = true
	return true, nil
}

func (m *Module) List(_ context.Context, workspaceID contract.WorkspaceID) []Request {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Request, 0)
	for _, request := range m.requests {
		if request.WorkspaceID == workspaceID {
			result = append(result, request)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

func requestedPayload(request Request) map[string]any {
	return map[string]any{"backupRequestId": request.ID, "logicalInstanceId": request.LogicalInstanceID, "regionId": request.RegionID, "kind": request.Kind, "objectKey": request.ObjectKey, "transferUrl": "object://" + request.ObjectKey, "relativePath": "instances/" + string(request.LogicalInstanceID) + "/world"}
}
