package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestWorkloadReconcileMetrics(t *testing.T) {
	registry := NewRegistry()
	registry.ObserveAgentHeartbeat("node-1", time.Unix(100, 0))
	registry.SetWorkloadBacklog("node-1", 2)
	registry.ObserveWorkloadReconcile("node-1", "server-1", 250*time.Millisecond, 1, true)

	body := registry.PrometheusText()
	for _, expected := range []string{
		`gamepanel_worker_last_heartbeat_timestamp_seconds{node_id="node-1"} 100`,
		`gamepanel_worker_reconcile_backlog{node_id="node-1"} 2`,
		`gamepanel_worker_reconcile_duration_seconds_count{node_id="node-1"} 1`,
		`gamepanel_worker_reconcile_failures_total{node_id="node-1"} 1`,
		`gamepanel_worker_generation_lag{node_id="node-1",server_id="server-1"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected worker metric %q, got:\n%s", expected, body)
		}
	}
}

func TestMiddlewarePreservesFlusher(t *testing.T) {
	registry := NewRegistry()
	router := chi.NewRouter()
	router.Use(registry.Middleware)
	router.Get("/api/servers/{id}/watch", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			http.Error(w, "streaming is not supported", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/servers/example-id/watch", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected middleware to preserve flusher, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
