package regional

import (
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

// CandidateQuery accepts one bounded page of already authorized Node IDs from
// the regional coordinator. It does not derive tenant permissions from IDs.
type CandidateQuery struct {
	OrganizationID string
	Networks       []workload.Network
	RegionID       string
	AllowedNodeIDs []string
	RequiredNodeID string
	Architecture   string
	Resources      instances.Resources
}

// Candidate is an observation, not a reservation. Admission must recheck its
// node version and session after acquiring the appropriate transaction locks.
type Candidate struct {
	NetworkIndex      int
	NodeID            string
	NodeVersion       int64
	SessionEpoch      int64
	RemainingCPU      float64
	RemainingMemoryMB int64
}
