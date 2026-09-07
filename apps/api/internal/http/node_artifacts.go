package http

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modlibrary"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func (h *Handler) downloadAgentArtifact(w http.ResponseWriter, r *http.Request) {
	node, ok := h.authenticateAgentNode(w, r)
	if !ok {
		return
	}
	generation, err := strconv.Atoi(r.URL.Query().Get("generation"))
	if err != nil || generation <= 0 {
		writeError(w, http.StatusBadRequest, "positive assignment generation is required")
		return
	}
	holderID := r.URL.Query().Get("holderId")
	if holderID == "" {
		holderID = r.Header.Get("X-Lease-Holder-ID")
	}
	fenceStr := r.URL.Query().Get("fence")
	if fenceStr == "" {
		fenceStr = r.Header.Get("X-Lease-Fence")
	}
	fence, err := strconv.ParseInt(fenceStr, 10, 64)
	if err != nil || fence <= 0 || holderID == "" || len(holderID) > 128 {
		writeError(w, http.StatusBadRequest, "valid execution lease holderId and fence are required")
		return
	}
	file, ref, err := h.modDelivery.Open(r.Context(), node.ID, chi.URLParam(r, "uid"), generation, chi.URLParam(r, "artifactId"), holderID, fence)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "artifact not found")
		case errors.Is(err, store.ErrReconciliationSuperseded), errors.Is(err, store.ErrInvalidModLibrary):
			writeError(w, http.StatusConflict, "artifact assignment changed; poll again")
		case errors.Is(err, store.ErrExecutionLeaseUnavailable):
			writeError(w, http.StatusConflict, "execution lease unavailable")
		case errors.Is(err, modlibrary.ErrArtifactUnavailable):
			writeError(w, http.StatusServiceUnavailable, "artifact bytes unavailable")
		default:
			h.logger.Error("artifact authorization failed", "nodeId", node.ID, "error", err)
			writeError(w, http.StatusInternalServerError, "artifact unavailable")
		}
		return
	}
	defer file.Close()
	currentNode, ok := h.authenticateAgentNode(w, r)
	if !ok {
		return
	}
	if currentNode.ID != node.ID {
		writeError(w, http.StatusUnauthorized, "node authorization changed")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(ref.SizeBytes, 10))
	w.Header().Set("ETag", strconv.Quote(ref.SHA256))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := io.CopyN(w, file, ref.SizeBytes); err != nil {
		h.logger.Warn("artifact stream interrupted", "nodeId", node.ID, "artifactId", ref.ID, "error", err)
	}
}
