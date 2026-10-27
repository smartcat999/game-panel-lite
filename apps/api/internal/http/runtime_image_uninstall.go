package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

var errLatestRuntimeNotInstalled = errors.New("latest server runtime is not installed; install the latest runtime from Game Library first")

// A runtime may support multiple game-file versions, but only the current
// runtime image is offered for new installations and new servers.
func latestRuntimeVersions(p provider.GameProvider) []string {
	var versions []string
	for _, version := range p.Versions() {
		if p.ImageFor(version) == p.Image() {
			versions = append(versions, version)
		}
	}
	return versions
}

func (h *Handler) uninstallRuntimeImage(w http.ResponseWriter, r *http.Request) {
	var payload prepareRuntimeImageRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.ProviderKey == "" {
		writeError(w, http.StatusBadRequest, "providerKey is required")
		return
	}
	p, ok := h.provider.Get(payload.ProviderKey)
	if !ok {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	if err := h.requireRuntimeAvailable(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.gameUpdateJobsMu.Lock()
	defer h.gameUpdateJobsMu.Unlock()
	if h.gameUpdateRuntimeLocked(r.Context()) || h.runtimeImagePrepareActive() {
		writeError(w, http.StatusConflict, "another runtime image or game update task is already running")
		return
	}
	refs, err := h.runtimeUninstallRefs(p)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	servers, err := h.store.ListGameServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var references []string
	var stopped []runtime.ImageRemovalTarget
	for _, server := range servers {
		if server.ProviderKey != payload.ProviderKey {
			continue
		}
		if !serverStoppedForImageRemoval(server) {
			references = append(references, server.Name)
			continue
		}
		if server.NodeID != "" && server.NodeID != "node-local" {
			continue
		}
		dataDir := server.Spec.Runtime.DataDir
		if dataDir == "" {
			dataDir = filepath.Join(h.cfg.DataDir, "instances", server.ID)
		}
		dataDir, err = filepath.Abs(dataDir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		stopped = append(stopped, runtime.ImageRemovalTarget{ServerID: server.ID, DataDir: dataDir})
	}
	if len(references) > 0 {
		writeError(w, http.StatusConflict, "runtime image is used by servers: "+strings.Join(references, ", "))
		return
	}
	removed := map[string]bool{}
	for _, ref := range refs {
		if removed[ref.Image] {
			continue
		}
		if err := h.runtime.RemoveImage(r.Context(), ref.Image, stopped); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		removed[ref.Image] = true
	}
	key := safeRuntimeInstallPathPart(string(payload.ProviderKey))
	for _, dir := range []string{"runtime-images", "runtime-installs"} {
		if err := os.RemoveAll(filepath.Join(h.cfg.DataDir, dir, key)); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	for image := range removed {
		h.setRuntimeImageJob(domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusMissing, UpdatedAt: time.Now()})
	}
	status := domain.RuntimeImageStatus{Image: p.Image(), Status: runtime.ImageStatusMissing, UpdatedAt: time.Now()}
	h.setRuntimeImageJob(status)
	writeJSON(w, http.StatusOK, status)
}

// Include historical installation markers as well as current catalog versions,
// so uninstall cannot leave old image archives behind.
func (h *Handler) runtimeUninstallRefs(p provider.GameProvider) ([]runtimeInstallRef, error) {
	refs := make([]runtimeInstallRef, 0, len(p.Versions()))
	index := map[string]int{}
	for _, version := range p.Versions() {
		index[version] = len(refs)
		refs = append(refs, runtimeInstallRef{ProviderKey: p.Key(), Version: version, Image: p.ImageFor(version)})
	}
	directory := filepath.Join(h.cfg.DataDir, "runtime-installs", safeRuntimeInstallPathPart(string(p.Key())))
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return refs, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		var marker runtimeInstallMarker
		if json.Unmarshal(data, &marker) != nil || marker.ProviderKey != p.Key() || marker.Version == "" ||
			safeRuntimeInstallPathPart(marker.Version)+".json" != entry.Name() {
			return nil, errors.New("runtime install marker is invalid")
		}
		ref := runtimeInstallRef{ProviderKey: p.Key(), Version: marker.Version, Image: marker.Image}
		if ref.Image == "" {
			ref.Image = p.ImageFor(marker.Version)
		}
		if marker.ArchivePath != "" && marker.ArchivePath != h.runtimeInstallArchiveRelPath(ref) {
			return nil, errors.New("runtime install marker is invalid")
		}
		if i, ok := index[ref.Version]; ok {
			refs[i] = ref
		} else {
			index[ref.Version] = len(refs)
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

// An update keeps the latest installation only. Dependencies which are still
// running or cannot safely be removed are left intact for later cleanup.
func (h *Handler) cleanupOldRuntimeImages(ctx context.Context, latest runtimeInstallRef) {
	p, ok := h.provider.Get(latest.ProviderKey)
	if !ok {
		return
	}
	refs, err := h.runtimeUninstallRefs(p)
	if err != nil {
		return
	}
	servers, err := h.store.ListGameServers(ctx)
	if err != nil {
		return
	}
	var stopped []runtime.ImageRemovalTarget
	for _, server := range servers {
		if server.ProviderKey != latest.ProviderKey || !serverStoppedForImageRemoval(server) || (server.NodeID != "" && server.NodeID != "node-local") {
			continue
		}
		dir := server.Spec.Runtime.DataDir
		if dir == "" {
			dir = filepath.Join(h.cfg.DataDir, "instances", server.ID)
		}
		dir, err = filepath.Abs(dir)
		if err != nil {
			return
		}
		stopped = append(stopped, runtime.ImageRemovalTarget{ServerID: server.ID, DataDir: dir})
	}
	for _, ref := range refs {
		if ref.Version == latest.Version {
			continue
		}
		inUse := false
		for _, server := range servers {
			if server.ProviderKey == latest.ProviderKey && (server.Spec.Version == ref.Version || p.ImageFor(server.Spec.Version) == ref.Image) && !serverStoppedForImageRemoval(server) && ref.Image != latest.Image {
				inUse = true
				break
			}
		}
		if inUse {
			continue
		}
		if ref.Image != latest.Image {
			if err := h.runtime.RemoveImage(ctx, ref.Image, stopped); err != nil {
				if h.logger != nil {
					h.logger.Warn("old runtime image cleanup deferred", "image", ref.Image, "error", err)
				}
				continue
			}
		}
		for _, path := range []string{h.runtimeInstallArchivePath(ref), h.runtimeInstallArchivePath(ref) + ".tmp", h.runtimeInstallMarkerPath(ref)} {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				if h.logger != nil {
					h.logger.Warn("old runtime archive cleanup failed", "image", ref.Image, "error", err)
				}
			}
		}
		if ref.Image != latest.Image {
			h.setRuntimeImageJob(domain.RuntimeImageStatus{Image: ref.Image, Status: runtime.ImageStatusMissing, UpdatedAt: time.Now()})
		}
	}
}

func serverStoppedForImageRemoval(server domain.GameServer) bool {
	return server.Spec.DesiredState == domain.DesiredStopped && server.Status.Phase == domain.PhaseStopped &&
		(server.Status.ActualState == domain.ActualStopped || server.Status.ActualState == domain.ActualMissing)
}

// The caller holds the lifecycle mutation locks. A restored local instance uses
// the latest installed runtime while keeping its data, configuration and mods.
func (h *Handler) useLatestRuntimeForRestore(ctx context.Context, server *domain.GameServer) error {
	if server.NodeID != "" && server.NodeID != "node-local" {
		return nil
	}
	recoverable := serverStoppedForImageRemoval(*server) ||
		(server.Status.Phase == domain.PhaseFailed && (server.Status.ActualState == domain.ActualStopped || server.Status.ActualState == domain.ActualMissing))
	if !recoverable {
		return nil
	}
	p, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return nil
	}
	latest := normalizeStoredProviderVersion(p, "")
	image := p.ImageFor(latest)
	// Some providers use one runtime image for several game-file versions.
	if server.Spec.Version == latest || p.ImageFor(server.Spec.Version) == image {
		return nil
	}
	ref := runtimeInstallRef{ProviderKey: p.Key(), Version: latest, Image: image}
	installed := h.runtimeInstallStatus(ctx, ref)
	if installed.Status != runtime.ImageStatusReady {
		actual, _, registered := h.runtimeInstallMarkerStatusWithArchive(ctx, ref)
		if installed.Status == runtime.ImageStatusPreparing || !registered || actual.Status != runtime.ImageStatusMissing {
			return errLatestRuntimeNotInstalled
		}
	}
	server.Spec.Version = latest
	server.Spec.Generation++
	server.UpdatedAt = time.Now()
	if err := h.store.SaveGameServer(ctx, server); err != nil {
		return err
	}
	// Keep the old runtime until the registered latest image has recovered.
	if installed.Status == runtime.ImageStatusReady {
		h.cleanupOldRuntimeImages(ctx, ref)
	}
	return nil
}
