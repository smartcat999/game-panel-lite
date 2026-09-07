package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

// Work is bounded to 90 seconds in the Agent; this leaves a cleanup margin.
const agentExecutionLeaseTTL = 2 * time.Minute

func (h *Handler) changeAgentLease(w http.ResponseWriter, r *http.Request) {
	node, ok := h.authenticateAgentNode(w, r)
	if !ok {
		return
	}
	var request workload.LeaseRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid lease request")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid lease request")
		return
	}
	if request.Generation <= 0 || strings.TrimSpace(request.HolderID) == "" || len(request.HolderID) > 128 || (request.Action != "acquire" && request.Action != "renew" && request.Action != "release") || (request.Action == "acquire" && request.Fence != 0) || (request.Action != "acquire" && request.Fence <= 0) {
		writeError(w, http.StatusBadRequest, "invalid lease identity or action")
		return
	}
	input := store.ExecutionLeaseRequest{NodeID: node.ID, NodeToken: node.Token, AssignmentUID: chi.URLParam(r, "uid"), HolderID: request.HolderID, Generation: request.Generation}
	var lease store.ExecutionLease
	var err error
	switch request.Action {
	case "acquire":
		lease, err = h.store.AcquireExecutionLease(r.Context(), input, agentExecutionLeaseTTL)
	case "renew":
		lease, err = h.store.RenewExecutionLease(r.Context(), input, request.Fence, agentExecutionLeaseTTL)
	case "release":
		err = h.store.ReleaseExecutionLease(r.Context(), input, request.Fence)
	}
	if err != nil {
		if errors.Is(err, store.ErrExecutionLeaseUnavailable) {
			writeError(w, http.StatusConflict, "execution lease unavailable")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to persist execution lease")
		return
	}
	if request.Action == "release" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, workload.LeaseGrant{AssignmentUID: lease.AssignmentUID, ServerID: lease.ServerID, NodeID: lease.NodeID, Generation: lease.Generation, HolderID: lease.HolderID, Fence: lease.Fence, ValidForMS: agentExecutionLeaseTTL.Milliseconds()})
}
