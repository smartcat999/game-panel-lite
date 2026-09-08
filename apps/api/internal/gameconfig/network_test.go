package gameconfig_test

import (
	"bytes"
	"context"
	"errors"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"reflect"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/configprotection"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/gameconfig"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/server"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestRegionalNetworkMatchesRuntime(t *testing.T) {
	ctx := context.Background()
	for _, p := range []provider.GameProvider{terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider()} {
		t.Run(string(p.Key()), func(t *testing.T) {
			registry, err := provider.NewRegistry(p)
			if err != nil {
				t.Fatal(err)
			}
			protector, err := configprotection.New("key", map[string][]byte{"key": bytes.Repeat([]byte{1}, 32)}, 4096)
			if err != nil {
				t.Fatal(err)
			}
			event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "org", ServerID: "server", RevisionID: "revision", RegionID: "region", PlacementEpoch: 1, SpecGeneration: 1}
			binding := instances.ConfigurationBinding{OrganizationID: event.OrganizationID, ServerID: event.ServerID, RevisionID: event.RevisionID, SpecGeneration: 1, ProviderKey: string(p.Key()), ConfigSchemaVersion: p.CatalogMetadata().ConfigVersion}
			protected, err := protector.Seal(ctx, binding, []byte(`{"password":"private","maxPlayers":5}`))
			if err != nil {
				t.Fatal(err)
			}
			snapshot := regional.RevisionSnapshot{Event: event, CurrentSpecGeneration: 1, DesiredState: "running", IntentVersion: 1, Revision: instances.Revision{
				ID: event.RevisionID, ServerID: event.ServerID, SpecGeneration: 1, Specification: instances.Specification{
					ProviderKey: string(p.Key()), GameVersion: p.Versions()[0], ConfigSchemaVersion: binding.ConfigSchemaVersion, Configuration: protected, Resources: instances.Resources{CPU: 1, MemoryMB: 512},
				},
			}}
			renderer := gameconfig.RegionalRenderer{Normalizer: gameconfig.LogicalNormalizer{Providers: registry, MaxBytes: 4096}, Configurations: protector}
			builder := server.NewProviderWorkloadBuilder(registry)
			for _, host := range []int{0, 32000} {
				network, err := renderer.Render(ctx, snapshot, host)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := builder.BuildWorkloadSpec(ctx, domain.GameServer{ID: event.ServerID, ProviderKey: p.Key(), Spec: domain.ServerSpec{DesiredState: domain.DesiredRunning, Version: p.Versions()[0], ConfigVersion: binding.ConfigSchemaVersion, Config: map[string]any{"password": "private", "maxPlayers": 5}, Network: domain.ServerNetworkSpec{HostPort: host}}})
				if err != nil {
					t.Fatal(err)
				}
				want, err := workload.ResolvePortBindings(actual.Network)
				if err != nil {
					t.Fatal(err)
				}
				got, err := workload.ResolvePortBindings(network)
				if err != nil || !reflect.DeepEqual(got, want) || len(got) == 0 {
					t.Fatalf("runtime/admission mismatch: %v %v %v", got, want, err)
				}
				assertRegionalWorkload(t, renderer, snapshot, got, actual)
			}
			cases := map[string]func(*regional.RevisionSnapshot){
				"tenant":   func(s *regional.RevisionSnapshot) { s.Event.OrganizationID = "other" },
				"revision": func(s *regional.RevisionSnapshot) { s.Event.RevisionID = "other"; s.Revision.ID = "other" },
				"generation": func(s *regional.RevisionSnapshot) {
					s.Event.SpecGeneration = 2
					s.Revision.SpecGeneration = 2
					s.CurrentSpecGeneration = 2
				},
				"obsolete": func(s *regional.RevisionSnapshot) { s.CurrentSpecGeneration = 2 },
				"version":  func(s *regional.RevisionSnapshot) { s.Revision.Specification.GameVersion = "unknown" },
				"schema":   func(s *regional.RevisionSnapshot) { s.Revision.Specification.ConfigSchemaVersion++ },
				"key":      func(s *regional.RevisionSnapshot) { s.Revision.Specification.Configuration.KeyID = "unknown" },
			}
			for name, mutate := range cases {
				t.Run(name, func(t *testing.T) {
					changed := snapshot
					mutate(&changed)
					got, err := renderer.Render(ctx, changed, 32000)
					if err == nil || !reflect.DeepEqual(got, workload.Network{}) {
						t.Fatal("invalid revision rendered")
					}
				})
			}
			if _, err := renderer.Render(ctx, snapshot, 65536); err == nil {
				t.Fatal("invalid host port accepted")
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if _, err := renderer.Render(cancelled, snapshot, 32000); err != context.Canceled {
				t.Fatalf("cancellation: %v", err)
			}
		})
	}
}

func assertRegionalWorkload(t *testing.T, renderer gameconfig.RegionalRenderer, snapshot regional.RevisionSnapshot, ports []workload.Port, expected workload.Spec) {
	t.Helper()
	allocation := regional.Allocation{ID: "allocation", CPU: 1, MemoryMB: 512, Status: "reserved", Ports: ports, CapacityRequest: regional.CapacityRequest{RegionID: snapshot.Event.RegionID, OrganizationID: snapshot.Event.OrganizationID, ServerID: snapshot.Event.ServerID, DeploymentID: "deployment", RevisionID: snapshot.Event.RevisionID, SpecGeneration: 1, PlacementEpoch: 1, IntentVersion: 1, NodeID: "node", NodeVersion: 1, SessionEpoch: 1}}
	rendered, err := renderer.RenderWorkload(context.Background(), snapshot, allocation)
	if err != nil {
		t.Fatal(err)
	}
	actualPorts, err := workload.ResolvePortBindings(rendered.Network)
	if err != nil || !reflect.DeepEqual(actualPorts, ports) || rendered.Image != expected.Image || !reflect.DeepEqual(rendered.Options.Files, expected.Options.Files) || rendered.Resources.CPULimitCores != 1 || rendered.Resources.MemoryLimitMB != 512 || rendered.DataDir != "" {
		t.Fatal("workload diverged from provider or reservation", err)
	}
	if !strings.Contains(rendered.Options.Files["serverconfig.txt"], "private") {
		t.Fatal("runtime configuration lost password")
	}
	for name, mutate := range map[string]func(*regional.Allocation){
		"tenant":        func(a *regional.Allocation) { a.OrganizationID = "other" },
		"region":        func(a *regional.Allocation) { a.RegionID = "other" },
		"revision":      func(a *regional.Allocation) { a.RevisionID = "other" },
		"intent":        func(a *regional.Allocation) { a.IntentVersion++ },
		"generation":    func(a *regional.Allocation) { a.SpecGeneration++ },
		"placement":     func(a *regional.Allocation) { a.PlacementEpoch++ },
		"cpu":           func(a *regional.Allocation) { a.CPU++ },
		"memory":        func(a *regional.Allocation) { a.MemoryMB++ },
		"released":      func(a *regional.Allocation) { a.Status = "released" },
		"missing ports": func(a *regional.Allocation) { a.Ports = nil },
		"extra ports": func(a *regional.Allocation) {
			a.Ports = append(append([]workload.Port{}, a.Ports...), workload.Port{Port: 1, HostPort: 1, Protocol: "tcp"})
		},
		"protocol": func(a *regional.Allocation) {
			a.Ports = append([]workload.Port{}, a.Ports...)
			a.Ports[0].Protocol = "udp"
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := allocation
			mutate(&changed)
			got, err := renderer.RenderWorkload(context.Background(), snapshot, changed)
			if err == nil || !reflect.DeepEqual(got, workload.Spec{}) {
				t.Fatal("invalid allocation produced secret workload")
			}
		})
	}
	withAssets := snapshot
	withAssets.Revision.Specification.Assets = []instances.AssetVersion{{AssetID: "world", Version: "v1"}}
	withAssets.Assets = []assets.PublishedVersion{{AssetID: "world", Version: "v1", OrganizationID: snapshot.Event.OrganizationID, SHA256: strings.Repeat("a", 64), SizeBytes: 4}}
	if _, err := renderer.RenderWorkload(context.Background(), withAssets, allocation); !errors.Is(err, gameconfig.ErrRegionalAssetMaterializationRequired) {
		t.Fatal("asset reference silently omitted", err)
	}
	stopped := snapshot
	stopped.DesiredState = "stopped"
	if _, err := renderer.RenderWorkload(context.Background(), stopped, allocation); err == nil {
		t.Fatal("stopped intent rendered running workload")
	}
	rendered.Options.Files["serverconfig.txt"] = "changed"
	again, err := renderer.RenderWorkload(context.Background(), snapshot, allocation)
	if err != nil || again.Options.Files["serverconfig.txt"] != expected.Options.Files["serverconfig.txt"] {
		t.Fatal("rendered config reused mutable state", err)
	}
}
