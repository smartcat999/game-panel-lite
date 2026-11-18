package blob

import (
	"encoding/json"
	"net/http"
	"strconv"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	blobmodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/blob"
)

type handler struct {
	op blobmodel.Operator
}

func (h *handler) ListBlobs(w http.ResponseWriter, r *http.Request) {
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

func (h *handler) RegisterBlob(w http.ResponseWriter, r *http.Request) {
	var in registryv1.Blob
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.op.Register(r.Context(), &in)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetBlob(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if r.URL.Query().Get("metadata") == "true" {
		b, err := h.op.Get(r.Context(), name)
		if err != nil {
			apiruntime.WriteModelError(w, err)
			return
		}
		apiruntime.WriteJSON(w, http.StatusOK, b)
		return
	}
	b, dlURL, _, err := h.op.PresignDownload(r.Context(), name, r.Host)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.Header().Set("Docker-Content-Digest", b.Digest)
	w.Header().Set("X-Content-Length", strconv.FormatInt(b.SizeBytes, 10))
	http.Redirect(w, r, dlURL, http.StatusTemporaryRedirect)
}

func (h *handler) DeleteBlob(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("name")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
