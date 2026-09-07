package workload

const ExecutionLeaseCapability = "execution-lease-v1"

// LeaseRequest is scoped to the assignment UID in the endpoint. HolderID is a
// fresh identity for one reconciliation attempt, never a shared node identity.
type LeaseRequest struct {
	Action     string `json:"action"`
	Generation int    `json:"generation"`
	HolderID   string `json:"holderId"`
	Fence      int64  `json:"fence"`
}

type LeaseGrant struct {
	AssignmentUID string `json:"assignmentUid"`
	ServerID      string `json:"serverId"`
	NodeID        string `json:"nodeId"`
	Generation    int    `json:"generation"`
	HolderID      string `json:"holderId"`
	Fence         int64  `json:"fence"`
	// ValidForMS is measured conservatively from the client's request start,
	// including transport and lock waits, rather than from response receipt.
	ValidForMS int64 `json:"validForMs"`
}
