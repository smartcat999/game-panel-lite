package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestCleanupRejectsUnsafeReferencesAndRetainsMetadata(t *testing.T) {
	for _, scenario := range []string{"foreign-owner", "traversal", "symlink", "outside-data", "file-error"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			db, err := store.Open(filepath.Join(t.TempDir(), "cleanup.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			instance := domain.GameServer{ID: "instance", Spec: domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(root, "instances", "instance")}}}
			if err := os.MkdirAll(instance.Spec.Runtime.DataDir, 0700); err != nil {
				t.Fatal(err)
			}
			world := domain.World{ID: "world", InstanceID: instance.ID, FileName: "world.wld"}
			worldRoot := filepath.Join(root, "worlds")
			if scenario == "symlink" {
				worldRoot = t.TempDir()
				if err := os.Symlink(worldRoot, filepath.Join(root, "worlds")); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Join(worldRoot, instance.ID), 0700); err != nil {
				t.Fatal(err)
			}
			worldPath := filepath.Join(worldRoot, instance.ID, world.FileName)
			if scenario == "file-error" {
				if err := os.MkdirAll(worldPath, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(worldPath, "keep"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(worldPath, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "foreign-owner":
				world.OrganizationID = "foreign-workspace"
			case "traversal":
				world.FileName = "../world.wld"
			case "outside-data":
				instance.Spec.Runtime.DataDir = t.TempDir()
			}
			if err := db.CreateWorld(ctx, &world); err != nil {
				t.Fatal(err)
			}
			if err := cleanupOwnedResources(ctx, db, instance, root); err == nil {
				t.Fatal("unsafe cleanup succeeded")
			}
			if _, err := db.GetWorld(ctx, world.ID); err != nil {
				t.Fatalf("failure lost retry metadata: %v", err)
			}
			if _, err := os.Stat(worldPath); err != nil {
				t.Fatalf("failure removed protected file: %v", err)
			}
			if scenario == "file-error" {
				if err := os.RemoveAll(worldPath); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(worldPath, []byte("retry"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := cleanupOwnedResources(ctx, db, instance, root); err != nil {
					t.Fatalf("retry: %v", err)
				}
				if _, err := db.GetWorld(ctx, world.ID); !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("retry retained metadata: %v", err)
				}
			}
		})
	}
}

type failingWorldDeleteStore struct {
	*store.Store
	fail bool
}

func (s *failingWorldDeleteStore) DeleteWorld(ctx context.Context, id string) error {
	if s.fail {
		return errors.New("metadata unavailable")
	}
	return s.Store.DeleteWorld(ctx, id)
}

func TestCleanupRetriesAfterFileRemovedButMetadataDeleteFailed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "cleanup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	instance := domain.GameServer{ID: "retry", Spec: domain.ServerSpec{DesiredState: domain.DesiredDeleted, Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(root, "instances", "retry")}}}
	world := domain.World{ID: "world", InstanceID: instance.ID, FileName: "world.wld"}
	worldPath := filepath.Join(root, "worlds", instance.ID, world.FileName)
	if err := os.MkdirAll(filepath.Dir(worldPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(worldPath, []byte("world"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateWorld(ctx, &world); err != nil {
		t.Fatal(err)
	}

	if err := db.CreateGameServer(ctx, &instance); err != nil {
		t.Fatal(err)
	}
	failing := &failingWorldDeleteStore{Store: db, fail: true}
	controller := NewController(failing, NewRuntimeReconciler(&fakeBuilder{}, &fakeRuntime{}), nil).WithDataRoot(root)
	controller.RunOnce(ctx)
	if _, err := db.GetGameServer(ctx, instance.ID); err != nil {
		t.Fatalf("failed cleanup deleted instance: %v", err)
	}
	if _, err := os.Stat(worldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected removed file: %v", err)
	}
	if _, err := db.GetWorld(ctx, world.ID); err != nil {
		t.Fatalf("lost retry metadata: %v", err)
	}
	failing.fail = false
	controller.RunOnce(ctx)
	if _, err := db.GetGameServer(ctx, instance.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("retry did not finish instance deletion: %v", err)
	}
	if _, err := db.GetWorld(ctx, world.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("metadata retained: %v", err)
	}
}
