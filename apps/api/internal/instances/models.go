// Package instances owns global logical server intent. Regional allocations,
// runtime paths, containers and observations do not belong to these records.
package instances

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

var (
	ErrInvalidIntent       = errors.New("invalid global server intent")
	ErrIdempotencyConflict = errors.New("idempotency key already used with different parameters")
	ErrVersionConflict     = errors.New("global server revision changed")
)

type Resources struct {
	CPU      float64 `json:"cpu"`
	MemoryMB int64   `json:"memoryMb"`
}

// ProtectedConfiguration is produced by a trusted configuration protector.
// The application boundary must validate provider configuration before sealing
// it. Plaintext configuration must never be placed in operations or the outbox.
type ProtectedConfiguration struct {
	KeyID      string `json:"keyId"`
	Ciphertext []byte `json:"ciphertext"`
}

type AssetVersion struct {
	AssetID string `json:"assetId"`
	Version string `json:"version"`
}

type Specification struct {
	ProviderKey         string                 `json:"providerKey"`
	GameVersion         string                 `json:"gameVersion"`
	ConfigSchemaVersion int                    `json:"configSchemaVersion"`
	Configuration       ProtectedConfiguration `json:"configuration"`
	Resources           Resources              `json:"resources"`
	Assets              []AssetVersion         `json:"assets,omitempty"`
}

func (s Specification) Validate() error {
	if !identifier(s.ProviderKey) || !identifier(s.GameVersion) || s.ConfigSchemaVersion < 1 ||
		!identifier(s.Configuration.KeyID) || len(s.Configuration.Ciphertext) == 0 ||
		s.Resources.CPU <= 0 || math.IsNaN(s.Resources.CPU) || math.IsInf(s.Resources.CPU, 0) || s.Resources.MemoryMB <= 0 {
		return ErrInvalidIntent
	}
	seen := map[string]bool{}
	for _, asset := range s.Assets {
		if !identifier(asset.AssetID) || !identifier(asset.Version) || seen[asset.AssetID] {
			return ErrInvalidIntent
		}
		seen[asset.AssetID] = true
	}
	return nil
}

type CreateRequest struct {
	OrganizationID string        `json:"organizationId"`
	Name           string        `json:"name"`
	RegionID       string        `json:"regionId"`
	IdempotencyKey string        `json:"-"`
	Specification  Specification `json:"specification"`
}

func (r CreateRequest) Validate() error {
	if !identifier(r.OrganizationID) || !identifier(r.Name) || !identifier(r.RegionID) || !identifier(r.IdempotencyKey) {
		return ErrInvalidIntent
	}
	return r.Specification.Validate()
}

type ReviseRequest struct {
	OrganizationID     string        `json:"organizationId"`
	ServerID           string        `json:"serverId"`
	ExpectedGeneration int64         `json:"expectedGeneration"`
	IdempotencyKey     string        `json:"-"`
	Specification      Specification `json:"specification"`
}

func (r ReviseRequest) Validate() error {
	if !identifier(r.OrganizationID) || !identifier(r.ServerID) || !identifier(r.IdempotencyKey) || r.ExpectedGeneration < 1 || r.ExpectedGeneration == math.MaxInt64 {
		return ErrInvalidIntent
	}
	return r.Specification.Validate()
}

func identifier(value string) bool { return value != "" && value == strings.TrimSpace(value) }

type Server struct {
	ID                string    `json:"id"`
	OrganizationID    string    `json:"organizationId"`
	Name              string    `json:"name"`
	CurrentRevisionID string    `json:"currentRevisionId"`
	SpecGeneration    int64     `json:"specGeneration"`
	DesiredState      string    `json:"desiredState"`
	IntentVersion     int64     `json:"intentVersion"`
	CreatedAt         time.Time `json:"createdAt"`
}

type Revision struct {
	ID             string        `json:"id"`
	ServerID       string        `json:"serverId"`
	SpecGeneration int64         `json:"specGeneration"`
	Specification  Specification `json:"specification"`
	CreatedAt      time.Time     `json:"createdAt"`
}

type Placement struct {
	ServerID       string `json:"serverId"`
	RegionID       string `json:"regionId"`
	PlacementEpoch int64  `json:"placementEpoch"`
}

type Operation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	ServerID       string    `json:"serverId"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
}

// IntentResult returns the requested operation's immutable revision together
// with current logical identity and placement. Retrying an old operation does
// not roll the server pointer back to that operation's revision.
type IntentResult struct {
	Server    Server    `json:"server"`
	Revision  Revision  `json:"revision"`
	Placement Placement `json:"placement"`
	Operation Operation `json:"operation"`
}

// RevisionAvailable carries identities only. An authorized receiver must fetch
// the protected immutable revision; it must not trust this event as authorization.
type RevisionAvailable struct {
	SchemaVersion  int    `json:"schemaVersion"`
	EventID        string `json:"eventId"`
	OperationID    string `json:"operationId"`
	OrganizationID string `json:"organizationId"`
	ServerID       string `json:"serverId"`
	RevisionID     string `json:"revisionId"`
	RegionID       string `json:"regionId"`
	PlacementEpoch int64  `json:"placementEpoch"`
	SpecGeneration int64  `json:"specGeneration"`
}

// EncodeSpecification takes a value snapshot, including slices held by
// callers, before persistence starts. The stored revision is never updated.
func EncodeSpecification(spec Specification) ([]byte, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(spec)
}
