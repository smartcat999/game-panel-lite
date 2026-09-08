package regional

import (
	"errors"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

var ErrPortsUnavailable = errors.New("regional node ports unavailable")

var ErrAllocationConflict = errors.New("regional allocation conflicts with existing reservation")

// CapacityRequest comes from a trusted regional coordinator after selecting an
// authorized node. It is not a public user request or an execution grant.
type CapacityRequest struct {
	RegionID       string
	OrganizationID string
	DeploymentID   string
	ServerID       string
	PlacementEpoch int64
	RevisionID     string
	SpecGeneration int64
	IntentVersion  int64
	NodeID         string
	NodeVersion    int64
	SessionEpoch   int64
}

type Allocation struct {
	ID    string
	Ports []workload.Port
	CapacityRequest
	CPU      float64
	MemoryMB int64
	Status   string
}
