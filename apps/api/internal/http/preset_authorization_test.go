package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestConfigPresetsAreTenantScoped(t *testing.T) {
	router, db, _ := newTestRouter(t)
	ctx := context.Background()
	if err := db.SetSetting(ctx, domain.SettingKeyAllowRegistration, "true"); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(cookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	body := `{"name":"Private preset","providerKey":"terraria-vanilla","config":{},"modIds":[]}`
	cookies := []*http.Cookie{}
	presets := []domain.ConfigPreset{}
	for _, username := range []string{"presetalice", "presetbruce"} {
		register := httptest.NewRecorder()
		router.ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"`+username+`","password":"secret123"}`)))
		if register.Code != http.StatusCreated {
			t.Fatalf("register: %d %s", register.Code, register.Body.String())
		}
		cookie := authCookieFromRecorder(t, register)
		cookies = append(cookies, cookie)
		response := request(http.MethodPost, "/api/config-presets", body, cookie)
		var preset domain.ConfigPreset
		if err := json.Unmarshal(response.Body.Bytes(), &preset); err != nil || response.Code != http.StatusCreated || preset.OrganizationID == "" {
			t.Fatalf("create: %d %s %v", response.Code, response.Body.String(), err)
		}
		presets = append(presets, preset)
	}
	if presets[0].OrganizationID == presets[1].OrganizationID {
		t.Fatal("shared workspace")
	}
	legacy := domain.ConfigPreset{ID: "legacy-unowned", ProviderKey: domain.ProviderTerrariaVanilla}
	if err := db.CreateConfigPreset(ctx, &legacy); err != nil {
		t.Fatal(err)
	}
	for i, cookie := range cookies {
		response := request(http.MethodGet, "/api/config-presets", "", cookie)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), presets[i].ID) || strings.Contains(response.Body.String(), presets[1-i].ID) || strings.Contains(response.Body.String(), legacy.ID) {
			t.Fatalf("list: %d %s", response.Code, response.Body.String())
		}
		for _, id := range []string{presets[1-i].ID, legacy.ID} {
			for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
				response := request(method, "/api/config-presets/"+id, body, cookie)
				if response.Code != http.StatusNotFound {
					t.Fatalf("foreign %s: %d %s", method, response.Code, response.Body.String())
				}
			}
		}
	}
	foreignBody := strings.Replace(body, `"name":`, `"organizationId":"`+presets[1].OrganizationID+`","name":`, 1)
	if response := request(http.MethodPost, "/api/config-presets", foreignBody, cookies[0]); response.Code != http.StatusNotFound {
		t.Fatalf("foreign create: %d %s", response.Code, response.Body.String())
	}
	if response := request(http.MethodPut, "/api/config-presets/"+presets[0].ID, foreignBody, cookies[0]); response.Code != http.StatusBadRequest {
		t.Fatalf("ownership move: %d", response.Code)
	}
	mixed := `{"ids":["` + presets[0].ID + `","` + presets[1].ID + `"]}`
	response := request(http.MethodPost, "/api/config-presets/batch-delete", mixed, cookies[0])
	var batch configPresetBatchDeleteResult
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil || response.Code != http.StatusOK || len(batch.Succeeded) != 1 || len(batch.Failed) != 1 {
		t.Fatalf("batch: %d %s", response.Code, response.Body.String())
	}
	if _, err := db.GetConfigPreset(ctx, presets[1].ID); err != nil {
		t.Fatal("foreign preset deleted")
	}
	actor, err := db.GetAdminAccountByUsername(ctx, "presetbruce")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RemoveOrganizationMember(ctx, presets[1].OrganizationID, actor.ID); err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodGet, "/api/config-presets/"+presets[1].ID, "", cookies[1]); response.Code != http.StatusNotFound {
		t.Fatalf("revoked get: %d", response.Code)
	}
}
