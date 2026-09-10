package deliverycontrol

import (
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
)

var (
	ErrInvalidCommand = errors.New("invalid delivery command")
	ErrNotFound       = errors.New("delivery resource not found")
	ErrImmutable      = errors.New("idempotency key already used with different content")
)

type ListenerRequirement struct {
	Name               string   `json:"name"`
	Purpose            string   `json:"purpose"`
	Transports         []string `json:"transports"`
	InternalPort       int      `json:"internalPort"`
	ExternalPortPolicy string   `json:"externalPortPolicy"`
	AddressMode        string   `json:"addressMode"`
	Primary            bool     `json:"primary"`
}

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
	QuoteID              string                `json:"-"`
	InstanceRevisionID   string                `json:"configurationRevisionId"`
	PlacementVersion     int64                 `json:"placementVersion"`
	ResourceSpec         billing.ResourceSpec  `json:"resourceSpec"`
	Configuration        map[string]any        `json:"-"`
	ListenerRequirements []ListenerRequirement `json:"-"`
	DesiredState         string                `json:"desiredState"`
	ObservedState        string                `json:"observedState"`
	ObservationSequence  int64                 `json:"-"`
	EndpointBindings     []EndpointBinding     `json:"endpoints"`
	LatestOperationID    string                `json:"latestOperationId"`
	CreatedAt            time.Time             `json:"createdAt"`
	UpdatedAt            time.Time             `json:"updatedAt"`
}

type CreateCommand struct {
	WorkspaceID          string
	Name                 string
	ProviderReleaseID    string
	Configuration        map[string]any
	ListenerRequirements []ListenerRequirement
	QuoteID              string
	IdempotencyKey       string
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
	ResourceSpec         billing.ResourceSpec  `json:"resourceSpec"`
	Configuration        map[string]any        `json:"configuration"`
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
