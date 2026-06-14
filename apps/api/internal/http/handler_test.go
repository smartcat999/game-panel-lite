package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	worldsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/world"
)

func newTestRouter(t *testing.T) (stdhttp.Handler, *store.Store, config.Config) {
	return newTestRouterWithAdapter(t, availableMockAdapter{MockAdapter: runtime.NewMockAdapter()})
}

func newMultipartFileRequest(t *testing.T, method string, target string, field string, fileName string, content []byte) *stdhttp.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(field, fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, target, body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func newTestRouterWithAdapter(t *testing.T, adapter runtime.Adapter) (stdhttp.Handler, *store.Store, config.Config) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{
		Host:       "127.0.0.1",
		Port:       "4000",
		DataDir:    filepath.Join(root, "data"),
		DBPath:     filepath.Join(root, "gamepanel.db"),
		DockerHost: "unix:///initial.sock",
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	runtimeAdapter := runtime.NewSwitchableAdapter(adapter)
	monitor := runtime.NewDockerMonitor(runtimeAdapter)
	monitor.Refresh(context.Background())
	handler := NewHandler(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		db,
		provider.NewRegistry(terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider()),
		runtimeAdapter,
		monitor,
		func(string) (runtime.Adapter, error) { return runtime.NewMockAdapter(), nil },
	)
	router := chi.NewRouter()
	handler.Register(router)
	return router, db, cfg
}

type availableMockAdapter struct {
	*runtime.MockAdapter
}

func (a availableMockAdapter) Check(context.Context) runtime.DockerStatus {
	return runtime.DockerStatus{Available: true, Message: "ok", Host: "mock"}
}

func TestCorsAllowsPatchPreflight(t *testing.T) {
	router, _, _ := newTestRouter(t)
	request := httptest.NewRequest(stdhttp.MethodOptions, "/api/servers/server-1/mods/mod-1", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != stdhttp.StatusNoContent {
		t.Fatalf("expected CORS preflight 204, got %d", recorder.Code)
	}
	if methods := recorder.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, "PATCH") {
		t.Fatalf("expected PATCH in allowed methods, got %q", methods)
	}
}

type inspectStatusAdapter struct {
	status domain.ServerStatus
}

func (a inspectStatusAdapter) Check(context.Context) runtime.DockerStatus {
	return runtime.DockerStatus{Available: true, Message: "ok", Host: "mock"}
}
func (a inspectStatusAdapter) Create(context.Context, runtime.ContainerSpec) (string, error) {
	return "created-container", nil
}
func (a inspectStatusAdapter) Start(context.Context, domain.GameServerInstance) error   { return nil }
func (a inspectStatusAdapter) Stop(context.Context, domain.GameServerInstance) error    { return nil }
func (a inspectStatusAdapter) Restart(context.Context, domain.GameServerInstance) error { return nil }
func (a inspectStatusAdapter) Remove(context.Context, domain.GameServerInstance) error  { return nil }
func (a inspectStatusAdapter) Inspect(context.Context, domain.GameServerInstance) (domain.ServerStatus, error) {
	return a.status, nil
}
func (a inspectStatusAdapter) Stats(context.Context, domain.GameServerInstance) (runtime.ContainerStats, error) {
	return runtime.ContainerStats{}, nil
}
func (a inspectStatusAdapter) Logs(context.Context, domain.GameServerInstance) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (a inspectStatusAdapter) SendCommand(context.Context, domain.GameServerInstance, string) error {
	return nil
}

type unavailableInspectAdapter struct {
	inspectCalls int
}

func (a *unavailableInspectAdapter) Check(context.Context) runtime.DockerStatus {
	return runtime.DockerStatus{Available: false, Message: "docker unavailable", Host: "mock"}
}
func (a *unavailableInspectAdapter) Create(context.Context, runtime.ContainerSpec) (string, error) {
	return "", fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) Start(context.Context, domain.GameServerInstance) error {
	return fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) Stop(context.Context, domain.GameServerInstance) error {
	return fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) Restart(context.Context, domain.GameServerInstance) error {
	return fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) Remove(context.Context, domain.GameServerInstance) error {
	return fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) Inspect(context.Context, domain.GameServerInstance) (domain.ServerStatus, error) {
	a.inspectCalls++
	return domain.StatusErrored, fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) Stats(context.Context, domain.GameServerInstance) (runtime.ContainerStats, error) {
	return runtime.ContainerStats{}, fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) Logs(context.Context, domain.GameServerInstance) (io.ReadCloser, error) {
	return nil, fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) SendCommand(context.Context, domain.GameServerInstance, string) error {
	return fmt.Errorf("docker unavailable")
}

type staleContainerAdapter struct {
	created          int
	startedContainer string
	stoppedContainer string
	commandContainer string
	logsContainer    string
}

func (a *staleContainerAdapter) Check(context.Context) runtime.DockerStatus {
	return runtime.DockerStatus{Available: true, Message: "ok", Host: "mock"}
}
func (a *staleContainerAdapter) Create(context.Context, runtime.ContainerSpec) (string, error) {
	a.created++
	return "new-container", nil
}
func (a *staleContainerAdapter) Start(_ context.Context, instance domain.GameServerInstance) error {
	a.startedContainer = instance.ContainerID
	return nil
}
func (a *staleContainerAdapter) Stop(_ context.Context, instance domain.GameServerInstance) error {
	a.stoppedContainer = instance.ContainerID
	if instance.ContainerID == "old-container" {
		return fmt.Errorf("stale container used for stop")
	}
	return nil
}
func (a *staleContainerAdapter) Restart(context.Context, domain.GameServerInstance) error { return nil }
func (a *staleContainerAdapter) Remove(context.Context, domain.GameServerInstance) error  { return nil }
func (a *staleContainerAdapter) Inspect(_ context.Context, instance domain.GameServerInstance) (domain.ServerStatus, error) {
	if instance.ContainerID == "old-container" {
		return domain.StatusErrored, fmt.Errorf("stale container")
	}
	return domain.StatusRunning, nil
}
func (a *staleContainerAdapter) Stats(context.Context, domain.GameServerInstance) (runtime.ContainerStats, error) {
	return runtime.ContainerStats{}, nil
}
func (a *staleContainerAdapter) Logs(_ context.Context, instance domain.GameServerInstance) (io.ReadCloser, error) {
	a.logsContainer = instance.ContainerID
	if instance.ContainerID != "new-container" {
		return nil, fmt.Errorf("stale container used for logs")
	}
	return io.NopCloser(strings.NewReader("[Info] recovered log\n")), nil
}
func (a *staleContainerAdapter) SendCommand(_ context.Context, instance domain.GameServerInstance, _ string) error {
	a.commandContainer = instance.ContainerID
	if instance.ContainerID != "new-container" {
		return fmt.Errorf("stale container used for command")
	}
	return nil
}

func TestServerLifecycleAndLogEndpoints(t *testing.T) {
	router, _, _ := newTestRouter(t)
	createPayload := `{
		"name":"Vanilla Test",
		"providerKey":"terraria-vanilla",
		"config":{
			"serverName":"Vanilla Test",
			"worldName":"TestWorld",
			"worldSize":"medium",
			"difficulty":"classic",
			"maxPlayers":8,
			"port":7777,
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(createPayload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server 201, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServerInstance
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.ID == "" || server.ContainerID != "" || server.ProviderKey != domain.ProviderTerrariaVanilla {
		t.Fatalf("expected created vanilla server record without fixed container, got %+v", server)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusOK {
		t.Fatalf("expected start 200, got %d: %s", start.Code, start.Body.String())
	}
	var started domain.GameServerInstance
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Status != domain.StatusRunning || started.ContainerID == "" {
		t.Fatalf("expected start to create runtime container and mark running, got %+v", started)
	}

	command := httptest.NewRecorder()
	router.ServeHTTP(command, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/command", bytes.NewBufferString(`{"command":"say hello"}`)))
	if command.Code != stdhttp.StatusOK {
		t.Fatalf("expected command 200, got %d: %s", command.Code, command.Body.String())
	}

	restart := httptest.NewRecorder()
	router.ServeHTTP(restart, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/restart", nil))
	if restart.Code != stdhttp.StatusOK {
		t.Fatalf("expected restart 200, got %d: %s", restart.Code, restart.Body.String())
	}
	var restarted domain.GameServerInstance
	if err := json.Unmarshal(restart.Body.Bytes(), &restarted); err != nil {
		t.Fatal(err)
	}
	if restarted.Status != domain.StatusRunning {
		t.Fatalf("expected restart running, got %+v", restarted)
	}

	stop := httptest.NewRecorder()
	router.ServeHTTP(stop, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/stop", nil))
	if stop.Code != stdhttp.StatusOK {
		t.Fatalf("expected stop 200, got %d: %s", stop.Code, stop.Body.String())
	}
	var stopped domain.GameServerInstance
	if err := json.Unmarshal(stop.Body.Bytes(), &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Status != domain.StatusStopped {
		t.Fatalf("expected stop status stopped, got %+v", stopped)
	}

	logs := httptest.NewRecorder()
	router.ServeHTTP(logs, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/logs", nil))
	if logs.Code != stdhttp.StatusOK {
		t.Fatalf("expected logs 200, got %d: %s", logs.Code, logs.Body.String())
	}
	if got := logs.Body.String(); !bytes.Contains([]byte(got), []byte("event: log")) || !bytes.Contains([]byte(got), []byte("Mock Terraria log stream")) {
		t.Fatalf("expected SSE log event, got %q", got)
	}

	snapshot := httptest.NewRecorder()
	router.ServeHTTP(snapshot, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/logs/snapshot", nil))
	if snapshot.Code != stdhttp.StatusOK {
		t.Fatalf("expected log snapshot 200, got %d: %s", snapshot.Code, snapshot.Body.String())
	}
	var logSnapshot struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(snapshot.Body.Bytes(), &logSnapshot); err != nil {
		t.Fatal(err)
	}
	if len(logSnapshot.Lines) == 0 || !strings.Contains(logSnapshot.Lines[0], "Mock Terraria log stream") {
		t.Fatalf("expected snapshot log lines, got %+v", logSnapshot)
	}

	activity := httptest.NewRecorder()
	router.ServeHTTP(activity, httptest.NewRequest(stdhttp.MethodGet, "/api/activity", nil))
	if activity.Code != stdhttp.StatusOK {
		t.Fatalf("expected activity 200, got %d: %s", activity.Code, activity.Body.String())
	}
	var events []domain.ActivityEvent
	if err := json.Unmarshal(activity.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected lifecycle actions to create activity events")
	}
	if events[0].Type == "" || events[0].Message == "" {
		t.Fatalf("expected populated activity event, got %+v", events[0])
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/"+server.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
}

func TestCreateServerRejectsUnsupportedVersion(t *testing.T) {
	router, _, _ := newTestRouter(t)
	payload := `{
		"name":"Unsupported Version",
		"providerKey":"terraria-vanilla",
		"version":"0.0.0",
		"config":{
			"serverName":"Unsupported Version",
			"worldName":"VersionWorld",
			"worldSize":"medium",
			"difficulty":"classic",
			"maxPlayers":8,
			"port":7777,
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(payload)))
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected unsupported version 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestRunningServerCommandAndLogsRecreateMissingRuntimeContainer(t *testing.T) {
	adapter := &staleContainerAdapter{}
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("stale-runtime", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "old-container"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	command := httptest.NewRecorder()
	router.ServeHTTP(command, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/command", bytes.NewBufferString(`{"command":"say hello"}`)))
	if command.Code != stdhttp.StatusOK {
		t.Fatalf("expected command to recover stale runtime container, got %d: %s", command.Code, command.Body.String())
	}
	if adapter.commandContainer != "new-container" || adapter.startedContainer != "new-container" {
		t.Fatalf("expected command path to use restarted container, got command=%q started=%q", adapter.commandContainer, adapter.startedContainer)
	}

	logs := httptest.NewRecorder()
	router.ServeHTTP(logs, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/logs", nil))
	if logs.Code != stdhttp.StatusOK {
		t.Fatalf("expected logs to recover stale runtime container, got %d: %s", logs.Code, logs.Body.String())
	}
	if adapter.logsContainer != "new-container" {
		t.Fatalf("expected logs path to use recreated container, got %q", adapter.logsContainer)
	}
	if adapter.created != 1 {
		t.Fatalf("expected a single runtime container recreation, got %d", adapter.created)
	}
}

func TestStoppedServerLogSnapshotToleratesMissingRuntimeContainer(t *testing.T) {
	adapter := &staleContainerAdapter{}
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("stopped-missing-runtime", cfg.DataDir)
	server.Status = domain.StatusStopped
	server.ContainerID = "old-container"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	snapshot := httptest.NewRecorder()
	router.ServeHTTP(snapshot, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/logs/snapshot", nil))
	if snapshot.Code != stdhttp.StatusOK {
		t.Fatalf("expected stopped missing runtime log snapshot to stay readable, got %d: %s", snapshot.Code, snapshot.Body.String())
	}
	var payload struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(snapshot.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Lines == nil || len(payload.Lines) != 0 {
		t.Fatalf("expected empty log history for missing stopped container, got %+v", payload)
	}
	stored, err := db.GetServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ContainerID != "" {
		t.Fatalf("expected stale stopped container id to be cleared, got %+v", stored)
	}
}

func TestStopServerClearsMissingRuntimeContainer(t *testing.T) {
	adapter := &staleContainerAdapter{}
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("stale-stop", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "old-container"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	stop := httptest.NewRecorder()
	router.ServeHTTP(stop, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/stop", nil))
	if stop.Code != stdhttp.StatusOK {
		t.Fatalf("expected stop to clear stale runtime container, got %d: %s", stop.Code, stop.Body.String())
	}
	var stopped domain.GameServerInstance
	if err := json.Unmarshal(stop.Body.Bytes(), &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Status != domain.StatusStopped || stopped.ContainerID != "" {
		t.Fatalf("expected stopped server with cleared container, got %+v", stopped)
	}
	if adapter.stoppedContainer != "" {
		t.Fatalf("expected stale stop to skip runtime stop call, got %q", adapter.stoppedContainer)
	}
}

func TestGetServerRefreshesStoredStatusFromRuntime(t *testing.T) {
	router, db, cfg := newTestRouterWithAdapter(t, inspectStatusAdapter{status: domain.StatusStopped})
	server := testServer("status-detail", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "runtime-container"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/status-detail", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected server detail 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var got domain.GameServerInstance
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusStopped {
		t.Fatalf("expected detail status refreshed from runtime, got %+v", got)
	}
	stored, err := db.GetServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.StatusStopped {
		t.Fatalf("expected refreshed status persisted, got %+v", stored)
	}
}

func TestListServersRefreshesStoredStatusFromRuntime(t *testing.T) {
	router, db, cfg := newTestRouterWithAdapter(t, inspectStatusAdapter{status: domain.StatusRunning})
	server := testServer("status-list", cfg.DataDir)
	server.Status = domain.StatusStopped
	server.ContainerID = "runtime-container"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/servers", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected server list 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var got []domain.GameServerInstance
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status != domain.StatusRunning {
		t.Fatalf("expected list status refreshed from runtime, got %+v", got)
	}
}

func TestListServersSkipsRuntimeInspectWhenDockerUnavailable(t *testing.T) {
	adapter := &unavailableInspectAdapter{}
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("unavailable-runtime-list", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "runtime-container"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/servers", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected server list to remain available, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if adapter.inspectCalls != 0 {
		t.Fatalf("expected list not to inspect runtime while Docker monitor is unavailable, got %d inspect calls", adapter.inspectCalls)
	}
	var got []domain.GameServerInstance
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status != domain.StatusRunning {
		t.Fatalf("expected stored status without runtime refresh, got %+v", got)
	}
}

func TestTModLoaderModUploadListAndDeleteEndpoints(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "example.tmod")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("mod")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	upload := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/servers/tmod/mods/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(upload, request)
	if upload.Code != stdhttp.StatusCreated {
		t.Fatalf("expected mod upload 201, got %d: %s", upload.Code, upload.Body.String())
	}
	var mod domain.ModFile
	if err := json.Unmarshal(upload.Body.Bytes(), &mod); err != nil {
		t.Fatal(err)
	}
	if mod.FileName != "example.tmod" || !mod.Enabled {
		t.Fatalf("expected uploaded enabled mod, got %+v", mod)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/tmod/mods", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod list 200, got %d: %s", list.Code, list.Body.String())
	}
	var mods []domain.ModFile
	if err := json.Unmarshal(list.Body.Bytes(), &mods); err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 || mods[0].ID != mod.ID {
		t.Fatalf("expected listed uploaded mod, got %+v", mods)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/tmod/mods/"+mod.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "mods", "tmod", "example.tmod")); !os.IsNotExist(err) {
		t.Fatalf("expected mod file deleted, stat err=%v", err)
	}
}

func TestTModLoaderModUploadIsIdempotentForSameFile(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	first := httptest.NewRecorder()
	router.ServeHTTP(first, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/servers/tmod/mods/upload", "file", "example.tmod", []byte("mod-v1")))
	if first.Code != stdhttp.StatusCreated {
		t.Fatalf("expected first upload 201, got %d: %s", first.Code, first.Body.String())
	}
	var firstMod domain.ModFile
	if err := json.Unmarshal(first.Body.Bytes(), &firstMod); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/servers/tmod/mods/upload", "file", "example.tmod", []byte("mod-v2")))
	if second.Code != stdhttp.StatusOK {
		t.Fatalf("expected repeated upload 200, got %d: %s", second.Code, second.Body.String())
	}
	var secondMod domain.ModFile
	if err := json.Unmarshal(second.Body.Bytes(), &secondMod); err != nil {
		t.Fatal(err)
	}
	if secondMod.ID != firstMod.ID {
		t.Fatalf("expected repeated upload to update existing mod, got first=%+v second=%+v", firstMod, secondMod)
	}
	if secondMod.SizeBytes != int64(len("mod-v2")) {
		t.Fatalf("expected updated size, got %+v", secondMod)
	}
	mods, err := db.ListMods(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 {
		t.Fatalf("expected one server mod record after repeated upload, got %+v", mods)
	}
}

func TestTModLoaderModEnabledEndpoint(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	mod := domain.ModFile{
		ID:         "mod-1",
		InstanceID: server.ID,
		FileName:   "example.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(`{"enabled":false}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPatch, "/api/servers/tmod/mods/mod-1", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod enabled update 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var got domain.ModFile
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatalf("expected disabled mod, got %+v", got)
	}
	persisted, err := db.GetMod(context.Background(), mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Enabled {
		t.Fatalf("expected persisted disabled mod, got %+v", persisted)
	}
}

func TestGlobalModUploadIsIdempotentForSameFile(t *testing.T) {
	router, db, _ := newTestRouter(t)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/mods/upload", "file", "example.tmod", []byte("mod-v1")))
	if first.Code != stdhttp.StatusCreated {
		t.Fatalf("expected first global upload 201, got %d: %s", first.Code, first.Body.String())
	}
	var firstMod domain.ModFile
	if err := json.Unmarshal(first.Body.Bytes(), &firstMod); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/mods/upload", "file", "example.tmod", []byte("mod-v2")))
	if second.Code != stdhttp.StatusOK {
		t.Fatalf("expected repeated global upload 200, got %d: %s", second.Code, second.Body.String())
	}
	var secondMod domain.ModFile
	if err := json.Unmarshal(second.Body.Bytes(), &secondMod); err != nil {
		t.Fatal(err)
	}
	if secondMod.ID != firstMod.ID {
		t.Fatalf("expected repeated global upload to update existing mod, got first=%+v second=%+v", firstMod, secondMod)
	}
	mods, err := db.ListMods(context.Background(), "unassigned")
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 {
		t.Fatalf("expected one global mod record after repeated upload, got %+v", mods)
	}
}

func TestAssignModIsIdempotentForSameServerFile(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	if _, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", "example.tmod", bytes.NewBufferString("mod-v1")); err != nil {
		t.Fatal(err)
	}
	globalMod := domain.ModFile{
		ID:         "global-mod",
		InstanceID: "unassigned",
		FileName:   "example.tmod",
		SizeBytes:  6,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &globalMod); err != nil {
		t.Fatal(err)
	}

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(stdhttp.MethodPost, "/api/mods/global-mod/assign", bytes.NewBufferString(`{"instanceId":"tmod"}`)))
	if first.Code != stdhttp.StatusCreated {
		t.Fatalf("expected first assign 201, got %d: %s", first.Code, first.Body.String())
	}
	var firstAssigned domain.ModFile
	if err := json.Unmarshal(first.Body.Bytes(), &firstAssigned); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(stdhttp.MethodPost, "/api/mods/global-mod/assign", bytes.NewBufferString(`{"instanceId":"tmod"}`)))
	if second.Code != stdhttp.StatusOK {
		t.Fatalf("expected repeated assign 200, got %d: %s", second.Code, second.Body.String())
	}
	var secondAssigned domain.ModFile
	if err := json.Unmarshal(second.Body.Bytes(), &secondAssigned); err != nil {
		t.Fatal(err)
	}
	if secondAssigned.ID != firstAssigned.ID {
		t.Fatalf("expected repeated assign to update existing mod, got first=%+v second=%+v", firstAssigned, secondAssigned)
	}
	mods, err := db.ListMods(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 {
		t.Fatalf("expected one server mod record after repeated assign, got %+v", mods)
	}
}

func TestGlobalModDeleteRejectsServerMod(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	mod := domain.ModFile{
		ID:         "server-mod",
		InstanceID: server.ID,
		FileName:   "example.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/api/mods/server-mod", nil))
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected global delete to reject server mod, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, err := db.GetMod(context.Background(), mod.ID); err != nil {
		t.Fatalf("expected server mod record to remain, got %v", err)
	}
}

func TestWorldImportListDownloadDuplicateAndDeleteEndpoints(t *testing.T) {
	router, _, cfg := newTestRouter(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "uploaded.wld")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("world-data")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	upload := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/import", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(upload, request)
	if upload.Code != stdhttp.StatusCreated {
		t.Fatalf("expected world import 201, got %d: %s", upload.Code, upload.Body.String())
	}
	var imported domain.World
	if err := json.Unmarshal(upload.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	if imported.InstanceID != "unassigned" || imported.FileName != "uploaded.wld" {
		t.Fatalf("expected unassigned uploaded world, got %+v", imported)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "worlds", "unassigned", "uploaded.wld")); err != nil {
		t.Fatal(err)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/worlds", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected world list 200, got %d: %s", list.Code, list.Body.String())
	}
	var worlds []domain.World
	if err := json.Unmarshal(list.Body.Bytes(), &worlds); err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 1 || worlds[0].ID != imported.ID {
		t.Fatalf("expected listed uploaded world, got %+v", worlds)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(stdhttp.MethodGet, "/api/worlds/"+imported.ID+"/download", nil))
	if download.Code != stdhttp.StatusOK {
		t.Fatalf("expected world download 200, got %d: %s", download.Code, download.Body.String())
	}
	if download.Body.String() != "world-data" {
		t.Fatalf("expected downloaded world content, got %q", download.Body.String())
	}

	duplicate := httptest.NewRecorder()
	router.ServeHTTP(duplicate, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/"+imported.ID+"/duplicate", bytes.NewBufferString(`{"name":"Copy","fileName":"copy.wld"}`)))
	if duplicate.Code != stdhttp.StatusCreated {
		t.Fatalf("expected world duplicate 201, got %d: %s", duplicate.Code, duplicate.Body.String())
	}
	var copied domain.World
	if err := json.Unmarshal(duplicate.Body.Bytes(), &copied); err != nil {
		t.Fatal(err)
	}
	if copied.Name != "Copy" || copied.FileName != "copy.wld" {
		t.Fatalf("expected copied world metadata, got %+v", copied)
	}
	copiedBytes, err := os.ReadFile(filepath.Join(cfg.DataDir, "worlds", "unassigned", "copy.wld"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copiedBytes) != "world-data" {
		t.Fatalf("expected copied world content, got %q", string(copiedBytes))
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/worlds/"+copied.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected world delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "worlds", "unassigned", "copy.wld")); !os.IsNotExist(err) {
		t.Fatalf("expected copied world file deleted, stat err=%v", err)
	}
}

func TestWorldImportIsIdempotentForSameInstanceFile(t *testing.T) {
	router, db, _ := newTestRouter(t)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/worlds/import", "file", "uploaded.wld", []byte("world-v1")))
	if first.Code != stdhttp.StatusCreated {
		t.Fatalf("expected first world import 201, got %d: %s", first.Code, first.Body.String())
	}
	var firstWorld domain.World
	if err := json.Unmarshal(first.Body.Bytes(), &firstWorld); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/worlds/import", "file", "uploaded.wld", []byte("world-v2")))
	if second.Code != stdhttp.StatusOK {
		t.Fatalf("expected repeated world import 200, got %d: %s", second.Code, second.Body.String())
	}
	var secondWorld domain.World
	if err := json.Unmarshal(second.Body.Bytes(), &secondWorld); err != nil {
		t.Fatal(err)
	}
	if secondWorld.ID != firstWorld.ID {
		t.Fatalf("expected repeated world import to update existing world, got first=%+v second=%+v", firstWorld, secondWorld)
	}
	if secondWorld.SizeBytes != int64(len("world-v2")) {
		t.Fatalf("expected updated world size, got %+v", secondWorld)
	}
	worlds, err := db.ListWorlds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 1 {
		t.Fatalf("expected one world record after repeated import, got %+v", worlds)
	}
}

func TestWorldImportRejectsUnknownInstance(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("instanceId", "missing-server"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "uploaded.wld")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("world-data")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	upload := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/import", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(upload, request)
	if upload.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected unknown instance import 404, got %d: %s", upload.Code, upload.Body.String())
	}
	worlds, err := db.ListWorlds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 0 {
		t.Fatalf("expected no world records for unknown instance, got %+v", worlds)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "worlds", "missing-server", "uploaded.wld")); !os.IsNotExist(err) {
		t.Fatalf("expected no file for unknown instance import, stat err=%v", err)
	}
}

func TestWorldListPrunesMissingFilesAndDownloadReturnsJSONError(t *testing.T) {
	router, db, _ := newTestRouter(t)
	world := domain.World{
		ID:         "missing-world",
		InstanceID: "unassigned",
		Name:       "Missing",
		FileName:   "missing.wld",
		SizeBytes:  5,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(stdhttp.MethodGet, "/api/worlds/missing-world/download", nil))
	if download.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected missing world download 404, got %d: %s", download.Code, download.Body.String())
	}
	if !strings.Contains(download.Body.String(), "world file not found on disk") {
		t.Fatalf("expected JSON missing file error, got %q", download.Body.String())
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/worlds", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected world list 200, got %d: %s", list.Code, list.Body.String())
	}
	var worlds []domain.World
	if err := json.Unmarshal(list.Body.Bytes(), &worlds); err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 0 {
		t.Fatalf("expected missing world record to be pruned, got %+v", worlds)
	}
	if _, err := db.GetWorld(context.Background(), world.ID); err == nil {
		t.Fatal("expected missing world record deleted")
	}
}

func TestAssignWorldUpdatesServerConfigAndClearsContainer(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("world-target", cfg.DataDir)
	server.ContainerID = "old-container"
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	oldWorld := domain.World{
		ID:               "old-world",
		InstanceID:       server.ID,
		ActiveInstanceID: server.ID,
		Name:             server.WorldName,
		FileName:         server.WorldName + ".wld",
		SizeBytes:        5,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &oldWorld); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(server.ID, "new-home.wld", bytes.NewBufferString("world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:         "assign-world",
		InstanceID: server.ID,
		Name:       "new-home",
		FileName:   "new-home.wld",
		SizeBytes:  5,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	assign := httptest.NewRecorder()
	router.ServeHTTP(assign, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/assign-world/assign", bytes.NewBufferString(`{"instanceId":"world-target"}`)))
	if assign.Code != stdhttp.StatusOK {
		t.Fatalf("expected assign world 200, got %d: %s", assign.Code, assign.Body.String())
	}
	updated, err := db.GetServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.WorldName != "new-home" || updated.Config.WorldName != "new-home" || updated.ContainerID != "" {
		t.Fatalf("expected server world/config update and cleared container, got %+v", updated)
	}
	configBytes, err := os.ReadFile(filepath.Join(server.DataDir, "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(configBytes, []byte("world=worlds/new-home.wld")) {
		t.Fatalf("expected serverconfig to point at assigned world, got %q", string(configBytes))
	}
	previous, err := db.GetWorld(context.Background(), oldWorld.ID)
	if err != nil {
		t.Fatal(err)
	}
	if previous.ActiveInstanceID != "" {
		t.Fatalf("expected previous active world to be cleared, got %+v", previous)
	}
	assigned, err := db.GetWorld(context.Background(), world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.ActiveInstanceID != server.ID {
		t.Fatalf("expected assigned world to be active, got %+v", assigned)
	}
}

func TestDeleteActiveWorldRequiresUnassigningFirst(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("active-world-server", cfg.DataDir)
	server.WorldName = "active"
	server.Config.WorldName = "active"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(server.ID, "active.wld", bytes.NewBufferString("world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:               "active-world",
		InstanceID:       server.ID,
		ActiveInstanceID: server.ID,
		Name:             "active",
		FileName:         "active.wld",
		SizeBytes:        5,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/worlds/active-world", nil))
	if remove.Code != stdhttp.StatusConflict {
		t.Fatalf("expected active world delete conflict, got %d: %s", remove.Code, remove.Body.String())
	}
	stored, err := db.GetWorld(context.Background(), world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != world.ID {
		t.Fatalf("expected active world to remain stored, got %+v", stored)
	}
}

func TestUpdateServerConfigRequiresStoppedAndRewritesRuntimeConfig(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("config-target", cfg.DataDir)
	server.ContainerID = "old-container"
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	payload := `{
		"config":{
			"serverName":"Edited Server",
			"worldName":"EditedWorld",
			"worldSize":"large",
			"difficulty":"expert",
			"maxPlayers":12,
			"port":17777,
			"password":"secret",
			"motd":"Updated from detail page",
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/config-target/config", bytes.NewBufferString(payload)))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected config update 200, got %d: %s", update.Code, update.Body.String())
	}
	updated, err := db.GetServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Edited Server" || updated.WorldName != "EditedWorld" || updated.Port != 17777 || updated.MaxPlayers != 12 || updated.Password != "secret" {
		t.Fatalf("expected server fields synchronized from config, got %+v", updated)
	}
	if updated.Config.Difficulty != domain.Difficulty("expert") || updated.Config.WorldSize != domain.WorldSize("large") {
		t.Fatalf("expected persisted config update, got %+v", updated.Config)
	}
	if updated.ContainerID != "" {
		t.Fatalf("expected stale container id cleared after config update, got %q", updated.ContainerID)
	}
	configBytes, err := os.ReadFile(filepath.Join(server.DataDir, "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(configBytes, []byte("world=worlds/EditedWorld.wld")) || !bytes.Contains(configBytes, []byte("maxplayers=12")) {
		t.Fatalf("expected rewritten serverconfig, got %q", string(configBytes))
	}
}

func TestBackupCreateListDownloadRestoreAndDeleteEndpoints(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("backup-source", cfg.DataDir)
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(server.DataDir, "serverconfig.txt")
	if err := os.WriteFile(configPath, []byte("original-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/backup-source/backups", nil))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected backup create 201, got %d: %s", create.Code, create.Body.String())
	}
	var backup domain.Backup
	if err := json.Unmarshal(create.Body.Bytes(), &backup); err != nil {
		t.Fatal(err)
	}
	if backup.InstanceID != "backup-source" || backup.FileName == "" {
		t.Fatalf("expected created backup metadata, got %+v", backup)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/backups", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup list 200, got %d: %s", list.Code, list.Body.String())
	}
	var backups []domain.Backup
	if err := json.Unmarshal(list.Body.Bytes(), &backups); err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || backups[0].ID != backup.ID {
		t.Fatalf("expected listed created backup, got %+v", backups)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(stdhttp.MethodGet, "/api/backups/"+backup.ID+"/download", nil))
	if download.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup download 200, got %d: %s", download.Code, download.Body.String())
	}
	if download.Body.Len() == 0 {
		t.Fatal("expected non-empty backup archive")
	}

	if err := os.WriteFile(configPath, []byte("mutated-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := httptest.NewRecorder()
	router.ServeHTTP(restore, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/"+backup.ID+"/restore", nil))
	if restore.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup restore 200, got %d: %s", restore.Code, restore.Body.String())
	}
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != "original-config" {
		t.Fatalf("expected restored config, got %q", string(restored))
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/backups/"+backup.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "backups", "backup-source", backup.FileName)); !os.IsNotExist(err) {
		t.Fatalf("expected backup archive deleted, stat err=%v", err)
	}
}

func TestSettingsEndpointsReadAndUpdateDockerHost(t *testing.T) {
	router, _, _ := newTestRouter(t)

	read := httptest.NewRecorder()
	router.ServeHTTP(read, httptest.NewRequest(stdhttp.MethodGet, "/api/settings", nil))
	if read.Code != stdhttp.StatusOK {
		t.Fatalf("expected settings read 200, got %d: %s", read.Code, read.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(read.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["dockerHost"] != "unix:///initial.sock" {
		t.Fatalf("expected initial docker host, got %q", got["dockerHost"])
	}

	body := bytes.NewBufferString(`{"dockerHost":"unix:///updated.sock"}`)
	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/settings", body))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected settings update 200, got %d: %s", update.Code, update.Body.String())
	}
	if err := json.Unmarshal(update.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["dockerHost"] != "unix:///updated.sock" {
		t.Fatalf("expected updated docker host, got %q", got["dockerHost"])
	}
}

func TestMigrateWorldEndpointCopiesToTargetServer(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("source", cfg.DataDir)
	target := testServer("target", cfg.DataDir)
	if err := db.CreateServer(context.Background(), &source); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import("source", "journey.wld", bytes.NewBufferString("world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{ID: "world-1", InstanceID: "source", Name: "Journey", FileName: "journey.wld", SizeBytes: 5, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/world-1/migrate", bytes.NewBufferString(`{"instanceId":"target"}`)))
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected migrate world 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var migrated domain.World
	if err := json.Unmarshal(recorder.Body.Bytes(), &migrated); err != nil {
		t.Fatal(err)
	}
	if migrated.InstanceID != "target" || migrated.ActiveInstanceID != "" {
		t.Fatalf("expected migrated world to be copied to target without activating it, got %+v", migrated)
	}
	storedTarget, err := db.GetServer(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedTarget.WorldName != target.WorldName {
		t.Fatalf("expected world migration to leave target server current world unchanged, got %q", storedTarget.WorldName)
	}
	got, err := os.ReadFile(filepath.Join(cfg.DataDir, "worlds", "target", "journey.wld"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world" {
		t.Fatalf("expected migrated world content, got %q", string(got))
	}

	again := httptest.NewRecorder()
	router.ServeHTTP(again, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/world-1/migrate", bytes.NewBufferString(`{"instanceId":"target"}`)))
	if again.Code != stdhttp.StatusOK {
		t.Fatalf("expected repeated migrate world 200, got %d: %s", again.Code, again.Body.String())
	}
	var repeated domain.World
	if err := json.Unmarshal(again.Body.Bytes(), &repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.ID != migrated.ID {
		t.Fatalf("expected repeated migration to update existing target world, got first=%+v second=%+v", migrated, repeated)
	}
}

func TestMigrateBackupEndpointCopiesToTargetServer(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("source", cfg.DataDir)
	target := testServer("target", cfg.DataDir)
	if err := os.MkdirAll(source.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source.DataDir, "serverconfig.txt"), []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &source); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	backupPath, size, err := backupsvc.NewService(cfg.DataDir).Create("source", source.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	backup := domain.Backup{ID: "backup-1", InstanceID: "source", FileName: filepath.Base(backupPath), WorldName: "Source World", SizeBytes: size, Type: "Manual", CreatedAt: time.Now()}
	if err := db.CreateBackup(context.Background(), &backup); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/backup-1/migrate", bytes.NewBufferString(`{"instanceId":"target"}`)))
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected migrate backup 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var migrated domain.Backup
	if err := json.Unmarshal(recorder.Body.Bytes(), &migrated); err != nil {
		t.Fatal(err)
	}
	if migrated.InstanceID != "target" || migrated.WorldName != target.WorldName {
		t.Fatalf("expected migrated backup target, got %+v", migrated)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "backups", "target", filepath.Base(backupPath))); err != nil {
		t.Fatal(err)
	}

	again := httptest.NewRecorder()
	router.ServeHTTP(again, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/backup-1/migrate", bytes.NewBufferString(`{"instanceId":"target"}`)))
	if again.Code != stdhttp.StatusOK {
		t.Fatalf("expected repeated migrate backup 200, got %d: %s", again.Code, again.Body.String())
	}
	var repeated domain.Backup
	if err := json.Unmarshal(again.Body.Bytes(), &repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.ID != migrated.ID {
		t.Fatalf("expected repeated backup migration to update existing target backup, got first=%+v second=%+v", migrated, repeated)
	}
}

func testServer(id string, dataDir string) domain.GameServerInstance {
	return domain.GameServerInstance{
		ID:          id,
		Name:        id + " server",
		GameKey:     "terraria",
		ProviderKey: domain.ProviderTerrariaVanilla,
		Status:      domain.StatusStopped,
		WorldName:   id + " world",
		Port:        7777,
		MaxPlayers:  8,
		DataDir:     filepath.Join(dataDir, "instances", id),
		Config:      terraria.Presets[0].Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}
