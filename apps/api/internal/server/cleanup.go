package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type CleanupPolicy string

const (
	CleanupKeepData   CleanupPolicy = "keepData"
	CleanupRemoveData CleanupPolicy = "removeData"
)

type ownedResourceStore interface {
	ListWorlds(context.Context) ([]domain.World, error)
	SaveWorld(context.Context, *domain.World) error
	DeleteWorld(context.Context, string) error
	ListBackups(context.Context) ([]domain.Backup, error)
	DeleteBackup(context.Context, string) error
	ListMods(context.Context, string) ([]domain.ModFile, error)
	DeleteMod(context.Context, string) error
	DeleteServerShareByInstance(context.Context, string) error
}

func cleanupOwnedResources(ctx context.Context, store any, server domain.GameServer, dataRoot string) error {
	resourceStore, ok := store.(ownedResourceStore)
	if !ok {
		return nil
	}
	if dataRoot == "" || !cleanupPathComponent(server.ID) {
		return fmt.Errorf("invalid cleanup root or instance ID")
	}
	expected, err := filepath.Abs(filepath.Join(dataRoot, "instances", server.ID))
	if err != nil {
		return err
	}
	actual, err := filepath.Abs(server.Spec.Runtime.DataDir)
	if err != nil {
		return err
	}
	if server.Spec.Runtime.DataDir == "" || actual != expected {
		return fmt.Errorf("instance data directory does not match configured cleanup root")
	}
	root, err := os.OpenRoot(dataRoot)
	if err != nil {
		return err
	}
	defer root.Close()

	worlds, err := resourceStore.ListWorlds(ctx)
	if err != nil {
		return err
	}
	backups, err := resourceStore.ListBackups(ctx)
	if err != nil {
		return err
	}
	mods, err := resourceStore.ListMods(ctx, server.ID)
	if err != nil {
		return err
	}
	// Validate every reference before removing any resource.
	for _, item := range worlds {
		if item.InstanceID != server.ID {
			continue
		}
		if item.OrganizationID != server.OrganizationID {
			return fmt.Errorf("world ownership does not match instance")
		}
		if item.Source != "server_snapshot" && !cleanupPathComponent(item.FileName) {
			return fmt.Errorf("invalid world cleanup filename")
		}
	}
	for _, item := range backups {
		if item.InstanceID == server.ID && !cleanupPathComponent(item.FileName) {
			return fmt.Errorf("invalid backup cleanup filename")
		}
	}
	for _, item := range mods {
		if item.InstanceID != server.ID || !cleanupPathComponent(item.FileName) {
			return fmt.Errorf("invalid mod cleanup reference")
		}
	}
	for _, item := range worlds {
		if item.InstanceID != server.ID {
			continue
		}
		if item.Source == "server_snapshot" && item.ActiveInstanceID == server.ID {
			item.ActiveInstanceID = ""
			if err := resourceStore.SaveWorld(ctx, &item); err != nil {
				return err
			}
		}
		if item.Source != "server_snapshot" {
			if err := removeResourceFile(root, "worlds", server.ID, item.FileName); err != nil {
				return err
			}
			if err := resourceStore.DeleteWorld(ctx, item.ID); err != nil {
				return err
			}
		}
	}
	for _, item := range backups {
		if item.InstanceID != server.ID {
			continue
		}
		if err := removeResourceFile(root, "backups", server.ID, item.FileName); err != nil {
			return err
		}
		if err := resourceStore.DeleteBackup(ctx, item.ID); err != nil {
			return err
		}
	}
	for _, item := range mods {
		if err := removeResourceFile(root, "mods", server.ID, item.FileName); err != nil {
			return err
		}
		if err := resourceStore.DeleteMod(ctx, item.ID); err != nil {
			return err
		}
	}
	if err := resourceStore.DeleteServerShareByInstance(ctx, server.ID); err != nil {
		return err
	}
	return root.RemoveAll(filepath.Join("instances", server.ID))
}

func cleanupPathComponent(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\")
}

func removeResourceFile(root *os.Root, category, serverID, fileName string) error {
	err := root.Remove(filepath.Join(category, serverID, fileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
