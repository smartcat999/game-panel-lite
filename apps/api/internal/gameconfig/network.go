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

// RegionalRenderer derives admission and runtime configuration from one immutable revision.
// The caller supplies an authorized snapshot and host port. Rendering neither
// grants execution authority nor proves that a host port is available.
type RegionalRenderer struct {
	Normalizer     LogicalNormalizer
	Configurations interface {
		Open(context.Context, instances.ConfigurationBinding, instances.ProtectedConfiguration) ([]byte, error)
	}
}

// Render exposes only admission ports to the scheduler, never provider secrets.
func (r RegionalRenderer) Render(ctx context.Context, snapshot regional.RevisionSnapshot, hostPort int) (workload.Network, error) {
	config, _, err := r.providerConfiguration(ctx, snapshot)
	if err != nil {
		return workload.Network{}, err
	}
	return provider.RuntimeNetwork(config, hostPort)
}

func (r RegionalRenderer) providerConfiguration(ctx context.Context, snapshot regional.RevisionSnapshot) (domain.ProviderRuntimeConfig, string, error) {
	if err := ctx.Err(); err != nil {
		return domain.ProviderRuntimeConfig{}, "", err
	}
	if r.Configurations == nil || snapshot.ValidateFor(snapshot.Event) != nil ||
		snapshot.CurrentSpecGeneration != snapshot.Revision.SpecGeneration {
		return domain.ProviderRuntimeConfig{}, "", ErrInvalidLogicalConfiguration
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
		return domain.ProviderRuntimeConfig{}, "", ErrInvalidLogicalConfiguration
	}
	normalized, err := r.Normalizer.Normalize(ctx, spec.ProviderKey, spec.GameVersion, spec.ConfigSchemaVersion, raw)
	if err != nil {
		return domain.ProviderRuntimeConfig{}, "", err
	}
	defer clear(normalized)
	var config map[string]any
	if json.Unmarshal(normalized, &config) != nil {
		return domain.ProviderRuntimeConfig{}, "", ErrInvalidLogicalConfiguration
	}
	p, ok := r.Normalizer.Providers.Get(domain.ProviderKey(spec.ProviderKey))
	if !ok {
		return domain.ProviderRuntimeConfig{}, "", ErrInvalidLogicalConfiguration
	}
	runtimeProvider, ok := p.(provider.ResourceRuntimeProvider)
	if !ok {
		return domain.ProviderRuntimeConfig{}, "", ErrInvalidLogicalConfiguration
	}
	runtimeConfig, err := runtimeProvider.RuntimeConfigForResource(domain.GameServer{
		ID: revision.ServerID, OrganizationID: snapshot.Event.OrganizationID,
		GameKey: p.GameKey(), ProviderKey: p.Key(),
		Spec: domain.ServerSpec{Version: spec.GameVersion, ConfigVersion: spec.ConfigSchemaVersion, Config: config},
	})
	if err != nil {
		return domain.ProviderRuntimeConfig{}, "", ErrInvalidLogicalConfiguration
	}
	if err := ctx.Err(); err != nil {
		return domain.ProviderRuntimeConfig{}, "", err
	}
	return runtimeConfig, p.ImageFor(spec.GameVersion), nil
}
