package instanceconfiguration

import (
	"context"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

var (
	ErrInvalidDraft = errors.New("invalid configuration draft")
	ErrStaleDraft   = errors.New("configuration draft is stale")
)

type Draft struct {
	ID                string                          `json:"id"`
	WorkspaceID       string                          `json:"-"`
	LogicalInstanceID string                          `json:"logicalInstanceId"`
	BaseRevisionID    string                          `json:"baseRevisionId"`
	SchemaVersion     int                             `json:"schemaVersion"`
	Values            map[string]any                  `json:"values"`
	ModSelections     []providercontract.ModSelection `json:"modSelections"`
	ValidationErrors  []string                        `json:"validationErrors"`
	UpdatedAt         time.Time                       `json:"updatedAt"`
}

type SaveCommand struct {
	WorkspaceID       string
	LogicalInstanceID string
	DraftID           string
	SchemaVersion     int
	Values            map[string]any
	ModSelections     []providercontract.ModSelection
}

type ApplyCommand struct {
	WorkspaceID       string
	LogicalInstanceID string
	DraftID           string
	IdempotencyKey    string
}

type Delivery interface {
	Instance(context.Context, string, string) (deliverycontrol.Instance, error)
	Revision(context.Context, string, string, string) (deliverycontrol.Revision, error)
	ApplyRevision(context.Context, deliverycontrol.ApplyRevisionCommand, time.Time) (deliverycontrol.Revision, deliverycontrol.Operation, error)
}
