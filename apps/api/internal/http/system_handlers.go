package http

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/observability"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"name": "GamePanel Lite", "version": "0.1.0"})
}

func (h *Handler) dockerStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.dockerMonitor.Status())
}

func (h *Handler) runtimeStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.runtime.HostStats(r.Context())
	if err != nil {
		stats = runtime.HostStats{}
	}
	if used, usageErr := dataDirUsageBytes(h.cfg.DataDir); usageErr == nil {
		stats.StorageUsedBytes = used
	}
	writeJSON(w, http.StatusOK, stats)
}

func dataDirUsageBytes(root string) (int64, error) {
	if strings.TrimSpace(root) == "" {
		return 0, nil
	}
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func (h *Handler) observabilityMetrics(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.observabilitySnapshot(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) prometheusMetrics(w http.ResponseWriter, r *http.Request) {
	body := ""
	if h.apiMetrics != nil {
		body = h.apiMetrics.PrometheusText()
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (h *Handler) observabilityPrometheus(w http.ResponseWriter, r *http.Request) {
	body, err := h.observabilityPrometheusText(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (h *Handler) observabilitySnapshot(ctx context.Context) (observability.Snapshot, error) {
	if h.observability != nil {
		return h.observability.Snapshot(ctx)
	}
	return observability.NewService(h.store, h.runtime).Snapshot(ctx, h.runtimeStatusAvailable())
}

func (h *Handler) observabilityPrometheusText(ctx context.Context) (string, error) {
	if h.observability != nil {
		return h.observability.PrometheusText(ctx)
	}
	return observability.NewService(h.store, h.runtime).PrometheusText(ctx, h.runtimeStatusAvailable())
}

func (h *Handler) listActivity(w http.ResponseWriter, r *http.Request) {
	events, err := h.store.ListActivity(r.Context(), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func isRuntimeContainerMissingError(err error) bool {
	if err == nil {
		return false
	}
	normalized := strings.ToLower(err.Error())
	return strings.Contains(normalized, "no docker container found") ||
		strings.Contains(normalized, "no such container") ||
		strings.Contains(normalized, "page not found")
}

func (h *Handler) runtimeStatusAvailable() bool {
	if h.dockerMonitor == nil {
		return true
	}
	return h.dockerMonitor.Status().Available
}

func statusCodeForRuntimeError(err error) int {
	if err != nil && err.Error() == "server not found" {
		return http.StatusNotFound
	}
	if err != nil && (strings.Contains(err.Error(), "required") ||
		strings.Contains(err.Error(), "must be") ||
		strings.Contains(err.Error(), "cannot contain") ||
		strings.Contains(err.Error(), "invalid")) {
		return http.StatusBadRequest
	}
	if err != nil && strings.Contains(err.Error(), "stop the server") {
		return http.StatusConflict
	}
	if err != nil && strings.Contains(err.Error(), "unknown provider") {
		return http.StatusBadRequest
	}
	return http.StatusServiceUnavailable
}
