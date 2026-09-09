// Package regionstatus defines the bounded operational summary a Region
// publishes to the global control plane. It contains no tenant configuration,
// node credentials, logs, or execution authority.
package regionstatus

import (
	"errors"
	"math"
	"strings"
)

var ErrInvalidSnapshot = errors.New("invalid Region status snapshot")
var ErrSnapshotConflict = errors.New("Region status snapshot conflicts with persisted state")

type NodeSummary struct {
	Total       int64 `json:"total"`
	Online      int64 `json:"online"`
	Schedulable int64 `json:"schedulable"`
}

type CapacitySummary struct {
	CPUTotal         float64 `json:"cpuTotal"`
	CPUReserved      float64 `json:"cpuReserved"`
	MemoryTotalMB    int64   `json:"memoryTotalMb"`
	MemoryReservedMB int64   `json:"memoryReservedMb"`
}

type DeploymentSummary struct {
	Total    int64 `json:"total"`
	Pending  int64 `json:"pending"`
	Reserved int64 `json:"reserved"`
	Rejected int64 `json:"rejected"`
}

type TaskSummary struct {
	AwaitingAuthority int64 `json:"awaitingAuthority"`
}

type Snapshot struct {
	SchemaVersion int64             `json:"schemaVersion"`
	EventID       string            `json:"eventId"`
	RegionID      string            `json:"regionId"`
	Sequence      int64             `json:"sequence"`
	ObservedAtMS  int64             `json:"observedAtMs"`
	Nodes         NodeSummary       `json:"nodes"`
	Capacity      CapacitySummary   `json:"capacity"`
	Deployments   DeploymentSummary `json:"deployments"`
	Tasks         TaskSummary       `json:"tasks"`
}

func (s Snapshot) Validate() error {
	if s.SchemaVersion != 1 || s.Sequence < 1 || s.ObservedAtMS < 1 || !validText(s.EventID, 255) || !validText(s.RegionID, 128) ||
		s.Nodes.Total < 0 || s.Nodes.Online < 0 || s.Nodes.Online > s.Nodes.Total || s.Nodes.Schedulable < 0 || s.Nodes.Schedulable > s.Nodes.Total ||
		invalidFloat(s.Capacity.CPUTotal) || invalidFloat(s.Capacity.CPUReserved) ||
		s.Capacity.MemoryTotalMB < 0 || s.Capacity.MemoryReservedMB < 0 ||
		s.Deployments.Total < 0 || s.Deployments.Pending < 0 || s.Deployments.Reserved < 0 || s.Deployments.Rejected < 0 ||
		s.Deployments.Pending+s.Deployments.Reserved+s.Deployments.Rejected != s.Deployments.Total || s.Tasks.AwaitingAuthority < 0 {
		return ErrInvalidSnapshot
	}
	return nil
}

func validText(value string, limit int) bool {
	return value != "" && len(value) <= limit && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func invalidFloat(value float64) bool {
	return value < 0 || math.IsNaN(value) || math.IsInf(value, 0)
}
