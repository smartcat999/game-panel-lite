package tenantrolebinding

import (
	"encoding/json"
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	tenantrolebindingmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrolebinding"
)

type handler struct {
	op tenantrolebindingmodel.Operator
	mq mq.Interface
}

func newHandler(op tenantrolebindingmodel.Operator, bus ...mq.Interface) *handler {
	var q mq.Interface
	if len(bus) > 0 {
		q = bus[0]
	}
	return &handler{op: op, mq: q}
}

func (h *handler) ListAllTenantRoleBindings(w http.ResponseWriter, r *http.Request) {
	h.ListTenantRoleBindings(w, r)
}

func (h *handler) ListTenantRoleBindings(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(w, r, h.mq, mq.TopicTenantRoleBinding, opts.Tenant, "", opts, h.op.Get, apiruntime.WithWatchSource(h.op))
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func recordBindingAudit(r *http.Request, b *iamv1.TenantRoleBinding) {
	if b == nil {
		return
	}
	request.SetAuditResourceName(r.Context(), b.Metadata.Name)
	var subject string
	if len(b.Subjects) > 0 {
		subject = b.Subjects[0].Name
	}
	var attrs map[string]string
	if b.RoleRef.Name != "" {
		attrs = map[string]string{"role": b.RoleRef.Name}
	}
	request.SetAuditTarget(r.Context(), "", subject, attrs)
}

func (h *handler) CreateTenantRoleBinding(w http.ResponseWriter, r *http.Request) {
	var binding iamv1.TenantRoleBinding
	if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.op.Create(r.Context(), r.PathValue("tenant"), &binding)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	recordBindingAudit(r, created)
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetTenantRoleBinding(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("tenant"), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, obj)
}

func (h *handler) UpdateTenantRoleBinding(w http.ResponseWriter, r *http.Request) {
	var binding iamv1.TenantRoleBinding
	if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("tenant"), r.PathValue("name"), &binding)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	recordBindingAudit(r, updated)
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) PatchTenantRoleBinding(w http.ResponseWriter, r *http.Request) {
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
	recordBindingAudit(r, updated)
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) DeleteTenantRoleBinding(w http.ResponseWriter, r *http.Request) {
	tenant := r.PathValue("tenant")
	name := r.PathValue("name")
	existing, _ := h.op.Get(r.Context(), tenant, name)
	if err := h.op.Delete(r.Context(), tenant, name); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	recordBindingAudit(r, existing)
	w.WriteHeader(http.StatusNoContent)
}
