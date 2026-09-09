package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
