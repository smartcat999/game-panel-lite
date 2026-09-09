package regional

import (
	"errors"
	"strings"
)

var ErrInvalidDeploymentOperations = errors.New("invalid regional deployment operations view")

// DeploymentOperations relates a global logical server identity to its
// Region-owned scheduling record and current allocation, if one exists.
type DeploymentOperations struct {
	ID               string `json:"id"`
	OrganizationID   string `json:"organizationId"`
	ServerID         string `json:"serverId"`
	PlacementEpoch   int64  `json:"placementEpoch"`
	RevisionID       string `json:"revisionId"`
	SpecGeneration   int64  `json:"specGeneration"`
	IntentVersion    int64  `json:"intentVersion"`
	DesiredState     string `json:"desiredState"`
	SchedulingStatus string `json:"schedulingStatus"`
	NodeID           string `json:"nodeId,omitempty"`
}

type DeploymentOperationsPage struct {
	RegionID     string                 `json:"regionId"`
	ObservedAtMS int64                  `json:"observedAtMs"`
	Deployments  []DeploymentOperations `json:"deployments"`
	NextCursor   string                 `json:"nextCursor,omitempty"`
}

func (p DeploymentOperationsPage) Validate() error {
	if !validOperationsID(p.RegionID, false) || p.ObservedAtMS < 1 || len(p.Deployments) > 200 {
		return ErrInvalidDeploymentOperations
	}
	previous := ""
	for _, deployment := range p.Deployments {
		if !validOperationsID(deployment.ID, false) || deployment.ID <= previous || !validOperationsID(deployment.OrganizationID, false) || !validOperationsID(deployment.ServerID, false) || deployment.PlacementEpoch < 1 || !validOperationsID(deployment.RevisionID, false) || deployment.SpecGeneration < 1 || deployment.IntentVersion < 1 || (deployment.DesiredState != "running" && deployment.DesiredState != "stopped") || (deployment.SchedulingStatus != "pending" && deployment.SchedulingStatus != "reserved" && deployment.SchedulingStatus != "rejected") || !validOperationsID(deployment.NodeID, true) || (deployment.SchedulingStatus == "reserved" && deployment.NodeID == "") {
			return ErrInvalidDeploymentOperations
		}
		previous = deployment.ID
	}
	if p.NextCursor != "" && (!validOperationsID(p.NextCursor, false) || len(p.Deployments) == 0 || p.NextCursor != p.Deployments[len(p.Deployments)-1].ID) {
		return ErrInvalidDeploymentOperations
	}
	return nil
}

func validOperationsID(value string, optional bool) bool {
	if value == "" {
		return optional
	}
	return len(value) <= 128 && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\x00\r\n")
}
