package server

import (
	"context"
	"fmt"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type ProviderRegistry interface {
	Get(domain.ProviderKey) (provider.GameProvider, bool)
}

type ProviderWorkloadBuilder struct {
	providers ProviderRegistry
	mods      ModPlanner
}

func NewProviderWorkloadBuilder(providers ProviderRegistry) *ProviderWorkloadBuilder {
	return &ProviderWorkloadBuilder{providers: providers}
}

func (b *ProviderWorkloadBuilder) WithModPlanner(planner ModPlanner) *ProviderWorkloadBuilder {
	b.mods = planner
	return b
}

func (b *ProviderWorkloadBuilder) BuildWorkloadSpec(ctx context.Context, server domain.GameServer) (domain.WorkloadSpec, error) {
	// Stop/delete operate on observed runtime identity and must remain available
	// when an uploaded source or provider configuration is no longer usable.
	if server.Spec.DesiredState == domain.DesiredStopped || server.Spec.DesiredState == domain.DesiredDeleted {
		return domain.WorkloadSpec{ServerID: server.ID, Name: server.Name}, nil
	}
	if b.providers == nil {
		return domain.WorkloadSpec{}, fmt.Errorf("provider registry is required")
	}
	gameProvider, ok := b.providers.Get(server.ProviderKey)
	if !ok {
		return domain.WorkloadSpec{}, fmt.Errorf("unknown provider: %s", server.ProviderKey)
	}
	if err := provider.CheckConfigVersion(gameProvider, server.Spec.ConfigVersion); err != nil {
		return domain.WorkloadSpec{}, err
	}
	version := server.Spec.Version
	if version == "" || !providerVersionSupported(gameProvider.Versions(), version) {
		version = recommendedProviderVersion(gameProvider.Versions())
	}
	remoteMods := workload.Options{}
	if planner, ok := b.mods.(RemoteModPlanner); ok {
		var err error
		remoteMods, err = planner.PlanRemoteMods(ctx, server)
		if err != nil {
			return domain.WorkloadSpec{}, err
		}
	} else if len(server.Spec.ModIDs) > 0 {
		return domain.WorkloadSpec{}, fmt.Errorf("remote mod planner is required")
	}
	runtimeConfig, err := runtimeConfigForResource(gameProvider, server)
	if err != nil {
		return domain.WorkloadSpec{}, err
	}
	files := map[string]string{}
	for name, content := range runtimeConfig.Options.Files {
		files[name] = content
	}
	for name, content := range remoteMods.Files {
		if _, exists := files[name]; exists {
			return domain.WorkloadSpec{}, fmt.Errorf("mod manifest conflicts with runtime config %q", name)
		}
		files[name] = content
	}
	if err := workload.ValidateArtifacts(workload.Options{Files: files, Artifacts: remoteMods.Artifacts}); err != nil {
		return domain.WorkloadSpec{}, err
	}
	additionalPorts := make([]domain.WorkloadPort, 0, len(runtimeConfig.AdditionalPorts))
	for _, port := range runtimeConfig.AdditionalPorts {
		additionalPorts = append(additionalPorts, domain.WorkloadPort{
			Port:     port,
			HostPort: server.Spec.Network.HostPort + (port - runtimeConfig.Port),
			Protocol: runtimeConfig.Protocol,
		})
	}
	return domain.WorkloadSpec{
		ServerID: server.ID,
		Name:     server.Name,
		Image:    gameProvider.ImageFor(version),
		Network: domain.WorkloadNetwork{
			Port:            runtimeConfig.Port,
			HostPort:        server.Spec.Network.HostPort,
			Protocol:        runtimeConfig.Protocol,
			AdditionalPorts: additionalPorts,
		},
		Resources: domain.WorkloadResources{
			CPULimitCores: server.Spec.Resources.CPULimitCores,
			MemoryLimitMB: server.Spec.Resources.MemoryLimitMB,
		},
		DataDir: server.Spec.Runtime.DataDir,
		Options: domain.WorkloadOptions{
			Env:        runtimeEnvironment(runtimeConfig.Options.Env, server),
			Cmd:        append([]string{}, runtimeConfig.Options.Cmd...),
			Files:      files,
			Artifacts:  remoteMods.Artifacts,
			DataMounts: append([]string{}, runtimeConfig.Options.DataMounts...),
		},
	}, nil
}

func runtimeEnvironment(providerEnv []string, server domain.GameServer) []string {
	env := append([]string{}, providerEnv...)
	env = append(env, server.Spec.Runtime.Env...)
	if server.ProviderKey == domain.ProviderDST && server.Spec.Runtime.ModSyncMode != "" {
		env = append(env, "DST_MOD_SYNC_MODE="+server.Spec.Runtime.ModSyncMode)
		env = append(env, "DST_GAME_UPDATE_MODE="+server.Spec.Runtime.ModSyncMode)
	}
	return env
}

func runtimeConfigForResource(gameProvider provider.GameProvider, server domain.GameServer) (domain.ProviderRuntimeConfig, error) {
	if resourceProvider, ok := gameProvider.(provider.ResourceRuntimeProvider); ok {
		return resourceProvider.RuntimeConfigForResource(server)
	}
	return domain.ProviderRuntimeConfig{}, fmt.Errorf("provider %s does not implement resource runtime config", gameProvider.Key())
}

func recommendedProviderVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	return versions[0]
}

func providerVersionSupported(versions []string, version string) bool {
	for _, item := range versions {
		if item == version {
			return true
		}
	}
	return false
}
