package user

import (
	"context"
	"encoding/json"
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	usermodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/user"
)

type handler struct {
	op usermodel.Operator
	mq mq.Interface
}

func newHandler(op usermodel.Operator, bus ...mq.Interface) *handler {
	var q mq.Interface
	if len(bus) > 0 {
		q = bus[0]
	}
	return &handler{op: op, mq: q}
}

func (h *handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(w, r, h.mq, mq.TopicUser, "", "", opts, func(ctx context.Context, _, name string) (*iamv1.User, error) {
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

func (h *handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var user iamv1.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.op.Create(r.Context(), &user)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), created.Metadata.Name)
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetUser(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	u, err := h.op.Get(r.Context(), name)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, u)
}

func (h *handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var incoming iamv1.User
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if userInfo, ok := request.UserFrom(r.Context()); ok && userInfo.Name == name {
		if current, err := h.op.Get(r.Context(), name); err == nil && current != nil {
			incoming.Spec.State = current.Spec.State
		}
		incoming.Spec.PasswordHash = ""
	}
	updated, err := h.op.Update(r.Context(), name, &incoming)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, sanitizeUser(updated))
}

func (h *handler) PatchUser(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	current, err := h.op.Get(r.Context(), name)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	origState := current.Spec.State
	patched, err := apiruntime.ApplyMergePatch(r, current)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if userInfo, ok := request.UserFrom(r.Context()); ok && userInfo.Name == name {
		patched.Spec.State = origState
		patched.Spec.PasswordHash = ""
	}
	updated, err := h.op.Update(r.Context(), name, patched)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, sanitizeUser(updated))
}

func sanitizeUser(u *iamv1.User) *iamv1.User {
	if u == nil {
		return nil
	}
	cloned := *u
	cloned.Spec.CurrentPassword = ""
	cloned.Spec.Password = ""
	cloned.Spec.PasswordHash = ""
	return &cloned
}

func (h *handler) UpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	var incoming iamv1.User
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.UpdateStatus(r.Context(), r.PathValue("name"), &incoming)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("name")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *handler) ListUserMessages(w http.ResponseWriter, r *http.Request) {
	list, err := h.op.ListMessages(r.Context(), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) MutateUserMessages(w http.ResponseWriter, r *http.Request) {
	var act iamv1.UserMessageAction
	if err := json.NewDecoder(r.Body).Decode(&act); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := h.op.MutateMessages(r.Context(), r.PathValue("name"), &act)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}
