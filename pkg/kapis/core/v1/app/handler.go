package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	appmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/app"
)

type handler struct {
	op appmodel.Operator
	mq mq.Interface
}

func newHandler(op appmodel.Operator, bus ...mq.Interface) *handler {
	var q mq.Interface
	if len(bus) > 0 {
		q = bus[0]
	}
	return &handler{op: op, mq: q}
}

func (h *handler) ListApps(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(w, r, h.mq, mq.TopicApp, "", "", opts, func(ctx context.Context, _, name string) (*corev1.App, error) {
			return h.op.Get(ctx, name)
		}, apiruntime.WithWatchSource(h.op))
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	if u, ok := request.UserFrom(r.Context()); !ok || u.IsAnonymous() {
		filtered := make([]*corev1.App, 0, len(list.Items))
		for _, item := range list.Items {
			if item != nil && item.Published {
				filtered = append(filtered, sanitizePublicApp(item))
			}
		}
		list.Items = filtered
		list.Metadata.TotalCount = len(filtered)
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func sanitizePublicApp(in *corev1.App) *corev1.App {
	if in == nil {
		return nil
	}
	cp := *in
	cp.FileTemplates = nil
	cp.ConsoleActions = nil
	cp.ConsoleTargets = nil
	cp.Metrics = nil
	cp.ConfigSchema = nil
	cp.ConfigPresets = nil
	return &cp
}

func (h *handler) CreateApp(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var appObj *corev1.App
	if appmodel.IsPterodactylEgg(raw) {
		converted, err := appmodel.ConvertPterodactylEgg(raw)
		if err != nil {
			apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		appObj = converted
	} else {
		var app corev1.App
		if err := json.Unmarshal(raw, &app); err != nil {
			apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		appObj = &app
	}
	created, err := h.op.Create(r.Context(), appObj)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetApp(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	if u, ok := request.UserFrom(r.Context()); !ok || u.IsAnonymous() {
		if !obj.Published {
			apiruntime.WriteError(w, http.StatusNotFound, "app not found")
			return
		}
		obj = sanitizePublicApp(obj)
	}
	apiruntime.WriteJSON(w, http.StatusOK, obj)
}

func (h *handler) UpdateApp(w http.ResponseWriter, r *http.Request) {
	var app corev1.App
	if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("name"), &app)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) PatchApp(w http.ResponseWriter, r *http.Request) {
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

func (h *handler) DeleteApp(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("name")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
