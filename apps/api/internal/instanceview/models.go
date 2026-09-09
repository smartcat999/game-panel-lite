// Package instanceview defines safe global instance read models. It contains
// no persistence, HTTP, Provider, runtime or regional infrastructure code.
package instanceview

import (
	"errors"
	"time"
)

var (
	ErrInvalidQuery     = errors.New("invalid instance query")
	ErrOperatorRequired = errors.New("platform operator required")
	ErrCorruptRecord    = errors.New("incomplete global instance record")
)

type Resources struct {
	CPU      float64
	MemoryMB int64
}

type Operation struct {
	ID        string
	Kind      string
	Status    string
	CreatedAt time.Time
}

type Deployment struct {
	OperationID string
	ActualState string
	Outcome     string
	ObservedAt  time.Time
}

type Record struct {
	ID                  string
	OrganizationID      string
	Name                string
	ProviderKey         string
	GameVersion         string
	ConfigSchemaVersion int
	Resources           Resources
	DesiredState        string
	RegionID            string
	RevisionID          string
	SpecGeneration      int64
	IntentVersion       int64
	PlacementEpoch      int64
	CreatedAt           time.Time
	LatestOperation     *Operation
	Deployment          *Deployment
}

type PlatformRecord struct {
	Record
	NodeID string
	TaskID string
}

type Page[T any] struct {
	Items      []T
	NextCursor string
}
