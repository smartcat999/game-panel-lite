package billingapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authentication"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/httpfilter"
)

type Services struct {
	Billing    *billing.Module
	Sessions   *authentication.SessionService
	Authorizer httpfilter.Authorizer
	Resolver   httpfilter.ScopeResolver
}

func Routes(services Services) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/regions/{regionId}/catalog", services.workspaceQueryAuthorized(authorization.ActionWorkspaceRead, http.HandlerFunc(services.getRegionCatalog)))
	mux.Handle("POST /v1/workspaces/{workspaceId}/quotes", services.workspaceAuthorized(authorization.ActionInstanceCreate, http.HandlerFunc(services.createQuote)))
	mux.Handle("GET /v1/workspaces/{workspaceId}/wallet", services.workspaceAuthorized(authorization.ActionBillingRead, http.HandlerFunc(services.getWallet)))
	mux.Handle("GET /v1/workspaces/{workspaceId}/ledger-entries", services.workspaceAuthorized(authorization.ActionBillingRead, http.HandlerFunc(services.listLedgerEntries)))
	mux.Handle("POST /v1/platform/credit-grants", services.platformAuthorized(authorization.ActionPlatformGrantCredit, http.HandlerFunc(services.grantPromotionalCredit)))
	mux.Handle("POST /v1/platform/price-books", services.platformAuthorized(authorization.ActionPlatformPriceWrite, http.HandlerFunc(services.publishPriceBook)))
	mux.Handle("POST /v1/platform/region-catalogs", services.platformAuthorized(authorization.ActionPlatformPriceWrite, http.HandlerFunc(services.publishRegionCatalog)))
	return mux
}

func WithFallback(billingRoutes, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if isBillingRoute(request.Method, request.URL.Path) {
			billingRoutes.ServeHTTP(response, request)
			return
		}
		fallback.ServeHTTP(response, request)
	})
}

func isBillingRoute(method, path string) bool {
	if method == http.MethodGet && strings.HasPrefix(path, "/v1/regions/") && strings.HasSuffix(path, "/catalog") {
		return true
	}
	if strings.HasPrefix(path, "/v1/workspaces/") {
		return strings.HasSuffix(path, "/quotes") || strings.HasSuffix(path, "/wallet") || strings.HasSuffix(path, "/ledger-entries")
	}
	return method == http.MethodPost && (path == "/v1/platform/credit-grants" || path == "/v1/platform/price-books" || path == "/v1/platform/region-catalogs")
}

func (s Services) authenticated(next http.Handler) http.Handler {
	return httpfilter.Authenticate(s.Sessions, next)
}

func (s Services) workspaceAuthorized(action authorization.Action, next http.Handler) http.Handler {
	policy := httpfilter.RoutePolicy{Action: action, ResourceType: httpfilter.ResourceWorkspace, ResourceIDs: httpfilter.PathID("workspaceId")}
	return s.authenticated(httpfilter.Authorize(s.Resolver, s.Authorizer, policy, next))
}

func (s Services) workspaceQueryAuthorized(action authorization.Action, next http.Handler) http.Handler {
	policy := httpfilter.RoutePolicy{Action: action, ResourceType: httpfilter.ResourceWorkspace, ResourceIDs: httpfilter.QueryIDs("workspaceId")}
	return s.authenticated(httpfilter.Authorize(s.Resolver, s.Authorizer, policy, next))
}

func (s Services) platformAuthorized(action authorization.Action, next http.Handler) http.Handler {
	next = httpfilter.RequireOperatorVerification(s.Sessions, next)
	policy := httpfilter.RoutePolicy{Action: action, ResourceType: httpfilter.ResourcePlatform, ResourceIDs: func(*http.Request) ([]string, error) { return []string{"platform"}, nil }}
	return s.authenticated(httpfilter.Authorize(s.Resolver, s.Authorizer, policy, next))
}

func (s Services) getRegionCatalog(response http.ResponseWriter, request *http.Request) {
	catalog, book, err := s.Billing.CatalogAndPriceBook(request.Context(), request.PathValue("regionId"))
	if err != nil {
		respondError(response, http.StatusNotFound, "region_catalog_not_found")
		return
	}
	respondJSON(response, http.StatusOK, map[string]any{
		"regionId":              catalog.RegionID,
		"currency":              billing.CurrencyCNY,
		"resourceBounds":        map[string]billing.Range{"cpuMilli": catalog.CPU, "memoryMiB": catalog.Memory, "diskGiB": catalog.Disk},
		"unitPrices":            book.UnitPrices,
		"capacityState":         catalog.Availability,
		"endpointDeliveryModes": catalog.EndpointDeliveryModes,
		"dedicatedIpAvailable":  catalog.DedicatedIPAvailable,
		"catalogVersion":        catalog.Version,
		"priceBookId":           book.ID,
	})
}

func (s Services) createQuote(response http.ResponseWriter, request *http.Request) {
	var body struct {
		RegionID          string               `json:"regionId"`
		ProviderReleaseID string               `json:"providerReleaseId"`
		ResourceSpec      billing.ResourceSpec `json:"resourceSpec"`
	}
	if !decode(response, request, &body) {
		return
	}
	if strings.TrimSpace(body.ProviderReleaseID) == "" {
		respondError(response, http.StatusBadRequest, "invalid_provider_release")
		return
	}
	quote, err := s.Billing.CreateQuote(request.Context(), request.PathValue("workspaceId"), body.RegionID, body.ResourceSpec, false)
	if err != nil {
		respondError(response, http.StatusBadRequest, "quote_not_created")
		return
	}
	respondJSON(response, http.StatusOK, map[string]any{
		"id": quote.ID, "workspaceId": quote.WorkspaceID, "regionId": quote.RegionID,
		"priceBookId": quote.PriceBookID, "resourceSpec": quote.ResourceSpec, "currency": quote.Currency,
		"estimatedHourlyMinor": quote.EstimatedHourlyMinor, "expiresAt": quote.ExpiresAt,
	})
}

func (s Services) getWallet(response http.ResponseWriter, request *http.Request) {
	wallet, err := s.Billing.Wallet(request.Context(), request.PathValue("workspaceId"))
	if err != nil {
		respondError(response, http.StatusInternalServerError, "wallet_unavailable")
		return
	}
	respondJSON(response, http.StatusOK, walletResponse(wallet))
}

func (s Services) listLedgerEntries(response http.ResponseWriter, request *http.Request) {
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	entries, err := s.Billing.LedgerEntries(request.Context(), request.PathValue("workspaceId"), limit)
	if err != nil {
		respondError(response, http.StatusInternalServerError, "ledger_unavailable")
		return
	}
	respondJSON(response, http.StatusOK, entries)
}

func (s Services) grantPromotionalCredit(response http.ResponseWriter, request *http.Request) {
	var body struct {
		WorkspaceID string `json:"workspaceId"`
		Currency    string `json:"currency"`
		AmountMinor int64  `json:"amountMinor"`
		Reason      string `json:"reason"`
	}
	if !decode(response, request, &body) {
		return
	}
	if body.Currency != billing.CurrencyCNY {
		respondError(response, http.StatusBadRequest, "unsupported_currency")
		return
	}
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		respondError(response, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	wallet, err := s.Billing.GrantPromotionalCredit(request.Context(), body.WorkspaceID, "credit:"+idempotencyKey, body.AmountMinor, body.Reason)
	if err != nil {
		respondError(response, http.StatusBadRequest, "credit_not_granted")
		return
	}
	respondJSON(response, http.StatusCreated, walletResponse(wallet))
}

func (s Services) publishPriceBook(response http.ResponseWriter, request *http.Request) {
	var book billing.PriceBook
	if !decode(response, request, &book) {
		return
	}
	if book.ID == "" {
		book.ID = fmt.Sprintf("pb_%s_%d", book.RegionID, book.Revision)
	}
	if book.CreatedAt.IsZero() {
		book.CreatedAt = time.Now().UTC()
	}
	if err := s.Billing.PublishPriceBook(request.Context(), book); err != nil {
		respondError(response, http.StatusBadRequest, "price_book_not_published")
		return
	}
	respondJSON(response, http.StatusCreated, book)
}

func (s Services) publishRegionCatalog(response http.ResponseWriter, request *http.Request) {
	var catalog billing.RegionCatalog
	if !decode(response, request, &catalog) {
		return
	}
	if catalog.ID == "" {
		catalog.ID = fmt.Sprintf("cat_%s_%d", catalog.RegionID, catalog.Version)
	}
	if catalog.CreatedAt.IsZero() {
		catalog.CreatedAt = time.Now().UTC()
	}
	if err := s.Billing.PublishCatalog(request.Context(), catalog); err != nil {
		respondError(response, http.StatusBadRequest, "region_catalog_not_published")
		return
	}
	respondJSON(response, http.StatusCreated, catalog)
}

func decode(response http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(response, request.Body, 128<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		respondError(response, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func respondJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func respondError(response http.ResponseWriter, status int, code string) {
	respondJSON(response, status, map[string]string{"error": code})
}

func walletResponse(wallet billing.Wallet) map[string]any {
	return map[string]any{
		"workspaceId": wallet.WorkspaceID, "currency": wallet.Currency,
		"availableMinor": wallet.AvailableMinor, "ledgerSequence": wallet.LedgerSequence, "state": wallet.State,
	}
}
