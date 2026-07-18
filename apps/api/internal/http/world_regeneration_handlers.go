package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	serverctrl "github.com/smartcat999/game-panel-lite/apps/api/internal/server"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/worldregen"
)

const worldRegenerationTimeout = 12 * time.Minute

type worldRegenerationView struct {
	Supported bool                         `json:"supported"`
	Job       *domain.WorldRegenerationJob `json:"job,omitempty"`
}

func (h *Handler) getWorldRegeneration(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		writeJSON(w, http.StatusOK, worldRegenerationView{})
		return
	}
	_, supported := gameProvider.(provider.WorldRegenerationProvider)
	view := worldRegenerationView{Supported: supported && gameProvider.Capabilities().WorldRegeneration}
	if !view.Supported {
		writeJSON(w, http.StatusOK, view)
		return
	}
	job, err := h.store.GetLatestWorldRegenerationJobByInstance(r.Context(), server.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, view)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view.Job = &job
	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) regenerateWorld(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	unlock := h.lockServerMutation(id)
	defer unlock()
	h.gameUpdateJobsMu.Lock()
	defer h.gameUpdateJobsMu.Unlock()

	server, err := h.store.GetGameServer(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		writeError(w, http.StatusBadRequest, "provider is unavailable")
		return
	}
	regenerationProvider, ok := gameProvider.(provider.WorldRegenerationProvider)
	if !ok || !gameProvider.Capabilities().WorldRegeneration {
		writeError(w, http.StatusBadRequest, "world regeneration is not supported for this provider")
		return
	}
	if _, err := regenerationProvider.WorldRegenerationPlan(server); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if server.Status.PlayersOnline > 0 {
		writeError(w, http.StatusConflict, "players are online; wait until the server is empty before regenerating the world")
		return
	}
	if h.maintenanceRuntimeLocked(r.Context()) || h.runtimeImagePrepareActive() {
		writeError(w, http.StatusConflict, "another maintenance task is already running")
		return
	}
	wasRunning := server.Spec.DesiredState == domain.DesiredRunning || server.Status.ActualState == domain.ActualRunning
	startAfter := wasRunning
	var payload struct {
		StartAfter *bool `json:"startAfter,omitempty"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if payload.StartAfter != nil {
		startAfter = *payload.StartAfter
	}
	now := time.Now().UTC()
	job := domain.WorldRegenerationJob{
		ID: uuid.NewString(), InstanceID: server.ID, ProviderKey: server.ProviderKey,
		Status: domain.WorldRegenerationJobQueued, Stage: domain.WorldRegenerationStageQueued,
		StartAfter: startAfter, WasRunning: wasRunning, CreatedAt: now, UpdatedAt: now,
	}
	if err := h.store.CreateWorldRegenerationJob(r.Context(), &job); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.world-regeneration.queued", "Queued world regeneration for "+server.Name, map[string]any{"jobId": job.ID})
	if !h.startGameUpdateWorker(func() { h.runWorldRegeneration(h.backgroundContext(), server, job) }) {
		h.failWorldRegenerationJob(&job, errors.New("server is shutting down"))
		writeError(w, http.StatusServiceUnavailable, "server is shutting down")
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (h *Handler) runWorldRegeneration(parent context.Context, server domain.GameServer, job domain.WorldRegenerationJob) {
	ctx, cancel := context.WithTimeout(parent, worldRegenerationTimeout)
	defer cancel()
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		h.failWorldRegenerationJob(&job, errors.New("provider is unavailable"))
		return
	}
	regenerationProvider, ok := gameProvider.(provider.WorldRegenerationProvider)
	if !ok {
		h.failWorldRegenerationJob(&job, errors.New("world regeneration is not supported"))
		return
	}
	plan, err := regenerationProvider.WorldRegenerationPlan(server)
	if err != nil {
		h.failWorldRegenerationJob(&job, err)
		return
	}
	dataDir, err := serverDataDir(server)
	if err != nil {
		h.failWorldRegenerationJob(&job, err)
		return
	}

	h.updateWorldRegenerationJob(&job, domain.WorldRegenerationJobRunning, domain.WorldRegenerationStageStopping, 8, "")
	if _, err := serverctrl.NewService(h.store).RequestStop(ctx, server.ID); err != nil {
		h.failWorldRegenerationJob(&job, err)
		return
	}
	if err := h.waitForServerState(ctx, server.ID, domain.ActualStopped, 2*time.Minute); err != nil {
		h.failWorldRegenerationAndRestore(server, &job, nil, err)
		return
	}

	h.updateWorldRegenerationJob(&job, domain.WorldRegenerationJobRunning, domain.WorldRegenerationStageBackingUp, 24, "")
	path, size, err := backupsvc.NewService(h.cfg.DataDir).Create(server.ID, dataDir)
	if err != nil {
		h.failWorldRegenerationAndRestore(server, &job, nil, fmt.Errorf("create pre-regeneration backup: %w", err))
		return
	}
	backup := domain.Backup{ID: uuid.NewString(), InstanceID: server.ID, FileName: filepath.Base(path), WorldName: serverWorldName(server), SizeBytes: size, Type: "Pre-regeneration", CreatedAt: time.Now().UTC()}
	if err := h.store.CreateBackup(ctx, &backup); err != nil {
		_ = os.Remove(path)
		h.failWorldRegenerationAndRestore(server, &job, nil, err)
		return
	}
	job.BackupID = backup.ID
	h.updateWorldRegenerationJob(&job, domain.WorldRegenerationJobRunning, domain.WorldRegenerationStageResetting, 48, "")

	worldService := worldregen.NewService()
	moves, err := worldService.Quarantine(dataDir, plan.SavePaths, job.ID)
	if err != nil {
		h.failWorldRegenerationAndRestore(server, &job, nil, err)
		return
	}
	if job.StartAfter {
		h.updateWorldRegenerationJob(&job, domain.WorldRegenerationJobRunning, domain.WorldRegenerationStageStarting, 68, "")
		if _, err := serverctrl.NewService(h.store).RequestStart(ctx, server.ID); err != nil {
			h.failWorldRegenerationAndRestore(server, &job, moves, err)
			return
		}
		h.updateWorldRegenerationJob(&job, domain.WorldRegenerationJobRunning, domain.WorldRegenerationStageHealthCheck, 82, "")
		if err := h.waitForGameServerHealthy(ctx, server.ID, 7*time.Minute); err != nil {
			h.failWorldRegenerationAndRestore(server, &job, moves, err)
			return
		}
	}
	if err := worldService.Commit(moves); err != nil {
		if h.logger != nil {
			h.logger.Warn("cleanup previous world quarantine", "jobId", job.ID, "instanceId", server.ID, "error", err)
		}
		h.recordActivity(context.Background(), server.ID, "server.world-regeneration.cleanup-warning", err.Error(), map[string]any{"jobId": job.ID, "backupId": job.BackupID})
	}
	h.completeWorldRegenerationJob(&job)
}

func (h *Handler) failWorldRegenerationAndRestore(server domain.GameServer, job *domain.WorldRegenerationJob, moves []worldregen.Move, regenerationErr error) {
	recoveryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	previousStage := job.Stage
	h.updateWorldRegenerationJob(job, domain.WorldRegenerationJobRunning, domain.WorldRegenerationStageRollingBack, maxInt(job.Progress, 86), "")
	if previousStage == domain.WorldRegenerationStageStarting || previousStage == domain.WorldRegenerationStageHealthCheck || len(moves) > 0 {
		_, _ = serverctrl.NewService(h.store).RequestStop(recoveryCtx, server.ID)
		_ = h.waitForServerState(recoveryCtx, server.ID, domain.ActualStopped, 2*time.Minute)
	}
	if err := worldregen.NewService().Restore(moves); err != nil {
		regenerationErr = fmt.Errorf("%w; rollback failed: %v", regenerationErr, err)
	}
	if job.WasRunning {
		if _, err := serverctrl.NewService(h.store).RequestStart(recoveryCtx, server.ID); err != nil {
			regenerationErr = fmt.Errorf("%w; failed to restart previous world: %v", regenerationErr, err)
		}
	}
	h.failWorldRegenerationJob(job, regenerationErr)
}

func (h *Handler) updateWorldRegenerationJob(job *domain.WorldRegenerationJob, status domain.WorldRegenerationJobStatus, stage domain.WorldRegenerationJobStage, progress int, errorText string) error {
	if progress < job.Progress && (status == domain.WorldRegenerationJobQueued || status == domain.WorldRegenerationJobRunning) {
		progress = job.Progress
	}
	next := *job
	next.Status, next.Stage, next.Progress, next.Error, next.UpdatedAt = status, stage, progress, errorText, time.Now().UTC()
	if err := h.store.SaveWorldRegenerationJob(context.Background(), &next); err != nil {
		if h.logger != nil {
			h.logger.Error("save world regeneration job", "jobId", job.ID, "error", err)
		}
		return err
	}
	*job = next
	return nil
}

func (h *Handler) failWorldRegenerationJob(job *domain.WorldRegenerationJob, err error) {
	now := time.Now().UTC()
	job.CompletedAt = &now
	h.updateWorldRegenerationJob(job, domain.WorldRegenerationJobFailed, job.Stage, job.Progress, err.Error())
	h.recordActivity(context.Background(), job.InstanceID, "server.world-regeneration.failed", err.Error(), map[string]any{"jobId": job.ID, "backupId": job.BackupID})
}

func (h *Handler) completeWorldRegenerationJob(job *domain.WorldRegenerationJob) {
	now := time.Now().UTC()
	job.CompletedAt = &now
	h.updateWorldRegenerationJob(job, domain.WorldRegenerationJobSucceeded, domain.WorldRegenerationStageCompleted, 100, "")
	h.recordActivity(context.Background(), job.InstanceID, "server.world-regeneration.succeeded", "World regeneration completed", map[string]any{"jobId": job.ID, "backupId": job.BackupID})
}

func (h *Handler) recoverInterruptedWorldRegenerations(ctx context.Context, startedAt time.Time) {
	jobs, err := h.store.ListActiveWorldRegenerationJobs(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("list interrupted world regeneration jobs", "error", err)
		}
		return
	}
	for index := range jobs {
		job := &jobs[index]
		if !job.CreatedAt.Before(startedAt) {
			continue
		}
		server, err := h.store.GetGameServer(ctx, job.InstanceID)
		if err != nil {
			h.failWorldRegenerationJob(job, errors.New("server missing after interrupted world regeneration"))
			continue
		}
		gameProvider, ok := h.provider.Get(server.ProviderKey)
		regenerationProvider, supported := gameProvider.(provider.WorldRegenerationProvider)
		if !ok || !supported {
			h.failWorldRegenerationJob(job, errors.New("provider unavailable after interrupted world regeneration"))
			continue
		}
		plan, err := regenerationProvider.WorldRegenerationPlan(server)
		if err != nil {
			h.failWorldRegenerationJob(job, err)
			continue
		}
		dataDir, err := serverDataDir(server)
		if err != nil {
			h.failWorldRegenerationJob(job, err)
			continue
		}
		moves, err := worldregen.NewService().FindQuarantine(dataDir, plan.SavePaths, job.ID)
		if err != nil {
			h.failWorldRegenerationJob(job, err)
			continue
		}
		h.failWorldRegenerationAndRestore(server, job, moves, errors.New("world regeneration was interrupted; restored the previous world"))
	}
}
