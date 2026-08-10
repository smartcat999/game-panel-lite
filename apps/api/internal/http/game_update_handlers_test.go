package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type gameUpdateHTTPAdapter struct {
	availableMockAdapter

	checkStarted chan runtime.GameUpdateRequest
	checkRelease chan struct{}
	releaseOnce  sync.Once
	checkResult  runtime.GameUpdateResult
	checkErr     error

	applyCalls   atomic.Int32
	applyStarted chan runtime.GameUpdateRequest
	applyResult  runtime.GameUpdateResult
	applyErr     error
	cleanupCalls atomic.Int32
}

type gameUpdateHealthAdapter struct {
	availableMockAdapter
	health runtime.WorkloadHealth
}

func (a *gameUpdateHealthAdapter) InspectWorkloadHealth(_ context.Context, _ string) (runtime.WorkloadHealth, error) {
	return a.health, nil
}

func newGameUpdateHTTPAdapter() *gameUpdateHTTPAdapter {
	return &gameUpdateHTTPAdapter{
		availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()},
		checkStarted:         make(chan runtime.GameUpdateRequest, 1),
		checkRelease:         make(chan struct{}),
		applyStarted:         make(chan runtime.GameUpdateRequest, 1),
		checkResult: runtime.GameUpdateResult{
			InstalledBuildID: "24088465",
			LatestBuildID:    "24181105",
		},
		applyResult: runtime.GameUpdateResult{
			InstalledBuildID: "24181105",
			LatestBuildID:    "24181105",
		},
	}
}

func (a *gameUpdateHTTPAdapter) releaseCheck() {
	a.releaseOnce.Do(func() { close(a.checkRelease) })
}

func (a *gameUpdateHTTPAdapter) CheckGameUpdate(ctx context.Context, request runtime.GameUpdateRequest) (runtime.GameUpdateResult, error) {
	select {
	case a.checkStarted <- request:
	default:
	}
	select {
	case <-ctx.Done():
		return runtime.GameUpdateResult{}, ctx.Err()
	case <-a.checkRelease:
		return a.checkResult, a.checkErr
	}
}

func (a *gameUpdateHTTPAdapter) ApplyGameUpdate(_ context.Context, request runtime.GameUpdateRequest, onProgress runtime.GameUpdateProgressFunc) (runtime.GameUpdateResult, error) {
	a.applyCalls.Add(1)
	select {
	case a.applyStarted <- request:
	default:
	}
	if onProgress != nil {
		onProgress(runtime.GameUpdateProgress{Stage: runtime.GameUpdateStageDownloading, Progress: 55, Message: "downloading"})
		onProgress(runtime.GameUpdateProgress{Stage: runtime.GameUpdateStageInstalling, Progress: 90, Message: "installing"})
	}
	return a.applyResult, a.applyErr
}

func (a *gameUpdateHTTPAdapter) CleanupGameUpdate(_ context.Context, _ string) error {
	a.cleanupCalls.Add(1)
	return nil
}

func TestGameUpdateUnsupportedForNonPalworldProvider(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("terraria-no-update", cfg.DataDir)
	createTestServer(t, db, server)

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/game-update", nil))
	if get.Code != stdhttp.StatusOK {
		t.Fatalf("expected update status 200, got %d: %s", get.Code, get.Body.String())
	}
	var view gameUpdateView
	if err := json.Unmarshal(get.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Supported || view.Status != "unknown" {
		t.Fatalf("expected unsupported unknown update state, got %+v", view)
	}

	check := httptest.NewRecorder()
	router.ServeHTTP(check, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/game-update/check", nil))
	if check.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected unsupported check 400, got %d: %s", check.Code, check.Body.String())
	}
}

func TestGameUpdateUnavailableRuntimeDoesNotCreateActiveJob(t *testing.T) {
	router, db, cfg := newTestRouterWithAdapter(t, availableMockAdapter{MockAdapter: runtime.NewMockAdapter()})
	server := palworldUpdateTestServer("palworld-update-unavailable", cfg.DataDir)
	createTestServer(t, db, server)

	check := httptest.NewRecorder()
	router.ServeHTTP(check, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/game-update/check", nil))
	if check.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("expected unsupported runtime to return 503, got %d: %s", check.Code, check.Body.String())
	}
	if _, err := db.GetActiveGameUpdateJobByInstance(context.Background(), server.ID); err != store.ErrNotFound {
		t.Fatalf("expected no active update job when runtime capability is unavailable, got %v", err)
	}
}

func TestGameUpdateAutoCheckDefaultsOnAndCanBeDisabled(t *testing.T) {
	router, db, cfg := newTestRouterWithAdapter(t, newGameUpdateHTTPAdapter())
	server := palworldUpdateTestServer("palworld-auto-check-setting", cfg.DataDir)
	createTestServer(t, db, server)

	readDefault := httptest.NewRecorder()
	router.ServeHTTP(readDefault, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/game-update", nil))
	if readDefault.Code != stdhttp.StatusOK {
		t.Fatalf("expected update status 200, got %d: %s", readDefault.Code, readDefault.Body.String())
	}
	var defaultView gameUpdateView
	if err := json.Unmarshal(readDefault.Body.Bytes(), &defaultView); err != nil {
		t.Fatal(err)
	}
	if !defaultView.AutoCheckEnabled || defaultView.AutoCheckHours != 6 {
		t.Fatalf("expected six-hour automatic checks by default, got %+v", defaultView)
	}

	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/"+server.ID+"/game-update/auto-check", strings.NewReader(`{"enabled":false}`)))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected auto-check update 200, got %d: %s", update.Code, update.Body.String())
	}

	readDisabled := httptest.NewRecorder()
	router.ServeHTTP(readDisabled, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/game-update", nil))
	var disabledView gameUpdateView
	if err := json.Unmarshal(readDisabled.Body.Bytes(), &disabledView); err != nil {
		t.Fatal(err)
	}
	if disabledView.AutoCheckEnabled {
		t.Fatalf("expected automatic checks to be disabled, got %+v", disabledView)
	}
}

func TestAutomaticGameUpdateScanQueuesOneStaleProviderCheck(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	t.Cleanup(adapter.releaseCheck)
	handler, db, _ := newGameUpdateUnitHandler(t, adapter)
	handler.ctx = context.Background()
	checkedAt := time.Now().UTC()
	if err := db.CreateGameUpdateJob(context.Background(), &domain.GameUpdateJob{
		ID: "stale-provider-check", InstanceID: providerGameUpdateScope(domain.ProviderPalworld), ProviderKey: domain.ProviderPalworld,
		Operation: domain.GameUpdateOperationCheck, Status: domain.GameUpdateJobSucceeded,
		Stage: domain.GameUpdateStageCompleted, CheckedAt: &checkedAt, CreatedAt: checkedAt, UpdatedAt: checkedAt, LatestBuildID: "24000000",
	}); err != nil {
		t.Fatal(err)
	}

	if err := handler.scanAutomaticGameUpdateChecks(context.Background(), checkedAt.Add(gameUpdateAutoCheckInterval)); err != nil {
		t.Fatal(err)
	}
	var request runtime.GameUpdateRequest
	select {
	case request = <-adapter.checkStarted:
	case <-time.After(time.Second):
		t.Fatal("expected stale provider to start an automatic check")
	}
	if request.RuntimeID != providerGameUpdateScope(domain.ProviderPalworld) || request.DataDir != "" {
		t.Fatalf("expected provider-scoped request without a server data directory, got %+v", request)
	}
	adapter.releaseCheck()
	active := waitForGameUpdateJobStatus(t, db, request.JobID, domain.GameUpdateJobSucceeded)
	if active.Operation != domain.GameUpdateOperationCheck {
		t.Fatalf("expected automatic check operation, got %+v", active)
	}
	latest, err := db.GetLatestGameUpdateCheckByProvider(context.Background(), domain.ProviderPalworld)
	if err != nil || latest.ID != active.ID {
		t.Fatalf("expected shared provider check to be latest, got %+v err=%v", latest, err)
	}
}

func TestCheckGameUpdateReturnsAcceptedAndPersistsResult(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	t.Cleanup(adapter.releaseCheck)
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := palworldUpdateTestServer("palworld-check", cfg.DataDir)
	server.ContainerID = "palworld-runtime"
	createTestServer(t, db, server)

	check := httptest.NewRecorder()
	router.ServeHTTP(check, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/game-update/check", nil))
	if check.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected update check 202, got %d: %s", check.Code, check.Body.String())
	}
	var queued domain.GameUpdateJob
	if err := json.Unmarshal(check.Body.Bytes(), &queued); err != nil {
		t.Fatal(err)
	}
	if queued.Operation != domain.GameUpdateOperationCheck || queued.Status != domain.GameUpdateJobQueued || queued.Stage != domain.GameUpdateStageRefreshingMetadata {
		t.Fatalf("expected queued update job, got %+v", queued)
	}

	var request runtime.GameUpdateRequest
	select {
	case request = <-adapter.checkStarted:
	case <-time.After(time.Second):
		t.Fatal("expected async update check to reach the runtime adapter")
	}
	if request.JobID != queued.ID || request.AppID != palworldSteamAppID || request.RuntimeID != providerGameUpdateScope(domain.ProviderPalworld) || request.DataDir != "" {
		t.Fatalf("unexpected update request: %+v", request)
	}
	adapter.releaseCheck()

	completed := waitForGameUpdateJobStatus(t, db, queued.ID, domain.GameUpdateJobSucceeded)
	if completed.InstalledBuildID != "" || completed.LatestBuildID != "24181105" {
		t.Fatalf("expected provider check to persist only the latest build id, got %+v", completed)
	}
	if completed.Stage != domain.GameUpdateStageCompleted || completed.Progress != 100 || completed.CheckedAt == nil || completed.CompletedAt == nil {
		t.Fatalf("expected completed job metadata, got %+v", completed)
	}

	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/game-update", nil))
	if status.Code != stdhttp.StatusOK {
		t.Fatalf("expected update status 200, got %d: %s", status.Code, status.Body.String())
	}
	var view gameUpdateView
	if err := json.Unmarshal(status.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Supported || view.Status != "available" || view.Job == nil || view.Job.ID != queued.ID {
		t.Fatalf("expected available update view, got %+v", view)
	}
}

func TestApplyGameUpdateRejectsOnlinePlayers(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	t.Cleanup(adapter.releaseCheck)
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := palworldUpdateTestServer("palworld-online", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "palworld-online-runtime"
	server.PlayersOnline = 2
	createTestServer(t, db, server)

	apply := httptest.NewRecorder()
	router.ServeHTTP(apply, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/game-update/apply", strings.NewReader(`{"startAfterUpdate":true}`)))
	if apply.Code != stdhttp.StatusConflict {
		t.Fatalf("expected online-player conflict 409, got %d: %s", apply.Code, apply.Body.String())
	}
	if adapter.applyCalls.Load() != 0 {
		t.Fatalf("expected runtime update not to start while players are online, got %d calls", adapter.applyCalls.Load())
	}
	if _, err := db.GetActiveGameUpdateJobByInstance(context.Background(), server.ID); err != store.ErrNotFound {
		t.Fatalf("expected no update job for rejected apply, got %v", err)
	}
}

func TestApplyGameUpdateBacksUpSaveAndCompletesAsynchronously(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	t.Cleanup(adapter.releaseCheck)
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := palworldUpdateTestServer("palworld-apply", cfg.DataDir)
	server.Version = "v2.5.0"
	server.ContainerID = "palworld-apply-runtime"
	createTestServer(t, db, server)
	saveDir := filepath.Join(server.DataDir, "Pal", "Saved", "SaveGames")
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "world.sav"), []byte("save-data"), 0o600); err != nil {
		t.Fatal(err)
	}

	apply := httptest.NewRecorder()
	router.ServeHTTP(apply, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/game-update/apply", strings.NewReader(`{"startAfterUpdate":false}`)))
	if apply.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected update apply 202, got %d: %s", apply.Code, apply.Body.String())
	}
	var queued domain.GameUpdateJob
	if err := json.Unmarshal(apply.Body.Bytes(), &queued); err != nil {
		t.Fatal(err)
	}
	if queued.Operation != domain.GameUpdateOperationApply || queued.Status != domain.GameUpdateJobQueued || queued.StartAfterUpdate || queued.WasRunning {
		t.Fatalf("expected queued stopped-server update job, got %+v", queued)
	}

	completed := waitForGameUpdateJobStatus(t, db, queued.ID, domain.GameUpdateJobSucceeded)
	if completed.InstalledBuildID != "24181105" || completed.Progress != 100 {
		t.Fatalf("expected completed update result, got %+v", completed)
	}
	select {
	case request := <-adapter.applyStarted:
		if request.JobID != queued.ID || request.Image != "smartcat99999/palworld-server:v2.5.0" || request.DataDir != server.DataDir {
			t.Fatalf("expected provider image and instance data dir, got %+v", request)
		}
	default:
		t.Fatal("expected runtime update request")
	}
	backups, err := db.ListBackupsByInstance(context.Background(), server.ID)
	if err != nil || len(backups) != 1 || backups[0].Type != "Pre-update" {
		t.Fatalf("expected one pre-update save backup, backups=%+v err=%v", backups, err)
	}
}

func TestActiveGameUpdateBlocksDuplicateTaskAndServerStart(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	t.Cleanup(adapter.releaseCheck)
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := palworldUpdateTestServer("palworld-update-lock", cfg.DataDir)
	createTestServer(t, db, server)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/game-update/check", nil))
	if first.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected first update check 202, got %d: %s", first.Code, first.Body.String())
	}
	var active domain.GameUpdateJob
	if err := json.Unmarshal(first.Body.Bytes(), &active); err != nil {
		t.Fatal(err)
	}
	select {
	case <-adapter.checkStarted:
	case <-time.After(time.Second):
		t.Fatal("expected first check to remain active in runtime adapter")
	}

	duplicate := httptest.NewRecorder()
	router.ServeHTTP(duplicate, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/game-update/check", nil))
	if duplicate.Code != stdhttp.StatusConflict {
		t.Fatalf("expected duplicate update conflict 409, got %d: %s", duplicate.Code, duplicate.Body.String())
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusConflict {
		t.Fatalf("expected lifecycle conflict 409 during update, got %d: %s", start.Code, start.Body.String())
	}

	adapter.releaseCheck()
	waitForGameUpdateJobStatus(t, db, active.ID, domain.GameUpdateJobSucceeded)
}

func TestApplyGameUpdateRejectsMalformedJSON(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	t.Cleanup(adapter.releaseCheck)
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := palworldUpdateTestServer("palworld-malformed-update", cfg.DataDir)
	createTestServer(t, db, server)

	apply := httptest.NewRecorder()
	router.ServeHTTP(apply, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/game-update/apply", strings.NewReader(`{"startAfterUpdate":`)))
	if apply.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected malformed payload 400, got %d: %s", apply.Code, apply.Body.String())
	}
	if adapter.applyCalls.Load() != 0 {
		t.Fatalf("expected malformed payload not to reach runtime, got %d calls", adapter.applyCalls.Load())
	}
	if _, err := db.GetActiveGameUpdateJobByInstance(context.Background(), server.ID); err != store.ErrNotFound {
		t.Fatalf("expected no update job for malformed payload, got %v", err)
	}
}

func TestCanceledGameUpdateRemainsActiveForStartupRecovery(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	handler, db, dataDir := newGameUpdateUnitHandler(t, adapter)
	fixture := palworldUpdateTestServer("palworld-canceled-update", dataDir)
	fixture.Status = domain.StatusStopped
	createTestServer(t, db, fixture)
	server, err := db.GetGameServer(context.Background(), fixture.ID)
	if err != nil {
		t.Fatal(err)
	}
	job := domain.GameUpdateJob{
		ID: "canceled-job", InstanceID: fixture.ID, ProviderKey: domain.ProviderPalworld,
		Operation: domain.GameUpdateOperationApply, Status: domain.GameUpdateJobRunning,
		Stage: domain.GameUpdateStageDownloading, Progress: 50, WasRunning: true,
		CreatedAt: time.Now().Add(-time.Minute), UpdatedAt: time.Now(),
	}
	if err := db.CreateGameUpdateJob(context.Background(), &job); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	handler.failGameUpdateTask(canceled, server, &job, context.Canceled, true)
	stored, err := db.GetGameUpdateJobByID(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.GameUpdateJobRunning || adapter.cleanupCalls.Load() != 0 {
		t.Fatalf("expected canceled task to remain active without racing cleanup, job=%+v cleanupCalls=%d", stored, adapter.cleanupCalls.Load())
	}
}

func TestCanceledPostUpdateStartStopsWorkloadAndKeepsJobRecoverable(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	handler, db, dataDir := newGameUpdateUnitHandler(t, adapter)
	fixture := palworldUpdateTestServer("palworld-canceled-health", dataDir)
	fixture.Status = domain.StatusRunning
	fixture.ContainerID = "palworld-canceled-health-runtime"
	createTestServer(t, db, fixture)
	server, err := db.GetGameServer(context.Background(), fixture.ID)
	if err != nil {
		t.Fatal(err)
	}
	job := domain.GameUpdateJob{
		ID: "canceled-health-job", InstanceID: fixture.ID, ProviderKey: domain.ProviderPalworld,
		Operation: domain.GameUpdateOperationApply, Status: domain.GameUpdateJobRunning,
		Stage: domain.GameUpdateStageHealthCheck, Progress: 97, WasRunning: true, StartAfterUpdate: true,
		CreatedAt: time.Now().Add(-time.Minute), UpdatedAt: time.Now(),
	}
	if err := db.CreateGameUpdateJob(context.Background(), &job); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	handler.failGameUpdateTask(canceled, server, &job, context.Canceled, false)
	stored, err := db.GetGameUpdateJobByID(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	server, err = db.GetGameServer(context.Background(), fixture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.GameUpdateJobRunning || server.Spec.DesiredState != domain.DesiredStopped {
		t.Fatalf("expected canceled post-update start to stay recoverable and request stop, job=%+v server=%+v", stored, server)
	}
	if adapter.cleanupCalls.Load() != 0 {
		t.Fatalf("expected cleanup to remain deferred to startup recovery, got %d calls", adapter.cleanupCalls.Load())
	}
}

func TestGameUpdateWorkerCannotStartAfterShutdownWaitBegins(t *testing.T) {
	handler, _, _ := newGameUpdateUnitHandler(t, newGameUpdateHTTPAdapter())
	if err := handler.WaitForGameUpdates(context.Background()); err != nil {
		t.Fatal(err)
	}
	ran := atomic.Bool{}
	if handler.startGameUpdateWorker(func() { ran.Store(true) }) {
		t.Fatal("expected update worker registration to close before WaitGroup.Wait")
	}
	if ran.Load() {
		t.Fatal("expected rejected update worker not to run")
	}
}

func TestRecoverInterruptedGameUpdateCleansAndRestoresDesiredRunning(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	handler, db, dataDir := newGameUpdateUnitHandler(t, adapter)
	fixture := palworldUpdateTestServer("palworld-recover-update", dataDir)
	fixture.Status = domain.StatusStopped
	createTestServer(t, db, fixture)
	job := domain.GameUpdateJob{
		ID: "interrupted-job", InstanceID: fixture.ID, ProviderKey: domain.ProviderPalworld,
		Operation: domain.GameUpdateOperationApply, Status: domain.GameUpdateJobRunning,
		Stage: domain.GameUpdateStageBackingUp, Progress: 10, WasRunning: true,
		CreatedAt: time.Now().Add(-time.Minute), UpdatedAt: time.Now(),
	}
	if err := db.CreateGameUpdateJob(context.Background(), &job); err != nil {
		t.Fatal(err)
	}

	handler.recoverInterruptedGameUpdates(context.Background(), time.Now())

	stored, err := db.GetGameUpdateJobByID(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	server, err := db.GetGameServer(context.Background(), fixture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.GameUpdateJobFailed || !strings.Contains(stored.Error, "API restart") {
		t.Fatalf("expected interrupted task to fail after cleanup, got %+v", stored)
	}
	if adapter.cleanupCalls.Load() != 1 || server.Spec.DesiredState != domain.DesiredRunning {
		t.Fatalf("expected cleanup and desired-running restoration, cleanupCalls=%d server=%+v", adapter.cleanupCalls.Load(), server)
	}
}

func TestRecoverInterruptedApplyRevalidatesBeforeCompleting(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	handler, db, dataDir := newGameUpdateUnitHandler(t, adapter)
	fixture := palworldUpdateTestServer("palworld-resume-update", dataDir)
	fixture.Status = domain.StatusStopped
	fixture.ContainerID = "palworld-resume-runtime"
	createTestServer(t, db, fixture)
	server, err := db.GetGameServer(context.Background(), fixture.ID)
	if err != nil {
		t.Fatal(err)
	}
	server.Spec.Runtime.Image = "smartcat99999/palworld-server:v2.5.0"
	if err := db.SaveGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	job := domain.GameUpdateJob{
		ID: "resume-job", InstanceID: fixture.ID, ProviderKey: domain.ProviderPalworld,
		Operation: domain.GameUpdateOperationApply, Status: domain.GameUpdateJobRunning,
		Stage: domain.GameUpdateStageDownloading, Progress: 55, WasRunning: true, StartAfterUpdate: false,
		CreatedAt: time.Now().Add(-time.Minute), UpdatedAt: time.Now(),
	}
	if err := db.CreateGameUpdateJob(context.Background(), &job); err != nil {
		t.Fatal(err)
	}

	handler.recoverInterruptedGameUpdates(context.Background(), time.Now())

	stored, err := db.GetGameUpdateJobByID(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	server, err = db.GetGameServer(context.Background(), fixture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.GameUpdateJobSucceeded || stored.InstalledBuildID != "24181105" || stored.CheckedAt == nil {
		t.Fatalf("expected interrupted apply to revalidate and complete, got %+v", stored)
	}
	if adapter.applyCalls.Load() != 1 || adapter.cleanupCalls.Load() != 1 {
		t.Fatalf("expected stale helper cleanup followed by one validation apply, apply=%d cleanup=%d", adapter.applyCalls.Load(), adapter.cleanupCalls.Load())
	}
	if server.Spec.DesiredState != domain.DesiredStopped {
		t.Fatalf("expected start-after=false to remain stopped after resumed update, got %s", server.Spec.DesiredState)
	}
}

func TestResumeInterruptedApplyPreservesValidationStageWhenStopWaitIsCanceled(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	handler, db, dataDir := newGameUpdateUnitHandler(t, adapter)
	fixture := palworldUpdateTestServer("palworld-resume-stop-canceled", dataDir)
	fixture.Status = domain.StatusRunning
	fixture.ContainerID = "palworld-resume-stop-canceled-runtime"
	createTestServer(t, db, fixture)
	job := domain.GameUpdateJob{
		ID: "resume-stop-canceled-job", InstanceID: fixture.ID, ProviderKey: domain.ProviderPalworld,
		Operation: domain.GameUpdateOperationApply, Status: domain.GameUpdateJobRunning,
		Stage: domain.GameUpdateStageDownloading, Progress: 55, WasRunning: true, StartAfterUpdate: true,
		CreatedAt: time.Now().Add(-time.Minute), UpdatedAt: time.Now(),
	}
	if err := db.CreateGameUpdateJob(context.Background(), &job); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.resumeInterruptedGameUpdate(ctx, &job)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		server, err := db.GetGameServer(context.Background(), fixture.ID)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if server.Spec.DesiredState == domain.DesiredStopped {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("expected resumed update to request a stop before waiting")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected cancellation to interrupt the stop wait")
	}

	stored, err := db.GetGameUpdateJobByID(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.GameUpdateJobRunning || stored.Stage != domain.GameUpdateStageDownloading || !interruptedApplyRequiresValidation(stored.Stage) {
		t.Fatalf("expected interrupted recovery to remain active at a validation-required stage, got %+v", stored)
	}
	if adapter.applyCalls.Load() != 0 {
		t.Fatalf("expected cancellation before validation reached the runtime, got %d apply calls", adapter.applyCalls.Load())
	}
}

func TestStartAfterUpdateFailureRequestsStop(t *testing.T) {
	for _, stage := range []domain.GameUpdateJobStage{domain.GameUpdateStageStarting, domain.GameUpdateStageHealthCheck} {
		t.Run(string(stage), func(t *testing.T) {
			adapter := newGameUpdateHTTPAdapter()
			handler, db, dataDir := newGameUpdateUnitHandler(t, adapter)
			fixture := palworldUpdateTestServer("palworld-start-failure-"+string(stage), dataDir)
			fixture.Status = domain.StatusRunning
			fixture.ContainerID = "palworld-start-failure-runtime-" + string(stage)
			createTestServer(t, db, fixture)
			server, err := db.GetGameServer(context.Background(), fixture.ID)
			if err != nil {
				t.Fatal(err)
			}
			initialGeneration := server.Spec.Generation
			job := domain.GameUpdateJob{
				ID: "start-failure-job-" + string(stage), InstanceID: fixture.ID, ProviderKey: domain.ProviderPalworld,
				Operation: domain.GameUpdateOperationApply, Status: domain.GameUpdateJobRunning,
				Stage: stage, Progress: 97, WasRunning: true, StartAfterUpdate: true,
				CreatedAt: time.Now().Add(-time.Minute), UpdatedAt: time.Now(),
			}
			if err := db.CreateGameUpdateJob(context.Background(), &job); err != nil {
				t.Fatal(err)
			}
			controllerDone := make(chan struct{})
			go func() {
				defer close(controllerDone)
				deadline := time.Now().Add(time.Second)
				for time.Now().Before(deadline) {
					current, err := db.GetGameServer(context.Background(), fixture.ID)
					if err == nil && current.Spec.DesiredState == domain.DesiredStopped {
						current.Status.Phase = domain.PhaseStopped
						current.Status.ActualState = domain.ActualStopped
						_ = db.SaveGameServer(context.Background(), &current)
						return
					}
					time.Sleep(5 * time.Millisecond)
				}
			}()

			handler.failGameUpdateApply(context.Background(), server, &job, errors.New("start-after-update failed"))
			<-controllerDone

			stored, err := db.GetGameUpdateJobByID(context.Background(), job.ID)
			if err != nil {
				t.Fatal(err)
			}
			server, err = db.GetGameServer(context.Background(), fixture.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status != domain.GameUpdateJobFailed {
				t.Fatalf("expected update job to fail, got %+v", stored)
			}
			if server.Spec.DesiredState != domain.DesiredStopped || server.Spec.Generation <= initialGeneration {
				t.Fatalf("expected %s failure to request a stop, initialGeneration=%d server=%+v", stage, initialGeneration, server)
			}
		})
	}
}

func TestRecoverInterruptedGameUpdateSkipsJobsCreatedByCurrentProcess(t *testing.T) {
	adapter := newGameUpdateHTTPAdapter()
	handler, db, dataDir := newGameUpdateUnitHandler(t, adapter)
	fixture := palworldUpdateTestServer("palworld-current-update", dataDir)
	createTestServer(t, db, fixture)
	startedAt := time.Now()
	job := domain.GameUpdateJob{
		ID: "current-job", InstanceID: fixture.ID, ProviderKey: domain.ProviderPalworld,
		Operation: domain.GameUpdateOperationCheck, Status: domain.GameUpdateJobQueued,
		Stage: domain.GameUpdateStageRefreshingMetadata, CreatedAt: startedAt.Add(time.Second), UpdatedAt: startedAt.Add(time.Second),
	}
	if err := db.CreateGameUpdateJob(context.Background(), &job); err != nil {
		t.Fatal(err)
	}

	handler.recoverInterruptedGameUpdates(context.Background(), startedAt)

	stored, err := db.GetGameUpdateJobByID(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.GameUpdateJobQueued || adapter.cleanupCalls.Load() != 0 {
		t.Fatalf("expected current-process task to remain queued, job=%+v cleanupCalls=%d", stored, adapter.cleanupCalls.Load())
	}
}

func TestGameServerHealthFallbackRequiresHealthyStableWindow(t *testing.T) {
	adapter := &gameUpdateHealthAdapter{
		availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()},
		health:               runtime.WorkloadHealth{Status: runtime.WorkloadHealthUnhealthy, HasHealthCheck: false},
	}
	handler, db, dataDir := newGameUpdateUnitHandler(t, adapter)
	fixture := palworldUpdateTestServer("palworld-health", dataDir)
	fixture.Status = domain.StatusRunning
	fixture.ContainerID = "palworld-health-runtime"
	createTestServer(t, db, fixture)
	if err := handler.waitForGameServerHealthyWithTiming(context.Background(), fixture.ID, 40*time.Millisecond, 5*time.Millisecond, 15*time.Millisecond); err == nil {
		t.Fatal("expected unhealthy workload without a Docker healthcheck not to pass the stable window")
	}
	adapter.health = runtime.WorkloadHealth{Status: runtime.WorkloadHealthHealthy, HasHealthCheck: false}
	if err := handler.waitForGameServerHealthyWithTiming(context.Background(), fixture.ID, 80*time.Millisecond, 5*time.Millisecond, 15*time.Millisecond); err != nil {
		t.Fatalf("expected healthy workload to pass the fallback stable window: %v", err)
	}
}

func newGameUpdateUnitHandler(t *testing.T, adapter runtime.Adapter) (*Handler, *store.Store, string) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "gamepanel.db"))
	if err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(root, "data")
	return &Handler{
		store:            db,
		provider:         provider.NewRegistry(palworld.NewProvider()),
		runtime:          runtime.NewSwitchableAdapter(adapter),
		runtimeImageJobs: map[string]domain.RuntimeImageStatus{},
	}, db, dataDir
}

func palworldUpdateTestServer(id string, dataDir string) testServerFixture {
	server := testServer(id, dataDir)
	server.GameKey = domain.GamePalworld
	server.ProviderKey = domain.ProviderPalworld
	server.Port = 7778
	server.HostPort = 7778
	server.Version = "latest"
	server.ConfigPayload = map[string]any{
		"serverName": id + " server",
		"saveName":   id + " world",
		"maxPlayers": 8,
		"port":       7778,
	}
	return server
}

func waitForGameUpdateJobStatus(t *testing.T, db *store.Store, id string, status domain.GameUpdateJobStatus) domain.GameUpdateJob {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := db.GetGameUpdateJobByID(context.Background(), id)
		if err == nil && job.Status == status {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, err := db.GetGameUpdateJobByID(context.Background(), id)
	t.Fatalf("expected update job %s to reach %s, got job=%+v err=%v", id, status, job, err)
	return domain.GameUpdateJob{}
}
