package log

import (
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	logmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/log"
)

type handler struct {
	op logmodel.Operator
}

func newHandler(op logmodel.Operator) *handler {
	return &handler{op: op}
}

func (h *handler) ListPlatformLogs(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	list, err := h.op.List(r.Context(), logmodel.QueryOptions{
		ListOptions: opts,
		Scope:       q.Get("scope"),
		Service:     q.Get("service"),
		Level:       q.Get("level"),
		Tenant:      q.Get("tenant"),
		Instance:    q.Get("instance"),
		Window:      q.Get("window"),
		Since:       q.Get("since"),
		Query:       q.Get("q"),
	})
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) ListTenantLogs(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	list, err := h.op.List(r.Context(), logmodel.QueryOptions{
		ListOptions:  opts,
		Scope:        corev1.LogScopeInstance,
		Service:      corev1.LogScopeInstance,
		Tenant:       r.PathValue("tenant"),
		TenantScoped: true,
		Instance:     q.Get("instance"),
		Level:        q.Get("level"),
		Window:       q.Get("window"),
		Since:        q.Get("since"),
		Query:        q.Get("q"),
	})
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}
