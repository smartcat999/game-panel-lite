package redeemcode

import (
	"encoding/json"
	"net/http"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/subscription"
)

type handler struct {
	svc *subscription.Service
}

func newHandler(svc *subscription.Service) *handler {
	return &handler{svc: svc}
}

func (h *handler) ListRedeemCodes(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if batch := strings.TrimSpace(r.URL.Query().Get("batchName")); batch != "" && opts.FieldSelector["batchName"] == "" {
		if opts.FieldSelector == nil {
			opts.FieldSelector = map[string]string{}
		}
		opts.FieldSelector["batchName"] = batch
	}
	list, err := h.svc.ListRedeemCodes(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) CreateRedeemCodeBatch(w http.ResponseWriter, r *http.Request) {
	var req corev1.CreateRedeemCodeBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, _ := request.UserFrom(r.Context())
	batch, err := h.svc.CreateRedeemBatch(r.Context(), user.Name, req)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), batch.BatchName)
	apiruntime.WriteJSON(w, http.StatusCreated, batch)
}
