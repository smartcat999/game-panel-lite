package upstream

import (
	"encoding/json"
	"net/http"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	upstreammodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/upstream"
)

type handler struct {
	op upstreammodel.Operator
}

func sanitizeUpstream(u *registryv1.Upstream) *registryv1.Upstream {
	if u == nil {
		return nil
	}
	cp := *u
	cp.APIKey = ""
	return &cp
}

func (h *handler) ListUpstreams(w http.ResponseWriter, r *http.Request) {
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
	for i := range list.Items {
		list.Items[i] = sanitizeUpstream(list.Items[i])
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) CreateUpstream(w http.ResponseWriter, r *http.Request) {
	var in registryv1.Upstream
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.op.Create(r.Context(), &in)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusCreated, sanitizeUpstream(created))
}

func (h *handler) GetUpstream(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, sanitizeUpstream(obj))
}

func (h *handler) UpdateUpstream(w http.ResponseWriter, r *http.Request) {
	var in registryv1.Upstream
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("name"), &in)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, sanitizeUpstream(updated))
}

func (h *handler) PatchUpstream(w http.ResponseWriter, r *http.Request) {
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
	apiruntime.WriteJSON(w, http.StatusOK, sanitizeUpstream(updated))
}

func (h *handler) DeleteUpstream(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("name")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
