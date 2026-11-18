package prewarmjob

import (
	"context"
	"encoding/json"
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	prewarmjobmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/prewarmjob"
)

type handler struct {
	op prewarmjobmodel.Operator
	mq mq.Interface
}

func newHandler(op prewarmjobmodel.Operator, bus mq.Interface) *handler {
	return &handler{op: op, mq: bus}
}

func (h *handler) ListPrewarmJobs(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(w, r, h.mq, mq.TopicPrewarmJob, "", "", opts, func(ctx context.Context, _, name string) (*corev1.PrewarmJob, error) {
			obj, err := h.op.Get(ctx, name)
			if err != nil {
				return nil, err
			}
			return redactPrewarmJobForNode(ctx, obj), nil
		}, apiruntime.WithWatchSource(h.op))
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	redactPrewarmJobListForNode(r.Context(), list)
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func redactPrewarmJobListForNode(ctx context.Context, list *metav1.List[*corev1.PrewarmJob]) {
	if list == nil {
		return
	}
	for i, item := range list.Items {
		list.Items[i] = redactPrewarmJobForNode(ctx, item)
	}
}

func redactPrewarmJobForNode(ctx context.Context, job *corev1.PrewarmJob) *corev1.PrewarmJob {
	if job == nil {
		return nil
	}
	u, ok := request.UserFrom(ctx)
	if !ok || u.Kind != request.KindNode {
		return job
	}
	cp := *job
	filtered := make([]corev1.PrewarmNodeStatus, 0, 1)
	for i := range job.Status.Nodes {
		if job.Status.Nodes[i].NodeName == u.Name {
			filtered = append(filtered, job.Status.Nodes[i])
		}
	}
	cp.Status.Nodes = filtered
	return &cp
}

func (h *handler) CreatePrewarmJob(w http.ResponseWriter, r *http.Request) {
	var job corev1.PrewarmJob
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.op.Create(r.Context(), &job)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetPrewarmJob(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, redactPrewarmJobForNode(r.Context(), obj))
}

func (h *handler) UpdatePrewarmJob(w http.ResponseWriter, r *http.Request) {
	var job corev1.PrewarmJob
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("name"), &job)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) UpdatePrewarmJobStatus(w http.ResponseWriter, r *http.Request) {
	var job corev1.PrewarmJob
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := r.PathValue("name")
	if u, ok := request.UserFrom(r.Context()); ok && u.Kind == request.KindNode {
		entry, ok := extractOwnNodeStatus(u.Name, job.Status.Nodes)
		if !ok {
			apiruntime.WriteError(w, http.StatusForbidden, "NodeRestriction: node "+u.Name+" may only report its own status.nodes entry")
			return
		}
		updated, err := h.op.ReportNodeStatus(r.Context(), name, u.Name, entry)
		if err != nil {
			apiruntime.WriteModelError(w, err)
			return
		}
		apiruntime.WriteJSON(w, http.StatusOK, redactPrewarmJobForNode(r.Context(), updated))
		return
	}
	updated, err := h.op.UpdateStatus(r.Context(), name, &job)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func extractOwnNodeStatus(nodeName string, nodes []corev1.PrewarmNodeStatus) (corev1.PrewarmNodeStatus, bool) {
	var own *corev1.PrewarmNodeStatus
	for i := range nodes {
		if nodes[i].NodeName == "" || nodes[i].NodeName == nodeName {
			if own != nil && nodes[i].NodeName != own.NodeName {
				return corev1.PrewarmNodeStatus{}, false
			}
			cp := nodes[i]
			cp.NodeName = nodeName
			own = &cp
			continue
		}
		return corev1.PrewarmNodeStatus{}, false
	}
	if own == nil {
		return corev1.PrewarmNodeStatus{}, false
	}
	return *own, true
}

func (h *handler) DeletePrewarmJob(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("name")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
