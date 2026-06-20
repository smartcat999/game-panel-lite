package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/minecraft"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
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

func tmodFixture(name string, version string, tmodVersion string) []byte {
	data := append([]byte("TMOD"), tmodBinaryString(tmodVersion)...)
	data = append(data, make([]byte, 20+256)...)
	data = append(data, 0x79, 0x9a, 0x05, 0x00)
	data = append(data, tmodBinaryString(name)...)
	data = append(data, tmodBinaryString(version)...)
	return data
}

func tmodBinaryString(value string) []byte {
	return append([]byte{byte(len(value))}, []byte(value)...)
}

func containsEnv(env []string, target string) bool {
	for _, item := range env {
		if item == target {
			return true
		}
	}
	return false
}

func newTestRouterWithAdapter(t *testing.T, adapter runtime.Adapter) (stdhttp.Handler, *store.Store, config.Config) {
	return newTestRouterWithAdapterAndInstallMarkers(t, adapter, true)
}

func newTestRouterWithAdapterAndInstallMarkers(t *testing.T, adapter runtime.Adapter, seedInstallMarkers bool) (stdhttp.Handler, *store.Store, config.Config) {
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
		provider.NewRegistry(terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider(), palworld.NewProvider(), dst.NewProvider(), minecraft.NewProvider()),
		runtimeAdapter,
		monitor,
		func(string) (runtime.Adapter, error) { return runtime.NewMockAdapter(), nil },
		nil,
	)
	if seedInstallMarkers {
		seedRuntimeInstallMarkers(t, handler)
	}
	router := chi.NewRouter()
	handler.Register(router)
	return router, db, cfg
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

func seedRuntimeInstallMarkers(t *testing.T, handler *Handler) {
	t.Helper()
	for _, game := range handler.provider.Games() {
		for _, providerCatalog := range game.Providers {
			gameProvider, ok := handler.provider.Get(providerCatalog.Key)
			if !ok {
				continue
			}
			for _, versionValue := range gameProvider.Versions() {
				version := normalizeStoredProviderVersion(gameProvider, versionValue)
				ref := runtimeInstallRef{
					ProviderKey: providerCatalog.Key,
					Version:     version,
					Image:       gameProvider.ImageFor(version),
				}
				if err := seedRuntimeInstallArchive(handler, ref); err != nil {
					t.Fatalf("seed runtime image archive: %v", err)
				}
				if err := handler.writeRuntimeInstallMarker(ref); err != nil {
					t.Fatalf("seed runtime install marker: %v", err)
				}
			}
		}
	}
}

func seedRuntimeInstallArchive(handler *Handler, ref runtimeInstallRef) error {
	archivePath := handler.runtimeInstallArchivePath(ref)
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(archivePath, []byte("test image archive\n"), 0o644)
}

type availableMockAdapter struct {
	*runtime.MockAdapter
}

func (a availableMockAdapter) Check(context.Context) runtime.DockerStatus {
	return runtime.DockerStatus{Available: true, Message: "ok", Host: "mock"}
}

type armMockAdapter struct {
	availableMockAdapter
}

func (a armMockAdapter) Check(context.Context) runtime.DockerStatus {
	return runtime.DockerStatus{Available: true, Message: "ok", Host: "mock", Architecture: "arm64"}
}

type missingImageAdapter struct {
	availableMockAdapter
}

func (a missingImageAdapter) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusMissing}
}

type commandCaptureAdapter struct {
	availableMockAdapter
	commands []string
}

func newCommandCaptureAdapter() *commandCaptureAdapter {
	return &commandCaptureAdapter{availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}}
}

func (a *commandCaptureAdapter) SendCommand(_ context.Context, _ domain.GameServerInstance, command string) error {
	a.commands = append(a.commands, command)
	return nil
}

type captureCreateAdapter struct {
	availableMockAdapter
	created chan runtime.ContainerSpec
	removed chan string
}

func newCaptureCreateAdapter() *captureCreateAdapter {
	return &captureCreateAdapter{
		availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()},
		created:              make(chan runtime.ContainerSpec, 1),
		removed:              make(chan string, 1),
	}
}

func (a *captureCreateAdapter) Create(ctx context.Context, spec runtime.ContainerSpec) (string, error) {
	a.created <- spec
	return a.availableMockAdapter.Create(ctx, spec)
}

func (a *captureCreateAdapter) Remove(ctx context.Context, instance domain.GameServerInstance) error {
	a.removed <- instance.ContainerID
	return a.availableMockAdapter.Remove(ctx, instance)
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

func TestAuthSetupLoginAndProtectedRoutes(t *testing.T) {
	router, _, _ := newTestRouter(t)

	bootstrap := httptest.NewRecorder()
	router.ServeHTTP(bootstrap, httptest.NewRequest(stdhttp.MethodGet, "/api/auth/bootstrap", nil))
	if bootstrap.Code != stdhttp.StatusOK {
		t.Fatalf("expected bootstrap 200, got %d: %s", bootstrap.Code, bootstrap.Body.String())
	}
	if !strings.Contains(bootstrap.Body.String(), `"initialized":false`) {
		t.Fatalf("expected uninitialized bootstrap, got %s", bootstrap.Body.String())
	}

	setup := httptest.NewRecorder()
	setupReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/setup", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	setupReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(setup, setupReq)
	if setup.Code != stdhttp.StatusCreated {
		t.Fatalf("expected setup 201, got %d: %s", setup.Code, setup.Body.String())
	}
	setupCookie := authCookieFromRecorder(t, setup)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(stdhttp.MethodGet, "/api/version", nil))
	if unauthorized.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("expected protected route 401 after setup, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	me := httptest.NewRecorder()
	meReq := httptest.NewRequest(stdhttp.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(setupCookie)
	router.ServeHTTP(me, meReq)
	if me.Code != stdhttp.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", me.Code, me.Body.String())
	}
	if !strings.Contains(me.Body.String(), `"username":"admin"`) {
		t.Fatalf("expected account response, got %s", me.Body.String())
	}

	login := httptest.NewRecorder()
	loginReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(login, loginReq)
	if login.Code != stdhttp.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", login.Code, login.Body.String())
	}
	loginCookie := authCookieFromRecorder(t, login)

	change := httptest.NewRecorder()
	changeReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/password", strings.NewReader(`{"currentPassword":"secret123","newPassword":"secret456"}`))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.AddCookie(loginCookie)
	router.ServeHTTP(change, changeReq)
	if change.Code != stdhttp.StatusOK {
		t.Fatalf("expected password change 200, got %d: %s", change.Code, change.Body.String())
	}

	oldLogin := httptest.NewRecorder()
	oldLoginReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"secret123"}`))
	oldLoginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(oldLogin, oldLoginReq)
	if oldLogin.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("expected old password login 401, got %d: %s", oldLogin.Code, oldLogin.Body.String())
	}

	newLogin := httptest.NewRecorder()
	newLoginReq := httptest.NewRequest(stdhttp.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"secret456"}`))
	newLoginReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(newLogin, newLoginReq)
	if newLogin.Code != stdhttp.StatusOK {
		t.Fatalf("expected new password login 200, got %d: %s", newLogin.Code, newLogin.Body.String())
	}
}

func authCookieFromRecorder(t *testing.T, recorder *httptest.ResponseRecorder) *stdhttp.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			return cookie
		}
	}
	t.Fatalf("expected %s cookie in response headers", sessionCookieName)
	return nil
}

type inspectStatusAdapter struct {
	status domain.ServerStatus
	err    error
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
	return a.status, a.err
}
func (a inspectStatusAdapter) Stats(context.Context, domain.GameServerInstance) (runtime.ContainerStats, error) {
	return runtime.ContainerStats{}, nil
}
func (a inspectStatusAdapter) HostStats(context.Context) (runtime.HostStats, error) {
	return runtime.HostStats{}, nil
}
func (a inspectStatusAdapter) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusReady}
}
func (a inspectStatusAdapter) PrepareImage(context.Context, string) error {
	return nil
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
func (a *unavailableInspectAdapter) HostStats(context.Context) (runtime.HostStats, error) {
	return runtime.HostStats{}, fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusFailed, Message: "docker unavailable"}
}
func (a *unavailableInspectAdapter) PrepareImage(context.Context, string) error {
	return fmt.Errorf("docker unavailable")
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
func (a *staleContainerAdapter) HostStats(context.Context) (runtime.HostStats, error) {
	return runtime.HostStats{}, nil
}
func (a *staleContainerAdapter) ImageStatus(_ context.Context, image string) domain.RuntimeImageStatus {
	return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusReady}
}
func (a *staleContainerAdapter) PrepareImage(context.Context, string) error {
	return nil
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

type statusCapturingLogsAdapter struct {
	availableMockAdapter
	logStatus domain.ServerStatus
}

func (a *statusCapturingLogsAdapter) Logs(_ context.Context, instance domain.GameServerInstance) (io.ReadCloser, error) {
	a.logStatus = instance.Status
	return io.NopCloser(strings.NewReader("[Info] running log snapshot\n")), nil
}

type blockingRuntimeAdapter struct {
	availableMockAdapter
	createStarted chan struct{}
	createRelease chan struct{}
	stopStarted   chan struct{}
	stopRelease   chan struct{}
	removeStarted chan struct{}
	removeRelease chan struct{}
}

func newBlockingRuntimeAdapter() *blockingRuntimeAdapter {
	return &blockingRuntimeAdapter{
		availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()},
		createStarted:        make(chan struct{}),
		createRelease:        make(chan struct{}),
		stopStarted:          make(chan struct{}),
		stopRelease:          make(chan struct{}),
		removeStarted:        make(chan struct{}),
		removeRelease:        make(chan struct{}),
	}
}

func (a *blockingRuntimeAdapter) Create(ctx context.Context, spec runtime.ContainerSpec) (string, error) {
	close(a.createStarted)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-a.createRelease:
		return a.availableMockAdapter.Create(ctx, spec)
	}
}

func (a *blockingRuntimeAdapter) Stop(ctx context.Context, instance domain.GameServerInstance) error {
	close(a.stopStarted)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.stopRelease:
		return nil
	}
}

func (a *blockingRuntimeAdapter) Remove(ctx context.Context, instance domain.GameServerInstance) error {
	close(a.removeStarted)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.removeRelease:
		return nil
	}
}

func TestStartServerReturnsAcceptedBeforeRuntimeCompletes(t *testing.T) {
	adapter := newBlockingRuntimeAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("async-start", cfg.DataDir)
	server.ContainerID = ""
	server.Status = domain.StatusStopped
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
		response <- recorder
	}()

	select {
	case <-adapter.createStarted:
	case <-time.After(time.Second):
		t.Fatal("expected async start worker to begin creating a runtime container")
	}

	select {
	case recorder := <-response:
		if recorder.Code != stdhttp.StatusAccepted {
			t.Fatalf("expected start 202, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var queued domain.GameServerInstance
		if err := json.Unmarshal(recorder.Body.Bytes(), &queued); err != nil {
			t.Fatal(err)
		}
		if queued.Status != domain.StatusStarting {
			t.Fatalf("expected queued start status starting, got %+v", queued)
		}
	case <-time.After(100 * time.Millisecond):
		close(adapter.createRelease)
		recorder := <-response
		t.Fatalf("start request blocked until runtime completed; got %d: %s", recorder.Code, recorder.Body.String())
	}

	close(adapter.createRelease)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		updated, err := db.GetServer(context.Background(), server.ID)
		if err == nil && updated.Status == domain.StatusRunning && updated.ContainerID != "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	updated, _ := db.GetServer(context.Background(), server.ID)
	t.Fatalf("expected async start worker to mark server running, got %+v", updated)
}

func TestStartTModLoaderServerNormalizesOldDockerTagVersion(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("old-tmod-version", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	server.Config = terraria.Presets[4].Config
	server.Version = "2024.10"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected runtime container to be created")
	}
	if spec.Image != "smartcat99999/tmodloader:v2026.04.3.0" {
		t.Fatalf("expected versioned tModLoader runtime image, got %q", spec.Image)
	}
	if strings.Contains(spec.Image, "radioactivehydra") || strings.Contains(spec.Image, "2024.10") {
		t.Fatalf("expected old Docker tag not to be used, got image %q", spec.Image)
	}
	updated := waitForServerStatus(t, db, server.ID, domain.StatusRunning)
	if updated.Version != "v2026.04.3.0" {
		t.Fatalf("expected stored version to be normalized, got %q", updated.Version)
	}
}

func TestStartPalworldServerRuntimeSpecUsesConfigPayload(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("palworld-payload-runtime", cfg.DataDir)
	server.GameKey = domain.GamePalworld
	server.ProviderKey = domain.ProviderPalworld
	server.Version = "latest"
	server.Port = palworld.DefaultInternalPort
	server.HostPort = 18211
	server.Config = palworld.NormalizeConfig(domain.TerrariaConfig{
		ServerName: "Old Pal",
		WorldName:  "Old Save",
		MaxPlayers: 4,
		Password:   "old-join",
		MOTD:       "old-admin",
	})
	server.ConfigPayloadJSON = `{"serverName":"Payload Pal","saveName":"Payload Save","maxPlayers":14,"serverPassword":"payload-join","adminPassword":"payload-admin"}`
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected async start to create runtime container")
	}
	env := strings.Join(spec.Options.Env, "\n")
	for _, expected := range []string{
		"PLAYERS=14",
		"SERVER_NAME=Payload Pal",
		"SERVER_PASSWORD=payload-join",
		"ADMIN_PASSWORD=payload-admin",
	} {
		if !strings.Contains(env, expected) {
			t.Fatalf("expected runtime env to contain %q, got:\n%s", expected, env)
		}
	}
	if !strings.Contains(spec.ConfigText, "saveName=Payload Save") || strings.Contains(spec.ConfigText, "Old Save") {
		t.Fatalf("expected runtime config text to use payload save, got:\n%s", spec.ConfigText)
	}
	waitForServerStatus(t, db, server.ID, domain.StatusRunning)
}

func TestStartServerReusesExistingContainer(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("start-reuse", cfg.DataDir)
	server.Status = domain.StatusStopped
	server.ContainerID = "old-container"
	server.Version = "1.4.5.6"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", recorder.Code, recorder.Body.String())
	}

	select {
	case removed := <-adapter.removed:
		t.Fatalf("expected start to reuse existing container, removed %q", removed)
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case spec := <-adapter.created:
		t.Fatalf("expected start to reuse existing container, created %+v", spec)
	case <-time.After(50 * time.Millisecond):
	}

	updated := waitForServerStatus(t, db, server.ID, domain.StatusRunning)
	if updated.ContainerID != "old-container" {
		t.Fatalf("expected existing container id to be reused, got %+v", updated)
	}
}

func TestRestartServerRecreatesExistingContainer(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("restart-recreate", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "old-container"
	server.Version = "1.4.5.6"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/restart", nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected restart 202, got %d: %s", recorder.Code, recorder.Body.String())
	}

	select {
	case removed := <-adapter.removed:
		if removed != "old-container" {
			t.Fatalf("expected old container to be removed, got %q", removed)
		}
	case <-time.After(time.Second):
		t.Fatal("expected restart to remove the old runtime container before recreation")
	}

	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected restart to recreate runtime container")
	}
	if spec.Image != "smartcat99999/terraria-vanilla:1.4.5.6" {
		t.Fatalf("expected recreated container to use current image tag, got %q", spec.Image)
	}
	updated := waitForServerStatus(t, db, server.ID, domain.StatusRunning)
	if updated.ContainerID == "old-container" || updated.ContainerID == "" {
		t.Fatalf("expected restarted server to use a recreated container, got %+v", updated)
	}
}

func TestStopServerReturnsAcceptedBeforeRuntimeCompletes(t *testing.T) {
	adapter := newBlockingRuntimeAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("async-stop", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "running-container"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/stop", nil))
		response <- recorder
	}()

	select {
	case <-adapter.stopStarted:
	case <-time.After(time.Second):
		t.Fatal("expected async stop worker to begin stopping the runtime container")
	}

	select {
	case recorder := <-response:
		if recorder.Code != stdhttp.StatusAccepted {
			t.Fatalf("expected stop 202, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var queued domain.GameServerInstance
		if err := json.Unmarshal(recorder.Body.Bytes(), &queued); err != nil {
			t.Fatal(err)
		}
		if queued.Status != domain.StatusStopping {
			t.Fatalf("expected queued stop status stopping, got %+v", queued)
		}
	case <-time.After(100 * time.Millisecond):
		close(adapter.stopRelease)
		recorder := <-response
		t.Fatalf("stop request blocked until runtime completed; got %d: %s", recorder.Code, recorder.Body.String())
	}

	close(adapter.stopRelease)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		updated, err := db.GetServer(context.Background(), server.ID)
		if err == nil && updated.Status == domain.StatusStopped {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	updated, _ := db.GetServer(context.Background(), server.ID)
	t.Fatalf("expected async stop worker to mark server stopped, got %+v", updated)
}

func TestDeleteServerReturnsAcceptedBeforeRuntimeCompletes(t *testing.T) {
	adapter := newBlockingRuntimeAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("async-delete", cfg.DataDir)
	server.ContainerID = "existing-container"
	server.Status = domain.StatusStopped
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/"+server.ID, nil))
		response <- recorder
	}()

	select {
	case <-adapter.removeStarted:
	case <-time.After(time.Second):
		t.Fatal("expected async delete worker to begin removing the runtime container")
	}

	select {
	case recorder := <-response:
		if recorder.Code != stdhttp.StatusAccepted {
			t.Fatalf("expected delete 202, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var queued domain.GameServerInstance
		if err := json.Unmarshal(recorder.Body.Bytes(), &queued); err != nil {
			t.Fatal(err)
		}
		if queued.Status != domain.StatusDeleting {
			t.Fatalf("expected queued delete status deleting, got %+v", queued)
		}
	case <-time.After(100 * time.Millisecond):
		close(adapter.removeRelease)
		recorder := <-response
		t.Fatalf("delete request blocked until runtime completed; got %d: %s", recorder.Code, recorder.Body.String())
	}

	close(adapter.removeRelease)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := db.GetServer(context.Background(), server.ID); errors.Is(err, store.ErrNotFound) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	updated, err := db.GetServer(context.Background(), server.ID)
	t.Fatalf("expected async delete worker to remove server record, got server=%+v err=%v", updated, err)
}

func TestServerLifecycleAndLogEndpoints(t *testing.T) {
	router, db, _ := newTestRouter(t)
	createPayload := `{
		"name":"Vanilla Test",
		"providerKey":"terraria-vanilla",
		"hostPort":17777,
		"config":{
			"serverName":"Vanilla Test",
			"worldName":"TestWorld",
			"worldSize":"medium",
			"difficulty":"classic",
			"maxPlayers":8,
			"port":7777,
			"secure":true,
			"language":"zh-Hans",
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
	if server.GameKey != domain.GameTerraria {
		t.Fatalf("expected provider game key %q, got %q", domain.GameTerraria, server.GameKey)
	}
	if server.Port != 7777 || server.HostPort != 17777 {
		t.Fatalf("expected fixed internal port and requested external port, got internal=%d external=%d", server.Port, server.HostPort)
	}
	if server.Config.Language != terraria.DefaultLanguage {
		t.Fatalf("expected created server language to be fixed to %s, got %q", terraria.DefaultLanguage, server.Config.Language)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", start.Code, start.Body.String())
	}
	var started domain.GameServerInstance
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Status != domain.StatusStarting {
		t.Fatalf("expected start to queue a starting status, got %+v", started)
	}
	server = waitForServerStatus(t, db, server.ID, domain.StatusRunning)
	if server.ContainerID == "" {
		t.Fatalf("expected async start to create runtime container, got %+v", server)
	}

	command := httptest.NewRecorder()
	router.ServeHTTP(command, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/command", bytes.NewBufferString(`{"command":"say hello"}`)))
	if command.Code != stdhttp.StatusOK {
		t.Fatalf("expected command 200, got %d: %s", command.Code, command.Body.String())
	}

	restart := httptest.NewRecorder()
	router.ServeHTTP(restart, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/restart", nil))
	if restart.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected restart 202, got %d: %s", restart.Code, restart.Body.String())
	}
	var restarted domain.GameServerInstance
	if err := json.Unmarshal(restart.Body.Bytes(), &restarted); err != nil {
		t.Fatal(err)
	}
	if restarted.Status != domain.StatusRestarting {
		t.Fatalf("expected restart to queue a restarting status, got %+v", restarted)
	}
	server = waitForServerStatus(t, db, server.ID, domain.StatusRunning)

	stop := httptest.NewRecorder()
	router.ServeHTTP(stop, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/stop", nil))
	if stop.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected stop 202, got %d: %s", stop.Code, stop.Body.String())
	}
	var stopped domain.GameServerInstance
	if err := json.Unmarshal(stop.Body.Bytes(), &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Status != domain.StatusStopping {
		t.Fatalf("expected stop status stopping, got %+v", stopped)
	}
	server = waitForServerStatus(t, db, server.ID, domain.StatusStopped)

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
	if remove.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected delete 202, got %d: %s", remove.Code, remove.Body.String())
	}
	waitForServerDeleted(t, db, server.ID)
}

func waitForServerStatus(t *testing.T, db *store.Store, id string, status domain.ServerStatus) domain.GameServerInstance {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		server, err := db.GetServer(context.Background(), id)
		if err == nil && server.Status == status {
			return server
		}
		time.Sleep(10 * time.Millisecond)
	}
	server, err := db.GetServer(context.Background(), id)
	t.Fatalf("expected server %s to reach status %s, got server=%+v err=%v", id, status, server, err)
	return domain.GameServerInstance{}
}

func waitForServerDeleted(t *testing.T, db *store.Store, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := db.GetServer(context.Background(), id); errors.Is(err, store.ErrNotFound) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	server, err := db.GetServer(context.Background(), id)
	t.Fatalf("expected server %s to be deleted, got server=%+v err=%v", id, server, err)
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
			"language":"zh-Hans",
			"autoCreateWorld":true
		}
	}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(payload)))
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected unsupported version 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestConfigPresetStripsSecretsAndListsSavedPreset(t *testing.T) {
	router, _, _ := newTestRouter(t)
	payload := `{
		"name":"Palworld Friends",
		"providerKey":"palworld",
		"version":"latest",
		"resources":{"cpuLimitCores":1,"memoryLimitMb":2048},
		"config":{
			"serverName":"Pal Friends",
			"saveName":"Starter Save",
			"maxPlayers":10,
			"serverPassword":"join-secret",
			"adminPassword":"admin-secret"
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/config-presets", bytes.NewBufferString(payload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected config preset 201, got %d: %s", create.Code, create.Body.String())
	}
	var preset domain.ConfigPreset
	if err := json.Unmarshal(create.Body.Bytes(), &preset); err != nil {
		t.Fatal(err)
	}
	if preset.GameKey != domain.GamePalworld || preset.ProviderKey != domain.ProviderPalworld {
		t.Fatalf("expected Palworld preset identity, got %+v", preset)
	}
	if preset.Config.Password != "" || preset.Config.MOTD != "" {
		t.Fatalf("expected preset config secrets to be stripped, got password=%q motd=%q", preset.Config.Password, preset.Config.MOTD)
	}
	if _, ok := preset.ConfigPayload["serverPassword"]; ok {
		t.Fatalf("expected server password to be stripped from payload, got %+v", preset.ConfigPayload)
	}
	if _, ok := preset.ConfigPayload["adminPassword"]; ok {
		t.Fatalf("expected admin password to be stripped from payload, got %+v", preset.ConfigPayload)
	}
	if preset.ConfigPayload["saveName"] != "Starter Save" || preset.CPULimitCores != 1 || preset.MemoryLimitMB != 2048 {
		t.Fatalf("expected non-secret preset values to be saved, got %+v payload=%+v", preset, preset.ConfigPayload)
	}
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/config-presets", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected list config presets 200, got %d: %s", list.Code, list.Body.String())
	}
	var presets []domain.ConfigPreset
	if err := json.Unmarshal(list.Body.Bytes(), &presets); err != nil {
		t.Fatal(err)
	}
	if len(presets) != 1 || presets[0].ID != preset.ID {
		t.Fatalf("expected saved preset in list, got %+v", presets)
	}
}

func TestCreatePalworldServerUsesPalworldRuntimeSpec(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, _ := newTestRouterWithAdapter(t, adapter)
	payload := `{
		"name":"Pal Friends",
		"providerKey":"palworld",
		"hostPort":18211,
		"config":{
			"serverName":"Pal Friends",
			"saveName":"Starter Save",
			"maxPlayers":10,
			"serverPassword":"join-secret",
			"adminPassword":"admin-secret"
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(payload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server 201, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServerInstance
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.GameKey != domain.GamePalworld || server.ProviderKey != domain.ProviderPalworld {
		t.Fatalf("expected Palworld server identity, got %+v", server)
	}
	if server.Port != 8211 || server.HostPort != 18211 {
		t.Fatalf("expected Palworld ports, got internal=%d external=%d", server.Port, server.HostPort)
	}
	if server.WorldName != "Starter Save" || server.Password != "join-secret" || server.Config.MOTD != "admin-secret" {
		t.Fatalf("expected Palworld config to be mapped to runtime fields, got server=%+v config=%+v", server, server.Config)
	}
	if server.ConfigPayload["saveName"] != "Starter Save" || server.ConfigPayload["adminPassword"] != "admin-secret" {
		t.Fatalf("expected semantic Palworld config payload, got %+v", server.ConfigPayload)
	}
	if server.JoinInfo.Port != 18211 || server.JoinInfo.Address != "127.0.0.1" || !strings.Contains(server.JoinInfo.InviteText, "Palworld") {
		t.Fatalf("expected Palworld join info in create response, got %+v", server.JoinInfo)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", start.Code, start.Body.String())
	}
	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected async start to create runtime container")
	}
	if spec.Image != "thijsvanloef/palworld-server-docker:latest" {
		t.Fatalf("expected Palworld image, got %q", spec.Image)
	}
	if spec.Port != 8211 || spec.Options.PortProtocol != "udp" {
		t.Fatalf("expected Palworld UDP port mapping, got port=%d protocol=%q", spec.Port, spec.Options.PortProtocol)
	}
	waitForServerStatus(t, db, server.ID, domain.StatusRunning)
}

func TestCreateDSTServerUsesDSTRuntimeSpec(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, _ := newTestRouterWithAdapter(t, adapter)
	payload := `{
		"name":"DST Friends",
		"providerKey":"dont-starve-together",
		"hostPort":11099,
		"config":{
			"serverName":"DST Friends",
			"clusterName":"FriendsCluster",
			"maxPlayers":6,
			"serverPassword":"join-secret",
			"clusterToken":"klei-token",
			"gameMode":"endless",
			"worldPreset":"forest_classic",
			"cavesEnabled":true,
			"workshopIds":"123456789,987654321"
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(payload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server 201, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServerInstance
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.GameKey != domain.GameDST || server.ProviderKey != domain.ProviderDST {
		t.Fatalf("expected DST server identity, got %+v", server)
	}
	if server.Port != 10999 || server.HostPort != 11099 {
		t.Fatalf("expected DST ports, got internal=%d external=%d", server.Port, server.HostPort)
	}
	if server.WorldName != "FriendsCluster" || server.Password != "join-secret" || server.Config.MOTD != "klei-token" {
		t.Fatalf("expected DST config to be mapped to runtime fields, got server=%+v config=%+v", server, server.Config)
	}
	if server.ConfigPayload["clusterName"] != "FriendsCluster" || server.ConfigPayload["clusterToken"] != "klei-token" || server.ConfigPayload["gameMode"] != "endless" || server.ConfigPayload["worldPreset"] != "forest_classic" || server.ConfigPayload["cavesEnabled"] != true {
		t.Fatalf("expected semantic DST config payload, got %+v", server.ConfigPayload)
	}
	if server.JoinInfo.Port != 11099 || server.JoinInfo.Address != "127.0.0.1" || !strings.Contains(server.JoinInfo.InviteText, "Don't Starve Together") {
		t.Fatalf("expected DST join info in create response, got %+v", server.JoinInfo)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", start.Code, start.Body.String())
	}
	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected async start to create runtime container")
	}
	if spec.Image != "smartcat99999/dst-server:latest" {
		t.Fatalf("expected DST image, got %q", spec.Image)
	}
	if spec.Port != 10999 || spec.Options.PortProtocol != "udp" {
		t.Fatalf("expected DST UDP port mapping, got port=%d protocol=%q", spec.Port, spec.Options.PortProtocol)
	}
	if !strings.Contains(spec.Options.Files["dst/FriendsCluster/cluster.ini"], "game_mode = endless") {
		t.Fatalf("expected DST cluster.ini in runtime files, got %+v", spec.Options.Files)
	}
	if !strings.Contains(spec.Options.Files["dst/FriendsCluster/Master/worldgen.lua"], `preset = "forest_classic"`) {
		t.Fatalf("expected DST world preset in runtime files, got %+v", spec.Options.Files)
	}
	if _, ok := spec.Options.Files["dst/FriendsCluster/Caves/server.ini"]; !ok {
		t.Fatalf("expected DST caves shard files, got %+v", spec.Options.Files)
	}
	if !strings.Contains(spec.Options.Files["dst/FriendsCluster/dedicated_server_mods_setup.lua"], `ServerModSetup("123456789")`) {
		t.Fatalf("expected DST workshop setup file, got %+v", spec.Options.Files)
	}
	if spec.Options.Files["dst/FriendsCluster/cluster_token.txt"] != "klei-token\n" {
		t.Fatalf("expected DST token file, got %q", spec.Options.Files["dst/FriendsCluster/cluster_token.txt"])
	}
	waitForServerStatus(t, db, server.ID, domain.StatusRunning)
}

func TestCreateDSTServerRejectsArmRuntime(t *testing.T) {
	router, _, _ := newTestRouterWithAdapter(t, armMockAdapter{availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}})
	payload := `{
		"name":"DST Friends",
		"providerKey":"dont-starve-together",
		"hostPort":11099,
		"config":{
			"serverName":"DST Friends",
			"clusterName":"FriendsCluster",
			"maxPlayers":6,
			"clusterToken":"klei-token"
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(payload)))
	if create.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected create server 400 on arm runtime, got %d: %s", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "amd64 Docker hosts") {
		t.Fatalf("expected arm runtime rejection detail, got %s", create.Body.String())
	}
}

func TestCreateMinecraftServerUsesMinecraftRuntimeSpec(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, _ := newTestRouterWithAdapter(t, adapter)
	payload := `{
		"name":"Friends MC",
		"providerKey":"minecraft",
		"hostPort":25565,
		"version":"1.20.4",
		"config":{
			"serverName":"Friends MC",
			"worldName":"survival-island",
			"maxPlayers":12,
			"gameMode":"survival",
			"difficulty":"normal",
			"onlineMode":true,
			"whitelistEnabled":false,
			"eulaAccepted":true
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(payload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server 201, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServerInstance
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.GameKey != domain.GameMinecraft || server.ProviderKey != domain.ProviderMinecraft {
		t.Fatalf("expected Minecraft server identity, got %+v", server)
	}
	if server.Port != 25565 {
		t.Fatalf("expected Minecraft internal port 25565, got %d", server.Port)
	}
	if server.ConfigPayload["eulaAccepted"] != true || server.ConfigPayload["gameMode"] != "survival" {
		t.Fatalf("expected semantic Minecraft config payload, got %+v", server.ConfigPayload)
	}
	if !strings.Contains(server.JoinInfo.InviteText, "Minecraft") || server.JoinInfo.Port != 25565 {
		t.Fatalf("expected Minecraft join info in create response, got %+v", server.JoinInfo)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", start.Code, start.Body.String())
	}
	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected async start to create runtime container")
	}
	if spec.Image != "itzg/minecraft-server:latest" {
		t.Fatalf("expected Minecraft image, got %q", spec.Image)
	}
	if !containsEnv(spec.Options.Env, "VERSION=1.20.4") {
		t.Fatalf("expected Minecraft VERSION=1.20.4 env, got %v", spec.Options.Env)
	}
	if spec.Port != 25565 || spec.Options.PortProtocol != "tcp" {
		t.Fatalf("expected Minecraft TCP port mapping, got port=%d protocol=%q", spec.Port, spec.Options.PortProtocol)
	}
	properties := spec.Options.Files["data/server.properties"]
	for _, expected := range []string{"max-players=12", "motd=Friends MC", "level-name=survival-island", "gamemode=survival", "difficulty=normal", "online-mode=true"} {
		if !strings.Contains(properties, expected) {
			t.Fatalf("expected server.properties to contain %q, got:\n%s", expected, properties)
		}
	}
	if spec.Options.Files["data/eula.txt"] != "eula=true\n" {
		t.Fatalf("expected Minecraft eula=true, got %q", spec.Options.Files["data/eula.txt"])
	}
	waitForServerStatus(t, db, server.ID, domain.StatusRunning)
}

func TestUpdatePalworldConfigPersistsSemanticPayload(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("palworld-config-update", cfg.DataDir)
	server.GameKey = domain.GamePalworld
	server.ProviderKey = domain.ProviderPalworld
	server.Port = 8211
	server.HostPort = 18211
	server.Config = palworld.NewProvider().DefaultConfig()
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	payload := `{
		"config":{
			"serverName":"Pal Update",
			"saveName":"Updated Save",
			"maxPlayers":16,
			"serverPassword":"join-updated",
			"adminPassword":"admin-updated"
		}
	}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/"+server.ID+"/config", bytes.NewBufferString(payload)))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected config update 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var updated domain.GameServerInstance
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.WorldName != "Updated Save" || updated.MaxPlayers != 16 || updated.Password != "join-updated" || updated.Config.MOTD != "admin-updated" {
		t.Fatalf("expected updated Palworld runtime mapping, got server=%+v config=%+v", updated, updated.Config)
	}
	if updated.ConfigPayload["saveName"] != "Updated Save" || updated.ConfigPayload["serverPassword"] != "join-updated" {
		t.Fatalf("expected updated semantic Palworld config payload, got %+v", updated.ConfigPayload)
	}
}

func TestGameCatalogEndpoints(t *testing.T) {
	router, _, _ := newTestRouter(t)

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected game catalog 200, got %d: %s", list.Code, list.Body.String())
	}
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(list.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	var terrariaGame *domain.GameCatalogEntry
	var palworldGame *domain.GameCatalogEntry
	var minecraftGame *domain.GameCatalogEntry
	for index := range games {
		switch games[index].Key {
		case domain.GameTerraria:
			terrariaGame = &games[index]
		case domain.GamePalworld:
			palworldGame = &games[index]
		case domain.GameMinecraft:
			minecraftGame = &games[index]
		}
	}
	if terrariaGame == nil || terrariaGame.Status != "available" || len(terrariaGame.Providers) != 2 {
		t.Fatalf("expected available Terraria entry with two providers, got %+v", terrariaGame)
	}
	if palworldGame == nil || palworldGame.Status != "available" || len(palworldGame.Providers) != 1 {
		t.Fatalf("expected available Palworld entry with provider, got %+v", palworldGame)
	}
	if minecraftGame == nil || minecraftGame.Status != "available" || len(minecraftGame.Providers) != 1 {
		t.Fatalf("expected available Minecraft entry with provider, got %+v", minecraftGame)
	}
	if palworldGame.Providers[0].Key != domain.ProviderPalworld || palworldGame.Providers[0].Capabilities.ConsoleCommands {
		t.Fatalf("expected Palworld provider without console commands, got %+v", palworldGame.Providers[0])
	}
	if len(terrariaGame.Providers[0].ConfigSchema) == 0 || !terrariaGame.Providers[0].Capabilities.ConsoleCommands {
		t.Fatalf("expected provider schema and capabilities, got %+v", terrariaGame.Providers[0])
	}

	versions := httptest.NewRecorder()
	router.ServeHTTP(versions, httptest.NewRequest(stdhttp.MethodGet, "/api/games/terraria/versions", nil))
	if versions.Code != stdhttp.StatusOK {
		t.Fatalf("expected game versions 200, got %d: %s", versions.Code, versions.Body.String())
	}
	var versionPayload map[domain.ProviderKey][]string
	if err := json.Unmarshal(versions.Body.Bytes(), &versionPayload); err != nil {
		t.Fatal(err)
	}
	if len(versionPayload[domain.ProviderTerrariaVanilla]) == 0 || len(versionPayload[domain.ProviderTerrariaTModLoader]) == 0 {
		t.Fatalf("expected provider versions, got %+v", versionPayload)
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(stdhttp.MethodGet, "/api/games/unknown-game", nil))
	if missing.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected unknown game 404, got %d: %s", missing.Code, missing.Body.String())
	}
}

func TestObservabilityMetricsEndpoints(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("metrics-server", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.PlayersOnline = 3
	server.MaxPlayers = 8
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
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
		"gamepanel_runtime_running_containers",
		"gamepanel_activity_events_24h_total 1",
		`gamepanel_server_players_online{server_id="metrics-server",game_key="terraria",provider_key="terraria-vanilla",status="running"} 3`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected prometheus metrics to contain %q, got:\n%s", expected, body)
		}
	}
}

func TestRuntimeInstallStateRequiresLocalArchive(t *testing.T) {
	router, _, cfg := newTestRouterWithAdapterAndInstallMarkers(t, availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}, false)

	runtimeStatus := func() domain.RuntimeImageStatus {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
		if recorder.Code != stdhttp.StatusOK {
			t.Fatalf("expected game catalog 200, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var games []domain.GameCatalogEntry
		if err := json.Unmarshal(recorder.Body.Bytes(), &games); err != nil {
			t.Fatal(err)
		}
		for _, game := range games {
			if game.Key != domain.GameTerraria {
				continue
			}
			for _, providerCatalog := range game.Providers {
				if providerCatalog.Key == domain.ProviderTerrariaVanilla {
					return providerCatalog.RuntimeImage
				}
			}
		}
		t.Fatal("expected Terraria vanilla provider in catalog")
		return domain.RuntimeImageStatus{}
	}

	if status := runtimeStatus(); status.Status != runtime.ImageStatusMissing {
		t.Fatalf("expected missing without local install marker and archive, got %+v", status)
	}

	createPayload := `{
		"name":"Vanilla Test",
		"providerKey":"terraria-vanilla",
		"version":"1.4.4.9",
		"hostPort":17777,
		"config":{
			"serverName":"Vanilla Test",
			"worldName":"TestWorld",
			"worldSize":"medium",
			"difficulty":"classic",
			"maxPlayers":8,
			"port":7777,
			"secure":true,
			"language":"zh-Hans",
			"autoCreateWorld":true
		}
	}`
	blockedCreate := httptest.NewRecorder()
	router.ServeHTTP(blockedCreate, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(createPayload)))
	if blockedCreate.Code != stdhttp.StatusConflict {
		t.Fatalf("expected create server to require local runtime install, got %d: %s", blockedCreate.Code, blockedCreate.Body.String())
	}

	markerPath := filepath.Join(cfg.DataDir, "runtime-installs", "terraria-vanilla", "1.4.4.9.json")
	if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected runtime install marker not to be repaired from Docker cache, got %v", err)
	}
}

func TestRuntimeInstallPullProgressIsReservedBelowComplete(t *testing.T) {
	tests := []struct {
		name     string
		progress int
		want     int
	}{
		{name: "empty", progress: 0, want: 0},
		{name: "half", progress: 50, want: 45},
		{name: "nearly done", progress: 99, want: 89},
		{name: "pull complete", progress: 100, want: 90},
		{name: "over complete", progress: 120, want: 90},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeInstallPullProgress(test.progress); got != test.want {
				t.Fatalf("expected %d, got %d", test.want, got)
			}
		})
	}
}

func TestRuntimeInstallStateUsesLocalArchiveWhenDockerImageIsMissing(t *testing.T) {
	router, _, _ := newTestRouterWithAdapter(t, missingImageAdapter{availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected game catalog 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, game := range games {
		if game.Key != domain.GameTerraria {
			continue
		}
		for _, providerCatalog := range game.Providers {
			if providerCatalog.Key != domain.ProviderTerrariaVanilla {
				continue
			}
			found = true
			if providerCatalog.RuntimeImage.Status != runtime.ImageStatusReady {
				t.Fatalf("expected local marker and archive to mark runtime ready, got %+v", providerCatalog.RuntimeImage)
			}
		}
	}
	if !found {
		t.Fatal("expected Terraria vanilla provider in catalog")
	}
}

func TestRuntimeInstallStateDoesNotTrustMarkerWhenArchiveIsMissing(t *testing.T) {
	router, _, cfg := newTestRouterWithAdapterAndInstallMarkers(t, availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}, true)
	archivePath := filepath.Join(cfg.DataDir, "runtime-images", "terraria-vanilla", "1.4.5.6.tar")
	if err := os.Remove(archivePath); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected game catalog 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(recorder.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	for _, game := range games {
		if game.Key != domain.GameTerraria {
			continue
		}
		for _, providerCatalog := range game.Providers {
			if providerCatalog.Key != domain.ProviderTerrariaVanilla {
				continue
			}
			if providerCatalog.RuntimeImage.Status != runtime.ImageStatusMissing {
				t.Fatalf("expected missing archive to make runtime missing, got %+v", providerCatalog.RuntimeImage)
			}
			return
		}
	}
	t.Fatal("expected Terraria vanilla provider in catalog")
}

func TestGameCatalogMarksDSTUnsupportedOnArmRuntime(t *testing.T) {
	router, _, _ := newTestRouterWithAdapter(t, armMockAdapter{availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}})

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected game catalog 200, got %d: %s", list.Code, list.Body.String())
	}
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(list.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	var dstGame *domain.GameCatalogEntry
	for index := range games {
		if games[index].Key == domain.GameDST {
			dstGame = &games[index]
			break
		}
	}
	if dstGame == nil || dstGame.Status != "unsupported" {
		t.Fatalf("expected DST to be marked unsupported on arm runtime, got %+v", dstGame)
	}

	detail := httptest.NewRecorder()
	router.ServeHTTP(detail, httptest.NewRequest(stdhttp.MethodGet, "/api/games/dont-starve-together", nil))
	if detail.Code != stdhttp.StatusOK {
		t.Fatalf("expected DST game detail 200, got %d: %s", detail.Code, detail.Body.String())
	}
	var got domain.GameCatalogEntry
	if err := json.Unmarshal(detail.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "unsupported" {
		t.Fatalf("expected DST game detail to be unsupported on arm runtime, got %+v", got)
	}
}

func TestCreateServerPersistsResourceLimitsInRuntimeSpec(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, _ := newTestRouterWithAdapter(t, adapter)
	payload := `{
		"name":"Limited Server",
		"providerKey":"terraria-vanilla",
		"hostPort":17778,
		"resources":{"cpuLimitCores":1.5,"memoryLimitMb":2048},
		"config":{
			"serverName":"Limited Server",
			"worldName":"LimitedWorld",
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
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(payload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server 201, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServerInstance
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.CPULimitCores != 1.5 || server.MemoryLimitMB != 2048 {
		t.Fatalf("expected persisted resource limits, got cpu=%v memory=%d", server.CPULimitCores, server.MemoryLimitMB)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", start.Code, start.Body.String())
	}
	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected async start to create runtime container")
	}
	if spec.Resources.CPULimitCores != 1.5 || spec.Resources.MemoryLimitMB != 2048 {
		t.Fatalf("expected runtime resource limits, got %+v", spec.Resources)
	}
	waitForServerStatus(t, db, server.ID, domain.StatusRunning)
}

func TestUpdateServerConfigPersistsResourceLimitsAndRequiresRestartWhenRunning(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("resource-update", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "container-1"
	server.ConfigRevision = 2
	server.AppliedConfigRevision = 2
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	payload := `{
		"hostPort":18888,
		"resources":{"cpuLimitCores":2,"memoryLimitMb":4096},
		"config":{
			"serverName":"Resource Update",
			"worldName":"ResourceWorld",
			"worldSize":"medium",
			"difficulty":"classic",
			"maxPlayers":10,
			"port":7777,
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/"+server.ID+"/config", bytes.NewBufferString(payload)))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected config update 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var updated domain.GameServerInstance
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.CPULimitCores != 2 || updated.MemoryLimitMB != 4096 || updated.HostPort != 18888 {
		t.Fatalf("expected updated resource and port limits, got %+v", updated)
	}
	if updated.ConfigRevision != 3 || updated.AppliedConfigRevision != 2 {
		t.Fatalf("expected running config update to wait for restart, got revision=%d applied=%d", updated.ConfigRevision, updated.AppliedConfigRevision)
	}
	stored, err := db.GetServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CPULimitCores != 2 || stored.MemoryLimitMB != 4096 {
		t.Fatalf("expected stored resource limits, got cpu=%v memory=%d", stored.CPULimitCores, stored.MemoryLimitMB)
	}
}

func TestDeleteServerRemovesOwnedResources(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("owned-resources", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(server.ID, "owned.wld", bytes.NewBufferString("world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:               "world-1",
		InstanceID:       server.ID,
		ActiveInstanceID: server.ID,
		Name:             "owned",
		FileName:         "owned.wld",
		SizeBytes:        5,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte("world"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupPath, backupSize, err := backupsvc.NewService(cfg.DataDir).Create(server.ID, server.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	backup := domain.Backup{
		ID:         "backup-1",
		InstanceID: server.ID,
		FileName:   filepath.Base(backupPath),
		WorldName:  server.WorldName,
		SizeBytes:  backupSize,
		Type:       "manual",
		CreatedAt:  time.Now(),
	}
	if err := db.CreateBackup(context.Background(), &backup); err != nil {
		t.Fatal(err)
	}
	if _, _, err := modsvc.NewService(cfg.DataDir).Upload(server.ID, "owned.tmod", bytes.NewBufferString("mod")); err != nil {
		t.Fatal(err)
	}
	mod := domain.ModFile{
		ID:         "mod-1",
		InstanceID: server.ID,
		FileName:   "owned.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/"+server.ID, nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected delete server 202, got %d: %s", recorder.Code, recorder.Body.String())
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := db.GetServer(context.Background(), server.ID); errors.Is(err, store.ErrNotFound) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := db.GetServer(context.Background(), server.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected server record deleted, got err=%v", err)
	}
	if _, err := db.GetWorld(context.Background(), world.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected owned world record deleted, got err=%v", err)
	}
	if _, err := db.GetBackup(context.Background(), backup.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected owned backup record deleted, got err=%v", err)
	}
	if _, err := db.GetMod(context.Background(), mod.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected owned mod record deleted, got err=%v", err)
	}
	for _, path := range []string{
		filepath.Join(cfg.DataDir, "worlds", server.ID, "owned.wld"),
		filepath.Join(cfg.DataDir, "backups", server.ID, backup.FileName),
		filepath.Join(cfg.DataDir, "mods", server.ID, "owned.tmod"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected owned resource file removed at %s, stat err=%v", path, err)
		}
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

func TestRunningServerLogSnapshotKeepsRunningStatus(t *testing.T) {
	adapter := &statusCapturingLogsAdapter{
		availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()},
	}
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("running-log-snapshot", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "runtime-container"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	snapshot := httptest.NewRecorder()
	router.ServeHTTP(snapshot, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/logs/snapshot", nil))
	if snapshot.Code != stdhttp.StatusOK {
		t.Fatalf("expected running log snapshot 200, got %d: %s", snapshot.Code, snapshot.Body.String())
	}
	if adapter.logStatus != domain.StatusRunning {
		t.Fatalf("expected runtime logs to be read with running status, got %q", adapter.logStatus)
	}
	var payload struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(snapshot.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Lines) != 1 || !strings.Contains(payload.Lines[0], "running log snapshot") {
		t.Fatalf("expected running snapshot logs, got %+v", payload)
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
	if stop.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected stop to clear stale runtime container, got %d: %s", stop.Code, stop.Body.String())
	}
	var stopped domain.GameServerInstance
	if err := json.Unmarshal(stop.Body.Bytes(), &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Status != domain.StatusStopping {
		t.Fatalf("expected queued stop status stopping, got %+v", stopped)
	}
	stored := waitForServerStatus(t, db, server.ID, domain.StatusStopped)
	if stored.ContainerID != "" {
		t.Fatalf("expected stopped server with cleared container, got %+v", stored)
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

func TestGetServerTreatsMissingRuntimeContainerAsStopped(t *testing.T) {
	router, db, cfg := newTestRouterWithAdapter(t, inspectStatusAdapter{
		status: domain.StatusErrored,
		err:    fmt.Errorf("no Docker container found for server stale-runtime"),
	})
	server := testServer("missing-runtime-detail", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "missing-container"
	server.LastError = "old runtime error"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/missing-runtime-detail", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected server detail 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var got domain.GameServerInstance
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusStopped || got.ContainerID != "" || got.LastError != "" {
		t.Fatalf("expected missing runtime to be recovered as stopped, got %+v", got)
	}
	stored, err := db.GetServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.StatusStopped || stored.ContainerID != "" || stored.LastError != "" {
		t.Fatalf("expected stored missing runtime recovery, got %+v", stored)
	}
}

func TestGetServerClearsStaleMissingRuntimeErrorWithoutContainerID(t *testing.T) {
	router, db, cfg := newTestRouterWithAdapter(t, inspectStatusAdapter{status: domain.StatusRunning})
	server := testServer("missing-runtime-without-container", cfg.DataDir)
	server.Status = domain.StatusErrored
	server.ContainerID = ""
	server.LastError = "no Docker container found for server missing-runtime-without-container"
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/missing-runtime-without-container", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected server detail 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var got domain.GameServerInstance
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusStopped || got.LastError != "" {
		t.Fatalf("expected stale missing runtime error to be cleared, got %+v", got)
	}
}

func TestGetServerMarksStaleLifecyclePendingWithoutContainerAsErrored(t *testing.T) {
	router, db, cfg := newTestRouterWithAdapter(t, inspectStatusAdapter{status: domain.StatusRunning})
	server := testServer("stale-starting-without-container", cfg.DataDir)
	server.Status = domain.StatusStarting
	server.ContainerID = ""
	server.UpdatedAt = time.Now().Add(-staleLifecyclePendingAfter - time.Second)
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/stale-starting-without-container", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected server detail 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var got domain.GameServerInstance
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusErrored || got.LastError == "" {
		t.Fatalf("expected stale pending server to be marked errored, got %+v", got)
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
	modBytes := tmodFixture("ExampleMod", "1.2.3", "2026.3.3.0")
	if _, err := part.Write(modBytes); err != nil {
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
	if mod.Title != "ExampleMod" || mod.ModVersion != "1.2.3" || mod.TModVersion != "2026.3.3.0" {
		t.Fatalf("expected parsed tmod metadata, got %+v", mod)
	}
	runtimeMod, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "example.tmod"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(runtimeMod, modBytes) {
		t.Fatalf("expected uploaded mod copied into runtime data dir")
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
	if mods[0].RuntimeEnabled == nil || !*mods[0].RuntimeEnabled {
		t.Fatalf("expected listed mod to be runtime enabled, got %+v", mods[0])
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "Mods", "enabled.json"), []byte("[]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	relist := httptest.NewRecorder()
	router.ServeHTTP(relist, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/tmod/mods", nil))
	if relist.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod relist 200, got %d: %s", relist.Code, relist.Body.String())
	}
	if err := json.Unmarshal(relist.Body.Bytes(), &mods); err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 || mods[0].RuntimeEnabled == nil || *mods[0].RuntimeEnabled {
		t.Fatalf("expected configured mod to be runtime disabled after enabled.json changed, got %+v", mods)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/tmod/mods/"+mod.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "mods", "tmod", "example.tmod")); !os.IsNotExist(err) {
		t.Fatalf("expected mod file deleted, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(server.DataDir, "Mods", "example.tmod")); !os.IsNotExist(err) {
		t.Fatalf("expected runtime mod file deleted, stat err=%v", err)
	}
}

func TestModListsPruneMissingFiles(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	serverMod := domain.ModFile{
		ID:         "server-missing-mod",
		InstanceID: server.ID,
		FileName:   "missing.tmod",
		SizeBytes:  10,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	globalMod := domain.ModFile{
		ID:         "global-missing-mod",
		InstanceID: "unassigned",
		FileName:   "library.tmod",
		SizeBytes:  10,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &serverMod); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateMod(context.Background(), &globalMod); err != nil {
		t.Fatal(err)
	}

	serverList := httptest.NewRecorder()
	router.ServeHTTP(serverList, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/tmod/mods", nil))
	if serverList.Code != stdhttp.StatusOK {
		t.Fatalf("expected server mod list 200, got %d: %s", serverList.Code, serverList.Body.String())
	}
	var serverMods []domain.ModFile
	if err := json.Unmarshal(serverList.Body.Bytes(), &serverMods); err != nil {
		t.Fatal(err)
	}
	if len(serverMods) != 0 {
		t.Fatalf("expected missing server mod pruned from response, got %+v", serverMods)
	}
	if _, err := db.GetMod(context.Background(), serverMod.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing server mod record pruned, got err=%v", err)
	}

	globalList := httptest.NewRecorder()
	router.ServeHTTP(globalList, httptest.NewRequest(stdhttp.MethodGet, "/api/mods", nil))
	if globalList.Code != stdhttp.StatusOK {
		t.Fatalf("expected global mod list 200, got %d: %s", globalList.Code, globalList.Body.String())
	}
	var globalMods []domain.ModFile
	if err := json.Unmarshal(globalList.Body.Bytes(), &globalMods); err != nil {
		t.Fatal(err)
	}
	if len(globalMods) != 0 {
		t.Fatalf("expected missing global mod pruned from response, got %+v", globalMods)
	}
	if _, err := db.GetMod(context.Background(), globalMod.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing global mod record pruned, got err=%v", err)
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

func TestTModLoaderWorkshopImportWritesInstallFile(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/servers/tmod/mods/workshop", bytes.NewBufferString(`{"workshopIds":["2563309347","2824688072","2563309347"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected workshop import 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var items []domain.ModFile
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected requested workshop records plus dependency, got %+v", items)
	}
	if items[0].Source != "workshop" || items[0].WorkshopID == "" || items[0].FileName == "install.txt" {
		t.Fatalf("expected workshop mod record, got %+v", items[0])
	}
	expected := "2563309347\n2824688072\n2908170107\n"
	runtimeInstall, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "install.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeInstall) != expected {
		t.Fatalf("expected runtime install.txt %q, got %q", expected, string(runtimeInstall))
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
	for _, mod := range mods {
		if mod.Source == "workshop" && (mod.RuntimePresent == nil || *mod.RuntimePresent) {
			t.Fatalf("expected workshop mod to be marked unsynced until runtime file exists, got %+v", mod)
		}
	}
}

func TestGlobalWorkshopImportCreatesLibraryWorkshopRecords(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_ = cfg

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/workshop", bytes.NewBufferString(`{"workshopIds":["2563309347","2824688072","2563309347"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected global workshop import 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var items []domain.ModFile
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two global workshop records, got %+v", items)
	}
	for _, item := range items {
		if item.InstanceID != "unassigned" || item.Source != "workshop" || item.WorkshopID == "" || item.FileName == "install.txt" {
			t.Fatalf("expected unassigned workshop record, got %+v", item)
		}
	}
	mods, err := db.ListMods(context.Background(), "unassigned")
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 2 {
		t.Fatalf("expected two global workshop mods, got %+v", mods)
	}
}

func TestGlobalWorkshopImportRejectsArmRuntime(t *testing.T) {
	router, _, _ := newTestRouterWithAdapter(t, armMockAdapter{availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/workshop", bytes.NewBufferString(`{"workshopIds":["2563309347"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusConflict {
		t.Fatalf("expected global workshop import conflict on arm runtime, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestGlobalWorkshopImportRejectsDuplicateWorkshopID(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_ = cfg
	existing := domain.ModFile{
		ID:             "existing-workshop",
		InstanceID:     "unassigned",
		FileName:       "workshop-2563309347",
		Source:         "workshop",
		WorkshopID:     "2563309347",
		Title:          "Magic Storage",
		CreatorSteamID: "76561198122163241",
		SizeBytes:      5643106,
		Enabled:        true,
		CreatedAt:      time.Now(),
	}
	if err := db.CreateMod(context.Background(), &existing); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/workshop", bytes.NewBufferString(`{"workshopIds":["2563309347"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusConflict {
		t.Fatalf("expected duplicate workshop import 409, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestRecommendedModsMarksExistingLibraryItems(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_ = cfg
	existing := domain.ModFile{
		ID:             "existing-workshop",
		InstanceID:     "unassigned",
		FileName:       "workshop-2563309347",
		Source:         "workshop",
		WorkshopID:     "2563309347",
		Title:          "Magic Storage",
		CreatorSteamID: "76561198122163241",
		SizeBytes:      5643106,
		Enabled:        true,
		CreatedAt:      time.Now(),
	}
	if err := db.CreateMod(context.Background(), &existing); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/mods/recommended", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected recommended mods 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var items []struct {
		WorkshopID   string             `json:"workshopId"`
		GameKey      domain.GameKey     `json:"gameKey"`
		ProviderKey  domain.ProviderKey `json:"providerKey"`
		ModName      string             `json:"modName"`
		Dependencies []string           `json:"dependencies"`
		InLibrary    bool               `json:"inLibrary"`
		ModID        string             `json:"modId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.WorkshopID == "2563309347" {
			found = true
			if item.ModName != "MagicStorage" || !reflect.DeepEqual(item.Dependencies, []string{"SerousCommonLib"}) {
				t.Fatalf("expected Magic Storage dependency metadata, got %+v", item)
			}
			if item.GameKey != domain.GameTerraria || item.ProviderKey != domain.ProviderTerrariaTModLoader {
				t.Fatalf("expected recommended mod game metadata, got %+v", item)
			}
			if !item.InLibrary || item.ModID != "existing-workshop" {
				t.Fatalf("expected recommended workshop mod marked as in library, got %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("expected Magic Storage to appear in recommended mods")
	}
}

func TestLegacyWorkshopInstallRecordMigratesToWorkshopMods(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", "install.txt", bytes.NewBufferString("2563309347\n2824688072\n2563309347\n"))
	if err != nil {
		t.Fatal(err)
	}
	legacy := domain.ModFile{
		ID:         "legacy-install",
		InstanceID: "unassigned",
		FileName:   "install.txt",
		SizeBytes:  int64(len("2563309347\n2824688072\n2563309347\n")),
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/mods", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected global mod list 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var items []domain.ModFile
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two migrated workshop mods, got %+v", items)
	}
	for _, item := range items {
		if item.Source != "workshop" || item.WorkshopID == "" || item.FileName == "install.txt" {
			t.Fatalf("expected migrated workshop mod, got %+v", item)
		}
	}
	if _, err := db.GetMod(context.Background(), legacy.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected legacy install record deleted, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "mods", "unassigned", "install.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy install file removed, got err=%v", err)
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
	other := domain.ModFile{
		ID:         "mod-2",
		InstanceID: server.ID,
		FileName:   "other.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &other); err != nil {
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
	runtimeEnabled, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "enabled.json"))
	if err != nil {
		t.Fatal(err)
	}
	var enabledMods []string
	if err := json.Unmarshal(runtimeEnabled, &enabledMods); err != nil {
		t.Fatalf("expected runtime enabled.json to be JSON list, got %q: %v", string(runtimeEnabled), err)
	}
	if !reflect.DeepEqual(enabledMods, []string{"other"}) {
		t.Fatalf("expected runtime enabled.json to contain only the enabled tmod package names, got %v", enabledMods)
	}
}

func TestRunningTModLoaderServerAllowsModMutation(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	server.Status = domain.StatusRunning
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
		t.Fatalf("expected running mod update success, got %d: %s", recorder.Code, recorder.Body.String())
	}
	persisted, err := db.GetMod(context.Background(), mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Enabled {
		t.Fatalf("expected running mod update to change enabled state, got %+v", persisted)
	}

	uploadRecorder := httptest.NewRecorder()
	router.ServeHTTP(uploadRecorder, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/servers/tmod/mods/upload", "file", "new.tmod", []byte("new")))
	if uploadRecorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected running mod upload success, got %d: %s", uploadRecorder.Code, uploadRecorder.Body.String())
	}
	if _, err := db.GetModByInstanceAndFile(context.Background(), server.ID, "new.tmod"); err != nil {
		t.Fatalf("expected running upload to create mod record: %v", err)
	}

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/tmod/mods/mod-1", nil)
	router.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected running mod delete success, got %d: %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
	if _, err := db.GetMod(context.Background(), mod.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected running delete to remove mod record, got err=%v", err)
	}

	if _, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", "library.tmod", bytes.NewBufferString("library")); err != nil {
		t.Fatal(err)
	}
	libraryMod := domain.ModFile{
		ID:         "library-mod",
		InstanceID: "unassigned",
		FileName:   "library.tmod",
		SizeBytes:  7,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &libraryMod); err != nil {
		t.Fatal(err)
	}
	assignRecorder := httptest.NewRecorder()
	assignRequest := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/library-mod/assign", strings.NewReader(`{"instanceId":"tmod"}`))
	assignRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(assignRecorder, assignRequest)
	if assignRecorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected running mod assign success, got %d: %s", assignRecorder.Code, assignRecorder.Body.String())
	}
	if _, err := db.GetModByInstanceAndFile(context.Background(), server.ID, "library.tmod"); err != nil {
		t.Fatalf("expected running assign to create server mod record: %v", err)
	}
}

func TestAssignModRefreshesExistingServerModCache(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	if _, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", "large.tmod", bytes.NewBufferString("library-version")); err != nil {
		t.Fatal(err)
	}
	libraryMod := domain.ModFile{
		ID:         "library-large-mod",
		InstanceID: "unassigned",
		FileName:   "large.tmod",
		SizeBytes:  int64(len("library-version")),
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &libraryMod); err != nil {
		t.Fatal(err)
	}
	if _, _, err := modsvc.NewService(cfg.DataDir).Upload(server.ID, "large.tmod", bytes.NewBufferString("cached-version")); err != nil {
		t.Fatal(err)
	}
	runtimeModPath := filepath.Join(server.DataDir, "Mods", "large.tmod")
	if err := os.MkdirAll(filepath.Dir(runtimeModPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimeModPath, []byte("cached-runtime-version"), 0o600); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/library-large-mod/assign", strings.NewReader(`{"instanceId":"tmod"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected mod assign success, got %d: %s", recorder.Code, recorder.Body.String())
	}
	cached, err := os.ReadFile(filepath.Join(cfg.DataDir, "mods", server.ID, "large.tmod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cached) != "library-version" {
		t.Fatalf("expected server mod cache to be refreshed from library, got %q", string(cached))
	}
	runtimeCached, err := os.ReadFile(runtimeModPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeCached) != "library-version" {
		t.Fatalf("expected runtime mod mount cache to be refreshed from library, got %q", string(runtimeCached))
	}
}

func TestTModLoaderModDeleteRejectsDifferentServerMod(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("source-tmod", cfg.DataDir)
	source.ProviderKey = domain.ProviderTerrariaTModLoader
	target := testServer("target-tmod", cfg.DataDir)
	target.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &source); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	if _, _, err := modsvc.NewService(cfg.DataDir).Upload(target.ID, "example.tmod", bytes.NewBufferString("mod")); err != nil {
		t.Fatal(err)
	}
	mod := domain.ModFile{
		ID:         "target-mod",
		InstanceID: target.ID,
		FileName:   "example.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/source-tmod/mods/target-mod", nil))
	if remove.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected cross-server mod delete 404, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := db.GetMod(context.Background(), mod.ID); err != nil {
		t.Fatalf("expected target mod record to remain, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "mods", target.ID, "example.tmod")); err != nil {
		t.Fatalf("expected target mod file to remain, stat err=%v", err)
	}
}

func TestTModLoaderModDeleteKeepsRecordWhenFileRemovalFails(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	blockedPath := filepath.Join(cfg.DataDir, "mods", server.ID, "blocked.tmod")
	if err := os.MkdirAll(filepath.Join(blockedPath, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	mod := domain.ModFile{
		ID:         "blocked-mod",
		InstanceID: server.ID,
		FileName:   "blocked.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/tmod/mods/blocked-mod", nil))
	if remove.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("expected mod delete to fail when file removal fails, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := db.GetMod(context.Background(), mod.ID); err != nil {
		t.Fatalf("expected mod record to remain after failed file removal, got %v", err)
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

func TestGlobalModUploadHydratesKnownDependencies(t *testing.T) {
	router, _, _ := newTestRouter(t)

	upload := httptest.NewRecorder()
	router.ServeHTTP(upload, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/mods/upload", "file", "ImproveGame.tmod", tmodFixture("ImproveGame", "1.8.2", "2026.3.3.0")))
	if upload.Code != stdhttp.StatusCreated {
		t.Fatalf("expected global upload 201, got %d: %s", upload.Code, upload.Body.String())
	}
	var uploaded domain.ModFile
	if err := json.Unmarshal(upload.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(uploaded.Dependencies, []string{"SilkyUIFramework"}) {
		t.Fatalf("expected uploaded ImproveGame dependency metadata, got %+v", uploaded)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/mods", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected global mod list 200, got %d: %s", list.Code, list.Body.String())
	}
	var mods []domain.ModFile
	if err := json.Unmarshal(list.Body.Bytes(), &mods); err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 || !reflect.DeepEqual(mods[0].Dependencies, []string{"SilkyUIFramework"}) {
		t.Fatalf("expected listed ImproveGame dependency metadata, got %+v", mods)
	}
	if mods[0].GameKey != domain.GameTerraria || mods[0].ProviderKey != domain.ProviderTerrariaTModLoader {
		t.Fatalf("expected listed mod game metadata, got %+v", mods[0])
	}
}

func TestModPackCreateListAndDelete(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	for _, name := range []string{"boss.tmod", "quality.tmod"} {
		if _, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", name, bytes.NewBufferString(name)); err != nil {
			t.Fatal(err)
		}
		item := domain.ModFile{
			ID:         strings.TrimSuffix(name, ".tmod"),
			InstanceID: "unassigned",
			FileName:   name,
			SizeBytes:  int64(len(name)),
			Enabled:    true,
			CreatedAt:  time.Now(),
		}
		if err := db.CreateMod(context.Background(), &item); err != nil {
			t.Fatal(err)
		}
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/mod-packs", bytes.NewBufferString(`{"name":"Boss Night","description":"Boss run mods","modIds":["boss","quality","boss"]}`)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected mod pack create 201, got %d: %s", create.Code, create.Body.String())
	}
	var created struct {
		ID          string             `json:"id"`
		Name        string             `json:"name"`
		Description string             `json:"description"`
		GameKey     domain.GameKey     `json:"gameKey"`
		ProviderKey domain.ProviderKey `json:"providerKey"`
		ModIDs      []string           `json:"modIds"`
		Mods        []domain.ModFile   `json:"mods"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "Boss Night" || created.Description != "Boss run mods" {
		t.Fatalf("unexpected created mod pack: %+v", created)
	}
	if !reflect.DeepEqual(created.ModIDs, []string{"boss", "quality"}) {
		t.Fatalf("expected unique ordered mod IDs, got %+v", created.ModIDs)
	}
	if len(created.Mods) != 2 {
		t.Fatalf("expected resolved mods in response, got %+v", created.Mods)
	}
	if created.GameKey != domain.GameTerraria || created.ProviderKey != domain.ProviderTerrariaTModLoader {
		t.Fatalf("expected created mod pack game metadata, got %+v", created)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/mod-packs", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod pack list 200, got %d: %s", list.Code, list.Body.String())
	}
	var packs []struct {
		ID          string             `json:"id"`
		GameKey     domain.GameKey     `json:"gameKey"`
		ProviderKey domain.ProviderKey `json:"providerKey"`
		ModIDs      []string           `json:"modIds"`
		Mods        []domain.ModFile   `json:"mods"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &packs); err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || packs[0].ID != created.ID || len(packs[0].Mods) != 2 {
		t.Fatalf("expected listed mod pack with resolved mods, got %+v", packs)
	}
	if packs[0].GameKey != domain.GameTerraria || packs[0].ProviderKey != domain.ProviderTerrariaTModLoader {
		t.Fatalf("expected listed mod pack game metadata, got %+v", packs[0])
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/mod-packs/"+created.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod pack delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := db.GetModPack(context.Background(), created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected mod pack deleted, got err=%v", err)
	}
}

func TestModPackCreateDoesNotAutoIncludeKnownDependencies(t *testing.T) {
	router, db, _ := newTestRouter(t)
	magic := domain.ModFile{
		ID:         "magic",
		InstanceID: "unassigned",
		FileName:   "workshop-2563309347",
		Source:     "workshop",
		WorkshopID: "2563309347",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &magic); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/mod-packs", bytes.NewBufferString(`{"name":"Storage","modIds":["magic"]}`)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected mod pack create 201, got %d: %s", create.Code, create.Body.String())
	}
	var created struct {
		ModIDs []string         `json:"modIds"`
		Mods   []domain.ModFile `json:"mods"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.ModIDs) != 1 || len(created.Mods) != 1 {
		t.Fatalf("expected mod pack to include only selected mods, got %+v", created)
	}
	if !reflect.DeepEqual(created.Mods[0].Dependencies, []string{"SerousCommonLib"}) {
		t.Fatalf("expected selected mod to expose dependency metadata, got %+v", created.Mods)
	}
}

func TestModPackCreateRejectsServerScopedMods(t *testing.T) {
	router, db, _ := newTestRouter(t)
	serverMod := domain.ModFile{
		ID:         "server-mod",
		InstanceID: "server-1",
		FileName:   "server.tmod",
		SizeBytes:  10,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &serverMod); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/mod-packs", bytes.NewBufferString(`{"name":"Bad Pack","modIds":["server-mod"]}`)))
	if create.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected server-scoped mod pack create to fail, got %d: %s", create.Code, create.Body.String())
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
	runtimeMod, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "example.tmod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeMod) != "mod-v1" {
		t.Fatalf("expected assigned mod copied into runtime data dir, got %q", string(runtimeMod))
	}
}

func TestAssignModCopiesKnownDependencies(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id       string
		fileName string
		modName  string
	}{
		{id: "magic", fileName: "MagicStorage.tmod", modName: "MagicStorage"},
		{id: "serous", fileName: "SerousCommonLib.tmod", modName: "SerousCommonLib"},
	} {
		if _, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", item.fileName, bytes.NewReader(tmodFixture(item.modName, "1.0.0", "2026.04.3.0"))); err != nil {
			t.Fatal(err)
		}
		globalMod := domain.ModFile{
			ID:         item.id,
			InstanceID: "unassigned",
			FileName:   item.fileName,
			ModName:    item.modName,
			Title:      item.modName,
			SizeBytes:  64,
			Enabled:    true,
			CreatedAt:  time.Now(),
		}
		if err := db.CreateMod(context.Background(), &globalMod); err != nil {
			t.Fatal(err)
		}
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/mods/magic/assign", bytes.NewBufferString(`{"instanceId":"tmod"}`)))
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected assign 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	mods, err := db.ListMods(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 2 {
		t.Fatalf("expected assigned mod and dependency, got %+v", mods)
	}
	for _, fileName := range []string{"MagicStorage.tmod", "SerousCommonLib.tmod"} {
		if _, err := os.Stat(filepath.Join(server.DataDir, "Mods", fileName)); err != nil {
			t.Fatalf("expected runtime mod %s to be copied: %v", fileName, err)
		}
	}
	enabled, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "enabled.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"MagicStorage", "SerousCommonLib"} {
		if !strings.Contains(string(enabled), name) {
			t.Fatalf("expected enabled.json to contain %s, got %s", name, enabled)
		}
	}
}

func TestAssignWorkshopModRejectsArmRuntime(t *testing.T) {
	router, db, cfg := newTestRouterWithAdapter(t, armMockAdapter{availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}})
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	item := domain.ModFile{
		ID:         "magic",
		InstanceID: "unassigned",
		FileName:   "workshop-2563309347",
		Source:     "workshop",
		WorkshopID: "2563309347",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &item); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/mods/magic/assign", bytes.NewBufferString(`{"instanceId":"tmod"}`)))
	if recorder.Code != stdhttp.StatusConflict {
		t.Fatalf("expected workshop assign conflict on arm runtime, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAssignGlobalWorkshopModWritesServerInstallFile(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	globalMod := domain.ModFile{
		ID:         "global-workshop-mod",
		InstanceID: "unassigned",
		FileName:   "workshop-2619954303",
		Source:     "workshop",
		WorkshopID: "2619954303",
		SizeBytes:  11,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &globalMod); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/global-workshop-mod/assign", bytes.NewBufferString(`{"instanceId":"tmod"}`))
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected assign 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var assigned domain.ModFile
	if err := json.Unmarshal(recorder.Body.Bytes(), &assigned); err != nil {
		t.Fatal(err)
	}
	if assigned.Source != "workshop" || assigned.WorkshopID != "2619954303" || assigned.FileName == "install.txt" {
		t.Fatalf("expected assigned workshop record, got %+v", assigned)
	}
	runtimeInstall, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "install.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeInstall) != "2619954303\n" {
		t.Fatalf("expected runtime install.txt to contain workshop id, got %q", string(runtimeInstall))
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

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/worlds/"+imported.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected world delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "worlds", "unassigned", "uploaded.wld")); !os.IsNotExist(err) {
		t.Fatalf("expected imported world file deleted, stat err=%v", err)
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

func TestDeleteWorldKeepsRecordWhenFileRemovalFails(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	worldDir := filepath.Join(cfg.DataDir, "worlds", "unassigned", "blocked.wld")
	if err := os.MkdirAll(filepath.Join(worldDir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	item := domain.World{
		ID:         "blocked-world",
		InstanceID: "unassigned",
		Name:       "Blocked",
		FileName:   "blocked.wld",
		SizeBytes:  1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &item); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/worlds/blocked-world", nil))
	if remove.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("expected world delete to fail when file removal fails, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := db.GetWorld(context.Background(), "blocked-world"); err != nil {
		t.Fatalf("expected world record to remain after failed file removal, got %v", err)
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
	if _, err := db.GetWorld(context.Background(), world.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing world record deleted after download miss, got err=%v", err)
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
}

func TestResourceListsIncludeGameMetadata(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("pal-resource", cfg.DataDir)
	server.GameKey = domain.GamePalworld
	server.ProviderKey = domain.ProviderPalworld
	server.Port = palworld.DefaultInternalPort
	server.Config = palworld.NewProvider().DefaultConfig()
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte("palworld config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(server.ID, "pal-save.wld", strings.NewReader("world-data")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:          "pal-world",
		InstanceID:  server.ID,
		ProviderKey: server.ProviderKey,
		Name:        "Pal Save",
		FileName:    "pal-save.wld",
		SizeBytes:   int64(len("world-data")),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	listWorlds := httptest.NewRecorder()
	router.ServeHTTP(listWorlds, httptest.NewRequest(stdhttp.MethodGet, "/api/worlds", nil))
	if listWorlds.Code != stdhttp.StatusOK {
		t.Fatalf("expected world list 200, got %d: %s", listWorlds.Code, listWorlds.Body.String())
	}
	var worlds []domain.World
	if err := json.Unmarshal(listWorlds.Body.Bytes(), &worlds); err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 1 || worlds[0].GameKey != domain.GamePalworld || worlds[0].ProviderKey != domain.ProviderPalworld {
		t.Fatalf("expected Palworld metadata on world list, got %+v", worlds)
	}

	createBackup := httptest.NewRecorder()
	router.ServeHTTP(createBackup, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/backups", nil))
	if createBackup.Code != stdhttp.StatusCreated {
		t.Fatalf("expected backup create 201, got %d: %s", createBackup.Code, createBackup.Body.String())
	}
	var createdBackup domain.Backup
	if err := json.Unmarshal(createBackup.Body.Bytes(), &createdBackup); err != nil {
		t.Fatal(err)
	}
	if createdBackup.GameKey != domain.GamePalworld || createdBackup.ProviderKey != domain.ProviderPalworld {
		t.Fatalf("expected Palworld metadata on backup create, got %+v", createdBackup)
	}

	listBackups := httptest.NewRecorder()
	router.ServeHTTP(listBackups, httptest.NewRequest(stdhttp.MethodGet, "/api/backups", nil))
	if listBackups.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup list 200, got %d: %s", listBackups.Code, listBackups.Body.String())
	}
	var backups []domain.Backup
	if err := json.Unmarshal(listBackups.Body.Bytes(), &backups); err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || backups[0].GameKey != domain.GamePalworld || backups[0].ProviderKey != domain.ProviderPalworld {
		t.Fatalf("expected Palworld metadata on backup list, got %+v", backups)
	}
}

func TestAssignWorldUpdatesServerConfigAndClearsContainer(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("world-target", cfg.DataDir)
	server.ContainerID = "old-container"
	expectedWorldName := server.Config.WorldName
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
	if updated.WorldName != expectedWorldName || updated.Config.WorldName != expectedWorldName || updated.ContainerID != "" {
		t.Fatalf("expected server world/config name to stay unchanged and cleared container, got %+v", updated)
	}
	if updated.SourceWorldID != world.ID || updated.SourceWorldName != world.Name {
		t.Fatalf("expected server source world to be recorded, got %+v", updated)
	}
	configBytes, err := os.ReadFile(filepath.Join(server.DataDir, "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	expectedConfigLine := "world=/home/container/Worlds/" + expectedWorldName + ".wld"
	if !bytes.Contains(configBytes, []byte(expectedConfigLine)) {
		t.Fatalf("expected serverconfig to point at assigned world, got %q", string(configBytes))
	}
	runtimeWorld, err := os.ReadFile(filepath.Join(server.DataDir, "Worlds", expectedWorldName+".wld"))
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeWorld) != "world" {
		t.Fatalf("expected assigned world copied into runtime data dir, got %q", string(runtimeWorld))
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

func TestAssignWorldAllowsReusableSnapshotForSameProvider(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("snapshot-source", cfg.DataDir)
	target := testServer("snapshot-target", cfg.DataDir)
	expectedWorldName := target.Config.WorldName
	if err := db.CreateServer(context.Background(), &source); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(source.ID, "shared.wld", bytes.NewBufferString("shared-world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:          "shared-world",
		InstanceID:  source.ID,
		ProviderKey: source.ProviderKey,
		Name:        "shared",
		FileName:    "shared.wld",
		SizeBytes:   12,
		Source:      "server_snapshot",
		Config:      source.Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	assign := httptest.NewRecorder()
	router.ServeHTTP(assign, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/shared-world/assign", bytes.NewBufferString(`{"instanceId":"snapshot-target"}`)))
	if assign.Code != stdhttp.StatusOK {
		t.Fatalf("expected same-provider snapshot assign 200, got %d: %s", assign.Code, assign.Body.String())
	}
	updated, err := db.GetServer(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.WorldName != expectedWorldName || updated.Config.WorldName != expectedWorldName {
		t.Fatalf("expected target server world name to stay unchanged, got %+v", updated)
	}
	if updated.SourceWorldID != world.ID || updated.SourceWorldName != world.Name {
		t.Fatalf("expected target server source world to be recorded, got %+v", updated)
	}
	got, err := os.ReadFile(filepath.Join(target.DataDir, "worlds", expectedWorldName+".wld"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "shared-world" {
		t.Fatalf("expected snapshot materialized into target runtime, got %q", string(got))
	}
}

func TestAssignWorldRejectsSnapshotForDifferentProvider(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("snapshot-source", cfg.DataDir)
	target := testServer("snapshot-tmod", cfg.DataDir)
	target.ProviderKey = domain.ProviderTerrariaTModLoader
	if err := db.CreateServer(context.Background(), &source); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(source.ID, "vanilla.wld", bytes.NewBufferString("vanilla-world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:          "vanilla-world",
		InstanceID:  source.ID,
		ProviderKey: source.ProviderKey,
		Name:        "vanilla",
		FileName:    "vanilla.wld",
		SizeBytes:   13,
		Source:      "server_snapshot",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	assign := httptest.NewRecorder()
	router.ServeHTTP(assign, httptest.NewRequest(stdhttp.MethodPost, "/api/worlds/vanilla-world/assign", bytes.NewBufferString(`{"instanceId":"snapshot-tmod"}`)))
	if assign.Code != stdhttp.StatusConflict {
		t.Fatalf("expected different-provider snapshot assign 409, got %d: %s", assign.Code, assign.Body.String())
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

func TestDeleteWorldRejectsTemplateUsedByServer(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("template-source", cfg.DataDir)
	target := testServer("template-target", cfg.DataDir)
	target.SourceWorldID = "template-world"
	target.SourceWorldName = "Template World"
	if err := db.CreateServer(context.Background(), &source); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(source.ID, "template.wld", bytes.NewBufferString("template-world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:          "template-world",
		InstanceID:  source.ID,
		ProviderKey: source.ProviderKey,
		Name:        "Template World",
		FileName:    "template.wld",
		SizeBytes:   14,
		Source:      "server_snapshot",
		Config:      source.Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/worlds/template-world", nil))
	if remove.Code != stdhttp.StatusConflict {
		t.Fatalf("expected template delete 409 while in use, got %d: %s", remove.Code, remove.Body.String())
	}
}

func TestCreateWorldSnapshotFromServerRuntimeWorld(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("snapshot-source", cfg.DataDir)
	server.Name = "Snapshot Source"
	server.WorldName = server.Config.WorldName
	if err := os.MkdirAll(filepath.Join(server.DataDir, "Worlds"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "Worlds", server.Config.WorldName+".wld"), []byte("world-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/snapshot-source/world-snapshots", bytes.NewBufferString(`{"name":"Reusable Snapshot"}`)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected world snapshot 201, got %d: %s", create.Code, create.Body.String())
	}
	var world domain.World
	if err := json.Unmarshal(create.Body.Bytes(), &world); err != nil {
		t.Fatal(err)
	}
	if world.Name != "Reusable Snapshot" || world.Source != "server_snapshot" || world.ActiveInstanceID != "" || world.ProviderKey != server.ProviderKey {
		t.Fatalf("expected server snapshot world record, got %+v", world)
	}
	if world.Config.WorldName != server.Config.WorldName || world.Config.MaxPlayers != server.Config.MaxPlayers {
		t.Fatalf("expected config snapshot, got %+v", world.Config)
	}
	path, err := worldsvc.NewService(cfg.DataDir).Path(server.ID, world.FileName)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world-data" {
		t.Fatalf("expected copied runtime world data, got %q", string(got))
	}
}

func TestCreateWorldSnapshotFindsVanillaRootWorldFile(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("snapshot-root-world", cfg.DataDir)
	server.WorldName = server.Config.WorldName
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, server.Config.WorldName+".wld"), []byte("root-world-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/snapshot-root-world/world-snapshots", nil))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected world snapshot 201, got %d: %s", create.Code, create.Body.String())
	}
	var world domain.World
	if err := json.Unmarshal(create.Body.Bytes(), &world); err != nil {
		t.Fatal(err)
	}
	path, err := worldsvc.NewService(cfg.DataDir).Path(server.ID, world.FileName)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "root-world-data" {
		t.Fatalf("expected copied root world data, got %q", string(got))
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
		"hostPort":17777,
		"config":{
			"serverName":"Edited Server",
			"worldName":"EditedWorld",
			"worldSize":"large",
			"difficulty":"expert",
			"maxPlayers":12,
			"port":18888,
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
	if updated.Name != "Edited Server" || updated.WorldName != "EditedWorld" || updated.Port != 7777 || updated.HostPort != 17777 || updated.MaxPlayers != 12 || updated.Password != "secret" {
		t.Fatalf("expected server fields synchronized from config, got %+v", updated)
	}
	if updated.Config.Difficulty != domain.Difficulty("expert") || updated.Config.WorldSize != domain.WorldSize("large") || updated.Config.Language != terraria.DefaultLanguage {
		t.Fatalf("expected persisted config update, got %+v", updated.Config)
	}
	if updated.ContainerID != "" {
		t.Fatalf("expected stale container id cleared after config update, got %q", updated.ContainerID)
	}
	if updated.ConfigRevision == 0 || updated.AppliedConfigRevision != updated.ConfigRevision {
		t.Fatalf("expected stopped config update to be considered applied, got config=%d applied=%d", updated.ConfigRevision, updated.AppliedConfigRevision)
	}
	configBytes, err := os.ReadFile(filepath.Join(server.DataDir, "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(configBytes, []byte("world=/home/container/Worlds/EditedWorld.wld")) || !bytes.Contains(configBytes, []byte("maxplayers=12")) || !bytes.Contains(configBytes, []byte("port=7777")) || !bytes.Contains(configBytes, []byte("language=en-US")) {
		t.Fatalf("expected rewritten serverconfig, got %q", string(configBytes))
	}
}

func TestUpdateRunningServerConfigPreservesLiveContainer(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("running-config-target", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "live-container"
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	payload := `{
		"hostPort":17778,
		"config":{
			"serverName":"Edited While Running",
			"worldName":"EditedWorld",
			"worldSize":"large",
			"difficulty":"expert",
			"maxPlayers":10,
			"port":18888,
			"password":"secret",
			"motd":"Restart to apply",
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/running-config-target/config", bytes.NewBufferString(payload)))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected running config update 200, got %d: %s", update.Code, update.Body.String())
	}
	updated, err := db.GetServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.StatusRunning || updated.ContainerID != "live-container" {
		t.Fatalf("expected running container to stay attached, got %+v", updated)
	}
	if updated.ConfigRevision == 0 || updated.AppliedConfigRevision >= updated.ConfigRevision {
		t.Fatalf("expected running config update to require restart, got config=%d applied=%d", updated.ConfigRevision, updated.AppliedConfigRevision)
	}
	if updated.Name != "Edited While Running" || updated.Config.MOTD != "Restart to apply" {
		t.Fatalf("expected persisted config metadata, got %+v", updated)
	}
	if updated.Port != 7777 || updated.HostPort != 17778 {
		t.Fatalf("expected fixed internal port and updated external port, got internal=%d external=%d", updated.Port, updated.HostPort)
	}
	configBytes, err := os.ReadFile(filepath.Join(server.DataDir, "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(configBytes, []byte("motd=Restart to apply")) {
		t.Fatalf("expected rewritten serverconfig, got %q", string(configBytes))
	}

	restart := httptest.NewRecorder()
	router.ServeHTTP(restart, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/running-config-target/restart", nil))
	if restart.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected restart 202, got %d: %s", restart.Code, restart.Body.String())
	}
	restarted := waitForServerStatus(t, db, server.ID, domain.StatusRunning)
	if restarted.AppliedConfigRevision != restarted.ConfigRevision {
		t.Fatalf("expected restart to apply config revision, got config=%d applied=%d", restarted.ConfigRevision, restarted.AppliedConfigRevision)
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

func TestServerSavesEndpointsAreGameAware(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("minecraft-saves", cfg.DataDir)
	server.GameKey = domain.GameMinecraft
	server.ProviderKey = domain.ProviderMinecraft
	server.Port = 25565
	server.DataDir = filepath.Join(cfg.DataDir, "instances", "minecraft-saves")
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "level.dat"), []byte("world-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	snapshot := httptest.NewRecorder()
	router.ServeHTTP(snapshot, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/minecraft-saves/saves/snapshot", nil))
	if snapshot.Code != stdhttp.StatusCreated {
		t.Fatalf("expected save snapshot 201, got %d: %s", snapshot.Code, snapshot.Body.String())
	}
	var snapshotResp struct {
		SaveDisplayName string        `json:"saveDisplayName"`
		Save            domain.Backup `json:"save"`
	}
	if err := json.Unmarshal(snapshot.Body.Bytes(), &snapshotResp); err != nil {
		t.Fatal(err)
	}
	if snapshotResp.SaveDisplayName != "world" {
		t.Fatalf("expected Minecraft save display name 'world', got %q", snapshotResp.SaveDisplayName)
	}
	if snapshotResp.Save.InstanceID != "minecraft-saves" {
		t.Fatalf("expected snapshot tied to server, got %+v", snapshotResp.Save)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/minecraft-saves/saves", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected saves list 200, got %d: %s", list.Code, list.Body.String())
	}
	var listResp struct {
		SaveDisplayName string          `json:"saveDisplayName"`
		Saves           []domain.Backup `json:"saves"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if listResp.SaveDisplayName != "world" || len(listResp.Saves) != 1 {
		t.Fatalf("expected one save with game-aware name, got %+v", listResp)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/minecraft-saves/saves/"+snapshotResp.Save.ID+"/download", nil))
	if download.Code != stdhttp.StatusOK {
		t.Fatalf("expected save download 200, got %d: %s", download.Code, download.Body.String())
	}
	if download.Body.Len() == 0 {
		t.Fatal("expected non-empty save archive")
	}

	restore := httptest.NewRecorder()
	router.ServeHTTP(restore, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/minecraft-saves/saves/"+snapshotResp.Save.ID+"/restore", nil))
	if restore.Code != stdhttp.StatusOK {
		t.Fatalf("expected save restore 200, got %d: %s", restore.Code, restore.Body.String())
	}

	crossInstance := httptest.NewRecorder()
	router.ServeHTTP(crossInstance, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/minecraft-saves/saves/unknown-id/download", nil))
	if crossInstance.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected missing save 404, got %d", crossInstance.Code)
	}
}

func TestFriendInviteAndPublicHostFlow(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("invite-server", cfg.DataDir)
	server.GameKey = domain.GameTerraria
	server.ProviderKey = domain.ProviderTerrariaVanilla
	server.Port = 7777
	server.HostPort = 17777
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	joinInfo := httptest.NewRecorder()
	router.ServeHTTP(joinInfo, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/invite-server/join-info", nil))
	if joinInfo.Code != stdhttp.StatusOK {
		t.Fatalf("expected join-info 200, got %d: %s", joinInfo.Code, joinInfo.Body.String())
	}
	var info domain.ServerJoinInfo
	if err := json.Unmarshal(joinInfo.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Port != 17777 || !strings.Contains(info.InviteText, "127.0.0.1:17777") {
		t.Fatalf("expected default join info, got %+v", info)
	}

	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/settings/public-host", bytes.NewBufferString(`{"publicHost":"play.example.com"}`)))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected public host update 200, got %d: %s", update.Code, update.Body.String())
	}

	joinInfoAfter := httptest.NewRecorder()
	router.ServeHTTP(joinInfoAfter, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/invite-server/join-info", nil))
	var infoAfter domain.ServerJoinInfo
	if err := json.Unmarshal(joinInfoAfter.Body.Bytes(), &infoAfter); err != nil {
		t.Fatal(err)
	}
	if infoAfter.Address != "play.example.com" || !strings.Contains(infoAfter.InviteText, "play.example.com:17777") {
		t.Fatalf("expected public host in join info, got %+v", infoAfter)
	}

	settings := httptest.NewRecorder()
	router.ServeHTTP(settings, httptest.NewRequest(stdhttp.MethodGet, "/api/settings", nil))
	var settingsResp map[string]string
	if err := json.Unmarshal(settings.Body.Bytes(), &settingsResp); err != nil {
		t.Fatal(err)
	}
	if settingsResp["publicHost"] != "play.example.com" {
		t.Fatalf("expected settings to expose public host, got %+v", settingsResp)
	}
}

func TestShareableServerPageFlow(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("share-server", cfg.DataDir)
	server.GameKey = domain.GameTerraria
	server.ProviderKey = domain.ProviderTerrariaVanilla
	server.Password = "secret"
	server.Config.Password = "secret"
	server.HostPort = 17778
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	enable := httptest.NewRecorder()
	router.ServeHTTP(enable, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/share-server/share", bytes.NewBufferString(`{"includePassword":false}`)))
	if enable.Code != stdhttp.StatusOK {
		t.Fatalf("expected share enable 200, got %d: %s", enable.Code, enable.Body.String())
	}
	var share serverShareResponse
	if err := json.Unmarshal(enable.Body.Bytes(), &share); err != nil {
		t.Fatal(err)
	}
	if !share.Enabled || share.Token == "" || share.SharePath != "/share/"+share.Token {
		t.Fatalf("expected enabled share response, got %+v", share)
	}
	getShare := httptest.NewRecorder()
	router.ServeHTTP(getShare, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/share-server/share", nil))
	if getShare.Code != stdhttp.StatusOK {
		t.Fatalf("expected share status 200, got %d: %s", getShare.Code, getShare.Body.String())
	}
	var shareStatus serverShareResponse
	if err := json.Unmarshal(getShare.Body.Bytes(), &shareStatus); err != nil {
		t.Fatal(err)
	}
	if shareStatus.Token != share.Token {
		t.Fatalf("expected share status to return token %s, got %+v", share.Token, shareStatus)
	}

	public := httptest.NewRecorder()
	router.ServeHTTP(public, httptest.NewRequest(stdhttp.MethodGet, "/api/public/servers/"+share.Token, nil))
	if public.Code != stdhttp.StatusOK {
		t.Fatalf("expected public share 200, got %d: %s", public.Code, public.Body.String())
	}
	var publicResp publicServerShareResponse
	if err := json.Unmarshal(public.Body.Bytes(), &publicResp); err != nil {
		t.Fatal(err)
	}
	if publicResp.Name != server.Name || publicResp.JoinInfo.Port != 17778 {
		t.Fatalf("expected public join info, got %+v", publicResp)
	}
	if publicResp.JoinInfo.Password != "" || strings.Contains(publicResp.JoinInfo.InviteText, "secret") {
		t.Fatalf("expected public share to hide password, got %+v", publicResp.JoinInfo)
	}

	enableWithPassword := httptest.NewRecorder()
	router.ServeHTTP(enableWithPassword, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/share-server/share", bytes.NewBufferString(`{"includePassword":true}`)))
	if enableWithPassword.Code != stdhttp.StatusOK {
		t.Fatalf("expected share update 200, got %d: %s", enableWithPassword.Code, enableWithPassword.Body.String())
	}
	var updatedShare serverShareResponse
	if err := json.Unmarshal(enableWithPassword.Body.Bytes(), &updatedShare); err != nil {
		t.Fatal(err)
	}
	if updatedShare.Token != share.Token || !updatedShare.IncludePassword {
		t.Fatalf("expected same share token with password enabled, got %+v", updatedShare)
	}

	publicWithPassword := httptest.NewRecorder()
	router.ServeHTTP(publicWithPassword, httptest.NewRequest(stdhttp.MethodGet, "/api/public/servers/"+share.Token, nil))
	var publicWithPasswordResp publicServerShareResponse
	if err := json.Unmarshal(publicWithPassword.Body.Bytes(), &publicWithPasswordResp); err != nil {
		t.Fatal(err)
	}
	if publicWithPasswordResp.JoinInfo.Password != "secret" {
		t.Fatalf("expected public share to include password after opt-in, got %+v", publicWithPasswordResp.JoinInfo)
	}

	disable := httptest.NewRecorder()
	router.ServeHTTP(disable, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/share-server/share", nil))
	if disable.Code != stdhttp.StatusOK {
		t.Fatalf("expected share disable 200, got %d: %s", disable.Code, disable.Body.String())
	}
	getDisabledShare := httptest.NewRecorder()
	router.ServeHTTP(getDisabledShare, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/share-server/share", nil))
	var disabledStatus serverShareResponse
	if err := json.Unmarshal(getDisabledShare.Body.Bytes(), &disabledStatus); err != nil {
		t.Fatal(err)
	}
	if disabledStatus.Enabled {
		t.Fatalf("expected disabled share status, got %+v", disabledStatus)
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(stdhttp.MethodGet, "/api/public/servers/"+share.Token, nil))
	if missing.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected disabled share 404, got %d: %s", missing.Code, missing.Body.String())
	}
}

func TestGameCatalogServerCountsAndRecommendedVersion(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	for i := 0; i < 2; i++ {
		server := testServer("mc-"+fmt.Sprint(i), cfg.DataDir)
		server.GameKey = domain.GameMinecraft
		server.ProviderKey = domain.ProviderMinecraft
		if err := db.CreateServer(context.Background(), &server); err != nil {
			t.Fatal(err)
		}
	}
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/games", nil))
	var games []domain.GameCatalogEntry
	if err := json.Unmarshal(list.Body.Bytes(), &games); err != nil {
		t.Fatal(err)
	}
	var minecraftGame *domain.GameCatalogEntry
	var terrariaGame *domain.GameCatalogEntry
	for index := range games {
		if games[index].Key == domain.GameMinecraft {
			minecraftGame = &games[index]
		}
		if games[index].Key == domain.GameTerraria {
			terrariaGame = &games[index]
		}
	}
	if minecraftGame == nil {
		t.Fatal("expected minecraft game in catalog")
	}
	if terrariaGame == nil {
		t.Fatal("expected terraria game in catalog")
	}
	if minecraftGame.ServerCount != 2 {
		t.Fatalf("expected minecraft server count 2, got %d", minecraftGame.ServerCount)
	}
	if minecraftGame.CoverImage != "minecraft" {
		t.Fatalf("expected minecraft cover image, got %q", minecraftGame.CoverImage)
	}
	if minecraftGame.Providers[0].RecommendedVersion != "1.21.4" {
		t.Fatalf("expected minecraft to recommend stable version after latest, got %+v", minecraftGame.Providers[0])
	}
	var vanillaProvider *domain.ProviderCatalog
	for index := range terrariaGame.Providers {
		if terrariaGame.Providers[index].Key == domain.ProviderTerrariaVanilla {
			vanillaProvider = &terrariaGame.Providers[index]
		}
	}
	if vanillaProvider == nil {
		t.Fatal("expected Terraria vanilla provider in catalog")
	}
	if vanillaProvider.RecommendedVersion != "1.4.5.6" {
		t.Fatalf("expected Terraria vanilla to recommend latest explicit version, got %+v", vanillaProvider)
	}
}

func TestPlayerManagementGatedByProviderCapability(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	palworldServer := testServer("palworld-players", cfg.DataDir)
	palworldServer.GameKey = domain.GamePalworld
	palworldServer.ProviderKey = domain.ProviderPalworld
	if err := db.CreateServer(context.Background(), &palworldServer); err != nil {
		t.Fatal(err)
	}

	palworldPlayers := httptest.NewRecorder()
	router.ServeHTTP(palworldPlayers, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/palworld-players/players", nil))
	if palworldPlayers.Code != stdhttp.StatusOK {
		t.Fatalf("expected players 200, got %d: %s", palworldPlayers.Code, palworldPlayers.Body.String())
	}
	var palworldResp struct {
		Supported bool            `json:"supported"`
		Players   []domain.Player `json:"players"`
	}
	if err := json.Unmarshal(palworldPlayers.Body.Bytes(), &palworldResp); err != nil {
		t.Fatal(err)
	}
	if palworldResp.Supported {
		t.Fatal("expected Palworld player list to be unsupported")
	}

	palworldKick := httptest.NewRecorder()
	router.ServeHTTP(palworldKick, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/palworld-players/players/Alice/kick", nil))
	if palworldKick.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected Palworld kick to be rejected 400, got %d: %s", palworldKick.Code, palworldKick.Body.String())
	}

	palworldBan := httptest.NewRecorder()
	router.ServeHTTP(palworldBan, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/palworld-players/players/Alice/ban", nil))
	if palworldBan.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected Palworld ban to be rejected 400, got %d: %s", palworldBan.Code, palworldBan.Body.String())
	}

	terraria := testServer("terraria-players", cfg.DataDir)
	terraria.GameKey = domain.GameTerraria
	terraria.ProviderKey = domain.ProviderTerrariaVanilla
	if err := db.CreateServer(context.Background(), &terraria); err != nil {
		t.Fatal(err)
	}
	terrariaPlayers := httptest.NewRecorder()
	router.ServeHTTP(terrariaPlayers, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/terraria-players/players", nil))
	if terrariaPlayers.Code != stdhttp.StatusOK {
		t.Fatalf("expected terraria players 200, got %d: %s", terrariaPlayers.Code, terrariaPlayers.Body.String())
	}
	var terrariaResp struct {
		Supported bool            `json:"supported"`
		Players   []domain.Player `json:"players"`
	}
	if err := json.Unmarshal(terrariaPlayers.Body.Bytes(), &terrariaResp); err != nil {
		t.Fatal(err)
	}
	if !terrariaResp.Supported {
		t.Fatal("expected Terraria player list to be supported")
	}

	terrariaKick := httptest.NewRecorder()
	router.ServeHTTP(terrariaKick, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/terraria-players/players/Alice/kick", nil))
	if terrariaKick.Code != stdhttp.StatusConflict {
		t.Fatalf("expected terraria kick conflict for stopped server, got %d: %s", terrariaKick.Code, terrariaKick.Body.String())
	}
}

func TestMinecraftWhitelistManagement(t *testing.T) {
	adapter := newCommandCaptureAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)

	minecraftServer := testServer("minecraft-whitelist", cfg.DataDir)
	minecraftServer.GameKey = domain.GameMinecraft
	minecraftServer.ProviderKey = domain.ProviderMinecraft
	minecraftServer.Status = domain.StatusRunning
	minecraftServer.ContainerID = "container-minecraft-whitelist"
	minecraftServer.Config = minecraft.NewProvider().DefaultConfig()
	minecraftServer.ConfigPayload = minecraft.PayloadFromConfig(minecraftServer.Config, nil)
	if err := db.CreateServer(context.Background(), &minecraftServer); err != nil {
		t.Fatal(err)
	}

	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/minecraft-whitelist/whitelist", nil))
	if status.Code != stdhttp.StatusOK {
		t.Fatalf("expected whitelist status 200, got %d: %s", status.Code, status.Body.String())
	}
	var statusResp struct {
		Supported bool `json:"supported"`
		Running   bool `json:"running"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &statusResp); err != nil {
		t.Fatal(err)
	}
	if !statusResp.Supported || !statusResp.Running {
		t.Fatalf("expected supported running whitelist response, got %+v", statusResp)
	}

	add := httptest.NewRecorder()
	router.ServeHTTP(add, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/minecraft-whitelist/whitelist/Steve", nil))
	if add.Code != stdhttp.StatusOK {
		t.Fatalf("expected whitelist add 200, got %d: %s", add.Code, add.Body.String())
	}
	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/minecraft-whitelist/whitelist/Alex", nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected whitelist remove 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if !reflect.DeepEqual(adapter.commands, []string{"whitelist add Steve", "whitelist remove Alex"}) {
		t.Fatalf("unexpected whitelist commands: %+v", adapter.commands)
	}

	palworldServer := testServer("palworld-whitelist", cfg.DataDir)
	palworldServer.GameKey = domain.GamePalworld
	palworldServer.ProviderKey = domain.ProviderPalworld
	if err := db.CreateServer(context.Background(), &palworldServer); err != nil {
		t.Fatal(err)
	}
	palworldStatus := httptest.NewRecorder()
	router.ServeHTTP(palworldStatus, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/palworld-whitelist/whitelist", nil))
	if palworldStatus.Code != stdhttp.StatusOK {
		t.Fatalf("expected Palworld whitelist status 200, got %d: %s", palworldStatus.Code, palworldStatus.Body.String())
	}
	var palworldResp struct {
		Supported bool `json:"supported"`
	}
	if err := json.Unmarshal(palworldStatus.Body.Bytes(), &palworldResp); err != nil {
		t.Fatal(err)
	}
	if palworldResp.Supported {
		t.Fatal("expected Palworld whitelist to be unsupported")
	}
	palworldAdd := httptest.NewRecorder()
	router.ServeHTTP(palworldAdd, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/palworld-whitelist/whitelist/Alice", nil))
	if palworldAdd.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected unsupported Palworld whitelist add 400, got %d: %s", palworldAdd.Code, palworldAdd.Body.String())
	}
}

func TestDownloadBackupPrunesMissingFileRecord(t *testing.T) {
	router, db, _ := newTestRouter(t)
	backup := domain.Backup{
		ID:         "missing-backup",
		InstanceID: "backup-source",
		FileName:   "missing.zip",
		WorldName:  "Missing",
		SizeBytes:  5,
		Type:       "Manual",
		CreatedAt:  time.Now(),
	}
	if err := db.CreateBackup(context.Background(), &backup); err != nil {
		t.Fatal(err)
	}

	download := httptest.NewRecorder()
	router.ServeHTTP(download, httptest.NewRequest(stdhttp.MethodGet, "/api/backups/missing-backup/download", nil))
	if download.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected missing backup download 404, got %d: %s", download.Code, download.Body.String())
	}
	if !strings.Contains(download.Body.String(), "backup file not found on disk") {
		t.Fatalf("expected JSON missing file error, got %q", download.Body.String())
	}
	if _, err := db.GetBackup(context.Background(), backup.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing backup record deleted after download miss, got err=%v", err)
	}
}

func TestRestoreBackupSynchronizesServerMetadataFromRestoredConfig(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("restore-sync", cfg.DataDir)
	server.Name = "Restore Sync"
	server.WorldName = "BackedUpWorld"
	server.Config.WorldName = "BackedUpWorld"
	server.Config.MaxPlayers = 14
	server.Config.Port = 17777
	server.MaxPlayers = 14
	server.Port = 17777
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configText, err := terraria.RenderServerConfig(server.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte(configText), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/restore-sync/backups", nil))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected backup create 201, got %d: %s", create.Code, create.Body.String())
	}
	var backup domain.Backup
	if err := json.Unmarshal(create.Body.Bytes(), &backup); err != nil {
		t.Fatal(err)
	}

	stale := server
	stale.WorldName = "StaleWorld"
	stale.Config.WorldName = "StaleWorld"
	stale.Config.MaxPlayers = 4
	stale.Config.Port = 18888
	stale.MaxPlayers = 4
	stale.Port = 18888
	if err := db.SaveServer(context.Background(), &stale); err != nil {
		t.Fatal(err)
	}
	staleConfig, err := terraria.RenderServerConfig(stale.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte(staleConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	restore := httptest.NewRecorder()
	router.ServeHTTP(restore, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/"+backup.ID+"/restore", nil))
	if restore.Code != stdhttp.StatusOK {
		t.Fatalf("expected backup restore 200, got %d: %s", restore.Code, restore.Body.String())
	}
	restored, err := db.GetServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.WorldName != "BackedUpWorld" || restored.Config.WorldName != "BackedUpWorld" || restored.MaxPlayers != 14 || restored.Port != 7777 {
		t.Fatalf("expected restored server metadata synchronized from backup config, got %+v", restored)
	}
}

func TestDeleteBackupKeepsRecordWhenFileRemovalFails(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	backupDir := filepath.Join(cfg.DataDir, "backups", "backup-source", "blocked.zip")
	if err := os.MkdirAll(filepath.Join(backupDir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	item := domain.Backup{
		ID:         "blocked-backup",
		InstanceID: "backup-source",
		FileName:   "blocked.zip",
		WorldName:  "Blocked",
		SizeBytes:  1,
		Type:       "Manual",
		CreatedAt:  time.Now(),
	}
	if err := db.CreateBackup(context.Background(), &item); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/backups/blocked-backup", nil))
	if remove.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("expected backup delete to fail when file removal fails, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := db.GetBackup(context.Background(), "blocked-backup"); err != nil {
		t.Fatalf("expected backup record to remain after failed file removal, got %v", err)
	}
}

func TestSettingsEndpointReadsConfiguredDockerHost(t *testing.T) {
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
	if update.Code != stdhttp.StatusMethodNotAllowed {
		t.Fatalf("expected settings update to be unavailable, got %d: %s", update.Code, update.Body.String())
	}
}

func TestBackupSourceMutationsPruneMissingFiles(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("source", cfg.DataDir)
	if err := db.CreateServer(context.Background(), &source); err != nil {
		t.Fatal(err)
	}
	backup := domain.Backup{ID: "missing-backup-source", InstanceID: "source", FileName: "missing.zip", WorldName: "Source World", SizeBytes: 5, Type: "Manual", CreatedAt: time.Now()}
	if err := db.CreateBackup(context.Background(), &backup); err != nil {
		t.Fatal(err)
	}

	restore := httptest.NewRecorder()
	router.ServeHTTP(restore, httptest.NewRequest(stdhttp.MethodPost, "/api/backups/missing-backup-source/restore", nil))
	if restore.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected missing backup restore 404, got %d: %s", restore.Code, restore.Body.String())
	}
	if _, err := db.GetBackup(context.Background(), backup.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing backup record pruned after restore miss, got err=%v", err)
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
