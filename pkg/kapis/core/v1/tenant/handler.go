package tenant

import (
	"context"
	"encoding/json"
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	alertmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert"
	tenantmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/tenant"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

type handler struct {
	op       tenantmodel.Operator
	reg      *repo.Registry
	mq       mq.Interface
	sessions sessionmodel.Operator
}

func newHandler(op tenantmodel.Operator, reg *repo.Registry, bus ...mq.Interface) *handler {
	var q mq.Interface
	if len(bus) > 0 {
		q = bus[0]
	}
	return &handler{op: op, reg: reg, mq: q}
}

func (h *handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(
			w, r, h.mq, mq.TopicTenant, opts.Tenant, "", opts,
			func(ctx context.Context, _, name string) (*corev1.Tenant, error) {
				return h.op.Get(ctx, name)
			},
			apiruntime.WithWatchSource(h.op),
			apiruntime.WithWatchSharedObject(),
			apiruntime.WithWatchFilter(func(ctx context.Context, _ metav1.WatchEventType, ev mq.Event, obj any) bool {
				hint, _ := obj.(*corev1.Tenant)
				if hint == nil {
					if mo, ok := obj.(metav1.Object); ok && mo.GetObjectMeta() != nil {
						hint = &corev1.Tenant{Metadata: *mo.GetObjectMeta()}
					}
				}
				return h.op.CanView(ctx, ev.Name, hint, ev.Attributes["owner"])
			}),
		)
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var tenant corev1.Tenant
	if err := json.NewDecoder(r.Body).Decode(&tenant); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	userInfo, _ := request.UserFrom(r.Context())
	created, err := h.op.Create(r.Context(), userInfo.Name, &tenant)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), created.Metadata.Name)
	request.SetAuditTenantUID(r.Context(), created.Metadata.UID)
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetTenant(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("tenant"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, obj)
}

func (h *handler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	var tenant corev1.Tenant
	if err := json.NewDecoder(r.Body).Decode(&tenant); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	alertmodel.SyncTenantAlertConfig(nil, &tenant)
	updated, err := h.op.Update(r.Context(), r.PathValue("tenant"), &tenant)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) PatchTenant(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("tenant")
	current, err := h.op.Get(r.Context(), name)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	beforeSpecJSON, _ := json.Marshal(map[string]any{"spec": current.Spec})
	prevAnns := cloneAnnotations(current.Metadata.Annotations)

	patched, err := apiruntime.ApplyMergePatch(r, current)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	alertmodel.SyncTenantAlertConfig(current, patched)

	updated, err := h.op.Update(r.Context(), name, patched)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	recordTenantPatchAuditDiff(r.Context(), name, beforeSpecJSON, prevAnns, updated)
	alertmodel.EnqueueChangedWebhookTests(r.Context(), h.reg, h.mq, prevAnns, updated)
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func cloneAnnotations(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func recordTenantPatchAuditDiff(ctx context.Context, name string, beforeSpecJSON []byte, prevAnns map[string]string, updated *corev1.Tenant) {
	afterSpecJSON, _ := json.Marshal(map[string]any{"spec": updated.Spec})
	if string(beforeSpecJSON) != string(afterSpecJSON) {
		request.SetAuditTarget(ctx, "", name, map[string]string{
			"diffBefore": string(beforeSpecJSON),
			"diffAfter":  string(afterSpecJSON),
		})
		return
	}
	trackedKeys := []string{
		corev1.AnnotationPlatformAlertRules,
		corev1.AnnotationPlatformAlertChannels,
		corev1.AnnotationAlertChannels,
		corev1.AnnotationAlertWebhook,
	}
	for _, key := range trackedKeys {
		prevVal := prevAnns[key]
		newVal := updated.Metadata.Annotations[key]
		if prevVal != newVal {
			b, _ := json.Marshal(map[string]any{"annotations": map[string]string{key: prevVal}})
			a, _ := json.Marshal(map[string]string{key: newVal})
			aWrap, _ := json.Marshal(map[string]any{"annotations": json.RawMessage(a)})
			request.SetAuditTarget(ctx, "", name, map[string]string{"diffBefore": string(b), "diffAfter": string(aWrap)})
			return
		}
	}
	request.SetAuditTarget(ctx, "", name, map[string]string{
		"diffBefore": string(beforeSpecJSON),
		"diffAfter":  string(afterSpecJSON),
	})
}

func (h *handler) UpdateTenantStatus(w http.ResponseWriter, r *http.Request) {
	var tenant corev1.Tenant
	if err := json.NewDecoder(r.Body).Decode(&tenant); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.UpdateStatus(r.Context(), r.PathValue("tenant"), &tenant)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("tenant")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *handler) ListTenantQuotaChanges(w http.ResponseWriter, r *http.Request) {
	tenantName := r.PathValue("tenant")
	verify := r.URL.Query().Get("verify")
	if verify == "true" || verify == "1" {
		report, err := h.op.VerifyQuotaChanges(r.Context(), tenantName)
		if err != nil {
			apiruntime.WriteModelError(w, err)
			return
		}
		apiruntime.WriteJSON(w, http.StatusOK, report)
		return
	}
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := h.op.ListQuotaChanges(r.Context(), tenantName, opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) RedeemTenantCode(w http.ResponseWriter, r *http.Request) {
	var req corev1.RedeemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.op.Redeem(r.Context(), r.PathValue("tenant"), req)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), resp.Order.ID)
	request.SetAuditTenantUID(r.Context(), resp.Order.TenantUID)
	apiruntime.WriteJSON(w, http.StatusOK, resp)
}

func (h *handler) ListTenantOrders(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := h.op.ListOrders(r.Context(), r.PathValue("tenant"), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) CreateTenantOrder(w http.ResponseWriter, r *http.Request) {
	var req corev1.CreateQuotaOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	order, err := h.op.CreateOrder(r.Context(), r.PathValue("tenant"), req)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), order.ID)
	request.SetAuditTenantUID(r.Context(), order.TenantUID)
	apiruntime.WriteJSON(w, http.StatusCreated, order)
}

func (h *handler) PatchTenantOrder(w http.ResponseWriter, r *http.Request) {
	var req corev1.PatchQuotaOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	order, err := h.op.CancelOrder(r.Context(), r.PathValue("tenant"), r.PathValue("order"), req)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), order.ID)
	request.SetAuditTenantUID(r.Context(), order.TenantUID)
	apiruntime.WriteJSON(w, http.StatusOK, order)
}

func (h *handler) ReclaimTenant(w http.ResponseWriter, r *http.Request) {
	tenantName := r.PathValue("tenant")
	resp, err := h.op.ReclaimExpired(r.Context(), tenantName)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), tenantName)
	apiruntime.WriteJSON(w, http.StatusOK, resp)
}
