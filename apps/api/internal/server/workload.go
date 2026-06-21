package server

import (
	"context"
	"fmt"
	"os"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
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
	if b.providers == nil {
		return domain.WorkloadSpec{}, fmt.Errorf("provider registry is required")
	}
	gameProvider, ok := b.providers.Get(server.ProviderKey)
	if !ok {
		return domain.WorkloadSpec{}, fmt.Errorf("unknown provider: %s", server.ProviderKey)
	}
	version := server.Spec.Version
	if version == "" || !providerVersionSupported(gameProvider.Versions(), version) {
		version = recommendedProviderVersion(gameProvider.Versions())
	}
	if server.Spec.Runtime.DataDir != "" {
		if err := os.MkdirAll(server.Spec.Runtime.DataDir, 0o755); err != nil {
			return domain.WorkloadSpec{}, err
		}
	}
	if b.mods != nil {
		if err := b.mods.PlanMods(ctx, server); err != nil {
			return domain.WorkloadSpec{}, err
		}
	}
	runtimeConfig, err := runtimeConfigForResource(gameProvider, server)
	if err != nil {
		return domain.WorkloadSpec{}, err
	}
	files := map[string]string{}
	if runtimeConfig.ConfigText != "" {
		files["serverconfig.txt"] = runtimeConfig.ConfigText
	}
	for name, content := range runtimeConfig.Options.Files {
		files[name] = content
	}
	return domain.WorkloadSpec{
		ServerID: server.ID,
		Name:     server.Name,
		Image:    gameProvider.ImageFor(version),
		Network: domain.WorkloadNetwork{
			Port:     runtimeConfig.Port,
			HostPort: server.Spec.Network.HostPort,
			Protocol: runtimeConfig.Protocol,
		},
		Resources: domain.WorkloadResources{
			CPULimitCores: server.Spec.Resources.CPULimitCores,
			MemoryLimitMB: server.Spec.Resources.MemoryLimitMB,
		},
		DataDir: server.Spec.Runtime.DataDir,
		Options: domain.WorkloadOptions{
			Env:        append([]string{}, runtimeConfig.Options.Env...),
			Cmd:        append([]string{}, runtimeConfig.Options.Cmd...),
			Files:      files,
			DataMounts: append([]string{}, runtimeConfig.Options.DataMounts...),
		},
	}, nil
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
