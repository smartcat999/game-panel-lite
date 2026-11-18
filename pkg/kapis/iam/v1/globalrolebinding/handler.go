package globalrolebinding

import (
	"context"
	"encoding/json"
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	globalrolebindingmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/globalrolebinding"
)

type handler struct {
	op globalrolebindingmodel.Operator
	mq mq.Interface
}

func newHandler(op globalrolebindingmodel.Operator, bus ...mq.Interface) *handler {
	var q mq.Interface
	if len(bus) > 0 {
		q = bus[0]
	}
	return &handler{op: op, mq: q}
}

func (h *handler) ListGlobalRoleBindings(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(w, r, h.mq, mq.TopicGlobalRoleBinding, "", "", opts, func(ctx context.Context, _, name string) (*iamv1.GlobalRoleBinding, error) {
			return h.op.Get(ctx, name)
		}, apiruntime.WithWatchSource(h.op))
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func recordGlobalBindingAudit(r *http.Request, b *iamv1.GlobalRoleBinding) {
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

func (h *handler) CreateGlobalRoleBinding(w http.ResponseWriter, r *http.Request) {
	var binding iamv1.GlobalRoleBinding
	if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.op.Create(r.Context(), &binding)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	recordGlobalBindingAudit(r, created)
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetGlobalRoleBinding(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, obj)
}

func (h *handler) UpdateGlobalRoleBinding(w http.ResponseWriter, r *http.Request) {
	var binding iamv1.GlobalRoleBinding
	if err := json.NewDecoder(r.Body).Decode(&binding); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("name"), &binding)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	recordGlobalBindingAudit(r, updated)
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) PatchGlobalRoleBinding(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
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
	recordGlobalBindingAudit(r, updated)
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) DeleteGlobalRoleBinding(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	existing, _ := h.op.Get(r.Context(), name)
	if err := h.op.Delete(r.Context(), name); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	recordGlobalBindingAudit(r, existing)
	w.WriteHeader(http.StatusNoContent)
}
