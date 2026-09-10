package billingapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authentication"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/httpfilter"
)

type scopeResolver struct {
	scopes map[string]authorization.Scope
}

func (r scopeResolver) Resolve(_ context.Context, _ httpfilter.ResourceType, ids []string) (map[string]authorization.Scope, error) {
	result := make(map[string]authorization.Scope, len(ids))
	for _, id := range ids {
		if scope, ok := r.scopes[id]; ok {
			result[id] = scope
		}
	}
	return result, nil
}

func setupBillingRoutes(t *testing.T) (http.Handler, *authentication.SessionService, *authentication.TOTPService, *billing.Module, *billing.MemoryStore) {
	t.Helper()
	authStore := authentication.NewMemoryStore()
	sessions := authentication.NewSessionService(authStore, authentication.SessionPolicy{AbsoluteLifetime: 24 * time.Hour, IdleTimeout: time.Hour, ReauthWindow: 10 * time.Minute, SecureCookies: true})
	cipher, err := authentication.NewSecretCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	totp := authentication.NewTOTPService(authStore, cipher)
	bindings, err := authorization.NewMemoryStore([]authorization.RoleBinding{
		{ID: "rb_workspace", PrincipalID: "usr_owner", Role: authorization.RoleWorkspaceOwner, Scope: authorization.Scope{Type: authorization.ScopeWorkspace, ID: "ws_ember"}},
		{ID: "rb_platform", PrincipalID: "usr_platform", Role: authorization.RolePlatformAdmin, Scope: authorization.Scope{Type: authorization.ScopePlatform, ID: "platform"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := billing.NewMemoryStore()
	module := billing.New(store, []byte("funding-signature-test-key-long"))
	now := time.Now().UTC().Truncate(time.Second)
	catalog := billing.RegionCatalog{
		ID: "cat_asia_1", RegionID: "reg_asia", Version: 1,
		CPU: billing.Range{Minimum: 500, Maximum: 8000, Step: 500}, Memory: billing.Range{Minimum: 1024, Maximum: 16384, Step: 1024}, Disk: billing.Range{Minimum: 10, Maximum: 100, Step: 5},
		DedicatedIPAvailable: true, EndpointDeliveryModes: []string{"gateway"}, Availability: billing.AvailabilityAvailable, EffectiveAt: now.Add(-time.Hour), CreatedAt: now,
	}
	book := billing.PriceBook{ID: "pb_asia_1", RegionID: "reg_asia", Revision: 1, Currency: billing.CurrencyCNY, EffectiveAt: now.Add(-time.Hour), CreatedAt: now, UnitPrices: []billing.UnitPrice{
		{ResourceKind: billing.ResourceCPU, PriceMinor: 10, UnitQuantity: 1000, Unit: "1000-millicpu-hour"},
		{ResourceKind: billing.ResourceMemory, PriceMinor: 5, UnitQuantity: 1024, Unit: "gib-hour"},
		{ResourceKind: billing.ResourceInstanceDisk, PriceMinor: 1, UnitQuantity: 1, Unit: "gib-hour"},
		{ResourceKind: billing.ResourceBackupStorage, PriceMinor: 2, UnitQuantity: 1, Unit: "gib-hour"},
		{ResourceKind: billing.ResourceDedicatedIP, PriceMinor: 3, UnitQuantity: 1, Unit: "address-hour"},
	}}
	if err := module.PublishCatalog(context.Background(), catalog); err != nil {
		t.Fatal(err)
	}
	if err := module.PublishPriceBook(context.Background(), book); err != nil {
		t.Fatal(err)
	}
	services := Services{
		Billing: module, Sessions: sessions, Authorizer: authorization.NewEngine(bindings),
		Resolver: scopeResolver{scopes: map[string]authorization.Scope{
			"platform": {Type: authorization.ScopePlatform, ID: "platform"},
			"ws_ember": {Type: authorization.ScopeWorkspace, ID: "ws_ember"},
			"ws_other": {Type: authorization.ScopeWorkspace, ID: "ws_other"},
		}},
	}
	return Routes(services), sessions, totp, module, store
}

func billingRequest(handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.AddCookie(&http.Cookie{Name: authentication.SessionCookieName, Value: token})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestWorkspaceBillingRoutesUseScopeFilterAndServerCatalog(t *testing.T) {
	handler, sessions, _, _, _ := setupBillingRoutes(t)
	token, _, _ := sessions.Issue(context.Background(), "usr_owner", true)
	catalog := billingRequest(handler, http.MethodGet, "/v1/regions/reg_asia/catalog?workspaceId=ws_ember", "", token)
	if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), `"priceBookId":"pb_asia_1"`) || !strings.Contains(catalog.Body.String(), `"cpuMilli"`) {
		t.Fatalf("catalog status=%d body=%s", catalog.Code, catalog.Body.String())
	}
	quoteBody := `{"regionId":"reg_asia","providerReleaseId":"gpr_terraria","resourceSpec":{"cpuMilli":1000,"memoryMiB":1024,"diskGiB":10}}`
	quote := billingRequest(handler, http.MethodPost, "/v1/workspaces/ws_ember/quotes", quoteBody, token)
	if quote.Code != http.StatusOK || !strings.Contains(quote.Body.String(), `"estimatedHourlyMinor":25`) {
		t.Fatalf("quote status=%d body=%s", quote.Code, quote.Body.String())
	}
	missingProvider := billingRequest(handler, http.MethodPost, "/v1/workspaces/ws_ember/quotes", `{"regionId":"reg_asia","resourceSpec":{"cpuMilli":1000,"memoryMiB":1024,"diskGiB":10}}`, token)
	if missingProvider.Code != http.StatusBadRequest || !strings.Contains(missingProvider.Body.String(), "invalid_provider_release") {
		t.Fatalf("missing provider status=%d body=%s", missingProvider.Code, missingProvider.Body.String())
	}
	forbidden := billingRequest(handler, http.MethodPost, "/v1/workspaces/ws_other/quotes", quoteBody, token)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("cross-workspace quote status=%d", forbidden.Code)
	}
}

func TestPromotionalCreditRequiresPlatformRoleTOTPAndIdempotencyKey(t *testing.T) {
	handler, sessions, totp, _, store := setupBillingRoutes(t)
	token, _, _ := sessions.Issue(context.Background(), "usr_platform", true)
	body := `{"workspaceId":"ws_ember","currency":"CNY","amountMinor":10000,"reason":"Beta credit"}`
	if response := billingRequest(handler, http.MethodPost, "/v1/platform/credit-grants", body, token); response.Code != http.StatusForbidden {
		t.Fatalf("credit without TOTP status=%d", response.Code)
	}
	secret, err := totp.Enroll(context.Background(), "usr_platform")
	if err != nil {
		t.Fatal(err)
	}
	if err := totp.Verify(context.Background(), "usr_platform", totpCode(secret, time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	elevated, _, err := sessions.RotateAfterOperatorVerification(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	missingKey := billingRequest(handler, http.MethodPost, "/v1/platform/credit-grants", body, elevated)
	if missingKey.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency key status=%d", missingKey.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/platform/credit-grants", strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: authentication.SessionCookieName, Value: elevated})
	request.Header.Set("Idempotency-Key", "beta-grant-ember")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("credit status=%d body=%s", response.Code, response.Body.String())
	}
	replayRequest := httptest.NewRequest(http.MethodPost, "/v1/platform/credit-grants", strings.NewReader(body))
	replayRequest.AddCookie(&http.Cookie{Name: authentication.SessionCookieName, Value: elevated})
	replayRequest.Header.Set("Idempotency-Key", "beta-grant-ember")
	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, replayRequest)
	if replay.Code != http.StatusCreated {
		t.Fatalf("credit replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	entries, err := store.LedgerEntries(context.Background(), "ws_ember", 100)
	if err != nil || len(entries) != 1 {
		t.Fatalf("credit replay entries=%d err=%v", len(entries), err)
	}
}

func totpCode(secret string, now time.Time) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, uint64(now.Unix()/30))
	digest := hmac.New(sha1.New, key)
	_, _ = digest.Write(message)
	sum := digest.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}
