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
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/minecraft"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	serverctrl "github.com/smartcat999/game-panel-lite/apps/api/internal/server"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
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
	registry := provider.NewRegistry(terraria.NewVanillaProvider(), terraria.NewTModLoaderProvider(), palworld.NewProvider(), dst.NewProvider(), minecraft.NewProvider())
	handler := NewHandler(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		db,
		registry,
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
	controllerCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go serverctrl.NewController(
		db,
		serverctrl.NewRuntimeReconciler(
			serverctrl.NewProviderWorkloadBuilder(registry),
			serverctrl.NewRuntimeAdapterClient(runtimeAdapter),
		),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).WithInterval(10 * time.Millisecond).Start(controllerCtx)
	return router, db, cfg
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

func (a *commandCaptureAdapter) SendCommandWorkload(_ context.Context, _ string, command string) error {
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

func (a *captureCreateAdapter) CreateWorkload(ctx context.Context, spec domain.WorkloadSpec) (string, error) {
	a.created <- runtime.ContainerSpecFromWorkload(spec)
	return a.availableMockAdapter.CreateWorkload(ctx, spec)
}

func (a *captureCreateAdapter) RemoveWorkload(ctx context.Context, runtimeID string) error {
	a.removed <- runtimeID
	return a.availableMockAdapter.RemoveWorkload(ctx, runtimeID)
}

type inspectStatusAdapter struct {
	status domain.ServerStatus
	err    error
}

func (a inspectStatusAdapter) Check(context.Context) runtime.DockerStatus {
	return runtime.DockerStatus{Available: true, Message: "ok", Host: "mock"}
}
func (a inspectStatusAdapter) CreateWorkload(context.Context, domain.WorkloadSpec) (string, error) {
	return "created-container", nil
}
func (a inspectStatusAdapter) StartWorkload(context.Context, string) error  { return nil }
func (a inspectStatusAdapter) StopWorkload(context.Context, string) error   { return nil }
func (a inspectStatusAdapter) RemoveWorkload(context.Context, string) error { return nil }
func (a inspectStatusAdapter) InspectWorkload(context.Context, string) (domain.WorkloadStatus, error) {
	if a.err != nil {
		return domain.WorkloadStatus{}, a.err
	}
	return domain.WorkloadStatus{RuntimeID: "created-container", State: domain.ActualRunning}, nil
}
func (a inspectStatusAdapter) StatsWorkload(context.Context, string) (runtime.WorkloadStats, error) {
	return runtime.WorkloadStats{}, nil
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
func (a inspectStatusAdapter) LogsWorkload(context.Context, string, bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (a inspectStatusAdapter) LogSnapshotWorkload(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (a inspectStatusAdapter) SendCommandWorkload(context.Context, string, string) error {
	return nil
}

type unavailableInspectAdapter struct {
	inspectCalls int
}

func (a *unavailableInspectAdapter) Check(context.Context) runtime.DockerStatus {
	return runtime.DockerStatus{Available: false, Message: "docker unavailable", Host: "mock"}
}
func (a *unavailableInspectAdapter) CreateWorkload(context.Context, domain.WorkloadSpec) (string, error) {
	return "", fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) StartWorkload(context.Context, string) error {
	return fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) StopWorkload(context.Context, string) error {
	return fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) RemoveWorkload(context.Context, string) error {
	return fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) InspectWorkload(context.Context, string) (domain.WorkloadStatus, error) {
	a.inspectCalls++
	return domain.WorkloadStatus{}, fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) StatsWorkload(context.Context, string) (runtime.WorkloadStats, error) {
	return runtime.WorkloadStats{}, fmt.Errorf("docker unavailable")
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
func (a *unavailableInspectAdapter) LogsWorkload(context.Context, string, bool) (io.ReadCloser, error) {
	return nil, fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) LogSnapshotWorkload(context.Context, string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("docker unavailable")
}
func (a *unavailableInspectAdapter) SendCommandWorkload(context.Context, string, string) error {
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
func (a *staleContainerAdapter) CreateWorkload(context.Context, domain.WorkloadSpec) (string, error) {
	a.created++
	return "new-container", nil
}
func (a *staleContainerAdapter) StartWorkload(_ context.Context, runtimeID string) error {
	a.startedContainer = runtimeID
	return nil
}
func (a *staleContainerAdapter) StopWorkload(_ context.Context, runtimeID string) error {
	a.stoppedContainer = runtimeID
	if runtimeID == "old-container" {
		return fmt.Errorf("stale container used for stop")
	}
	return nil
}
func (a *staleContainerAdapter) RemoveWorkload(context.Context, string) error { return nil }
func (a *staleContainerAdapter) InspectWorkload(_ context.Context, runtimeID string) (domain.WorkloadStatus, error) {
	if runtimeID == "old-container" {
		return domain.WorkloadStatus{}, fmt.Errorf("stale container")
	}
	return domain.WorkloadStatus{RuntimeID: runtimeID, State: domain.ActualRunning}, nil
}
func (a *staleContainerAdapter) StatsWorkload(_ context.Context, runtimeID string) (runtime.WorkloadStats, error) {
	if runtimeID != "new-container" {
		return runtime.WorkloadStats{}, fmt.Errorf("stale container used for stats")
	}
	return runtime.WorkloadStats{}, nil
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
func (a *staleContainerAdapter) LogsWorkload(_ context.Context, runtimeID string, _ bool) (io.ReadCloser, error) {
	a.logsContainer = runtimeID
	if runtimeID != "new-container" {
		return nil, fmt.Errorf("stale container used for logs")
	}
	return io.NopCloser(strings.NewReader("[Info] recovered log\n")), nil
}
func (a *staleContainerAdapter) LogSnapshotWorkload(_ context.Context, runtimeID string) (io.ReadCloser, error) {
	return a.LogsWorkload(context.Background(), runtimeID, false)
}
func (a *staleContainerAdapter) SendCommandWorkload(_ context.Context, runtimeID string, _ string) error {
	a.commandContainer = runtimeID
	if runtimeID != "new-container" {
		return fmt.Errorf("stale container used for command")
	}
	return nil
}

type statusCapturingLogsAdapter struct {
	availableMockAdapter
	logRuntimeID string
}

func (a *statusCapturingLogsAdapter) LogSnapshotWorkload(_ context.Context, runtimeID string) (io.ReadCloser, error) {
	a.logRuntimeID = runtimeID
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

func (a *blockingRuntimeAdapter) CreateWorkload(ctx context.Context, spec domain.WorkloadSpec) (string, error) {
	close(a.createStarted)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-a.createRelease:
		return a.availableMockAdapter.CreateWorkload(ctx, spec)
	}
}

func (a *blockingRuntimeAdapter) StopWorkload(ctx context.Context, runtimeID string) error {
	close(a.stopStarted)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.stopRelease:
		return nil
	}
}

func (a *blockingRuntimeAdapter) RemoveWorkload(ctx context.Context, runtimeID string) error {
	close(a.removeStarted)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.removeRelease:
		return nil
	}
}

type deleteOrderRuntimeAdapter struct {
	availableMockAdapter
	calls []string
}

func newDeleteOrderRuntimeAdapter() *deleteOrderRuntimeAdapter {
	return &deleteOrderRuntimeAdapter{availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}}
}

func (a *deleteOrderRuntimeAdapter) InspectWorkload(context.Context, string) (domain.WorkloadStatus, error) {
	a.calls = append(a.calls, "inspect")
	return domain.WorkloadStatus{}, errors.New("exited (exit code 127)")
}

func (a *deleteOrderRuntimeAdapter) StopWorkload(context.Context, string) error {
	a.calls = append(a.calls, "stop")
	return nil
}

func (a *deleteOrderRuntimeAdapter) RemoveWorkload(context.Context, string) error {
	a.calls = append(a.calls, "remove")
	return nil
}

type testServerFixture struct {
	ID                    string
	Name                  string
	GameKey               domain.GameKey
	ProviderKey           domain.ProviderKey
	Status                domain.ServerStatus
	WorldName             string
	PlayersOnline         int
	Port                  int
	MaxPlayers            int
	Password              string
	DataDir               string
	ContainerID           string
	HostPort              int
	CPULimitCores         float64
	MemoryLimitMB         int
	Version               string
	LastError             string
	SourceWorldID         string
	SourceWorldName       string
	Config                terraria.Config
	ConfigPayloadJSON     string
	ConfigPayload         map[string]any
	ConfigRevision        int
	AppliedConfigRevision int
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func createTestServer(t *testing.T, db *store.Store, server testServerFixture) {
	t.Helper()
	resource := gameServerFromTestFixture(server)
	if err := db.CreateGameServer(context.Background(), &resource); err != nil {
		t.Fatal(err)
	}
}

func saveTestServer(t *testing.T, db *store.Store, server testServerFixture) {
	t.Helper()
	resource := gameServerFromTestFixture(server)
	if err := db.SaveGameServer(context.Background(), &resource); err != nil {
		t.Fatal(err)
	}
}

func loadTestServer(db *store.Store, id string) (testServerFixture, error) {
	server, err := db.GetGameServer(context.Background(), id)
	if err != nil {
		return testServerFixture{}, err
	}
	return testFixtureFromGameServer(server), nil
}

func waitForServerStatus(t *testing.T, db *store.Store, id string, status domain.ServerStatus) testServerFixture {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		server, err := loadTestServer(db, id)
		if err == nil && server.Status == status {
			return server
		}
		time.Sleep(10 * time.Millisecond)
	}
	server, err := loadTestServer(db, id)
	t.Fatalf("expected server %s to reach status %s, got server=%+v err=%v", id, status, server, err)
	return testServerFixture{}
}

func waitForServerDeleted(t *testing.T, db *store.Store, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := loadTestServer(db, id); errors.Is(err, store.ErrNotFound) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	server, err := loadTestServer(db, id)
	t.Fatalf("expected server %s to be deleted, got server=%+v err=%v", id, server, err)
}

func gameServerFromTestFixture(server testServerFixture) domain.GameServer {
	config := cloneTestPayload(server.ConfigPayload)
	if len(config) == 0 && strings.TrimSpace(server.ConfigPayloadJSON) != "" {
		_ = json.Unmarshal([]byte(server.ConfigPayloadJSON), &config)
	}
	if len(config) == 0 {
		if buf, err := json.Marshal(server.Config); err == nil {
			_ = json.Unmarshal(buf, &config)
		}
	}
	fillTestConfigPayload(config, server)
	createdAt := server.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	updatedAt := server.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	generation := server.ConfigRevision
	if generation <= 0 {
		generation = 1
	}
	appliedGeneration := server.AppliedConfigRevision
	if appliedGeneration == 0 && server.ContainerID != "" && (server.Status == domain.StatusRunning || server.Status == domain.StatusStopped) {
		appliedGeneration = generation
	}
	version := server.Version
	switch server.ProviderKey {
	case domain.ProviderTerrariaTModLoader:
		switch version {
		case "v2026.04.3.0", "v2026.02.3.1":
		default:
			version = "v2026.04.3.0"
		}
	case domain.ProviderTerrariaVanilla:
		if version == "" || version != "1.4.5.6" {
			version = "1.4.5.6"
		}
	}
	return domain.GameServer{
		ID:          server.ID,
		Name:        server.Name,
		GameKey:     server.GameKey,
		ProviderKey: server.ProviderKey,
		Spec: domain.ServerSpec{
			Generation:      generation,
			DesiredState:    desiredStateFromTestStatus(server.Status),
			Version:         version,
			Config:          cloneTestPayload(config),
			SourceWorldID:   server.SourceWorldID,
			SourceWorldName: server.SourceWorldName,
			Resources: domain.ServerResources{
				CPULimitCores: server.CPULimitCores,
				MemoryLimitMB: server.MemoryLimitMB,
			},
			Network: domain.ServerNetworkSpec{
				Port:     server.Port,
				HostPort: server.HostPort,
			},
			Runtime: domain.ServerRuntimeSpec{DataDir: server.DataDir},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              phaseFromTestStatus(server.Status),
			ActualState:        actualStateFromTestStatus(server.Status),
			RuntimeID:          server.ContainerID,
			PlayersOnline:      server.PlayersOnline,
			ObservedGeneration: generation,
			AppliedGeneration:  appliedGeneration,
			LastError:          server.LastError,
			LastTransitionAt:   updatedAt,
		},
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}

func testFixtureFromGameServer(server domain.GameServer) testServerFixture {
	config := terraria.Config{}
	payloadJSON := ""
	if server.Spec.Config != nil {
		if buf, err := json.Marshal(server.Spec.Config); err == nil {
			payloadJSON = string(buf)
			_ = json.Unmarshal(buf, &config)
		}
	}
	if config.Port == 0 {
		config.Port = server.Spec.Network.Port
	}
	if config.MaxPlayers == 0 {
		config.MaxPlayers = intFromTestPayload(server.Spec.Config, "maxPlayers", 0)
	}
	worldName := config.WorldName
	password := config.Password
	motd := config.MOTD
	switch server.ProviderKey {
	case domain.ProviderPalworld:
		worldName = stringFromTestPayload(server.Spec.Config, "saveName", worldName)
		password = stringFromTestPayload(server.Spec.Config, "serverPassword", password)
		motd = stringFromTestPayload(server.Spec.Config, "adminPassword", motd)
	case domain.ProviderDST:
		worldName = stringFromTestPayload(server.Spec.Config, "clusterName", worldName)
		password = stringFromTestPayload(server.Spec.Config, "serverPassword", password)
		motd = stringFromTestPayload(server.Spec.Config, "clusterToken", motd)
	case domain.ProviderMinecraft:
		worldName = stringFromTestPayload(server.Spec.Config, "worldName", worldName)
	}
	config.WorldName = worldName
	config.Password = password
	config.MOTD = motd
	return testServerFixture{
		ID:                    server.ID,
		Name:                  server.Name,
		GameKey:               server.GameKey,
		ProviderKey:           server.ProviderKey,
		Status:                domain.ServerStatusFromRuntime(server.Spec.DesiredState, server.Status),
		WorldName:             worldName,
		PlayersOnline:         server.Status.PlayersOnline,
		Port:                  config.Port,
		MaxPlayers:            config.MaxPlayers,
		Password:              password,
		DataDir:               server.Spec.Runtime.DataDir,
		ContainerID:           server.Status.RuntimeID,
		HostPort:              server.Spec.Network.HostPort,
		CPULimitCores:         server.Spec.Resources.CPULimitCores,
		MemoryLimitMB:         server.Spec.Resources.MemoryLimitMB,
		Version:               server.Spec.Version,
		LastError:             server.Status.LastError,
		SourceWorldID:         server.Spec.SourceWorldID,
		SourceWorldName:       server.Spec.SourceWorldName,
		Config:                config,
		ConfigPayloadJSON:     payloadJSON,
		ConfigPayload:         cloneTestPayload(server.Spec.Config),
		ConfigRevision:        server.Spec.Generation,
		AppliedConfigRevision: server.Status.AppliedGeneration,
		CreatedAt:             server.CreatedAt,
		UpdatedAt:             server.UpdatedAt,
	}
}

func testConfigPayload(config terraria.Config) map[string]any {
	payload := map[string]any{}
	if buf, err := json.Marshal(config); err == nil {
		_ = json.Unmarshal(buf, &payload)
	}
	return payload
}

func fillTestConfigPayload(config map[string]any, server testServerFixture) {
	if config == nil {
		return
	}
	if _, ok := config["serverName"]; !ok && server.Name != "" {
		config["serverName"] = server.Name
	}
	if _, ok := config["worldName"]; !ok && server.WorldName != "" {
		config["worldName"] = server.WorldName
	}
	if _, ok := config["maxPlayers"]; !ok && server.MaxPlayers > 0 {
		config["maxPlayers"] = server.MaxPlayers
	}
	if _, ok := config["port"]; !ok && server.Port > 0 {
		config["port"] = server.Port
	}
	if _, ok := config["password"]; !ok && server.Password != "" {
		config["password"] = server.Password
	}
}

func desiredStateFromTestStatus(status domain.ServerStatus) domain.ServerDesiredState {
	switch status {
	case domain.StatusStopped, domain.StatusStopping:
		return domain.DesiredStopped
	case domain.StatusDeleting:
		return domain.DesiredDeleted
	default:
		return domain.DesiredRunning
	}
}

func phaseFromTestStatus(status domain.ServerStatus) domain.ServerPhase {
	switch status {
	case domain.StatusRunning:
		return domain.PhaseRunning
	case domain.StatusStopped:
		return domain.PhaseStopped
	case domain.StatusErrored:
		return domain.PhaseFailed
	case domain.StatusDeleting:
		return domain.PhaseDeleting
	default:
		return domain.PhasePending
	}
}

func actualStateFromTestStatus(status domain.ServerStatus) domain.ServerActualState {
	switch status {
	case domain.StatusRunning, domain.StatusStopping, domain.StatusRestarting:
		return domain.ActualRunning
	case domain.StatusStopped:
		return domain.ActualStopped
	default:
		return domain.ActualUnknown
	}
}

func cloneTestPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}

func stringFromTestPayload(payload map[string]any, key string, fallback string) string {
	if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func intFromTestPayload(payload map[string]any, key string, fallback int) int {
	switch value := payload[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
	}
	return fallback
}

func testServer(id string, dataDir string) testServerFixture {
	return testServerFixture{
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
