package resourceplan

import (
	"context"
	"encoding/json"
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	resourceplanmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/resourceplan"
)

type handler struct {
	op resourceplanmodel.Operator
	mq mq.Interface
}

func newHandler(op resourceplanmodel.Operator, bus ...mq.Interface) *handler {
	var q mq.Interface
	if len(bus) > 0 {
		q = bus[0]
	}
	return &handler{op: op, mq: q}
}

func (h *handler) ListResourcePlans(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(w, r, h.mq, mq.TopicResourcePlan, "", "", opts, func(ctx context.Context, _, name string) (*corev1.ResourcePlan, error) {
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

func (h *handler) CreateResourcePlan(w http.ResponseWriter, r *http.Request) {
	var plan corev1.ResourcePlan
	if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.op.Create(r.Context(), &plan)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), created.Metadata.Name)
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetResourcePlan(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("resourceplan"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, obj)
}

func (h *handler) UpdateResourcePlan(w http.ResponseWriter, r *http.Request) {
	var plan corev1.ResourcePlan
	if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("resourceplan"), &plan)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) PatchResourcePlan(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("resourceplan")
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

func (h *handler) DeleteResourcePlan(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("resourceplan")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
