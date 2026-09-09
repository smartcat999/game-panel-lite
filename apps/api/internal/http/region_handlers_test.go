package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regionstatus"
)

func TestRegionDirectoryDoesNotInferRegionsFromNodes(t *testing.T) {
	router, db, _ := newTestRouter(t)
	ctx := context.Background()
	if _, err := db.RegisterRegion(ctx, "region-east", "East"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/regions?limit=1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"region-east"`) || strings.Contains(rec.Body.String(), `nodeCount`) {
		t.Fatalf("region directory response: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/regions?limit=101", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unbounded region query returned %d", rec.Code)
	}
}

func TestRegionStatusIsAPlatformProjection(t *testing.T) {
	router, db, _ := newTestRouter(t)
	ctx := context.Background()
	if _, err := db.RegisterRegion(ctx, "status-east", "Status East"); err != nil {
		t.Fatal(err)
	}
	admin := domain.AdminAccount{ID: "status-admin", Username: "status-admin", Role: domain.RoleAdmin}
	if err := db.CreateAdminAccount(ctx, &admin); err != nil {
		t.Fatal(err)
	}
	session := domain.Session{ID: "status-session", AccountID: admin.ID, TokenHash: hashSessionToken("status-token"), ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.CreateSession(ctx, &session); err != nil {
		t.Fatal(err)
	}
	request := func(path string, authenticated bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if authenticated {
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "status-token"})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if got := request("/api/regions/status-east/status", false); got.Code != http.StatusUnauthorized {
		t.Fatalf("public status returned %d", got.Code)
	}
	if got := request("/api/regions/status-east/status", true); got.Code != http.StatusNoContent {
		t.Fatalf("missing projection returned %d", got.Code)
	}
	snapshot := regionstatus.Snapshot{SchemaVersion: 1, EventID: "status-event", RegionID: "status-east", Sequence: 1, ObservedAtMS: 10}
	if err := db.RecordRegionStatus(ctx, "status-east", snapshot); err != nil {
		t.Fatal(err)
	}
	got := request("/api/regions/status-east/status", true)
	var body regionstatus.Snapshot
	if json.Unmarshal(got.Body.Bytes(), &body) != nil || got.Code != http.StatusOK || body != snapshot || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status response %d %s", got.Code, got.Body.String())
	}
	if got := request("/api/regions/missing/status", true); got.Code != http.StatusNotFound {
		t.Fatalf("missing Region returned %d", got.Code)
	}
}
