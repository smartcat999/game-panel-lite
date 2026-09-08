package gameconfig

import (
	"context"
	"encoding/json"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

// RegionalNetworkRenderer derives admission ports from an immutable revision.
// The caller supplies an authorized snapshot and host port. Rendering neither
// grants execution authority nor proves that a host port is available.
type RegionalNetworkRenderer struct {
	Normalizer     LogicalNormalizer
	Configurations interface {
		Open(context.Context, instances.ConfigurationBinding, instances.ProtectedConfiguration) ([]byte, error)
	}
}

func (r RegionalNetworkRenderer) Render(ctx context.Context, snapshot regional.RevisionSnapshot, hostPort int) (workload.Network, error) {
	if err := ctx.Err(); err != nil {
		return workload.Network{}, err
	}
	if r.Configurations == nil || snapshot.ValidateFor(snapshot.Event) != nil ||
		snapshot.CurrentSpecGeneration != snapshot.Revision.SpecGeneration {
		return workload.Network{}, ErrInvalidLogicalConfiguration
	}
	revision := snapshot.Revision
	spec := revision.Specification
	binding := instances.ConfigurationBinding{
		OrganizationID: snapshot.Event.OrganizationID, ServerID: revision.ServerID,
		RevisionID: revision.ID, SpecGeneration: revision.SpecGeneration,
		ProviderKey: spec.ProviderKey, ConfigSchemaVersion: spec.ConfigSchemaVersion,
	}
	raw, err := r.Configurations.Open(ctx, binding, spec.Configuration)
	defer clear(raw)
	if err != nil {
		return workload.Network{}, ErrInvalidLogicalConfiguration
	}
	normalized, err := r.Normalizer.Normalize(ctx, spec.ProviderKey, spec.GameVersion, spec.ConfigSchemaVersion, raw)
	if err != nil {
		return workload.Network{}, err
	}
	defer clear(normalized)
	var config map[string]any
	if json.Unmarshal(normalized, &config) != nil {
		return workload.Network{}, ErrInvalidLogicalConfiguration
	}
	p, ok := r.Normalizer.Providers.Get(domain.ProviderKey(spec.ProviderKey))
	if !ok {
		return workload.Network{}, ErrInvalidLogicalConfiguration
	}
	runtimeProvider, ok := p.(provider.ResourceRuntimeProvider)
	if !ok {
		return workload.Network{}, ErrInvalidLogicalConfiguration
	}
	runtimeConfig, err := runtimeProvider.RuntimeConfigForResource(domain.GameServer{
		ID: revision.ServerID, OrganizationID: snapshot.Event.OrganizationID,
		GameKey: p.GameKey(), ProviderKey: p.Key(),
		Spec: domain.ServerSpec{Version: spec.GameVersion, ConfigVersion: spec.ConfigSchemaVersion, Config: config},
	})
	if err != nil {
		return workload.Network{}, ErrInvalidLogicalConfiguration
	}
	if err := ctx.Err(); err != nil {
		return workload.Network{}, err
	}
	// Do not return secret-bearing provider files or environment to the scheduler.
	return provider.RuntimeNetwork(runtimeConfig, hostPort)
}
