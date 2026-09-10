package deliverycontrol

import (
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

var (
	ErrInvalidCommand = errors.New("invalid delivery command")
	ErrNotFound       = errors.New("delivery resource not found")
	ErrImmutable      = errors.New("idempotency key already used with different content")
)

type ListenerRequirement = contract.ListenerRequirement
type ModLockEntry = contract.ModLockEntry

type EndpointBinding struct {
	Name           string   `json:"name"`
	Purpose        string   `json:"purpose"`
	Address        string   `json:"address"`
	Port           *int     `json:"port,omitempty"`
	Transports     []string `json:"transports"`
	Stability      string   `json:"stability"`
	DisplayAddress string   `json:"displayAddress"`
	Primary        bool     `json:"primary"`
}

type Step struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type Operation struct {
	ID             string    `json:"id"`
	WorkspaceID    string    `json:"-"`
	Kind           string    `json:"kind"`
	ResourceType   string    `json:"resourceType"`
	ResourceID     string    `json:"resourceId"`
	IdempotencyKey string    `json:"-"`
	Status         string    `json:"status"`
	Steps          []Step    `json:"steps"`
	FailureCode    string    `json:"failureCode,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Instance struct {
	ID                   string                `json:"id"`
	WorkspaceID          string                `json:"workspaceId"`
	RegionID             string                `json:"regionId"`
	Name                 string                `json:"name"`
	ProviderReleaseID    string                `json:"providerReleaseId"`
	GameVersion          string                `json:"gameVersion"`
	QuoteID              string                `json:"-"`
	InstanceRevisionID   string                `json:"configurationRevisionId"`
	PlacementVersion     int64                 `json:"placementVersion"`
	ResourceSpec         billing.ResourceSpec  `json:"resourceSpec"`
	Configuration        map[string]any        `json:"-"`
	ModLock              []ModLockEntry        `json:"-"`
	ListenerRequirements []ListenerRequirement `json:"-"`
	DesiredState         string                `json:"desiredState"`
	ObservedState        string                `json:"observedState"`
	ObservationSequence  int64                 `json:"-"`
	EndpointBindings     []EndpointBinding     `json:"endpoints"`
	LatestOperationID    string                `json:"latestOperationId"`
	CreatedAt            time.Time             `json:"createdAt"`
	UpdatedAt            time.Time             `json:"updatedAt"`
}

type Revision struct {
	ID                string         `json:"id"`
	OperationID       string         `json:"operationId"`
	WorkspaceID       string         `json:"-"`
	LogicalInstanceID string         `json:"logicalInstanceId"`
	ProviderReleaseID string         `json:"providerReleaseId"`
	GameVersion       string         `json:"gameVersion"`
	SchemaVersion     int            `json:"schemaVersion"`
	Configuration     map[string]any `json:"configuration"`
	ModLock           []ModLockEntry `json:"modLock"`
	ApplyBehavior     string         `json:"applyBehavior"`
	CreatedAt         time.Time      `json:"createdAt"`
}

type CreateCommand struct {
	WorkspaceID          string
	Name                 string
	ProviderReleaseID    string
	GameVersion          string
	SchemaVersion        int
	Configuration        map[string]any
	ModLock              []ModLockEntry
	ListenerRequirements []ListenerRequirement
	QuoteID              string
	IdempotencyKey       string
}

type ApplyRevisionCommand struct {
	WorkspaceID       string
	LogicalInstanceID string
	BaseRevisionID    string
	ProviderReleaseID string
	GameVersion       string
	SchemaVersion     int
	Configuration     map[string]any
	ModLock           []ModLockEntry
	ApplyBehavior     string
	IdempotencyKey    string
}

type ChangeStateCommand struct {
	WorkspaceID       string
	LogicalInstanceID string
	Action            string
	IdempotencyKey    string
}

type DesiredPayload struct {
	WorkspaceID          string                `json:"workspaceId"`
	LogicalInstanceID    string                `json:"logicalInstanceId"`
	RegionID             string                `json:"regionId"`
	PlacementVersion     int64                 `json:"placementVersion"`
	InstanceRevisionID   string                `json:"instanceRevisionId"`
	OperationID          string                `json:"operationId"`
	DesiredState         string                `json:"desiredState"`
	ProviderReleaseID    string                `json:"providerReleaseId"`
	GameVersion          string                `json:"gameVersion"`
	ApplyBehavior        string                `json:"applyBehavior"`
	ResourceSpec         billing.ResourceSpec  `json:"resourceSpec"`
	Configuration        map[string]any        `json:"configuration"`
	ModLock              []ModLockEntry        `json:"modLock"`
	ListenerRequirements []ListenerRequirement `json:"listenerRequirements"`
	AuthorityGrant       AuthorityGrant        `json:"authorityGrant"`
}

type AuthorityGrant struct {
	Issuer           string    `json:"issuer"`
	Action           string    `json:"action"`
	RegionID         string    `json:"regionId"`
	PlacementVersion int64     `json:"placementVersion"`
	ExpiresAt        time.Time `json:"expiresAt"`
	Signature        string    `json:"signature"`
}

type Observation struct {
	MessageID            string            `json:"-"`
	WorkspaceID          string            `json:"workspaceId"`
	LogicalInstanceID    string            `json:"logicalInstanceId"`
	RegionalDeploymentID string            `json:"regionalDeploymentId"`
	RuntimeAttemptID     string            `json:"runtimeAttemptId"`
	RegionID             string            `json:"regionId"`
	PlacementVersion     int64             `json:"placementVersion"`
	Sequence             int64             `json:"sequence"`
	ObservedState        string            `json:"observedState"`
	EndpointBindings     []EndpointBinding `json:"endpointBindings"`
	ReasonCode           string            `json:"reasonCode,omitempty"`
	ObservedAt           time.Time         `json:"observedAt"`
}
