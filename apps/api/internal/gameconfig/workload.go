package gameconfig

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

var ErrRegionalAssetMaterializationRequired = errors.New("regional asset materialization is required")

// RenderWorkload constructs secret-bearing runtime data for an already verified
// allocation. It grants no execution authority and performs no persistence.
// Callers must protect the output and authorize delivery separately. Host data
// directories are selected by the Node adapter, never supplied by this payload.
func (r RegionalRenderer) RenderWorkload(ctx context.Context, snapshot regional.RevisionSnapshot, allocation regional.Allocation) (workload.Spec, error) {
	if err := ctx.Err(); err != nil {
		return workload.Spec{}, err
	}
	event := snapshot.Event
	resources := snapshot.Revision.Specification.Resources
	if snapshot.ValidateFor(event) != nil || snapshot.DesiredState != "running" ||
		allocation.ID == "" || allocation.DeploymentID == "" || allocation.NodeID == "" || allocation.NodeVersion < 1 || allocation.SessionEpoch < 1 ||
		allocation.RegionID != event.RegionID || allocation.OrganizationID != event.OrganizationID || allocation.ServerID != event.ServerID ||
		allocation.PlacementEpoch != event.PlacementEpoch || allocation.RevisionID != event.RevisionID || allocation.SpecGeneration != event.SpecGeneration ||
		allocation.IntentVersion != snapshot.IntentVersion || allocation.Status != "reserved" || allocation.CPU != resources.CPU || allocation.MemoryMB != resources.MemoryMB || resources.MemoryMB > int64(math.MaxInt) || len(allocation.Ports) > 128 {
		return workload.Spec{}, regional.ErrAllocationConflict
	}
	// Do not silently omit saves/mods or turn catalog metadata into node files.
	if len(snapshot.Revision.Specification.Assets) > 0 {
		return workload.Spec{}, ErrRegionalAssetMaterializationRequired
	}
	config, image, err := r.providerConfiguration(ctx, snapshot)
	if err != nil {
		return workload.Spec{}, err
	}
	if image == "" || strings.TrimSpace(image) != image {
		return workload.Spec{}, ErrInvalidLogicalConfiguration
	}
	if len(config.Options.Artifacts) > 0 {
		return workload.Spec{}, ErrRegionalAssetMaterializationRequired
	}
	protocol := config.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	hostPort := 0
	for _, port := range allocation.Ports {
		if port.Port == config.Port && port.Protocol == protocol {
			hostPort = port.HostPort
			break
		}
	}
	network, err := provider.RuntimeNetwork(config, hostPort)
	if err != nil {
		return workload.Spec{}, err
	}
	ports, err := workload.ResolvePortBindings(network)
	if err != nil || !slices.Equal(ports, allocation.Ports) {
		return workload.Spec{}, regional.ErrAllocationConflict
	}
	if err := ctx.Err(); err != nil {
		return workload.Spec{}, err
	}
	return workload.Spec{ServerID: event.ServerID, Name: event.ServerID, Image: image, Network: network,
		Resources: workload.Resources{CPULimitCores: resources.CPU, MemoryLimitMB: int(resources.MemoryMB)}, Options: config.Options}, nil
}
