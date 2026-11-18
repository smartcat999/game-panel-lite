package alert

import (
	"net/http"
	"strings"

	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	alertmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert"
)

type handler struct {
	op alertmodel.Operator
}

func newHandler(op alertmodel.Operator) *handler {
	return &handler{op: op}
}

func (h *handler) ListClusterAlerts(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) ListTenantAlerts(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts.Tenant = strings.TrimSpace(r.PathValue("tenant"))
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}
