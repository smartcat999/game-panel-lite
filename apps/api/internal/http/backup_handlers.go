package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func (h *Handler) listBackups(w http.ResponseWriter, r *http.Request) {
	backups, err := h.store.ListBackups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	svc := backupsvc.NewService(h.cfg.DataDir)
	visible := make([]domain.Backup, 0, len(backups))
	for _, b := range backups {
		path, err := svc.Path(b.InstanceID, b.FileName)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			h.logger.Warn("backup file missing, pruning orphaned record", "backupId", b.ID, "path", path)
			_ = h.store.DeleteBackup(r.Context(), b.ID)
			continue
		}
		visible = append(visible, h.hydrateBackupResource(r.Context(), b))
	}
	writeJSON(w, http.StatusOK, visible)
}

func (h *Handler) createBackup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	unlock := h.lockServerMutation(id)
	defer unlock()
	server, err := h.store.GetGameServer(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if h.gameUpdateLocked(r.Context(), server.ID) {
		writeError(w, http.StatusConflict, "server maintenance is in progress")
		return
	}
	dataDir, err := serverDataDir(server)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	path, size, err := h.createInstanceBackupArchive(r.Context(), server, dataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	item := domain.Backup{ID: uuid.NewString(), InstanceID: server.ID, FileName: filepath.Base(path), WorldName: serverWorldName(server), SizeBytes: size, Type: "Manual", CreatedAt: time.Now()}
	if err := h.store.CreateBackup(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "backup.created", fmt.Sprintf("Created backup %s for %s", item.FileName, server.Name), activityBackupPayload(item, &server))
	writeJSON(w, http.StatusCreated, h.hydrateBackupResource(r.Context(), item))
}

func (h *Handler) hydrateBackupResource(ctx context.Context, backup domain.Backup) domain.Backup {
	if backup.InstanceID == "" {
		return backup
	}
	server, err := h.store.GetGameServer(ctx, backup.InstanceID)
	if err != nil {
		return backup
	}
	backup.GameKey = server.GameKey
	backup.ProviderKey = server.ProviderKey
	return backup
}

func (h *Handler) downloadBackup(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetBackup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	path, err := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := os.Stat(path); err != nil {
		h.logger.Warn("backup file missing during download, pruning orphaned record", "backupId", item.ID, "path", path)
		_ = h.store.DeleteBackup(r.Context(), item.ID)
		writeError(w, http.StatusNotFound, "backup file not found on disk")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", item.FileName))
	http.ServeFile(w, r, path)
}

func (h *Handler) restoreBackup(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetBackup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	unlock := h.lockServerMutation(item.InstanceID)
	defer unlock()
	resource, err := h.store.GetGameServer(r.Context(), item.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if h.gameUpdateLocked(r.Context(), resource.ID) {
		writeError(w, http.StatusConflict, "server maintenance is in progress")
		return
	}
	if isGameServerLockedForMutation(resource) {
		writeError(w, http.StatusConflict, "stop the server before restoring a backup")
		return
	}
	missing, err := h.pruneMissingBackupSource(r.Context(), item)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if missing {
		writeError(w, http.StatusNotFound, "backup file not found on disk")
		return
	}
	dataDir, err := serverDataDir(resource)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := backupsvc.NewService(h.cfg.DataDir).Restore(item.InstanceID, item.FileName, dataDir); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.syncRestoredGameServerConfig(r.Context(), &resource); err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	h.recordActivity(r.Context(), resource.ID, "backup.restored", fmt.Sprintf("Restored backup %s for %s", item.FileName, resource.Name), activityBackupPayload(item, &resource))
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "backupId": item.ID})
}

func (h *Handler) saveDisplayName(providerKey domain.ProviderKey) string {
	gameProvider, ok := h.provider.Get(providerKey)
	if !ok {
		return "save"
	}
	if saveProvider, ok := gameProvider.(provider.SaveMetadataProvider); ok {
		return saveProvider.SaveDisplayName()
	}
	return "save"
}

func (h *Handler) listServerSaves(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	backups, err := h.store.ListBackupsByInstance(r.Context(), server.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	svc := backupsvc.NewService(h.cfg.DataDir)
	visible := make([]domain.Backup, 0, len(backups))
	for _, b := range backups {
		path, err := svc.Path(b.InstanceID, b.FileName)
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			_ = h.store.DeleteBackup(r.Context(), b.ID)
			continue
		}
		visible = append(visible, h.hydrateBackupResource(r.Context(), b))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"saveDisplayName": h.saveDisplayName(server.ProviderKey),
		"saves":           visible,
	})
}

func (h *Handler) createServerSaveSnapshot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	unlock := h.lockServerMutation(id)
	defer unlock()
	server, err := h.store.GetGameServer(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if h.gameUpdateLocked(r.Context(), server.ID) {
		writeError(w, http.StatusConflict, "server maintenance is in progress")
		return
	}
	dataDir, err := serverDataDir(server)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	path, size, err := h.createInstanceBackupArchive(r.Context(), server, dataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	item := domain.Backup{ID: uuid.NewString(), InstanceID: server.ID, FileName: filepath.Base(path), WorldName: serverWorldName(server), SizeBytes: size, Type: "Manual", CreatedAt: time.Now()}
	if err := h.store.CreateBackup(r.Context(), &item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	saveName := h.saveDisplayName(server.ProviderKey)
	h.recordActivity(r.Context(), server.ID, "save.snapshot.created", fmt.Sprintf("Created %s snapshot %s for %s", saveName, item.FileName, server.Name), activitySavePayload(item, server, saveName))
	writeJSON(w, http.StatusCreated, map[string]any{
		"saveDisplayName": saveName,
		"save":            h.hydrateBackupResource(r.Context(), item),
	})
}

func (h *Handler) downloadServerSave(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	saveID := chi.URLParam(r, "saveId")
	item, err := h.store.GetBackup(r.Context(), saveID)
	if err != nil {
		writeError(w, http.StatusNotFound, "save snapshot not found")
		return
	}
	if item.InstanceID != instanceID {
		writeError(w, http.StatusNotFound, "save snapshot not found")
		return
	}
	path, err := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := os.Stat(path); err != nil {
		_ = h.store.DeleteBackup(r.Context(), item.ID)
		writeError(w, http.StatusNotFound, "save snapshot file not found on disk")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", item.FileName))
	http.ServeFile(w, r, path)
}

func (h *Handler) restoreServerSave(w http.ResponseWriter, r *http.Request) {
	instanceID := chi.URLParam(r, "id")
	unlock := h.lockServerMutation(instanceID)
	defer unlock()
	saveID := chi.URLParam(r, "saveId")
	item, err := h.store.GetBackup(r.Context(), saveID)
	if err != nil {
		writeError(w, http.StatusNotFound, "save snapshot not found")
		return
	}
	if item.InstanceID != instanceID {
		writeError(w, http.StatusNotFound, "save snapshot not found")
		return
	}
	resource, err := h.store.GetGameServer(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if h.gameUpdateLocked(r.Context(), resource.ID) {
		writeError(w, http.StatusConflict, "server maintenance is in progress")
		return
	}
	if isGameServerLockedForMutation(resource) {
		writeError(w, http.StatusConflict, "stop the server before restoring a save snapshot")
		return
	}
	missing, err := h.pruneMissingBackupSource(r.Context(), item)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if missing {
		writeError(w, http.StatusNotFound, "save snapshot file not found on disk")
		return
	}
	dataDir, err := serverDataDir(resource)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := backupsvc.NewService(h.cfg.DataDir).Restore(item.InstanceID, item.FileName, dataDir); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.syncRestoredGameServerConfig(r.Context(), &resource); err != nil {
		writeError(w, statusCodeForRuntimeError(err), err.Error())
		return
	}
	saveName := h.saveDisplayName(resource.ProviderKey)
	h.recordActivity(r.Context(), resource.ID, "save.snapshot.restored", fmt.Sprintf("Restored %s snapshot %s for %s", saveName, item.FileName, resource.Name), activitySavePayload(item, resource, saveName))
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "saveId": item.ID})
}

func (h *Handler) syncRestoredGameServerConfig(ctx context.Context, server *domain.GameServer) error {
	if server.ProviderKey != domain.ProviderTerrariaVanilla && server.ProviderKey != domain.ProviderTerrariaTModLoader {
		return nil
	}
	dataDir, err := serverDataDir(*server)
	if err != nil {
		return err
	}
	configBytes, err := os.ReadFile(filepath.Join(dataDir, "serverconfig.txt"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	fallback, err := terraria.ConfigFromPayload(server.Spec.Config, terraria.NewVanillaProvider().DefaultConfig())
	if err != nil {
		return err
	}
	if server.ProviderKey == domain.ProviderTerrariaTModLoader {
		fallback, err = terraria.ConfigFromPayload(server.Spec.Config, terraria.NewTModLoaderProvider().DefaultConfig())
		if err != nil {
			return err
		}
	}
	nextConfig, err := terraria.ParseServerConfig(fallback, string(configBytes))
	if err != nil {
		return err
	}
	nextConfig = normalizeTerrariaRuntimeConfig(nextConfig)
	configPayload := terraria.PayloadFromConfig(nextConfig)
	server.Spec.Config = configPayload
	server.Spec.Network.Port = nextConfig.Port
	server.Spec.Generation++
	if server.Spec.Generation <= 0 {
		server.Spec.Generation = 1
	}
	server.Status.Phase = domain.PhasePending
	server.UpdatedAt = time.Now()
	return h.store.SaveGameServer(ctx, server)
}

func (h *Handler) upsertBackupRecord(ctx context.Context, instanceID string, fileName string, worldName string, size int64, backupType string) (domain.Backup, bool, error) {
	if existing, err := h.store.GetBackupByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.WorldName = worldName
		existing.SizeBytes = size
		existing.Type = backupType
		return existing, false, h.store.SaveBackup(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.Backup{}, false, err
	}
	item := domain.Backup{ID: uuid.NewString(), InstanceID: instanceID, FileName: fileName, WorldName: worldName, SizeBytes: size, Type: backupType, CreatedAt: time.Now()}
	return item, true, h.store.CreateBackup(ctx, &item)
}

func (h *Handler) pruneMissingBackupSource(ctx context.Context, item domain.Backup) (bool, error) {
	path, err := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			h.logger.Warn("backup file missing during mutation, pruning orphaned record", "backupId", item.ID, "path", path)
			return true, h.store.DeleteBackup(ctx, item.ID)
		}
		return false, err
	}
	return false, nil
}

func (h *Handler) deleteBackup(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.GetBackup(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	path, _ := backupsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err := removeStoredFile(path); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.DeleteBackup(r.Context(), item.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), item.InstanceID, "backup.deleted", fmt.Sprintf("Deleted backup %s", item.FileName), activityBackupPayload(item, nil))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) createInstanceBackupArchive(ctx context.Context, server domain.GameServer, dataDir string) (string, int64, error) {
	svc := backupsvc.NewService(h.cfg.DataDir)
	if h.provider != nil {
		if gameProvider, ok := h.provider.Get(server.ProviderKey); ok {
			if subtreeProvider, ok := gameProvider.(provider.BackupSubtreeProvider); ok {
				if subtree := subtreeProvider.BackupSubtree(server); strings.TrimSpace(subtree) != "" {
					return svc.CreateSubtree(server.ID, dataDir, subtree)
				}
			}
		}
	}
	return svc.Create(server.ID, dataDir)
}

const (
	backupSchedulerStartWait = 15 * time.Second
	backupSchedulerScanEvery = 1 * time.Minute
)

func (h *Handler) getServerBackupPolicy(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	policy := server.Spec.BackupPolicy
	if policy.IntervalHours <= 0 {
		policy.IntervalHours = 6
	}
	if policy.RetentionCount <= 0 {
		policy.RetentionCount = 7
	}
	writeJSON(w, http.StatusOK, policy)
}

type updateBackupPolicyPayload struct {
	Enabled        bool `json:"enabled"`
	IntervalHours  int  `json:"intervalHours"`
	RetentionCount int  `json:"retentionCount"`
}

func (h *Handler) updateServerBackupPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	unlock := h.lockServerMutation(id)
	defer unlock()

	server, err := h.store.GetGameServer(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	var payload updateBackupPolicyPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if payload.IntervalHours < 1 || payload.IntervalHours > 168 {
		writeError(w, http.StatusBadRequest, "intervalHours must be between 1 and 168")
		return
	}
	if payload.RetentionCount < 1 || payload.RetentionCount > 50 {
		writeError(w, http.StatusBadRequest, "retentionCount must be between 1 and 50")
		return
	}

	server.Spec.BackupPolicy.Enabled = payload.Enabled
	server.Spec.BackupPolicy.IntervalHours = payload.IntervalHours
	server.Spec.BackupPolicy.RetentionCount = payload.RetentionCount

	now := time.Now().UTC()
	if payload.Enabled {
		if server.Spec.BackupPolicy.LastRunAt != nil {
			next := server.Spec.BackupPolicy.LastRunAt.Add(time.Duration(payload.IntervalHours) * time.Hour)
			server.Spec.BackupPolicy.NextRunAt = &next
		} else {
			next := now.Add(time.Duration(payload.IntervalHours) * time.Hour)
			server.Spec.BackupPolicy.NextRunAt = &next
		}
	} else {
		server.Spec.BackupPolicy.NextRunAt = nil
	}
	server.UpdatedAt = now

	if err := h.store.SaveGameServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.recordActivity(r.Context(), server.ID, "backup.policy.updated", fmt.Sprintf("Updated backup policy for %s (enabled: %v, interval: %dh, retention: %d)", server.Name, payload.Enabled, payload.IntervalHours, payload.RetentionCount))
	writeJSON(w, http.StatusOK, server.Spec.BackupPolicy)
}

func (h *Handler) runAutomaticBackupScheduler(ctx context.Context) {
	timer := time.NewTimer(backupSchedulerStartWait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	ticker := time.NewTicker(backupSchedulerScanEvery)
	defer ticker.Stop()
	for {
		if err := h.scanAutomaticBackups(ctx, time.Now().UTC()); err != nil && h.logger != nil {
			h.logger.Warn("scan automatic backups", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *Handler) scanAutomaticBackups(ctx context.Context, now time.Time) error {
	servers, err := h.store.ListGameServers(ctx)
	if err != nil {
		return err
	}
	for _, server := range servers {
		if !server.Spec.BackupPolicy.Enabled {
			continue
		}
		if h.provider != nil {
			gameProvider, ok := h.provider.Get(server.ProviderKey)
			if ok && !gameProvider.Capabilities().Backups {
				continue
			}
		}
		// Auto backup only runs when the game server is actively running
		if server.Status.Phase != domain.PhaseRunning && server.Status.ActualState != domain.ActualRunning {
			continue
		}

		intervalHours := server.Spec.BackupPolicy.IntervalHours
		if intervalHours <= 0 {
			intervalHours = 6
		}
		retentionCount := server.Spec.BackupPolicy.RetentionCount
		if retentionCount <= 0 {
			retentionCount = 7
		}
		interval := time.Duration(intervalHours) * time.Hour

		isDue := false
		if server.Spec.BackupPolicy.LastRunAt == nil {
			if server.Spec.BackupPolicy.NextRunAt != nil {
				isDue = now.After(*server.Spec.BackupPolicy.NextRunAt) || now.Equal(*server.Spec.BackupPolicy.NextRunAt)
			} else if now.Sub(server.CreatedAt) >= 5*time.Minute {
				isDue = true
			}
		} else {
			isDue = now.Sub(*server.Spec.BackupPolicy.LastRunAt) >= interval
		}

		if !isDue {
			continue
		}

		if h.gameUpdateLocked(ctx, server.ID) {
			continue
		}

		if err := h.executeScheduledBackup(ctx, server, now, interval, retentionCount); err != nil {
			if h.logger != nil {
				h.logger.Error("failed to execute scheduled backup", "serverId", server.ID, "error", err)
			}
		}
	}
	return nil
}

func (h *Handler) executeScheduledBackup(ctx context.Context, server domain.GameServer, now time.Time, interval time.Duration, retentionCount int) error {
	unlock := h.lockServerMutation(server.ID)
	defer unlock()

	current, err := h.store.GetGameServer(ctx, server.ID)
	if err != nil {
		return err
	}
	if !current.Spec.BackupPolicy.Enabled {
		return nil
	}
	if current.Status.Phase != domain.PhaseRunning && current.Status.ActualState != domain.ActualRunning {
		return nil
	}

	dataDir, err := serverDataDir(current)
	if err != nil {
		return err
	}

	path, size, err := h.createInstanceBackupArchive(ctx, current, dataDir)
	if err != nil {
		return err
	}

	item := domain.Backup{
		ID:         uuid.NewString(),
		InstanceID: current.ID,
		FileName:   filepath.Base(path),
		WorldName:  serverWorldName(current),
		SizeBytes:  size,
		Type:       "Auto",
		CreatedAt:  now,
	}
	if err := h.store.CreateBackup(ctx, &item); err != nil {
		return err
	}

	next := now.Add(interval)
	current.Spec.BackupPolicy.LastRunAt = &now
	current.Spec.BackupPolicy.NextRunAt = &next
	current.UpdatedAt = now
	if err := h.store.SaveGameServer(ctx, &current); err != nil {
		return err
	}

	h.recordActivity(ctx, current.ID, "backup.scheduled.created", fmt.Sprintf("Created scheduled backup %s for %s", item.FileName, current.Name), activityBackupPayload(item, &current))

	h.pruneScheduledBackups(ctx, current.ID, retentionCount)
	return nil
}

func (h *Handler) pruneScheduledBackups(ctx context.Context, serverID string, retentionCount int) {
	if retentionCount <= 0 {
		retentionCount = 7
	}
	backups, err := h.store.ListBackupsByInstance(ctx, serverID)
	if err != nil {
		return
	}
	var scheduled []domain.Backup
	for _, b := range backups {
		if b.Type == "Auto" || b.Type == "Scheduled" {
			scheduled = append(scheduled, b)
		}
	}
	if len(scheduled) <= retentionCount {
		return
	}
	sort.Slice(scheduled, func(i, j int) bool {
		return scheduled[i].CreatedAt.After(scheduled[j].CreatedAt)
	})

	toDelete := scheduled[retentionCount:]
	svc := backupsvc.NewService(h.cfg.DataDir)
	for _, b := range toDelete {
		if path, err := svc.Path(b.InstanceID, b.FileName); err == nil {
			_ = os.Remove(path)
		}
		_ = h.store.DeleteBackup(ctx, b.ID)
		h.recordActivity(ctx, serverID, "backup.scheduled.pruned", fmt.Sprintf("Pruned scheduled backup %s (retention: %d)", b.FileName, retentionCount))
	}
}
