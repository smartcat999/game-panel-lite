package http

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	serverctrl "github.com/smartcat999/game-panel-lite/apps/api/internal/server"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func setupDeclarativeFixtures(t *testing.T, db *store.Store, orgID, nodeID string) {
	t.Helper()
	ctx := context.Background()
	org := domain.Organization{
		ID:   orgID,
		Name: "Declarative Test Org",
		Slug: orgID,
	}
	if err := db.CreateOrganization(ctx, &org, "owner-admin"); err != nil {
		t.Fatal(err)
	}
	if nodeID != "" {
		node := domain.ComputeNode{
			ID:            nodeID,
			Name:          "Edge Worker Node 1",
			Token:         "remote-token",
			Status:        "online",
			LastHeartbeat: time.Now().UTC(),
		}
		if err := db.CreateComputeNode(ctx, &node); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDeclarativeModUploadWithoutLocalDisk(t *testing.T) {
	router, db, _ := newTestRouter(t)
	setupDeclarativeFixtures(t, db, "org-1", "edge-worker-node-1")
	ghostDir := filepath.Join(t.TempDir(), "control-must-never-create")

	server := domain.GameServer{
		ID:             "remote-crd-server",
		Name:           "Remote CRD Server",
		OrganizationID: "org-1",
		NodeID:         "edge-worker-node-1",
		GameKey:        domain.GameTerraria,
		ProviderKey:    domain.ProviderTerrariaTModLoader,
		Spec: domain.ServerSpec{
			Generation:   1,
			DesiredState: domain.DesiredStopped,
			Runtime: domain.ServerRuntimeSpec{
				DataDir: ghostDir,
			},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhaseStopped,
			ActualState:        domain.ActualStopped,
			ObservedGeneration: 1,
			AppliedGeneration:  1,
		},
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "ExampleMod.tmod")
	if err != nil {
		t.Fatal(err)
	}
	modBytes := tmodFixture("ExampleMod", "1.0.0", "2026.3.3.0")
	if _, err := part.Write(modBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	upload := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/servers/remote-crd-server/mods/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(upload, request)

	if upload.Code != stdhttp.StatusCreated {
		t.Fatalf("expected mod upload 201, got %d: %s", upload.Code, upload.Body.String())
	}
	var mod domain.ModFile
	if err := json.Unmarshal(upload.Body.Bytes(), &mod); err != nil {
		t.Fatal(err)
	}

	// 1. 验证控制面绝未在本地创建目标实例目录
	if _, err := os.Stat(ghostDir); !os.IsNotExist(err) {
		t.Fatalf("control plane must never touch or create remote instance directory on host: %s", ghostDir)
	}

	// 2. 验证 GameServer.Spec.ModIDs 与 Generation 声明式自增
	updated, err := db.GetGameServer(context.Background(), "remote-crd-server")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(updated.Spec.ModIDs, mod.ID) {
		t.Fatalf("expected server Spec.ModIDs to contain uploaded mod %s, got %+v", mod.ID, updated.Spec.ModIDs)
	}
	if updated.Spec.Generation != 2 {
		t.Fatalf("expected server Spec.Generation to increment to 2, got %d", updated.Spec.Generation)
	}
}

func TestDeclarativeModToggleStateConvergence(t *testing.T) {
	router, db, _ := newTestRouter(t)
	setupDeclarativeFixtures(t, db, "org-1", "edge-worker-node-1")

	server := domain.GameServer{
		ID:             "remote-toggle-server",
		Name:           "Remote Toggle Server",
		OrganizationID: "org-1",
		NodeID:         "edge-worker-node-1",
		GameKey:        domain.GameTerraria,
		ProviderKey:    domain.ProviderTerrariaTModLoader,
		Spec: domain.ServerSpec{
			Generation:   1,
			DesiredState: domain.DesiredStopped,
			ModIDs:       []string{"mod-alpha"},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhaseStopped,
			ActualState:        domain.ActualStopped,
			ObservedGeneration: 1,
			AppliedGeneration:  1,
		},
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	mod := domain.ModFile{
		ID:             "mod-alpha",
		InstanceID:     server.ID,
		OrganizationID: server.OrganizationID,
		ProviderKey:    server.ProviderKey,
		GameKey:        server.GameKey,
		FileName:       "Alpha.tmod",
		Enabled:        true,
	}
	if err := db.SaveMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	// 禁用模组 (enabled: false)
	disablePayload := []byte(`{"enabled": false}`)
	disableRec := httptest.NewRecorder()
	disableReq := httptest.NewRequest(stdhttp.MethodPatch, "/api/servers/remote-toggle-server/mods/mod-alpha", bytes.NewReader(disablePayload))
	disableReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(disableRec, disableReq)

	if disableRec.Code != stdhttp.StatusOK {
		t.Fatalf("expected toggle 200, got %d: %s", disableRec.Code, disableRec.Body.String())
	}

	afterDisable, err := db.GetGameServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(afterDisable.Spec.ModIDs, "mod-alpha") {
		t.Fatalf("expected mod-alpha removed from Spec.ModIDs after disable, got %+v", afterDisable.Spec.ModIDs)
	}
	if afterDisable.Spec.Generation != 2 {
		t.Fatalf("expected Generation incremented to 2, got %d", afterDisable.Spec.Generation)
	}

	// 再次启用模组 (enabled: true)
	enablePayload := []byte(`{"enabled": true}`)
	enableRec := httptest.NewRecorder()
	enableReq := httptest.NewRequest(stdhttp.MethodPatch, "/api/servers/remote-toggle-server/mods/mod-alpha", bytes.NewReader(enablePayload))
	enableReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(enableRec, enableReq)

	if enableRec.Code != stdhttp.StatusOK {
		t.Fatalf("expected toggle 200, got %d: %s", enableRec.Code, enableRec.Body.String())
	}

	afterEnable, err := db.GetGameServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(afterEnable.Spec.ModIDs, "mod-alpha") {
		t.Fatalf("expected mod-alpha restored to Spec.ModIDs after enable, got %+v", afterEnable.Spec.ModIDs)
	}
	if afterEnable.Spec.Generation != 3 {
		t.Fatalf("expected Generation incremented to 3, got %d", afterEnable.Spec.Generation)
	}
}

func TestDeclarativeModDeletePureCRD(t *testing.T) {
	router, db, _ := newTestRouter(t)
	setupDeclarativeFixtures(t, db, "org-1", "edge-worker-node-1")

	server := domain.GameServer{
		ID:             "remote-delete-server",
		Name:           "Remote Delete Server",
		OrganizationID: "org-1",
		NodeID:         "edge-worker-node-1",
		GameKey:        domain.GameTerraria,
		ProviderKey:    domain.ProviderTerrariaTModLoader,
		Spec: domain.ServerSpec{
			Generation:   1,
			DesiredState: domain.DesiredStopped,
			ModIDs:       []string{"mod-beta"},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhaseStopped,
			ActualState:        domain.ActualStopped,
			ObservedGeneration: 1,
			AppliedGeneration:  1,
		},
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	mod := domain.ModFile{
		ID:             "mod-beta",
		InstanceID:     server.ID,
		OrganizationID: server.OrganizationID,
		ProviderKey:    server.ProviderKey,
		GameKey:        server.GameKey,
		FileName:       "Beta.tmod",
		Enabled:        true,
	}
	if err := db.SaveMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	delRec := httptest.NewRecorder()
	delReq := httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/remote-delete-server/mods/mod-beta", nil)
	router.ServeHTTP(delRec, delReq)

	if delRec.Code != stdhttp.StatusOK {
		t.Fatalf("expected delete 200, got %d: %s", delRec.Code, delRec.Body.String())
	}

	afterDelete, err := db.GetGameServer(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(afterDelete.Spec.ModIDs, "mod-beta") {
		t.Fatalf("expected mod-beta removed from Spec.ModIDs after delete, got %+v", afterDelete.Spec.ModIDs)
	}
	if afterDelete.Spec.Generation != 2 {
		t.Fatalf("expected Generation incremented to 2, got %d", afterDelete.Spec.Generation)
	}

	// 确认数据库已删除记录
	if _, err := db.GetMod(context.Background(), "mod-beta"); err == nil {
		t.Fatalf("expected mod record deleted from store")
	}
}

func TestDeclarativeModReconciliationToAssignment(t *testing.T) {
	_, db, cfg := newTestRouter(t)
	ctx := context.Background()
	setupDeclarativeFixtures(t, db, "org-1", "edge-worker-node-1")

	server := domain.GameServer{
		ID:             "remote-reconcile-server",
		Name:           "Remote Reconcile Server",
		OrganizationID: "org-1",
		NodeID:         "edge-worker-node-1",
		GameKey:        domain.GameTerraria,
		ProviderKey:    domain.ProviderTerrariaTModLoader,
		Spec: domain.ServerSpec{
			Generation:   1,
			DesiredState: domain.DesiredRunning,
			ModIDs:       []string{"mod-gamma"},
			Network: domain.ServerNetworkSpec{
				Port:     7777,
				HostPort: 7777,
			},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhasePending,
			ActualState:        domain.ActualStopped,
			ObservedGeneration: 0,
			AppliedGeneration:  0,
		},
	}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}

	mod := domain.ModFile{
		ID:             "mod-gamma",
		InstanceID:     "unassigned",
		OrganizationID: server.OrganizationID,
		ProviderKey:    server.ProviderKey,
		GameKey:        server.GameKey,
		Source:         "upload",
		FileName:       "Gamma.tmod",
		ModName:        "Gamma",
		Enabled:        true,
		SizeBytes:      4,
		ContentHash:    strings.Repeat("a", 64),
	}
	if err := db.CreateMod(ctx, &mod); err != nil {
		t.Fatal(err)
	}

	registry := mustRegistry(t, terraria.NewTModLoaderProvider())
	builder := serverctrl.NewProviderWorkloadBuilder(registry).WithModPlanner(serverctrl.NewRuntimeModPlanner(cfg.DataDir, db, registry))
	controller := serverctrl.NewController(db, serverctrl.NewRuntimeReconciler(builder, nil), nil).WithDataRoot(cfg.DataDir)

	// 执行单轮调谐
	controller.RunOnce(ctx)

	// 验证生成了对应的 WorkloadAssignment，并携带有目标代次
	assignment, err := db.GetWorkloadAssignmentByServer(ctx, server.ID)
	if err != nil {
		t.Fatalf("expected published workload assignment for server %s: %v", server.ID, err)
	}
	if assignment.Generation != 1 {
		t.Fatalf("expected assignment generation 1, got %d", assignment.Generation)
	}
	if assignment.DesiredState != domain.DesiredRunning {
		t.Fatalf("expected desired running, got %s", assignment.DesiredState)
	}

	// 模拟边缘 Agent 执行完成后上报包含 ArtifactsReady Condition 的 WorkloadObservation
	obs := domain.WorkloadObservation{
		AssignmentUID:      assignment.UID,
		ServerID:           server.ID,
		NodeID:             server.NodeID,
		ObservedGeneration: 1,
		ActualState:        domain.ActualRunning,
		Conditions: []domain.ServerCondition{
			{
				Type:   workload.ConditionArtifactsReady,
				Status: workload.ConditionStatusTrue,
			},
		},
	}
	if err := db.UpsertWorkloadObservation(ctx, &obs); err != nil {
		t.Fatal(err)
	}

	// 再次调谐观察结果
	controller.RunOnce(ctx)

	converged, err := db.GetGameServer(ctx, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if converged.Status.Phase != domain.PhaseRunning {
		t.Fatalf("expected converged server phase running, got %s", converged.Status.Phase)
	}
	if converged.Status.AppliedGeneration != 1 {
		t.Fatalf("expected applied generation 1, got %d", converged.Status.AppliedGeneration)
	}
}
