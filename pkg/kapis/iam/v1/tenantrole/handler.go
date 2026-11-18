package tenantrole

import (
	"encoding/json"
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	tenantrolemodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrole"
)

type handler struct {
	op tenantrolemodel.Operator
	mq mq.Interface
}

func newHandler(op tenantrolemodel.Operator, bus ...mq.Interface) *handler {
	var q mq.Interface
	if len(bus) > 0 {
		q = bus[0]
	}
	return &handler{op: op, mq: q}
}

func (h *handler) ListAllTenantRoles(w http.ResponseWriter, r *http.Request) {
	h.ListTenantRoles(w, r)
}

func (h *handler) ListTenantRoles(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(w, r, h.mq, mq.TopicTenantRole, opts.Tenant, "", opts, h.op.Get, apiruntime.WithWatchSource(h.op))
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) CreateTenantRole(w http.ResponseWriter, r *http.Request) {
	var role iamv1.TenantRole
	if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.op.Create(r.Context(), r.PathValue("tenant"), &role)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), created.Metadata.Name)
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetTenantRole(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("tenant"), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, obj)
}

func (h *handler) UpdateTenantRole(w http.ResponseWriter, r *http.Request) {
	var role iamv1.TenantRole
	if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("tenant"), r.PathValue("name"), &role)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) PatchTenantRole(w http.ResponseWriter, r *http.Request) {
	tenant := r.PathValue("tenant")
	name := r.PathValue("name")
	current, err := h.op.Get(r.Context(), tenant, name)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	patched, err := apiruntime.ApplyMergePatch(r, current)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), tenant, name, patched)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) DeleteTenantRole(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("tenant"), r.PathValue("name")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
