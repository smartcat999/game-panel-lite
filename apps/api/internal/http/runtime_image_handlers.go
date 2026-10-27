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
	requestedVersion := normalizeStoredProviderVersion(gameProvider, payload.Version)
	if gameProvider.ImageFor(requestedVersion) != gameProvider.Image() {
		writeError(w, http.StatusBadRequest, "only the latest runtime image can be installed")
		return
	}
	version := normalizeStoredProviderVersion(gameProvider, "")
	image := gameProvider.ImageFor(version)
	ref := runtimeInstallRef{ProviderKey: payload.ProviderKey, Version: version, Image: image}
	h.gameUpdateJobsMu.Lock()
	defer h.gameUpdateJobsMu.Unlock()
	if h.gameUpdateRuntimeLocked(r.Context()) {
		writeError(w, http.StatusConflict, "a game update task is in progress")
		return
	}
	if status := h.runtimeInstallStatus(r.Context(), ref); status.Status == runtime.ImageStatusReady {
		writeJSON(w, http.StatusOK, status)
		return
	} else if status.Status == runtime.ImageStatusPreparing {
		writeJSON(w, http.StatusAccepted, status)
		return
	}
	if h.runtimeImagePrepareActive() {
		writeError(w, http.StatusConflict, "another runtime image task is already running")
		return
	}
	status := domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusPreparing, Phase: "resolving", UpdatedAt: time.Now()}
	h.setRuntimeImageJob(status)
	go h.prepareRuntimeImageAsync(ref)
	writeJSON(w, http.StatusAccepted, status)
}

func (h *Handler) prepareRuntimeImageAsync(ref runtimeInstallRef) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	status := domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusPreparing, Phase: "resolving"}
	report := func(p runtime.ImagePrepareProgress) {
		status.Phase = p.Phase
		if status.Phase == "" {
			status.Phase = "resolving"
		}
		status.Progress = 0
		// A percentage belongs only to a stage with a measured fixed total.
		if p.TotalBytes > 0 {
			status.Progress = int(float64(p.CurrentBytes) / float64(p.TotalBytes) * 100)
			status.Progress = max(0, min(100, status.Progress))
		}
		status.Message = p.Message
		status.CurrentBytes, status.TotalBytes = p.CurrentBytes, p.TotalBytes
		status.BytesPerSecond, status.CompletedLayers = p.BytesPerSecond, p.CompletedLayers
		status.UpdatedAt = time.Now()
		h.setRuntimeImageJob(status)
	}
	fail := func(err error) {
		status.Status = runtime.ImageStatusFailed
		status.Message = err.Error()
		status.UpdatedAt = time.Now()
		h.setRuntimeImageJob(status)
		if h.logger != nil {
			h.logger.Warn("runtime image installation failed", "image", ref.Image, "phase", status.Phase, "error", err)
		}
	}
	if err := h.runtime.PrepareImageWithProgress(ctx, ref.Image, report); err != nil {
		fail(err)
		return
	}
	report(runtime.ImagePrepareProgress{Phase: "saving"})
	if err := h.runtime.SaveImageArchiveWithProgress(ctx, ref.Image, h.runtimeInstallArchivePath(ref), report); err != nil {
		fail(err)
		return
	}
	report(runtime.ImagePrepareProgress{Phase: "finalizing"})
	if err := h.writeRuntimeInstallMarker(ref); err != nil {
		fail(err)
		return
	}
	h.cleanupOldRuntimeImages(ctx, ref)
	h.setRuntimeImageJob(domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusReady, Phase: "complete", Progress: 100, UpdatedAt: time.Now()})
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

func (h *Handler) runtimeImagePrepareActive() bool {
	h.runtimeImageJobsMu.Lock()
	defer h.runtimeImageJobsMu.Unlock()
	for _, status := range h.runtimeImageJobs {
		if status.Status == runtime.ImageStatusPreparing {
			return true
		}
	}
	return false
}

func (h *Handler) workshopSyncUnsupported() bool {
	if h == nil || h.dockerMonitor == nil {
		return false
	}
	architecture := strings.ToLower(strings.TrimSpace(h.dockerMonitor.Status().Architecture))
	return strings.HasPrefix(architecture, "arm") || strings.Contains(architecture, "aarch64")
}

func (h *Handler) providerRuntimeUnsupported(providerKey domain.ProviderKey) bool {
	if (providerKey != domain.ProviderDST && providerKey != domain.ProviderSevenDaysToDie) || h.dockerMonitor == nil {
		return false
	}
	architecture := strings.ToLower(strings.TrimSpace(h.dockerMonitor.Status().Architecture))
	return strings.HasPrefix(architecture, "arm") || strings.Contains(architecture, "aarch64")
}

func (h *Handler) requireProviderRuntimeSupported(providerKey domain.ProviderKey) error {
	if h.providerRuntimeUnsupported(providerKey) {
		return fmt.Errorf("%s server runtime is currently supported only on amd64 Docker hosts", providerKey)
	}
	return nil
}
