package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestOrganizationListDoesNotAdoptLegacyInstances(t *testing.T) {
	router, db, _ := newTestRouter(t)
	ctx := context.Background()
	legacy := domain.GameServer{ID: "unassigned-legacy"}
	if err := db.CreateGameServer(ctx, &legacy); err != nil {
		t.Fatal(err)
	}
	setup := httptest.NewRecorder()
	router.ServeHTTP(setup, httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(`{"username":"admin","password":"secret123"}`)))
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", setup.Code, setup.Body.String())
	}
	cookie := authCookieFromRecorder(t, setup)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations", nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Fatalf("read created workspace: %d %s", rec.Code, rec.Body.String())
		}
	}
	actual, err := db.GetGameServer(ctx, legacy.ID)
	if err != nil || actual.OrganizationID != "" {
		t.Fatalf("read adopted legacy instance: %+v %v", actual, err)
	}
	if _, err := db.GetTenantQuota(ctx, "default-org"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("read created quota: %v", err)
	}
}
