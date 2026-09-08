package regional

import "errors"

var ErrNodeTaskConflict = errors.New("regional node task conflicts with its reservation")

// NodeTask is a durable, metadata-only intent. Awaiting_authority tasks must
// never be delivered as executable assignments. IDs and versions are bindings,
// not proof of entitlement, runtime fencing or a valid execution lease.
type NodeTask struct {
	ID           string
	AllocationID string
	CapacityRequest
	Kind   string
	Status string
}
