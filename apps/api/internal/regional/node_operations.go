package regional

import (
	"errors"
	"math"
	"strings"
)

var ErrInvalidNodeOperations = errors.New("invalid regional node operations view")

// NodeOperations is a point-in-time Region-owned view for platform operators.
// It contains no tenant configuration, credentials, host paths, or runtime logs.
type NodeOperations struct {
	Node
	Online           bool    `json:"online"`
	RuntimeReady     bool    `json:"runtimeReady"`
	LastSeenMS       int64   `json:"lastSeenMs"`
	SessionEpoch     int64   `json:"sessionEpoch"`
	ReservedCPU      float64 `json:"reservedCpu"`
	ReservedMemoryMB int64   `json:"reservedMemoryMb"`
	Allocations      int64   `json:"allocations"`
	PendingTasks     int64   `json:"pendingTasks"`
}

type NodeOperationsPage struct {
	RegionID     string           `json:"regionId"`
	ObservedAtMS int64            `json:"observedAtMs"`
	Nodes        []NodeOperations `json:"nodes"`
	NextCursor   string           `json:"nextCursor,omitempty"`
}

func (p NodeOperationsPage) Validate() error {
	if p.RegionID == "" || len(p.RegionID) > 128 || p.RegionID != strings.TrimSpace(p.RegionID) || p.ObservedAtMS < 1 || len(p.Nodes) > 200 {
		return ErrInvalidNodeOperations
	}
	previous := ""
	for _, node := range p.Nodes {
		if node.NodeConfiguration.Validate() != nil || node.Version < 1 || node.ID <= previous || node.LastSeenMS < 0 || node.SessionEpoch < 0 || node.ReservedCPU < 0 || math.IsNaN(node.ReservedCPU) || math.IsInf(node.ReservedCPU, 0) || node.ReservedMemoryMB < 0 || node.Allocations < 0 || node.PendingTasks < 0 || (node.Online && (!node.RuntimeReady || node.LastSeenMS < 1)) {
			return ErrInvalidNodeOperations
		}
		previous = node.ID
	}
	if p.NextCursor != "" && (len(p.NextCursor) > 128 || len(p.Nodes) == 0 || p.NextCursor != p.Nodes[len(p.Nodes)-1].ID) {
		return ErrInvalidNodeOperations
	}
	return nil
}
