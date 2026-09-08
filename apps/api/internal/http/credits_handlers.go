package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type topUpRequest struct {
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
}

type userCreditsResponse struct {
	OrganizationID   string                     `json:"organizationId"`
	OrganizationName string                     `json:"organizationName"`
	Credits          int64                      `json:"credits"`
	Transactions     []domain.CreditTransaction `json:"transactions"`
}

// adminTopUpCredits handles manual admin quota/credit top-up: POST /api/organizations/{id}/topup
func (h *Handler) adminTopUpCredits(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "organization id is required")
		return
	}

	var req topUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "amount must be greater than zero")
		return
	}
	if req.Description == "" {
		req.Description = "Admin manual top-up"
	}

	account, ok := accountFromContext(r.Context())
	operatorID := ""
	if ok {
		operatorID = account.ID
	}

	tx, err := h.store.TopUpCredits(r.Context(), orgID, req.Amount, req.Description, operatorID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "organization not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to top up credits: "+err.Error())
		return
	}

	h.recordActivity(r.Context(), orgID, "credits.topup", "Admin topped up credits", map[string]any{
		"organizationId": orgID,
		"amount":         req.Amount,
		"balanceAfter":   tx.BalanceAfter,
	})

	writeJSON(w, http.StatusOK, tx)
}

// getUserCredits handles getting credits for the current user's active/personal organization: GET /api/user/credits
func (h *Handler) getUserCredits(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orgID := r.URL.Query().Get("organizationId")
	var org domain.Organization

	if orgID != "" {
		o, err := h.store.GetUserOrganization(r.Context(), account.ID, orgID)
		if err != nil {
			writeError(w, http.StatusNotFound, "organization not found")
			return
		}
		org = o
	} else {
		// Default to user's personal organization
		orgs, err := h.store.ListUserOrganizations(r.Context(), account.ID)
		if err != nil || len(orgs) == 0 {
			writeError(w, http.StatusInternalServerError, "failed to resolve user organization")
			return
		}
		org = orgs[0]
	}

	limit := 20
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	txs, err := h.store.ListCreditTransactions(r.Context(), org.ID, limit)
	if err != nil {
		txs = []domain.CreditTransaction{}
	}

	writeJSON(w, http.StatusOK, userCreditsResponse{
		OrganizationID:   org.ID,
		OrganizationName: org.Name,
		Credits:          org.Credits,
		Transactions:     txs,
	})
}

// getOrganizationCredits handles admin fetching organization credits: GET /api/organizations/{id}/credits
func (h *Handler) getOrganizationCredits(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "organization id is required")
		return
	}

	credits, err := h.store.GetOrganizationCredits(r.Context(), orgID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "organization not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	txs, err := h.store.ListCreditTransactions(r.Context(), orgID, 50)
	if err != nil {
		txs = []domain.CreditTransaction{}
	}

	writeJSON(w, http.StatusOK, userCreditsResponse{
		OrganizationID: orgID,
		Credits:        credits,
		Transactions:   txs,
	})
}
