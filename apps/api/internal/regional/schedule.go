package regional

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

// SchedulingScope is supplied by trusted regional authorization, not copied
// from a user's node IDs. One invocation handles at most 200 authorized nodes.
type SchedulingScope struct {
	AllowedNodeIDs []string
	RequiredNodeID string
	Architecture   string
	HostPort       int
	MaxHostPort    int
}

// Scheduler composes rendering, candidate observation and atomic admission.
// Its result is a reservation receipt, never a Node execution grant.
type Scheduler struct {
	Resources interface {
		RegionalAllocation(context.Context, string) (*Allocation, error)
		RegionalCapacityCandidates(context.Context, CandidateQuery, time.Duration) ([]Candidate, error)
		ReserveRegionalResources(context.Context, CapacityRequest, workload.Network, time.Duration) (Allocation, error)
	}
	Networks interface {
		Render(context.Context, RevisionSnapshot, int) (workload.Network, error)
	}
	MaxHeartbeatAge time.Duration
}

func (s Scheduler) Schedule(ctx context.Context, deployment Deployment, snapshot RevisionSnapshot, scope SchedulingScope) (Allocation, error) {
	if err := ctx.Err(); err != nil {
		return Allocation{}, err
	}
	if s.Resources == nil || s.Networks == nil || s.MaxHeartbeatAge < time.Millisecond || s.MaxHeartbeatAge > time.Hour ||
		snapshot.ValidateFor(snapshot.Event) != nil || snapshot.CurrentSpecGeneration != snapshot.Revision.SpecGeneration {
		return Allocation{}, ErrDeploymentConflict
	}
	event := snapshot.Event
	if deployment.ID == "" || deployment.OrganizationID != event.OrganizationID || deployment.ServerID != event.ServerID ||
		deployment.RevisionOperationID != event.OperationID || deployment.RevisionID != event.RevisionID || deployment.SpecGeneration != event.SpecGeneration ||
		deployment.PlacementEpoch != event.PlacementEpoch || deployment.IntentVersion != snapshot.IntentVersion ||
		deployment.DesiredState != "running" || snapshot.DesiredState != "running" || deployment.Status != "awaiting_authority" {
		return Allocation{}, ErrDeploymentConflict
	}
	if len(scope.AllowedNodeIDs) < 1 || len(scope.AllowedNodeIDs) > 200 || scope.Architecture == "" || len(scope.Architecture) > 128 {
		return Allocation{}, ErrInvalidNode
	}
	allowed := make(map[string]bool, len(scope.AllowedNodeIDs))
	for _, id := range scope.AllowedNodeIDs {
		if id == "" || len(id) > 128 || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\x00\r\n") || allowed[id] {
			return Allocation{}, ErrInvalidNode
		}
		allowed[id] = true
	}
	if scope.RequiredNodeID != "" && !allowed[scope.RequiredNodeID] {
		return Allocation{}, ErrNodeUnavailable
	}
	network, err := s.Networks.Render(ctx, snapshot, scope.HostPort)
	if err != nil {
		return Allocation{}, err
	}
	end := scope.MaxHostPort
	if end == 0 {
		end = scope.HostPort
	}
	if scope.HostPort < 0 || end > 65535 || end < scope.HostPort || end-scope.HostPort >= 64 || (scope.HostPort == 0 && end != 0) {
		return Allocation{}, workload.ErrInvalidNetwork
	}
	networks := []workload.Network{network}
	for port := scope.HostPort + 1; port <= end; port++ {
		shifted, err := workload.OffsetHostPorts(network, port-scope.HostPort)
		if err != nil {
			return Allocation{}, err
		}
		networks = append(networks, shifted)
	}
	request := CapacityRequest{RegionID: event.RegionID, OrganizationID: event.OrganizationID, DeploymentID: deployment.ID,
		ServerID: event.ServerID, PlacementEpoch: event.PlacementEpoch, RevisionID: event.RevisionID, SpecGeneration: event.SpecGeneration, IntentVersion: deployment.IntentVersion}
	// Recover a prior receipt before capacity filtering: a full node may contain
	// this deployment's own reservation. Never allocate elsewhere on a lost reply.
	existing, err := s.Resources.RegionalAllocation(ctx, deployment.ID)
	if err != nil {
		return Allocation{}, err
	}
	if existing != nil {
		request.NodeID, request.NodeVersion, request.SessionEpoch = existing.NodeID, existing.NodeVersion, existing.SessionEpoch
		if !allowed[request.NodeID] || (scope.RequiredNodeID != "" && scope.RequiredNodeID != request.NodeID) || existing.CapacityRequest != request {
			return Allocation{}, ErrAllocationConflict
		}
		for _, plan := range networks {
			ports, err := workload.ResolvePortBindings(plan)
			if err != nil {
				return Allocation{}, err
			}
			if slices.Equal(ports, existing.Ports) {
				return s.Resources.ReserveRegionalResources(ctx, request, plan, s.MaxHeartbeatAge)
			}
		}
		return Allocation{}, ErrAllocationConflict
	}
	candidates, err := s.Resources.RegionalCapacityCandidates(ctx, CandidateQuery{OrganizationID: event.OrganizationID, RegionID: event.RegionID, AllowedNodeIDs: scope.AllowedNodeIDs,
		RequiredNodeID: scope.RequiredNodeID, Architecture: scope.Architecture, Resources: snapshot.Revision.Specification.Resources, Networks: networks}, s.MaxHeartbeatAge)
	if err != nil {
		return Allocation{}, err
	}
	if len(candidates) == 0 {
		return Allocation{}, ErrNodeUnavailable
	}
	// Store returns stable node order. A stale candidate fails admission; the
	// durable caller retries later, without silently overriding a pinned node.
	candidate := candidates[0]
	if candidate.NetworkIndex < 0 || candidate.NetworkIndex >= len(networks) || !allowed[candidate.NodeID] || (scope.RequiredNodeID != "" && scope.RequiredNodeID != candidate.NodeID) {
		return Allocation{}, ErrNodeUnavailable
	}
	request.NodeID, request.NodeVersion, request.SessionEpoch = candidate.NodeID, candidate.NodeVersion, candidate.SessionEpoch
	return s.Resources.ReserveRegionalResources(ctx, request, networks[candidate.NetworkIndex], s.MaxHeartbeatAge)
}
