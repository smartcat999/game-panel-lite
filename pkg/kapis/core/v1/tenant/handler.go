package tenant

import (
	"context"
	"encoding/json"
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	tenantmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/tenant"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

type handler struct {
	op       tenantmodel.Operator
	reg      *repo.Registry
	mq       mq.Interface
	sessions sessionmodel.Operator
}

func newHandler(op tenantmodel.Operator, reg *repo.Registry, bus ...mq.Interface) *handler {
	var q mq.Interface
	if len(bus) > 0 {
		q = bus[0]
	}
	return &handler{op: op, reg: reg, mq: q}
}

func (h *handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(
			w, r, h.mq, mq.TopicTenant, opts.Tenant, "", opts,
			func(ctx context.Context, _, name string) (*corev1.Tenant, error) {
				return h.op.Get(ctx, name)
			},
			apiruntime.WithWatchSource(h.op),
			apiruntime.WithWatchSharedObject(),
			apiruntime.WithWatchFilter(func(ctx context.Context, _ metav1.WatchEventType, ev mq.Event, obj any) bool {
				hint, _ := obj.(*corev1.Tenant)
				if hint == nil {
					if mo, ok := obj.(metav1.Object); ok && mo.GetObjectMeta() != nil {
						hint = &corev1.Tenant{Metadata: *mo.GetObjectMeta()}
					}
				}
				return h.op.CanView(ctx, ev.Name, hint, ev.Attributes["owner"])
			}),
		)
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var tenant corev1.Tenant
	if err := json.NewDecoder(r.Body).Decode(&tenant); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	userInfo, _ := request.UserFrom(r.Context())
	created, err := h.op.Create(r.Context(), userInfo.Name, &tenant)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetTenant(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("tenant"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, obj)
}

func (h *handler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	var tenant corev1.Tenant
	if err := json.NewDecoder(r.Body).Decode(&tenant); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("tenant"), &tenant)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) PatchTenant(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("tenant")
	current, err := h.op.Get(r.Context(), name)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	patched, err := apiruntime.ApplyMergePatch(r, current)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), name, patched)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) UpdateTenantStatus(w http.ResponseWriter, r *http.Request) {
	var tenant corev1.Tenant
	if err := json.NewDecoder(r.Body).Decode(&tenant); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.UpdateStatus(r.Context(), r.PathValue("tenant"), &tenant)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("tenant")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
