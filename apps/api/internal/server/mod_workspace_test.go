package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modruntime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestPlannerWorkspaceSources(t *testing.T) {
	for _, mode := range []string{"owned-chain", "foreign-root", "legacy-root", "wrong-provider-root", "stale-target", "missing-private-dependency"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			db, err := store.Open(filepath.Join(root, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			registry := mustRegistry(t, terraria.NewTModLoaderProvider())
			files := modsvc.NewService(root, modruntime.NewService(registry, db).StoredFileName)
			target := domain.GameServer{ID: "target", OrganizationID: "own", ProviderKey: domain.ProviderTerrariaTModLoader, Spec: domain.ServerSpec{ModIDs: []string{"own-Root"}, Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(root, "instances", "target")}}}
			for _, scope := range []string{"own", "foreign", ""} {
				for _, name := range []string{"Root", "Dependency", "Leaf"} {
					if mode == "missing-private-dependency" && scope == "own" && name != "Root" {
						continue
					}
					item := domain.ModFile{ID: scope + "-" + name, OrganizationID: scope, InstanceID: "unassigned", ProviderKey: target.ProviderKey, FileName: name + ".tmod", ModName: name, Title: name, Enabled: true, Source: "upload"}
					if name == "Root" {
						item.DependenciesJSON = `["Dependency"]`
					}
					if name == "Dependency" {
						item.DependenciesJSON = `["Leaf"]`
					}
					payload := scope + ":" + name
					if scope == "" {
						_, _, err = files.Upload("unassigned", item.ProviderKey, item.FileName, strings.NewReader(payload))
					} else {
						_, err = files.PutLibrary(ctx, item, strings.NewReader(payload), 1024)
					}
					if err != nil {
						t.Fatal(err)
					}
					if err = db.CreateMod(ctx, &item); err != nil {
						t.Fatal(err)
					}
				}
			}
			switch mode {
			case "foreign-root":
				target.Spec.ModIDs = append(target.Spec.ModIDs, "foreign-Root")
			case "legacy-root":
				target.Spec.ModIDs = append(target.Spec.ModIDs, "-Root")
			case "wrong-provider-root":
				wrong := domain.ModFile{ID: "wrong", InstanceID: "unassigned", OrganizationID: "own", ProviderKey: domain.ProviderDST}
				if err := db.CreateMod(ctx, &wrong); err != nil {
					t.Fatal(err)
				}
				target.Spec.ModIDs = append(target.Spec.ModIDs, wrong.ID)
			}
			if err := db.CreateGameServer(ctx, &target); err != nil {
				t.Fatal(err)
			}
			if mode == "stale-target" {
				target.NodeID = "different-node"
			}
			planner := NewRuntimeModPlanner(root, db, registry)
			err = planner.PlanMods(ctx, target)
			if mode == "owned-chain" {
				if err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"Root", "Dependency", "Leaf"} {
					content, err := os.ReadFile(filepath.Join(target.Spec.Runtime.DataDir, "Mods", name+".tmod"))
					if err != nil || string(content) != "own:"+name {
						t.Fatalf("%s bytes: %q %v", name, content, err)
					}
				}
			} else {
				if err == nil {
					t.Fatal("invalid plan accepted")
				}
				fileName := "Root.tmod"
				if mode == "missing-private-dependency" {
					fileName = "Dependency.tmod"
				}
				if _, err := os.Stat(filepath.Join(root, "mods", target.ID, fileName)); !os.IsNotExist(err) {
					t.Fatalf("unexpected file mutation: %v", err)
				}
			}
		})
	}
}
