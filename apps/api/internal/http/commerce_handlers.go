package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/paymentingress"
)

type createOrderRequest struct {
	OrganizationID string `json:"organizationId"`
	ServerID       string `json:"serverId"`
	PlanID         string `json:"planId"`
	PlanVersion    int64  `json:"planVersion"`
	Periods        int64  `json:"periods"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type cancelOrderRequest struct {
	Reason string `json:"reason"`
}

// GET /api/commerce/plans
func (h *Handler) listCommercePlans(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	plans, err := h.store.ListAvailablePrepaidPlans(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list plans: "+err.Error())
		return
	}
	if plans == nil {
		plans = []commerce.PlanVersion{}
	}
	if providerKey := r.URL.Query().Get("providerKey"); providerKey != "" {
		filtered := make([]commerce.PlanVersion, 0, len(plans))
		for _, p := range plans {
			if p.ProviderKey == providerKey {
				filtered = append(filtered, p)
			}
		}
		plans = filtered
	}
	writeJSON(w, http.StatusOK, plans)
}

// POST /api/commerce/orders
func (h *Handler) createCommerceOrder(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = uuid.NewString()
	}
	if req.Periods <= 0 {
		req.Periods = 1
	}
	orgID := req.OrganizationID
	if orgID == "" {
		orgs, err := h.store.ListUserOrganizations(r.Context(), account.ID)
		if err != nil || len(orgs) == 0 {
			writeError(w, http.StatusBadRequest, "organization required")
			return
		}
		orgID = orgs[0].ID
	} else {
		if _, err := h.store.GetUserOrganization(r.Context(), account.ID, orgID); err != nil {
			writeError(w, http.StatusForbidden, "not a member of specified organization")
			return
		}
	}

	orderReq := commerce.OrderRequest{
		OrganizationID: orgID,
		ServerID:       req.ServerID,
		PlanID:         req.PlanID,
		PlanVersion:    req.PlanVersion,
		Periods:        req.Periods,
		IdempotencyKey: req.IdempotencyKey,
	}

	order, err := h.store.CreatePrepaidOrder(r.Context(), account.ID, orderReq, 30*time.Minute)
	if err != nil {
		if errors.Is(err, commerce.ErrPlanUnavailable) || errors.Is(err, commerce.ErrInvalidPlan) {
			writeError(w, http.StatusBadRequest, "plan is not available for purchase: "+err.Error())
			return
		}
		if errors.Is(err, commerce.ErrOrderConflict) {
			writeError(w, http.StatusConflict, "order parameter conflict with existing idempotency key")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create order: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

// POST /api/commerce/orders/{id}/cancel
func (h *Handler) cancelCommerceOrder(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	orderID := chi.URLParam(r, "id")
	if orderID == "" {
		writeError(w, http.StatusBadRequest, "order id required")
		return
	}
	var req cancelOrderRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	orgs, err := h.store.ListUserOrganizations(r.Context(), account.ID)
	if err != nil || len(orgs) == 0 {
		writeError(w, http.StatusForbidden, "organization membership required")
		return
	}

	var cancelled commerce.Order
	var cancelErr error
	for _, o := range orgs {
		cancelled, cancelErr = h.store.CancelPrepaidOrder(r.Context(), account.ID, o.ID, orderID)
		if cancelErr == nil {
			break
		}
	}
	if cancelErr != nil {
		writeError(w, http.StatusBadRequest, "failed to cancel order: "+cancelErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, cancelled)
}

// GET /api/commerce/subscriptions
func (h *Handler) listCommerceSubscriptions(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	orgID := r.URL.Query().Get("organizationId")
	if orgID == "" {
		orgs, err := h.store.ListUserOrganizations(r.Context(), account.ID)
		if err != nil || len(orgs) == 0 {
			writeError(w, http.StatusBadRequest, "no organization found for user")
			return
		}
		orgID = orgs[0].ID
	} else {
		if _, err := h.store.GetUserOrganization(r.Context(), account.ID, orgID); err != nil {
			writeError(w, http.StatusForbidden, "not a member of specified organization")
			return
		}
	}

	subs, err := h.store.ListOrganizationSubscriptions(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list subscriptions: "+err.Error())
		return
	}
	if subs == nil {
		subs = []commerce.Subscription{}
	}
	writeJSON(w, http.StatusOK, subs)
}

// POST /api/commerce/payments/webhook
func (h *Handler) handlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	if h.paymentCaptures == nil {
		writeError(w, http.StatusServiceUnavailable, "payment provider is not configured")
		return
	}
	const maxNotificationBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(r.Body, maxNotificationBytes+1))
	if err != nil || len(body) == 0 || len(body) > maxNotificationBytes {
		writeError(w, http.StatusBadRequest, "invalid payment notification")
		return
	}
	defer clear(body)
	headers := make(map[string][]string, len(r.Header))
	for name, values := range r.Header {
		headers[name] = append([]string(nil), values...)
	}
	receipt, err := h.paymentCaptures.Capture(r.Context(), paymentingress.Notification{Body: body, Headers: headers})
	if err != nil {
		switch {
		case errors.Is(err, paymentingress.ErrInvalidNotification):
			writeError(w, http.StatusBadRequest, "invalid payment notification")
		case errors.Is(err, paymentingress.ErrVerificationFailed):
			writeError(w, http.StatusUnauthorized, "payment notification verification failed")
		case errors.Is(err, commerce.ErrPaymentConflict), errors.Is(err, commerce.ErrOrderUnavailable):
			writeError(w, http.StatusConflict, "payment notification conflicts with existing records")
		default:
			writeError(w, http.StatusServiceUnavailable, "payment notification could not be recorded")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"receipt": receipt,
	})
}

// GET /api/operations/{id}
func (h *Handler) getOperationStatus(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	opID := chi.URLParam(r, "id")
	if opID == "" {
		writeError(w, http.StatusBadRequest, "operation id required")
		return
	}
	orgs, err := h.store.ListUserOrganizations(r.Context(), account.ID)
	if err != nil || len(orgs) == 0 {
		writeError(w, http.StatusForbidden, "organization membership required")
		return
	}
	for _, o := range orgs {
		op, err := h.store.GetServerOperation(r.Context(), o.ID, opID)
		if err == nil {
			writeJSON(w, http.StatusOK, op)
			return
		}
	}
	writeError(w, http.StatusNotFound, "operation not found")
}
