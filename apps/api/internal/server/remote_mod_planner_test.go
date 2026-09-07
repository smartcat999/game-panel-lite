package server

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/minecraft"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestRemoteModPlanPublishesWorkspaceClosureWithoutLocalFiles(t *testing.T) {
	for _, mode := range []string{"chain", "cycle", "foreign-root", "foreign-dependency", "ambiguous", "bad-digest", "bad-dependencies", "workshop", "stale", "empty"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			db, err := store.Open(filepath.Join(root, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			org := domain.Organization{ID: "workspace", Slug: "workspace"}
			if err := db.CreateOrganization(ctx, &org, "owner"); err != nil {
				t.Fatal(err)
			}
			target := domain.GameServer{ID: "remote-server", NodeID: "remote-node", OrganizationID: org.ID, ProviderKey: domain.ProviderTerrariaTModLoader, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning, ModIDs: []string{"Root"}, Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(root, "must-not-exist")}, Network: domain.ServerNetworkSpec{Port: 7777, HostPort: 47777}}}
			for _, name := range []string{"Root", "Dependency", "Leaf"} {
				item := domain.ModFile{ID: name, OrganizationID: org.ID, InstanceID: "unassigned", ProviderKey: target.ProviderKey, Source: "upload", FileName: name + ".tmod", ModName: name, SizeBytes: 4, ContentHash: strings.Repeat("a", 64)}
				if name == "Root" {
					item.DependenciesJSON = `["Dependency"]`
				}
				if name == "Dependency" {
					item.DependenciesJSON = `["Leaf"]`
				}
				if mode == "cycle" && name == "Leaf" {
					item.DependenciesJSON = `["Root"]`
				}
				if mode == "foreign-root" && name == "Root" || mode == "foreign-dependency" && name == "Dependency" {
					item.OrganizationID = "foreign"
				}
				if mode == "bad-digest" && name == "Leaf" {
					item.ContentHash = "invalid"
				}
				if mode == "bad-dependencies" && name == "Root" {
					item.DependenciesJSON = `broken`
				}
				if mode == "workshop" && name == "Root" {
					item.Source = "workshop"
				}
				if err := db.CreateMod(ctx, &item); err != nil {
					t.Fatal(err)
				}
				if mode == "ambiguous" && name == "Dependency" {
					item.ID = "OtherDependency"
					if err := db.CreateMod(ctx, &item); err != nil {
						t.Fatal(err)
					}
				}
			}
			if mode == "empty" {
				target.Spec.ModIDs = nil
			}
			if err := db.CreateGameServer(ctx, &target); err != nil {
				t.Fatal(err)
			}
			if mode == "stale" {
				target.Spec.Generation++
			}
			registry := mustRegistry(t, terraria.NewTModLoaderProvider())
			builder := NewProviderWorkloadBuilder(registry).WithModPlanner(NewRuntimeModPlanner(root, db, registry))
			spec, err := builder.BuildWorkloadSpec(ctx, target)
			success := mode == "chain" || mode == "cycle" || mode == "empty"
			if success {
				if err != nil {
					t.Fatal(err)
				}
				expected := map[string]bool{"Root": false, "Dependency": false, "Leaf": false}
				if mode == "empty" {
					expected = map[string]bool{}
				}
				for _, artifact := range spec.Options.Artifacts {
					if _, ok := expected[artifact.ID]; !ok {
						t.Fatalf("unexpected artifact %+v", artifact)
					}
					expected[artifact.ID] = true
				}
				for name, found := range expected {
					if !found {
						t.Fatalf("missing dependency %s", name)
					}
				}
				if !strings.Contains(spec.Options.Files["Mods/enabled.json"], "Root") && mode != "empty" {
					t.Fatal("missing provider manifest")
				}
				again, err := builder.BuildWorkloadSpec(ctx, target)
				if err != nil || !reflect.DeepEqual(spec, again) {
					t.Fatalf("non-deterministic plan: %v", err)
				}
				assignment := domain.WorkloadAssignment{ID: "assignment", UID: "uid", NodeID: target.NodeID, ServerID: target.ID, Generation: 1, DesiredState: domain.DesiredRunning, Spec: spec}
				if err := db.PublishWorkloadAssignment(ctx, target, &assignment); err != nil {
					t.Fatal(err)
				}
				for _, artifact := range spec.Options.Artifacts {
					item, ref, err := db.ResolveArtifactForNode(ctx, target.NodeID, assignment.UID, 1, artifact.ID)
					if err != nil || item.ID != artifact.ID || ref.SHA256 != artifact.SHA256 {
						t.Fatalf("published artifact not authorized: %v", err)
					}
				}
			} else if err == nil {
				t.Fatal("accepted invalid remote plan")
			}
			if _, err := os.Stat(target.Spec.Runtime.DataDir); !os.IsNotExist(err) {
				t.Fatalf("remote plan touched local data: %v", err)
			}
			installed, err := db.ListMods(ctx, target.ID)
			if err != nil || len(installed) != 0 {
				t.Fatalf("remote plan created installed records: %v %+v", err, installed)
			}
		})
	}
}

func TestRemoteStopAndDeleteDoNotRequireModSources(t *testing.T) {
	for _, state := range []domain.ServerDesiredState{domain.DesiredStopped, domain.DesiredDeleted} {
		target := domain.GameServer{ID: "server", NodeID: "remote", Spec: domain.ServerSpec{DesiredState: state, ModIDs: []string{"missing"}}}
		spec, err := NewProviderWorkloadBuilder(nil).BuildWorkloadSpec(context.Background(), target)
		if err != nil || spec.ServerID != target.ID || len(spec.Options.Artifacts) != 0 {
			t.Fatalf("%s requires source/provider: %+v %v", state, spec, err)
		}
	}
}

func TestRemoteModPlanAllowsProvidersWithoutManifestWhenNoModsSelected(t *testing.T) {
	for _, game := range []provider.GameProvider{dst.NewProvider(), palworld.NewProvider(), minecraft.NewProvider()} {
		t.Run(string(game.Key()), func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			target := domain.GameServer{ID: "server", NodeID: "remote", ProviderKey: game.Key(), Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
			if err := db.CreateGameServer(context.Background(), &target); err != nil {
				t.Fatal(err)
			}
			registry := mustRegistry(t, game)
			if _, err := NewRuntimeModPlanner(t.TempDir(), db, registry).PlanRemoteMods(context.Background(), target); err != nil {
				t.Fatal(err)
			}
		})
	}
}
