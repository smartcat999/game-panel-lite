package metering

import (
	"net/http"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	meteringmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/metering"
)

type handler struct {
	op meteringmodel.Operator
}

func newHandler(op meteringmodel.Operator) *handler {
	return &handler{op: op}
}

func (h *handler) ListClusterMetering(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	window := strings.TrimSpace(r.URL.Query().Get("window"))
	list, err := h.op.List(r.Context(), window, opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	writeMeteringResponse(w, r, list)
}

func (h *handler) ListTenantMetering(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts.Tenant = strings.TrimSpace(r.PathValue("tenant"))
	window := strings.TrimSpace(r.URL.Query().Get("window"))
	list, err := h.op.List(r.Context(), window, opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	writeMeteringResponse(w, r, list)
}

func writeMeteringResponse(w http.ResponseWriter, r *http.Request, list *metav1.List[*corev1.TenantMetering]) {
	res, err := meteringmodel.Export(list, r.URL.Query().Get("format"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	if ct := strings.TrimSpace(res.ContentType); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.Body)
}
