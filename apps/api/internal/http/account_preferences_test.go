package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountPreferencesAPI(t *testing.T) {
	router, _, _ := newTestRouter(t)
	setup := httptest.NewRecorder()
	setupRequest := httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	setupRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(setup, setupRequest)
	cookie := authCookieFromRecorder(t, setup)

	update := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/account/preferences", strings.NewReader(`{"locale":"en","theme":"dark"}`))
	updateRequest.Header.Set("Content-Type", "application/json")
	updateRequest.AddCookie(cookie)
	router.ServeHTTP(update, updateRequest)
	if update.Code != http.StatusOK || !strings.Contains(update.Body.String(), `"theme":"dark"`) {
		t.Fatalf("expected saved preferences, got %d: %s", update.Code, update.Body.String())
	}

	bootstrap := httptest.NewRecorder()
	bootstrapRequest := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap", nil)
	bootstrapRequest.AddCookie(cookie)
	router.ServeHTTP(bootstrap, bootstrapRequest)
	if bootstrap.Code != http.StatusOK || !strings.Contains(bootstrap.Body.String(), `"preferences":{"locale":"en","theme":"dark"}`) {
		t.Fatalf("expected preferences in bootstrap, got %d: %s", bootstrap.Code, bootstrap.Body.String())
	}

	invalid := httptest.NewRecorder()
	invalidRequest := httptest.NewRequest(http.MethodPut, "/api/account/preferences", strings.NewReader(`{"locale":"fr","theme":"neon"}`))
	invalidRequest.Header.Set("Content-Type", "application/json")
	invalidRequest.AddCookie(cookie)
	router.ServeHTTP(invalid, invalidRequest)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid preferences to be rejected, got %d: %s", invalid.Code, invalid.Body.String())
	}
}
