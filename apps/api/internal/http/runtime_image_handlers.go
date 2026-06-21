package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type prepareRuntimeImageRequest struct {
	ProviderKey domain.ProviderKey `json:"providerKey"`
	Version     string             `json:"version,omitempty"`
}

type runtimeInstallRef struct {
	ProviderKey domain.ProviderKey `json:"providerKey"`
	Version     string             `json:"version"`
	Image       string             `json:"image"`
}

type runtimeInstallMarker struct {
	ProviderKey domain.ProviderKey `json:"providerKey"`
	Version     string             `json:"version"`
	Image       string             `json:"image"`
	ArchivePath string             `json:"archivePath,omitempty"`
	InstalledAt time.Time          `json:"installedAt"`
}

func (h *Handler) prepareRuntimeImage(w http.ResponseWriter, r *http.Request) {
	var payload prepareRuntimeImageRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	gameProvider, ok := h.provider.Get(payload.ProviderKey)
	if !ok {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	if err := h.requireProviderRuntimeSupported(payload.ProviderKey); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := h.requireRuntimeAvailable(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	version := normalizeStoredProviderVersion(gameProvider, payload.Version)
	image := gameProvider.ImageFor(version)
	ref := runtimeInstallRef{ProviderKey: payload.ProviderKey, Version: version, Image: image}
	if status := h.runtimeInstallStatus(r.Context(), ref); status.Status == runtime.ImageStatusReady {
		writeJSON(w, http.StatusOK, status)
		return
	} else if status.Status == runtime.ImageStatusPreparing {
		writeJSON(w, http.StatusAccepted, status)
		return
	}
	status := domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusPreparing, UpdatedAt: time.Now()}
	h.setRuntimeImageJob(status)
	go h.prepareRuntimeImageAsync(ref)
	writeJSON(w, http.StatusAccepted, status)
}

func (h *Handler) prepareRuntimeImageAsync(ref runtimeInstallRef) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	lastProgress := 0
	status := domain.RuntimeImageStatus{Image: ref.Image, Progress: lastProgress, UpdatedAt: time.Now()}
	if err := h.runtime.PrepareImageWithProgress(ctx, ref.Image, func(progress runtime.ImagePrepareProgress) {
		nextProgress := runtimeInstallPullProgress(progress.Progress)
		if nextProgress < lastProgress {
			nextProgress = lastProgress
		}
		lastProgress = nextProgress
		h.setRuntimeImageJob(domain.RuntimeImageStatus{
			Image:     ref.Image,
			Status:    runtime.ImageStatusPreparing,
			Message:   progress.Message,
			Progress:  nextProgress,
			UpdatedAt: time.Now(),
		})
	}); err != nil {
		status.Status = runtime.ImageStatusFailed
		status.Message = err.Error()
		status.Progress = lastProgress
		if h.logger != nil {
			h.logger.Warn("runtime image prepare failed", "image", ref.Image, "provider", ref.ProviderKey, "version", ref.Version, "error", err)
		}
	} else {
		saveProgress := lastProgress
		if saveProgress < 95 {
			saveProgress = 95
		}
		h.setRuntimeImageJob(domain.RuntimeImageStatus{
			Image:     ref.Image,
			Status:    runtime.ImageStatusPreparing,
			Message:   "saving runtime image",
			Progress:  saveProgress,
			UpdatedAt: time.Now(),
		})
		if err := h.runtime.SaveImageArchive(ctx, ref.Image, h.runtimeInstallArchivePath(ref)); err != nil {
			status.Status = runtime.ImageStatusFailed
			status.Message = err.Error()
			status.Progress = saveProgress
			if h.logger != nil {
				h.logger.Warn("runtime image archive save failed", "image", ref.Image, "provider", ref.ProviderKey, "version", ref.Version, "error", err)
			}
		} else if err := h.writeRuntimeInstallMarker(ref); err != nil {
			status.Status = runtime.ImageStatusFailed
			status.Message = err.Error()
			status.Progress = saveProgress
			if h.logger != nil {
				h.logger.Warn("runtime install marker write failed", "image", ref.Image, "provider", ref.ProviderKey, "version", ref.Version, "error", err)
			}
		} else {
			status.Status = runtime.ImageStatusReady
			status.Progress = 100
			if h.logger != nil {
				h.logger.Info("runtime image prepared", "image", ref.Image, "provider", ref.ProviderKey, "version", ref.Version)
			}
		}
	}
	status.UpdatedAt = time.Now()
	h.setRuntimeImageJob(status)
}

func clampRuntimeImageProgress(progress int) int {
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func runtimeInstallPullProgress(progress int) int {
	progress = clampRuntimeImageProgress(progress)
	if progress == 0 {
		return 0
	}
	if progress >= 100 {
		return 90
	}
	scaled := int(float64(progress) * 0.9)
	if scaled <= 0 {
		return 1
	}
	if scaled > 90 {
		return 90
	}
	return scaled
}

func (h *Handler) ensureRuntimeImageLoaded(ctx context.Context, ref runtimeInstallRef) error {
	imageStatus := h.runtime.ImageStatus(ctx, ref.Image)
	if imageStatus.Status == runtime.ImageStatusReady {
		return nil
	}
	archiveStatus, archivePath, ok := h.runtimeInstallMarkerStatusWithArchive(ref)
	if !ok || archiveStatus.Status != runtime.ImageStatusReady {
		if archiveStatus.Message != "" {
			return fmt.Errorf("server runtime is not installed: %s", archiveStatus.Message)
		}
		return fmt.Errorf("server runtime is not installed; install it from Game Library first")
	}
	if err := h.runtime.LoadImageArchive(ctx, archivePath); err != nil {
		return fmt.Errorf("load server runtime image from local archive: %w", err)
	}
	imageStatus = h.runtime.ImageStatus(ctx, ref.Image)
	if imageStatus.Status != runtime.ImageStatusReady {
		if imageStatus.Message != "" {
			return fmt.Errorf("load server runtime image from local archive: %s", imageStatus.Message)
		}
		return fmt.Errorf("load server runtime image from local archive failed")
	}
	return nil
}

func (h *Handler) runtimeInstallStatus(ctx context.Context, ref runtimeInstallRef) domain.RuntimeImageStatus {
	if job, ok := h.getRuntimeImageJob(ref.Image); ok && job.Status == runtime.ImageStatusPreparing {
		return job
	}
	if markerStatus, ok := h.runtimeInstallMarkerStatus(ref); ok {
		return markerStatus
	}
	if job, ok := h.getRuntimeImageJob(ref.Image); ok && job.Status == runtime.ImageStatusFailed {
		return job
	}
	return domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusMissing, Message: "runtime image is not installed", UpdatedAt: time.Now()}
}

func (h *Handler) runtimeInstallMarkerStatus(ref runtimeInstallRef) (domain.RuntimeImageStatus, bool) {
	status, _, ok := h.runtimeInstallMarkerStatusWithArchive(ref)
	return status, ok
}

func (h *Handler) runtimeInstallMarkerStatusWithArchive(ref runtimeInstallRef) (domain.RuntimeImageStatus, string, bool) {
	path := h.runtimeInstallMarkerPath(ref)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.RuntimeImageStatus{}, "", false
	}
	if err != nil {
		return domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusFailed, Message: err.Error(), UpdatedAt: time.Now()}, "", true
	}
	var marker runtimeInstallMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusFailed, Message: err.Error(), UpdatedAt: time.Now()}, "", true
	}
	if marker.ProviderKey != "" && marker.ProviderKey != ref.ProviderKey {
		return domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusFailed, Message: "runtime install marker provider mismatch", UpdatedAt: time.Now()}, "", true
	}
	if marker.Version != "" && marker.Version != ref.Version {
		return domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusFailed, Message: "runtime install marker version mismatch", UpdatedAt: time.Now()}, "", true
	}
	if marker.Image != "" && marker.Image != ref.Image {
		return domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusFailed, Message: "runtime install marker image mismatch", UpdatedAt: time.Now()}, "", true
	}
	archivePath, err := h.runtimeInstallMarkerArchivePath(ref, marker)
	if err != nil {
		return domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusFailed, Message: err.Error(), UpdatedAt: time.Now()}, "", true
	}
	return runtimeInstallArchiveStatus(ref.Image, archivePath), archivePath, true
}

func runtimeInstallArchiveStatus(image string, archivePath string) domain.RuntimeImageStatus {
	stat, err := os.Stat(archivePath)
	if err == nil && !stat.IsDir() && stat.Size() > 0 {
		return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusReady, UpdatedAt: stat.ModTime()}
	}
	if errors.Is(err, os.ErrNotExist) {
		return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusMissing, Message: "runtime image archive is missing", UpdatedAt: time.Now()}
	}
	if err == nil && stat.IsDir() {
		return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusMissing, Message: "runtime image archive path is a directory", UpdatedAt: time.Now()}
	}
	if err == nil && stat.Size() <= 0 {
		return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusMissing, Message: "runtime image archive is empty", UpdatedAt: time.Now()}
	}
	return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusFailed, Message: err.Error(), UpdatedAt: time.Now()}
}

func (h *Handler) writeRuntimeInstallMarker(ref runtimeInstallRef) error {
	if ref.ProviderKey == "" || strings.TrimSpace(ref.Version) == "" || strings.TrimSpace(ref.Image) == "" {
		return fmt.Errorf("runtime install marker is missing provider, version, or image")
	}
	path := h.runtimeInstallMarkerPath(ref)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	marker := runtimeInstallMarker{
		ProviderKey: ref.ProviderKey,
		Version:     ref.Version,
		Image:       ref.Image,
		ArchivePath: h.runtimeInstallArchiveRelPath(ref),
		InstalledAt: time.Now(),
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (h *Handler) runtimeInstallMarkerPath(ref runtimeInstallRef) string {
	return filepath.Join(h.cfg.DataDir, "runtime-installs", safeRuntimeInstallPathPart(string(ref.ProviderKey)), safeRuntimeInstallPathPart(ref.Version)+".json")
}

func (h *Handler) runtimeInstallArchiveRelPath(ref runtimeInstallRef) string {
	return filepath.Join("runtime-images", safeRuntimeInstallPathPart(string(ref.ProviderKey)), safeRuntimeInstallPathPart(ref.Version)+".tar")
}

func (h *Handler) runtimeInstallArchivePath(ref runtimeInstallRef) string {
	return filepath.Join(h.cfg.DataDir, h.runtimeInstallArchiveRelPath(ref))
}

func (h *Handler) runtimeInstallMarkerArchivePath(ref runtimeInstallRef, marker runtimeInstallMarker) (string, error) {
	archivePath := strings.TrimSpace(marker.ArchivePath)
	if archivePath == "" {
		archivePath = h.runtimeInstallArchiveRelPath(ref)
	}
	clean := filepath.Clean(archivePath)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("runtime install marker archive path is invalid")
	}
	return filepath.Join(h.cfg.DataDir, clean), nil
}

func safeRuntimeInstallPathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '.', char == '-', char == '_':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func (h *Handler) getRuntimeImageJob(image string) (domain.RuntimeImageStatus, bool) {
	h.runtimeImageJobsMu.Lock()
	defer h.runtimeImageJobsMu.Unlock()
	status, ok := h.runtimeImageJobs[image]
	return status, ok
}

func (h *Handler) setRuntimeImageJob(status domain.RuntimeImageStatus) {
	if status.Image == "" {
		return
	}
	h.runtimeImageJobsMu.Lock()
	defer h.runtimeImageJobsMu.Unlock()
	h.runtimeImageJobs[status.Image] = status
}

func (h *Handler) workshopSyncUnsupported() bool {
	architecture := strings.ToLower(strings.TrimSpace(h.dockerMonitor.Status().Architecture))
	return strings.HasPrefix(architecture, "arm") || strings.Contains(architecture, "aarch64")
}

func (h *Handler) providerRuntimeUnsupported(providerKey domain.ProviderKey) bool {
	if providerKey != domain.ProviderDST || h.dockerMonitor == nil {
		return false
	}
	architecture := strings.ToLower(strings.TrimSpace(h.dockerMonitor.Status().Architecture))
	return strings.HasPrefix(architecture, "arm") || strings.Contains(architecture, "aarch64")
}

func (h *Handler) requireProviderRuntimeSupported(providerKey domain.ProviderKey) error {
	if h.providerRuntimeUnsupported(providerKey) {
		return fmt.Errorf("Don't Starve Together server runtime is currently supported only on amd64 Docker hosts")
	}
	return nil
}
