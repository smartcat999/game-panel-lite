package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMiddlewareUsesRoutePatternLabels(t *testing.T) {
	registry := NewRegistry()
	router := chi.NewRouter()
	router.Use(registry.Middleware)
	router.Get("/api/servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/servers/example-id", nil))
	body := registry.PrometheusText()
	if !strings.Contains(body, `route="/api/servers/{id}"`) {
		t.Fatalf("expected templated route label, got:\n%s", body)
	}
	if strings.Contains(body, "example-id") {
		t.Fatalf("route label leaked concrete id:\n%s", body)
	}
}
