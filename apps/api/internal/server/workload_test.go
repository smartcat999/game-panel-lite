package server

import (
	"context"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/minecraft"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

func TestProviderWorkloadBuilderUsesProviderRuntimeContract(t *testing.T) {
	registry := provider.NewRegistry(terraria.NewVanillaProvider(runtimecatalog.Catalog{}))
	builder := NewProviderWorkloadBuilder(registry)
	server := domain.GameServer{
		ID:          "srv-1",
		Name:        "Friends",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec: domain.ServerSpec{
			Generation:   2,
			DesiredState: domain.DesiredRunning,
			Version:      "1.4.5.6",
			Config: map[string]any{
				"serverName":      "Friends",
				"worldName":       "Friends World",
				"worldSize":       "medium",
				"worldEvil":       "random",
				"difficulty":      "classic",
				"maxPlayers":      float64(8),
				"port":            float64(7777),
				"secure":          true,
				"language":        "en-US",
				"autoCreateWorld": true,
			},
			Network: domain.ServerNetworkSpec{Port: 7777, HostPort: 47777},
			Runtime: domain.ServerRuntimeSpec{DataDir: t.TempDir()},
		},
	}

	spec, err := builder.BuildWorkloadSpec(context.Background(), server)
	if err != nil {
		t.Fatalf("build workload spec: %v", err)
	}
	if spec.ServerID != server.ID {
		t.Fatalf("expected server id %q, got %q", server.ID, spec.ServerID)
	}
	if spec.Network.Port != 7777 || spec.Network.HostPort != 47777 {
		t.Fatalf("expected network ports to round trip, got %+v", spec.Network)
	}
	if spec.Image == "" {
		t.Fatal("expected provider image")
	}
	if !strings.Contains(spec.Options.Files["serverconfig.txt"], "worldname=Friends World") {
		t.Fatalf("expected rendered server config, got %q", spec.Options.Files["serverconfig.txt"])
	}
}

func TestProviderWorkloadBuilderUsesResourceRuntimeProviders(t *testing.T) {
	tests := []struct {
		name        string
		provider    provider.GameProvider
		gameKey     domain.GameKey
		providerKey domain.ProviderKey
		config      map[string]any
		assert      func(t *testing.T, spec domain.WorkloadSpec)
	}{
		{
			name:        "palworld",
			provider:    palworld.NewProvider(runtimecatalog.Catalog{}),
			gameKey:     domain.GamePalworld,
			providerKey: domain.ProviderPalworld,
			config: map[string]any{
				"serverName":     "Payload Pal",
				"saveName":       "Payload Save",
				"maxPlayers":     float64(14),
				"serverPassword": "join-secret",
				"adminPassword":  "admin-secret",
			},
			assert: func(t *testing.T, spec domain.WorkloadSpec) {
				t.Helper()
				if spec.Network.Protocol != "udp" || spec.Network.Port != palworld.DefaultInternalPort {
					t.Fatalf("unexpected Palworld network: %+v", spec.Network)
				}
				if _, ok := spec.Options.Files["serverconfig.txt"]; ok {
					t.Fatal("Palworld resource runtime should not create legacy serverconfig.txt")
				}
				if !containsEnv(spec.Options.Env, "SERVER_NAME=Payload Pal") || !containsEnv(spec.Options.Env, "PLAYERS=14") {
					t.Fatalf("expected Palworld env from payload, got %+v", spec.Options.Env)
				}
			},
		},
		{
			name:        "dst",
			provider:    dst.NewProvider(runtimecatalog.Catalog{}),
			gameKey:     domain.GameDST,
			providerKey: domain.ProviderDST,
			config: map[string]any{
				"serverName":         "DST Friends",
				"clusterName":        "FriendsCluster",
				"maxPlayers":         float64(6),
				"clusterToken":       "klei-token",
				"gameMode":           "endless",
				"worldPreset":        "forest_classic",
				"clusterDescription": "Friends only",
			},
			assert: func(t *testing.T, spec domain.WorkloadSpec) {
				t.Helper()
				cluster := spec.Options.Files["dst/FriendsCluster/cluster.ini"]
				if spec.Network.Protocol != "udp" || !strings.Contains(cluster, "game_mode = endless") || !strings.Contains(cluster, "cluster_description = Friends only") {
					t.Fatalf("expected DST runtime files from payload, network=%+v files=%+v", spec.Network, spec.Options.Files)
				}
				if _, ok := spec.Options.Files["serverconfig.txt"]; ok {
					t.Fatal("DST resource runtime should not create legacy serverconfig.txt")
				}
			},
		},
		{
			name:        "minecraft",
			provider:    minecraft.NewProvider(runtimecatalog.Catalog{}),
			gameKey:     domain.GameMinecraft,
			providerKey: domain.ProviderMinecraft,
			config: map[string]any{
				"serverName":   "Minecraft Friends",
				"worldName":    "friends-world",
				"maxPlayers":   float64(20),
				"eulaAccepted": true,
				"gameMode":     "creative",
				"difficulty":   "peaceful",
			},
			assert: func(t *testing.T, spec domain.WorkloadSpec) {
				t.Helper()
				properties := spec.Options.Files["data/server.properties"]
				if spec.Network.Protocol != "tcp" || !strings.Contains(properties, "gamemode=creative") || !strings.Contains(properties, "difficulty=peaceful") {
					t.Fatalf("expected Minecraft runtime files from payload, network=%+v files=%+v", spec.Network, spec.Options.Files)
				}
				if _, ok := spec.Options.Files["serverconfig.txt"]; ok {
					t.Fatal("Minecraft resource runtime should not create legacy serverconfig.txt")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := provider.NewRegistry(tt.provider)
			builder := NewProviderWorkloadBuilder(registry)
			spec, err := builder.BuildWorkloadSpec(context.Background(), domain.GameServer{
				ID:          "srv-" + tt.name,
				Name:        "Friends",
				GameKey:     tt.gameKey,
				ProviderKey: tt.providerKey,
				Spec: domain.ServerSpec{
					Generation:   1,
					DesiredState: domain.DesiredRunning,
					Version:      tt.provider.Versions()[0],
					Config:       tt.config,
					Runtime:      domain.ServerRuntimeSpec{DataDir: t.TempDir()},
				},
			})
			if err != nil {
				t.Fatalf("build workload spec: %v", err)
			}
			tt.assert(t, spec)
		})
	}
}

func containsEnv(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
