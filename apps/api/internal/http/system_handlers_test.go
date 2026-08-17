package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/systemupdate"
)

func TestAutomaticSystemUpdateCheckDoesNotRunAtStartup(t *testing.T) {
	var requests atomic.Int32
	manifest := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schemaVersion":1,"channel":"stable","version":"v9.9.9"}`))
	}))
	defer manifest.Close()

	_, db, cfg := newTestRouter(t)
	cfg.SystemUpdateInterval = time.Hour
	handler := &Handler{
		cfg:          cfg,
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		store:        db,
		systemUpdate: systemupdate.New(manifest.URL, "", "", time.Second),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		handler.runAutomaticSystemUpdateChecks(ctx)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if got := requests.Load(); got != 0 {
		t.Fatalf("automatic update check sent %d request(s) during startup", got)
	}
}

func TestMonitoringRoutesAreRegistered(t *testing.T) {
	router, _, _ := newTestRouter(t)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(stdhttp.MethodGet, "/api/monitoring/overview", nil))
	if response.Code == stdhttp.StatusNotFound {
		t.Fatalf("expected monitoring overview route to be registered, got 404")
	}
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("expected monitoring overview to return 200 before admin setup, got %d: %s", response.Code, response.Body.String())
	}
}

func TestObservabilityMetricsEndpoints(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("metrics-server", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.PlayersOnline = 3
	server.MaxPlayers = 8
	createTestServer(t, db, server)
	if err := db.CreateActivity(context.Background(), &domain.ActivityEvent{
		ID:         "metrics-activity",
		InstanceID: server.ID,
		Type:       "server.started",
		Message:    "Started server Metrics Server",
		CreatedAt:  time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	jsonRecorder := httptest.NewRecorder()
	router.ServeHTTP(jsonRecorder, httptest.NewRequest(stdhttp.MethodGet, "/api/observability/metrics", nil))
	if jsonRecorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected observability metrics 200, got %d: %s", jsonRecorder.Code, jsonRecorder.Body.String())
	}
	var payload struct {
		Servers []struct {
			ID            string `json:"id"`
			PlayersOnline int    `json:"playersOnline"`
		} `json:"servers"`
		Activity struct {
			Total     int `json:"total"`
			Lifecycle int `json:"lifecycle"`
		} `json:"activity"`
	}
	if err := json.Unmarshal(jsonRecorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Servers) != 1 || payload.Servers[0].ID != server.ID || payload.Servers[0].PlayersOnline != 3 {
		t.Fatalf("expected server metric in snapshot, got %+v", payload.Servers)
	}
	if payload.Activity.Total != 1 || payload.Activity.Lifecycle != 1 {
		t.Fatalf("expected lifecycle activity summary, got %+v", payload.Activity)
	}

	textRecorder := httptest.NewRecorder()
	router.ServeHTTP(textRecorder, httptest.NewRequest(stdhttp.MethodGet, "/metrics", nil))
	if textRecorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected prometheus metrics 200, got %d: %s", textRecorder.Code, textRecorder.Body.String())
	}
	if contentType := textRecorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") || !strings.Contains(contentType, "version=0.0.4") {
		t.Fatalf("expected prometheus text content type, got %q", contentType)
	}
	body := textRecorder.Body.String()
	for _, expected := range []string{
		"gamepanel_api_http_requests_total",
		"gamepanel_api_http_request_duration_seconds",
		"gamepanel_api_http_requests_in_flight",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected prometheus metrics to contain %q, got:\n%s", expected, body)
		}
	}
	for _, unexpected := range []string{
		"gamepanel_runtime_running_containers",
		"gamepanel_server_players_online",
	} {
		if strings.Contains(body, unexpected) {
			t.Fatalf("expected API scrape metrics to exclude runtime/business metric %q, got:\n%s", unexpected, body)
		}
	}

	observabilityRecorder := httptest.NewRecorder()
	router.ServeHTTP(observabilityRecorder, httptest.NewRequest(stdhttp.MethodGet, "/api/observability/prometheus", nil))
	if observabilityRecorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected observability prometheus 200, got %d: %s", observabilityRecorder.Code, observabilityRecorder.Body.String())
	}
	observabilityBody := observabilityRecorder.Body.String()
	for _, expected := range []string{
		"gamepanel_runtime_running_containers",
		"gamepanel_server_cpu_percent",
		"gamepanel_server_memory_bytes",
		"gamepanel_server_players_online",
	} {
		if !strings.Contains(observabilityBody, expected) {
			t.Fatalf("expected observability prometheus metrics to contain %q, got:\n%s", expected, observabilityBody)
		}
	}
}
