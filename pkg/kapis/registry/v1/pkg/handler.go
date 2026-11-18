package pkg

import (
	"encoding/json"
	"net/http"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	pkgmodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/pkg"
)

type handler struct {
	op pkgmodel.Operator
}

func (h *handler) ListPackages(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	for _, key := range []string{"repository", "q", "sort", "gameVersion", "resolve"} {
		if val := strings.TrimSpace(q.Get(key)); val != "" {
			if opts.FieldSelector == nil {
				opts.FieldSelector = make(map[string]string)
			}
			if _, exists := opts.FieldSelector[key]; !exists {
				opts.FieldSelector[key] = val
			}
		}
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) CreatePackage(w http.ResponseWriter, r *http.Request) {
	var in registryv1.Package
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.op.Create(r.Context(), &in)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), created.Metadata.Name)
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetPackage(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, obj)
}

func (h *handler) UpdatePackage(w http.ResponseWriter, r *http.Request) {
	var in registryv1.Package
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("name"), &in)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) PatchPackage(w http.ResponseWriter, r *http.Request) {
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
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) DeletePackage(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("name")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) GetPackageIcon(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data, ctype, err := h.op.FetchIcon(r.Context(), r.PathValue("name"), q.Get("u"), q.Get("sig"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=7200, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
