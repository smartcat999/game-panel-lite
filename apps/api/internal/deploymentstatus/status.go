// Package deploymentstatus defines the bounded actual-state event a Region
// publishes for one globally owned logical server.
package deploymentstatus

import (
	"errors"
	"strings"
)

var ErrInvalidEvent = errors.New("invalid regional deployment status event")
var ErrEventConflict = errors.New("regional deployment status conflicts with global state")

type Event struct {
	SchemaVersion  int64  `json:"schemaVersion"`
	EventID        string `json:"eventId"`
	RegionID       string `json:"regionId"`
	OrganizationID string `json:"organizationId"`
	OperationID    string `json:"operationId"`
	ServerID       string `json:"serverId"`
	RevisionID     string `json:"revisionId"`
	TaskID         string `json:"taskId"`
	NodeID         string `json:"nodeId"`
	PlacementEpoch int64  `json:"placementEpoch"`
	SpecGeneration int64  `json:"specGeneration"`
	IntentVersion  int64  `json:"intentVersion"`
	Fence          int64  `json:"fence"`
	ActualState    string `json:"actualState"`
	Outcome        string `json:"outcome"`
	RuntimeID      string `json:"runtimeId,omitempty"`
	ObservedAtMS   int64  `json:"observedAtMs"`
}

func (e Event) Validate() error {
	if e.SchemaVersion != 1 || e.PlacementEpoch < 1 || e.SpecGeneration < 1 || e.IntentVersion < 1 || e.Fence < 1 || e.ObservedAtMS < 1 ||
		!statusText(e.EventID, 255) || !statusText(e.RegionID, 128) || !statusText(e.OrganizationID, 128) || !statusText(e.OperationID, 128) ||
		!statusText(e.ServerID, 128) || !statusText(e.RevisionID, 128) || !statusText(e.TaskID, 128) || !statusText(e.NodeID, 128) ||
		(e.RuntimeID != "" && !statusText(e.RuntimeID, 255)) || !actualState(e.ActualState) || (e.Outcome != "succeeded" && e.Outcome != "failed") ||
		(e.Outcome == "succeeded" && (e.ActualState != "running" || e.RuntimeID == "")) {
		return ErrInvalidEvent
	}
	return nil
}

func statusText(value string, limit int) bool {
	return value != "" && len(value) <= limit && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func actualState(value string) bool {
	return value == "running" || value == "stopped" || value == "missing" || value == "unknown"
}
