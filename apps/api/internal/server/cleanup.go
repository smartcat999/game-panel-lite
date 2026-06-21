package server

import (
	"context"
	"os"
	"path/filepath"

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

func cleanupOwnedResources(ctx context.Context, store any, server domain.GameServer) error {
	resourceStore, ok := store.(ownedResourceStore)
	if !ok {
		return nil
	}
	dataRoot := dataRootFromServerDataDir(server.Spec.Runtime.DataDir)

	worlds, err := resourceStore.ListWorlds(ctx)
	if err != nil {
		return err
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
			if err := resourceStore.DeleteWorld(ctx, item.ID); err != nil {
				return err
			}
			removeResourceFile(dataRoot, "worlds", server.ID, item.FileName)
		}
	}

	backups, err := resourceStore.ListBackups(ctx)
	if err != nil {
		return err
	}
	for _, item := range backups {
		if item.InstanceID != server.ID {
			continue
		}
		if err := resourceStore.DeleteBackup(ctx, item.ID); err != nil {
			return err
		}
		removeResourceFile(dataRoot, "backups", server.ID, item.FileName)
	}

	mods, err := resourceStore.ListMods(ctx, server.ID)
	if err != nil {
		return err
	}
	for _, item := range mods {
		if err := resourceStore.DeleteMod(ctx, item.ID); err != nil {
			return err
		}
		removeResourceFile(dataRoot, "mods", server.ID, item.FileName)
	}
	if err := resourceStore.DeleteServerShareByInstance(ctx, server.ID); err != nil {
		return err
	}
	if server.Spec.Runtime.DataDir != "" {
		_ = os.RemoveAll(server.Spec.Runtime.DataDir)
	}
	return nil
}

func dataRootFromServerDataDir(dataDir string) string {
	if dataDir == "" {
		return ""
	}
	return filepath.Dir(filepath.Dir(dataDir))
}

func removeResourceFile(dataRoot string, category string, serverID string, fileName string) {
	if dataRoot == "" || fileName == "" {
		return
	}
	_ = os.Remove(filepath.Join(dataRoot, category, serverID, fileName))
}
