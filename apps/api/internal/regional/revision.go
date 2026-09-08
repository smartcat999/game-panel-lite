package regional

import (
	"errors"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

var ErrRevisionUnavailable = errors.New("revision is not available to this region")

// RevisionSnapshot is data, not an execution grant. CurrentSpecGeneration can
// be newer than Revision.SpecGeneration after a delayed notification arrives.
// A materializer still needs entitlement, intent and bounded execution authority.
type RevisionSnapshot struct {
	Event                 instances.RevisionAvailable `json:"event"`
	Revision              instances.Revision          `json:"revision"`
	CurrentSpecGeneration int64                       `json:"currentSpecGeneration"`
	DesiredState          string                      `json:"desiredState"`
	IntentVersion         int64                       `json:"intentVersion"`
}
