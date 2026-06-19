package monitoring

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/api/monitoring/overview", h.overview)
	r.Get("/api/monitoring/metrics", h.metrics)
	r.Get("/api/monitoring/server-load", h.serverLoad)
	r.Get("/api/monitoring/events", h.events)
	r.Get("/api/monitoring/platform", h.platform)
	r.Get("/api/servers/{id}/metrics", h.serverMetrics)
	r.Get("/api/servers/{id}/events", h.serverEvents)
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	payload, err := h.service.Overview(r.Context())
	writeResponse(w, payload, err)
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	payload, err := h.service.Metrics(r.Context(), r.URL.Query().Get("range"), r.URL.Query().Get("step"))
	writeResponse(w, payload, err)
}

func (h *Handler) platform(w http.ResponseWriter, r *http.Request) {
	payload, err := h.service.Platform(r.Context(), r.URL.Query().Get("range"), r.URL.Query().Get("step"))
	writeResponse(w, payload, err)
}

func (h *Handler) serverLoad(w http.ResponseWriter, r *http.Request) {
	payload, err := h.service.ServerLoad(r.Context())
	writeResponse(w, payload, err)
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	payload, err := h.service.Events(r.Context(), "", limit(query.Get("limit")), query.Get("severity"), query.Get("type"), query.Get("game"))
	writeResponse(w, payload, err)
}

func (h *Handler) serverMetrics(w http.ResponseWriter, r *http.Request) {
	payload, err := h.service.ServerMetrics(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("range"), r.URL.Query().Get("step"))
	writeResponse(w, payload, err)
}

func (h *Handler) serverEvents(w http.ResponseWriter, r *http.Request) {
	payload, err := h.service.Events(r.Context(), chi.URLParam(r, "id"), limit(r.URL.Query().Get("limit")), "", "", "")
	writeResponse(w, payload, err)
}

func limit(raw string) int {
	if raw == "" {
		return 50
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 50
	}
	if value > 200 {
		return 200
	}
	return value
}

func writeResponse(w http.ResponseWriter, payload any, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
