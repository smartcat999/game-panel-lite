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
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	serverctrl "github.com/smartcat999/game-panel-lite/apps/api/internal/server"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

const (
	palworldSteamAppID           = "2394010"
	gameUpdateCheckTimeout       = 5 * time.Minute
	gameUpdateApplyTimeout       = 90 * time.Minute
	gameUpdateRecoveryRetry      = 10 * time.Second
	gameUpdatePersistenceRetry   = 100 * time.Millisecond
	gameUpdateHealthStableWindow = 30 * time.Second
	minGameUpdateFreeDiskBytes   = int64(8 * 1024 * 1024 * 1024)
	minGameUpdateFreeMemoryMB    = int64(2560)
)

type gameUpdateView struct {
	Supported        bool                  `json:"supported"`
	Status           string                `json:"status"`
	InstalledBuildID string                `json:"installedBuildId,omitempty"`
	LatestBuildID    string                `json:"latestBuildId,omitempty"`
	CheckedAt        *time.Time            `json:"checkedAt,omitempty"`
	Job              *domain.GameUpdateJob `json:"job,omitempty"`
}

func (h *Handler) getGameUpdate(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	view := gameUpdateView{Supported: server.ProviderKey == domain.ProviderPalworld, Status: "unknown"}
	if !view.Supported {
		writeJSON(w, http.StatusOK, view)
		return
	}
	job, err := h.store.GetLatestGameUpdateJobByInstance(r.Context(), server.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, view)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view.Job, view.InstalledBuildID, view.LatestBuildID = &job, job.InstalledBuildID, job.LatestBuildID
	view.CheckedAt = job.CheckedAt
	switch job.Status {
	case domain.GameUpdateJobQueued, domain.GameUpdateJobRunning:
		if job.Operation == domain.GameUpdateOperationCheck || (job.Operation == "" && job.Stage == domain.GameUpdateStageRefreshingMetadata) {
			view.Status = "checking"
		} else {
			view.Status = "updating"
		}
	case domain.GameUpdateJobFailed:
		view.Status = "failed"
	default:
		if job.LatestBuildID != "" && job.InstalledBuildID != job.LatestBuildID {
			view.Status = "available"
		} else {
			view.Status = "up_to_date"
		}
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) checkGameUpdate(w http.ResponseWriter, r *http.Request) {
	unlock := h.lockServerMutation(chi.URLParam(r, "id"))
	defer unlock()
	server, ok := h.gameUpdateServer(w, r)
	if !ok {
		return
	}
	job, ok := h.newGameUpdateJob(w, r, server, domain.GameUpdateOperationCheck, false, false, domain.GameUpdateStageRefreshingMetadata)
	if !ok {
		return
	}
	h.startGameUpdateWorker(func() { h.runGameUpdateCheck(h.backgroundContext(), server, job) })
	writeJSON(w, http.StatusAccepted, job)
}

func (h *Handler) applyGameUpdate(w http.ResponseWriter, r *http.Request) {
	unlock := h.lockServerMutation(chi.URLParam(r, "id"))
	defer unlock()
	server, ok := h.gameUpdateServer(w, r)
	if !ok {
		return
	}
	var payload struct {
		StartAfterUpdate bool `json:"startAfterUpdate"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if server.Status.PlayersOnline > 0 {
		writeError(w, http.StatusConflict, "players are online; stop play before updating")
		return
	}
	wasRunning := server.Spec.DesiredState == domain.DesiredRunning || server.Status.ActualState == domain.ActualRunning
	job, ok := h.newGameUpdateJob(w, r, server, domain.GameUpdateOperationApply, payload.StartAfterUpdate, wasRunning, domain.GameUpdateStageQueued)
	if !ok {
		return
	}
	h.startGameUpdateWorker(func() { h.runGameUpdateApply(h.backgroundContext(), server, job) })
	writeJSON(w, http.StatusAccepted, job)
}

func (h *Handler) gameUpdateServer(w http.ResponseWriter, r *http.Request) (domain.GameServer, bool) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return server, false
	}
	if server.ProviderKey != domain.ProviderPalworld {
		writeError(w, http.StatusBadRequest, "game updates are not supported for this provider")
		return server, false
	}
	if err := h.requireRuntimeAvailable(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return server, false
	}
	if !h.runtime.SupportsGameUpdates() {
		writeError(w, http.StatusServiceUnavailable, "runtime adapter does not support safe game updates")
		return server, false
	}
	return server, true
}

func (h *Handler) newGameUpdateJob(w http.ResponseWriter, r *http.Request, server domain.GameServer, operation domain.GameUpdateOperation, startAfter, wasRunning bool, initialStage domain.GameUpdateJobStage) (domain.GameUpdateJob, bool) {
	h.gameUpdateJobsMu.Lock()
	defer h.gameUpdateJobsMu.Unlock()

	activeJobs, err := h.store.ListActiveGameUpdateJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return domain.GameUpdateJob{}, false
	}
	if len(activeJobs) > 0 {
		message := "another game update task is already running"
		if activeJobs[0].InstanceID == server.ID {
			message = "a game update task is already running for this server"
		}
		writeError(w, http.StatusConflict, message)
		return domain.GameUpdateJob{}, false
	}
	worldJobs, err := h.store.ListActiveWorldRegenerationJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return domain.GameUpdateJob{}, false
	}
	if len(worldJobs) > 0 {
		writeError(w, http.StatusConflict, "a world regeneration task is already running")
		return domain.GameUpdateJob{}, false
	}
	if h.runtimeImagePrepareActive() {
		writeError(w, http.StatusConflict, "a runtime image task is already running")
		return domain.GameUpdateJob{}, false
	}
	now := time.Now().UTC()
	job := domain.GameUpdateJob{ID: uuid.NewString(), InstanceID: server.ID, ProviderKey: server.ProviderKey, Operation: operation, Status: domain.GameUpdateJobQueued, Stage: initialStage, Progress: 0, StartAfterUpdate: startAfter, WasRunning: wasRunning, CreatedAt: now, UpdatedAt: now}
	if previous, err := h.store.GetLatestGameUpdateJobByInstance(r.Context(), server.ID); err == nil {
		job.InstalledBuildID = previous.InstalledBuildID
		job.LatestBuildID = previous.LatestBuildID
		job.CheckedAt = previous.CheckedAt
	} else if !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return domain.GameUpdateJob{}, false
	}
	if err := h.store.CreateGameUpdateJob(r.Context(), &job); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return job, false
	}
	h.recordActivity(r.Context(), server.ID, "server.game-update.queued", "Queued game update task for "+server.Name, map[string]any{"jobId": job.ID})
	return job, true
}

func (h *Handler) runGameUpdateCheck(ctx context.Context, server domain.GameServer, job domain.GameUpdateJob) {
	ctx, cancel := context.WithTimeout(ctx, gameUpdateCheckTimeout)
	defer cancel()
	h.updateGameJob(&job, domain.GameUpdateJobRunning, domain.GameUpdateStageRefreshingMetadata, 10, "")
	request, err := h.gameUpdateRequest(server, job.ID)
	if err != nil {
		h.failGameJob(&job, err)
		return
	}
	result, err := h.runtime.CheckGameUpdate(ctx, request)
	if err != nil {
		h.failGameUpdateTask(ctx, server, &job, err, false)
		return
	}
	job.InstalledBuildID, job.LatestBuildID = result.InstalledBuildID, result.LatestBuildID
	checkedAt := time.Now().UTC()
	job.CheckedAt = &checkedAt
	h.completeGameJob(&job)
}

func (h *Handler) runGameUpdateApply(ctx context.Context, server domain.GameServer, job domain.GameUpdateJob) {
	ctx, cancel := context.WithTimeout(ctx, gameUpdateApplyTimeout)
	defer cancel()
	h.updateGameJob(&job, domain.GameUpdateJobRunning, domain.GameUpdateStagePreflight, 2, "")
	dataDir, err := serverDataDir(server)
	if err != nil {
		h.failGameJob(&job, err)
		return
	}
	availableDisk, err := availableDiskBytes(dataDir)
	if err != nil {
		h.failGameJob(&job, fmt.Errorf("inspect available disk space: %w", err))
		return
	}
	if availableDisk < minGameUpdateFreeDiskBytes {
		h.failGameJob(&job, fmt.Errorf("at least 8 GiB of free disk space is required for a safe game update"))
		return
	}
	h.updateGameJob(&job, domain.GameUpdateJobRunning, domain.GameUpdateStageStopping, 5, "")
	if _, err := serverctrl.NewService(h.store).RequestStop(ctx, server.ID); err != nil {
		h.failGameJob(&job, err)
		return
	}
	if err := h.waitForServerState(ctx, server.ID, domain.ActualStopped, 2*time.Minute); err != nil {
		h.failGameUpdateApply(ctx, server, &job, err)
		return
	}
	refreshedServer, err := h.store.GetGameServer(ctx, server.ID)
	if err != nil {
		h.failGameUpdateApply(ctx, server, &job, err)
		return
	}
	server = refreshedServer
	hostStats, err := h.runtime.HostStats(ctx)
	if err != nil {
		h.failGameUpdateApply(ctx, server, &job, fmt.Errorf("inspect runtime memory before update: %w", err))
		return
	}
	if hostStats.MemoryLimitMB > 0 && hostStats.MemoryLimitMB-hostStats.TotalMemoryMB < minGameUpdateFreeMemoryMB {
		h.failGameUpdateApply(ctx, server, &job, fmt.Errorf("at least 2560 MB of runtime memory headroom is required for a safe game update"))
		return
	}
	h.updateGameJob(&job, domain.GameUpdateJobRunning, domain.GameUpdateStageBackingUp, 10, "")
	savedDir := filepath.Join(dataDir, "Pal", "Saved")
	if err := os.MkdirAll(savedDir, 0o755); err != nil {
		h.failGameUpdateApply(ctx, server, &job, fmt.Errorf("prepare Palworld save directory: %w", err))
		return
	}
	path, size, err := backupsvc.NewService(h.cfg.DataDir).CreateSubtree(server.ID, dataDir, filepath.Join("Pal", "Saved"))
	if err != nil {
		h.failGameUpdateApply(ctx, server, &job, err)
		return
	}
	backup := domain.Backup{ID: uuid.NewString(), InstanceID: server.ID, FileName: filepath.Base(path), WorldName: serverWorldName(server), SizeBytes: size, Type: "Pre-update", CreatedAt: time.Now().UTC()}
	if err := h.store.CreateBackup(ctx, &backup); err != nil {
		_ = os.Remove(path)
		h.failGameUpdateApply(ctx, server, &job, err)
		return
	}
	availableDisk, err = availableDiskBytes(dataDir)
	if err != nil {
		h.failGameUpdateApply(ctx, server, &job, fmt.Errorf("recheck available disk space after backup: %w", err))
		return
	}
	if availableDisk < minGameUpdateFreeDiskBytes {
		h.failGameUpdateApply(ctx, server, &job, fmt.Errorf("at least 8 GiB of free disk space is required after the save backup"))
		return
	}
	request, err := h.gameUpdateRequest(server, job.ID)
	if err != nil {
		h.failGameUpdateApply(ctx, server, &job, err)
		return
	}
	if err := h.updateGameJob(&job, domain.GameUpdateJobRunning, domain.GameUpdateStageRefreshingMetadata, 12, ""); err != nil {
		h.failGameUpdateApply(ctx, server, &job, fmt.Errorf("persist update checkpoint before starting SteamCMD: %w", err))
		return
	}
	result, err := h.runtime.ApplyGameUpdate(ctx, request, func(progress runtime.GameUpdateProgress) {
		stage := mapRuntimeUpdateStage(progress.Stage)
		h.updateGameJob(&job, domain.GameUpdateJobRunning, stage, maxInt(12, progress.Progress), "")
	})
	if err != nil {
		h.failGameUpdateApply(ctx, server, &job, err)
		return
	}
	job.InstalledBuildID, job.LatestBuildID = result.InstalledBuildID, result.LatestBuildID
	checkedAt := time.Now().UTC()
	job.CheckedAt = &checkedAt
	if job.StartAfterUpdate {
		if err := h.updateGameJob(&job, domain.GameUpdateJobRunning, domain.GameUpdateStageStarting, 94, ""); err != nil {
			h.failGameUpdateTask(ctx, server, &job, fmt.Errorf("persist restart checkpoint after game update: %w", err), false)
			return
		}
		if _, err := serverctrl.NewService(h.store).RequestStart(ctx, server.ID); err != nil {
			h.failGameUpdateApply(ctx, server, &job, err)
			return
		}
		h.updateGameJob(&job, domain.GameUpdateJobRunning, domain.GameUpdateStageHealthCheck, 97, "")
		if err := h.waitForGameServerHealthy(ctx, server.ID, 5*time.Minute); err != nil {
			h.failGameUpdateApply(ctx, server, &job, err)
			return
		}
	}
	h.completeGameJob(&job)
}

func (h *Handler) gameUpdateRequest(server domain.GameServer, jobID string) (runtime.GameUpdateRequest, error) {
	dataDir, err := serverDataDir(server)
	if err != nil {
		return runtime.GameUpdateRequest{}, err
	}
	image := server.Spec.Runtime.Image
	if image == "" {
		gameProvider, ok := h.provider.Get(server.ProviderKey)
		if !ok {
			return runtime.GameUpdateRequest{}, fmt.Errorf("provider %s is unavailable", server.ProviderKey)
		}
		image = gameProvider.ImageFor(server.Spec.Version)
	}
	runtimeID := server.Status.RuntimeID
	if runtimeID == "" {
		runtimeID = "gamepanel-" + server.ID
	}
	return runtime.GameUpdateRequest{JobID: jobID, RuntimeID: runtimeID, Image: image, DataDir: dataDir, AppID: palworldSteamAppID}, nil
}
func (h *Handler) updateGameJob(job *domain.GameUpdateJob, status domain.GameUpdateJobStatus, stage domain.GameUpdateJobStage, progress int, errorText string) error {
	if progress < job.Progress && (status == domain.GameUpdateJobQueued || status == domain.GameUpdateJobRunning) {
		progress = job.Progress
	}
	next := *job
	next.Status, next.Stage, next.Progress, next.Error, next.UpdatedAt = status, stage, progress, errorText, time.Now().UTC()
	if err := h.store.SaveGameUpdateJob(context.Background(), &next); err != nil {
		if h.logger != nil {
			h.logger.Error("save game update job", "jobId", job.ID, "error", err)
		}
		return err
	}
	*job = next
	return nil
}

func (h *Handler) failGameUpdateApply(ctx context.Context, server domain.GameServer, job *domain.GameUpdateJob, updateErr error) {
	// Before SteamCMD starts, restoring a previously running instance is safe.
	// Once installation has begun, keep the server stopped on failure so it
	// cannot boot from a partially updated runtime tree; the next update retry
	// performs a controlled validate/repair first.
	restoreRunning := job.WasRunning && !interruptedApplyRequiresValidation(job.Stage)
	h.failGameUpdateTask(ctx, server, job, updateErr, restoreRunning)
}

func (h *Handler) failGameUpdateTask(ctx context.Context, server domain.GameServer, job *domain.GameUpdateJob, updateErr error, restoreRunning bool) {
	if errors.Is(updateErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		h.stopCanceledPostUpdateServer(server, job)
		h.logCanceledGameUpdate(job)
		return
	}
	if gameUpdateFailureMayHaveStartedServer(job.Stage) {
		if err := h.stopGameServerAfterFailedUpdate(server.ID); err != nil {
			updateErr = fmt.Errorf("%w; failed to stop server after update failure: %v", updateErr, err)
		}
		restoreRunning = false
	}
	if err := h.cleanupGameUpdater(context.Background(), job.ID); err != nil {
		jobCopy := *job
		if !h.startGameUpdateWorker(func() { h.retryFailedGameUpdate(h.backgroundContext(), server, jobCopy, updateErr, restoreRunning) }) {
			h.logCanceledGameUpdate(job)
		}
		return
	}
	h.restoreAndFailGameUpdate(server, job, updateErr, restoreRunning)
}

func (h *Handler) stopCanceledPostUpdateServer(server domain.GameServer, job *domain.GameUpdateJob) {
	if !gameUpdateFailureMayHaveStartedServer(job.Stage) {
		return
	}
	serverID := server.ID
	if serverID == "" {
		serverID = job.InstanceID
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := serverctrl.NewService(h.store).RequestStop(stopCtx, serverID); err != nil {
		if h.logger != nil {
			h.logger.Error("persist stop after canceled game update", "jobId", job.ID, "instanceId", serverID, "error", err)
		}
		return
	}
	if server.Status.RuntimeID != "" {
		if err := h.runtime.StopWorkload(stopCtx, server.Status.RuntimeID); err != nil && h.logger != nil {
			h.logger.Error("stop workload after canceled game update", "jobId", job.ID, "instanceId", serverID, "runtimeId", server.Status.RuntimeID, "error", err)
		}
	}
}

func gameUpdateFailureMayHaveStartedServer(stage domain.GameUpdateJobStage) bool {
	return stage == domain.GameUpdateStageStarting || stage == domain.GameUpdateStageHealthCheck
}

func (h *Handler) stopGameServerAfterFailedUpdate(serverID string) error {
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := serverctrl.NewService(h.store).RequestStop(stopCtx, serverID); err != nil {
		return err
	}
	return h.waitForServerState(stopCtx, serverID, domain.ActualStopped, 2*time.Minute)
}

func (h *Handler) retryFailedGameUpdate(ctx context.Context, server domain.GameServer, job domain.GameUpdateJob, updateErr error, restoreRunning bool) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := h.cleanupGameUpdater(ctx, job.ID); err == nil {
			h.restoreAndFailGameUpdate(server, &job, updateErr, restoreRunning)
			return
		} else if h.logger != nil {
			h.logger.Error("retry cleanup for failed game update", "jobId", job.ID, "error", err)
		}
		if !waitForRetry(ctx, gameUpdateRecoveryRetry) {
			return
		}
	}
}

func (h *Handler) cleanupGameUpdater(ctx context.Context, jobID string) error {
	cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return h.runtime.CleanupGameUpdate(cleanupCtx, jobID)
}

func (h *Handler) restoreAndFailGameUpdate(server domain.GameServer, job *domain.GameUpdateJob, updateErr error, restoreRunning bool) {
	if restoreRunning {
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if _, err := serverctrl.NewService(h.store).RequestStart(recoveryCtx, server.ID); err != nil {
			updateErr = fmt.Errorf("%w; failed to restore previous running state: %v", updateErr, err)
		} else if err := h.waitForServerState(recoveryCtx, server.ID, domain.ActualRunning, 3*time.Minute); err != nil {
			updateErr = fmt.Errorf("%w; failed to restore previous running state: %v", updateErr, err)
		}
	}
	h.failGameJob(job, updateErr)
}

func (h *Handler) logCanceledGameUpdate(job *domain.GameUpdateJob) {
	if h.logger != nil {
		h.logger.Info("leave canceled game update active for startup recovery", "jobId", job.ID, "instanceId", job.InstanceID)
	}
}
func (h *Handler) failGameJob(job *domain.GameUpdateJob, err error) {
	now := time.Now().UTC()
	job.CompletedAt = &now
	h.updateTerminalGameJob(job, domain.GameUpdateJobFailed, job.Stage, job.Progress, err.Error())
	h.recordActivity(context.Background(), job.InstanceID, "server.game-update.failed", err.Error(), map[string]any{"jobId": job.ID})
}
func (h *Handler) completeGameJob(job *domain.GameUpdateJob) {
	now := time.Now().UTC()
	job.CompletedAt = &now
	h.updateTerminalGameJob(job, domain.GameUpdateJobSucceeded, domain.GameUpdateStageCompleted, 100, "")
	h.recordActivity(context.Background(), job.InstanceID, "server.game-update.succeeded", "Game update task completed", map[string]any{"jobId": job.ID, "buildId": job.InstalledBuildID})
}

func (h *Handler) updateTerminalGameJob(job *domain.GameUpdateJob, status domain.GameUpdateJobStatus, stage domain.GameUpdateJobStage, progress int, errorText string) {
	for attempt := 0; attempt < 3; attempt++ {
		if err := h.updateGameJob(job, status, stage, progress, errorText); err == nil {
			return
		}
		if attempt < 2 {
			time.Sleep(gameUpdatePersistenceRetry)
		}
	}
}
func (h *Handler) waitForServerState(ctx context.Context, id string, state domain.ServerActualState, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for server to become %s", state)
		case <-tick.C:
			server, err := h.store.GetGameServer(ctx, id)
			if err != nil {
				return err
			}
			if state == domain.ActualStopped && server.Spec.DesiredState == domain.DesiredStopped && server.Status.Phase == domain.PhaseStopped && (server.Status.ActualState == domain.ActualStopped || server.Status.ActualState == domain.ActualMissing) {
				return nil
			}
			if state != domain.ActualStopped && server.Status.ActualState == state {
				return nil
			}
			if server.Status.Phase == domain.PhaseFailed {
				return errors.New(server.Status.LastError)
			}
		}
	}
}
func mapRuntimeUpdateStage(stage string) domain.GameUpdateJobStage {
	switch stage {
	case "refreshing_metadata":
		return domain.GameUpdateStageRefreshingMetadata
	case "validating":
		return domain.GameUpdateStageValidating
	case "downloading":
		return domain.GameUpdateStageDownloading
	case "installing":
		return domain.GameUpdateStageInstalling
	}
	return domain.GameUpdateStageInstalling
}
func (h *Handler) backgroundContext() context.Context {
	if h.ctx != nil {
		return h.ctx
	}
	return context.Background()
}

func (h *Handler) startGameUpdateWorker(run func()) bool {
	h.gameUpdateWorkersMu.Lock()
	if h.gameUpdateClosing {
		h.gameUpdateWorkersMu.Unlock()
		return false
	}
	h.gameUpdateJobsWG.Add(1)
	h.gameUpdateWorkersMu.Unlock()
	go func() {
		defer h.gameUpdateJobsWG.Done()
		run()
	}()
	return true
}
func (h *Handler) recoverInterruptedGameUpdates(ctx context.Context, startedAt time.Time) {
	for {
		jobs, err := h.store.ListActiveGameUpdateJobs(ctx)
		if err != nil {
			if h.logger != nil {
				h.logger.Error("list interrupted game update jobs", "error", err)
			}
			if !waitForRetry(ctx, gameUpdateRecoveryRetry) {
				return
			}
			continue
		}
		retryNeeded := false
		for i := range jobs {
			job := &jobs[i]
			if !job.CreatedAt.Before(startedAt) {
				continue
			}
			cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := h.runtime.CleanupGameUpdate(cleanupCtx, job.ID)
			cancel()
			if err != nil {
				retryNeeded = true
				if h.logger != nil {
					h.logger.Error("clean interrupted game updater", "jobId", job.ID, "error", err)
				}
				continue
			}
			if job.Operation == domain.GameUpdateOperationApply && interruptedApplyRequiresValidation(job.Stage) {
				h.resumeInterruptedGameUpdate(ctx, job)
				continue
			}
			interruptedErr := errors.New("update task was interrupted by API restart")
			if job.WasRunning {
				restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 10*time.Second)
				_, restoreErr := serverctrl.NewService(h.store).RequestStart(restoreCtx, job.InstanceID)
				restoreCancel()
				if restoreErr != nil {
					interruptedErr = fmt.Errorf("%w; failed to restore previous running state: %v", interruptedErr, restoreErr)
				}
			}
			h.failGameJob(job, interruptedErr)
		}
		if !retryNeeded || !waitForRetry(ctx, gameUpdateRecoveryRetry) {
			return
		}
	}
}

func interruptedApplyRequiresValidation(stage domain.GameUpdateJobStage) bool {
	switch stage {
	case domain.GameUpdateStageRefreshingMetadata,
		domain.GameUpdateStageValidating,
		domain.GameUpdateStageDownloading,
		domain.GameUpdateStageInstalling,
		domain.GameUpdateStageStarting,
		domain.GameUpdateStageHealthCheck:
		return true
	default:
		return false
	}
}

func (h *Handler) resumeInterruptedGameUpdate(ctx context.Context, job *domain.GameUpdateJob) {
	resumeCtx, cancel := context.WithTimeout(ctx, gameUpdateApplyTimeout)
	defer cancel()
	server, err := h.store.GetGameServer(resumeCtx, job.InstanceID)
	if err != nil {
		h.failGameUpdateTask(resumeCtx, server, job, fmt.Errorf("load server before resuming interrupted update: %w", err), false)
		return
	}
	if !isGameServerStableStopped(server) {
		if _, err := serverctrl.NewService(h.store).RequestStop(resumeCtx, server.ID); err != nil {
			h.failGameUpdateTask(resumeCtx, server, job, fmt.Errorf("stop server before resuming interrupted update: %w", err), false)
			return
		}
		if err := h.waitForServerState(resumeCtx, server.ID, domain.ActualStopped, 2*time.Minute); err != nil {
			h.failGameUpdateTask(resumeCtx, server, job, fmt.Errorf("wait for server before resuming interrupted update: %w", err), false)
			return
		}
	}
	server, err = h.store.GetGameServer(resumeCtx, server.ID)
	if err != nil {
		h.failGameUpdateTask(resumeCtx, server, job, err, false)
		return
	}
	request, err := h.gameUpdateRequest(server, job.ID)
	if err != nil {
		h.failGameUpdateTask(resumeCtx, server, job, err, false)
		return
	}
	if err := h.updateGameJob(job, domain.GameUpdateJobRunning, domain.GameUpdateStageValidating, job.Progress, ""); err != nil {
		h.failGameUpdateTask(resumeCtx, server, job, fmt.Errorf("persist validation checkpoint before resuming SteamCMD: %w", err), false)
		return
	}
	result, err := h.runtime.ApplyGameUpdate(resumeCtx, request, func(progress runtime.GameUpdateProgress) {
		h.updateGameJob(job, domain.GameUpdateJobRunning, mapRuntimeUpdateStage(progress.Stage), maxInt(job.Progress, progress.Progress), "")
	})
	if err != nil {
		h.failGameUpdateTask(resumeCtx, server, job, fmt.Errorf("resume interrupted game update: %w", err), false)
		return
	}
	job.InstalledBuildID, job.LatestBuildID = result.InstalledBuildID, result.LatestBuildID
	checkedAt := time.Now().UTC()
	job.CheckedAt = &checkedAt
	if job.StartAfterUpdate {
		if err := h.updateGameJob(job, domain.GameUpdateJobRunning, domain.GameUpdateStageStarting, maxInt(job.Progress, 94), ""); err != nil {
			h.failGameUpdateTask(resumeCtx, server, job, fmt.Errorf("persist restart checkpoint after resumed game update: %w", err), false)
			return
		}
		if _, err := serverctrl.NewService(h.store).RequestStart(resumeCtx, server.ID); err != nil {
			h.failGameUpdateTask(resumeCtx, server, job, err, false)
			return
		}
		h.updateGameJob(job, domain.GameUpdateJobRunning, domain.GameUpdateStageHealthCheck, maxInt(job.Progress, 97), "")
		if err := h.waitForGameServerHealthy(resumeCtx, server.ID, 5*time.Minute); err != nil {
			h.failGameUpdateTask(resumeCtx, server, job, err, false)
			return
		}
	}
	h.completeGameJob(job)
}

func isGameServerStableStopped(server domain.GameServer) bool {
	return server.Spec.DesiredState == domain.DesiredStopped &&
		server.Status.Phase == domain.PhaseStopped &&
		(server.Status.ActualState == domain.ActualStopped || server.Status.ActualState == domain.ActualMissing)
}

func (h *Handler) waitForGameServerHealthy(ctx context.Context, id string, timeout time.Duration) error {
	return h.waitForGameServerHealthyWithTiming(ctx, id, timeout, 2*time.Second, gameUpdateHealthStableWindow)
}

func (h *Handler) waitForGameServerHealthyWithTiming(ctx context.Context, id string, timeout, pollInterval, stableWindow time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	var stableSince time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for game server health")
		case <-tick.C:
			server, err := h.store.GetGameServer(ctx, id)
			if err != nil {
				return err
			}
			if server.Status.Phase == domain.PhaseFailed {
				return errors.New(server.Status.LastError)
			}
			if server.Status.ActualState != domain.ActualRunning || server.Status.RuntimeID == "" {
				stableSince = time.Time{}
				continue
			}
			health, err := h.runtime.InspectWorkloadHealth(ctx, server.Status.RuntimeID)
			if err != nil {
				return fmt.Errorf("inspect game server health: %w", err)
			}
			if health.HasHealthCheck {
				switch health.Status {
				case runtime.WorkloadHealthHealthy:
					return nil
				case runtime.WorkloadHealthUnhealthy:
					return fmt.Errorf("game server health check reported unhealthy")
				default:
					continue
				}
			}
			if health.Status != runtime.WorkloadHealthHealthy {
				stableSince = time.Time{}
				continue
			}
			if stableSince.IsZero() {
				stableSince = time.Now()
				continue
			}
			if time.Since(stableSince) >= stableWindow {
				return nil
			}
		}
	}
}

func availableDiskBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (h *Handler) gameUpdateLocked(ctx context.Context, instanceID string) bool {
	_, err := h.store.GetActiveGameUpdateJobByInstance(ctx, instanceID)
	if err == nil || !errors.Is(err, store.ErrNotFound) {
		return true
	}
	_, err = h.store.GetActiveWorldRegenerationJobByInstance(ctx, instanceID)
	return err == nil || !errors.Is(err, store.ErrNotFound)
}

func (h *Handler) gameUpdateRuntimeLocked(ctx context.Context) bool {
	return h.maintenanceRuntimeLocked(ctx)
}

func (h *Handler) maintenanceRuntimeLocked(ctx context.Context) bool {
	jobs, err := h.store.ListActiveGameUpdateJobs(ctx)
	if err != nil || len(jobs) > 0 {
		return true
	}
	worldJobs, err := h.store.ListActiveWorldRegenerationJobs(ctx)
	return err != nil || len(worldJobs) > 0
}

func (h *Handler) lockServerMutation(instanceID string) func() {
	value, _ := h.serverMutationLocks.LoadOrStore(instanceID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}
