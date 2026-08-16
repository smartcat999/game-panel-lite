package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const systemUpdateAutoCheckSetting = "systemUpdateAutoCheck"

func (h *Handler) systemUpdatePreferences(ctx context.Context) (bool, int) {
	enabled := true
	if raw, err := h.store.GetSetting(ctx, systemUpdateAutoCheckSetting); err == nil {
		if parsed, parseErr := strconv.ParseBool(raw); parseErr == nil {
			enabled = parsed
		}
	}
	hours := int(h.cfg.SystemUpdateInterval / time.Hour)
	if hours < 1 {
		hours = 24
	}
	return enabled, hours
}

func (h *Handler) getSystemUpdate(w http.ResponseWriter, r *http.Request) {
	enabled, hours := h.systemUpdatePreferences(r.Context())
	writeJSON(w, http.StatusOK, h.systemUpdate.Status(r.Context(), enabled, hours))
}

func (h *Handler) checkSystemUpdate(w http.ResponseWriter, r *http.Request) {
	enabled, hours := h.systemUpdatePreferences(r.Context())
	status, err := h.systemUpdate.Check(r.Context(), enabled, hours)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, status)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) updateSystemUpdateAutoCheck(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Enabled == nil {
		writeError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	if err := h.store.SetSetting(r.Context(), systemUpdateAutoCheckSetting, strconv.FormatBool(*payload.Enabled)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, hours := h.systemUpdatePreferences(r.Context())
	writeJSON(w, http.StatusOK, h.systemUpdate.Status(r.Context(), *payload.Enabled, hours))
}

func (h *Handler) applySystemUpdate(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	job, err := h.systemUpdate.Apply(r.Context(), payload.Version)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	h.recordActivity(r.Context(), "", "system.update.queued", "Queued GamePanel Lite update to "+payload.Version, map[string]any{"version": payload.Version, "jobId": job.ID})
	writeJSON(w, http.StatusAccepted, job)
}

func (h *Handler) runAutomaticSystemUpdateChecks(ctx context.Context) {
	interval := h.cfg.SystemUpdateInterval
	if interval < time.Hour {
		interval = 24 * time.Hour
	}
	check := func() {
		enabled, hours := h.systemUpdatePreferences(ctx)
		if !enabled {
			return
		}
		status, err := h.systemUpdate.Check(ctx, enabled, hours)
		if err != nil {
			h.logger.Warn("automatic panel update check failed", "error", err)
			return
		}
		if status.UpdateAvailable && status.Latest != nil {
			h.recordActivity(context.Background(), "", "system.update.available", fmt.Sprintf("GamePanel Lite %s is available", status.Latest.Version), map[string]any{"version": status.Latest.Version})
		}
	}
	check()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
