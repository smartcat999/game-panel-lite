package http

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	serverctrl "github.com/smartcat999/game-panel-lite/apps/api/internal/server"
)

func TestDeclarativeServerCreationWithoutLocalDisk(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	setupDeclarativeFixtures(t, db, "org-lifecycle", "node-edge-1")

	payload := map[string]any{
		"name":           "Declarative Terraria",
		"providerKey":    domain.ProviderTerrariaVanilla,
		"organizationId": "org-lifecycle",
		"nodeId":         "node-edge-1",
		"hostPort":       7777,
		"resources": map[string]any{
			"cpuLimitCores": 1.0,
			"memoryLimitMb": 2048,
		},
		"config": map[string]any{
			"serverName":      "Declarative World",
			"worldName":       "DecWorld",
			"worldSize":       "small",
			"difficulty":      "classic",
			"maxPlayers":      float64(8),
			"port":            float64(7777),
			"secure":          true,
			"language":        "en-US",
			"autoCreateWorld": true,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != stdhttp.StatusCreated {
		t.Fatalf("expected server create 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var created domain.GameServer
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	if created.Spec.DesiredState != domain.DesiredRunning {
		t.Fatalf("expected desired state running, got %s", created.Spec.DesiredState)
	}
	if created.Spec.Generation != 1 {
		t.Fatalf("expected generation 1, got %d", created.Spec.Generation)
	}
	if created.NodeID != "node-edge-1" {
		t.Fatalf("expected node-edge-1, got %s", created.NodeID)
	}

	// 1. 验证控制面宿主机绝对未创建实例物理目录 (零宿主机写盘)
	expectedDataDir := filepath.Join(cfg.DataDir, "instances", created.ID)
	if _, err := os.Stat(expectedDataDir); !os.IsNotExist(err) {
		t.Fatalf("control plane must never create instance directory on host: %s", expectedDataDir)
	}
}

func TestDeclarativeLifecycleConvergenceViaObservation(t *testing.T) {
	_, db, cfg := newTestRouter(t)
	ctx := context.Background()
	setupDeclarativeFixtures(t, db, "org-lifecycle-2", "node-edge-2")

	server := domain.GameServer{
		ID:             "remote-lifecycle-server",
		Name:           "Remote Lifecycle Server",
		OrganizationID: "org-lifecycle-2",
		NodeID:         "node-edge-2",
		GameKey:        domain.GameTerraria,
		ProviderKey:    domain.ProviderTerrariaVanilla,
		Spec: domain.ServerSpec{
			Generation:   1,
			DesiredState: domain.DesiredRunning,
			Network: domain.ServerNetworkSpec{
				Port:     7777,
				HostPort: 7777,
			},
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhasePending,
			ActualState:        domain.ActualMissing,
			ObservedGeneration: 0,
			AppliedGeneration:  0,
		},
	}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}

	registry := mustRegistry(t, terraria.NewVanillaProvider())
	builder := serverctrl.NewProviderWorkloadBuilder(registry)
	controller := serverctrl.NewController(db, serverctrl.NewRuntimeReconciler(builder, nil), nil).WithDataRoot(cfg.DataDir)

	// 第 1 轮调谐：发布 WorkloadAssignment
	controller.RunOnce(ctx)

	assignment, err := db.GetWorkloadAssignmentByServer(ctx, server.ID)
	if err != nil {
		t.Fatalf("expected workload assignment published: %v", err)
	}
	if assignment.Generation != 1 || assignment.DesiredState != domain.DesiredRunning {
		t.Fatalf("unexpected assignment: %+v", assignment)
	}

	afterFirstReconcile, err := db.GetGameServer(ctx, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFirstReconcile.Status.Phase != domain.PhaseReconciling {
		t.Fatalf("expected reconciling phase before observation, got %s", afterFirstReconcile.Status.Phase)
	}

	// 模拟边缘 Agent 容器就绪后上报 WorkloadObservation
	obs := domain.WorkloadObservation{
		AssignmentUID:      assignment.UID,
		ServerID:           server.ID,
		NodeID:             server.NodeID,
		ObservedGeneration: 1,
		RuntimeID:          "docker-container-abc",
		ActualState:        domain.ActualRunning,
	}
	if err := db.UpsertWorkloadObservation(ctx, &obs); err != nil {
		t.Fatal(err)
	}

	// 第 2 轮调谐：依据 Observation 收敛至终态
	controller.RunOnce(ctx)

	converged, err := db.GetGameServer(ctx, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if converged.Status.Phase != domain.PhaseRunning {
		t.Fatalf("expected running phase, got %s", converged.Status.Phase)
	}
	if converged.Status.AppliedGeneration != 1 {
		t.Fatalf("expected applied generation 1, got %d", converged.Status.AppliedGeneration)
	}
	if converged.Status.RuntimeID != "docker-container-abc" {
		t.Fatalf("expected runtime container ID converged, got %s", converged.Status.RuntimeID)
	}
}

func TestDeclarativeCommandTaskDispatch(t *testing.T) {
	router, db, _ := newTestRouter(t)
	setupDeclarativeFixtures(t, db, "org-lifecycle-cmd", "node-edge-cmd")

	server := domain.GameServer{
		ID:             "running-cmd-server",
		Name:           "Running Cmd Server",
		OrganizationID: "org-lifecycle-cmd",
		NodeID:         "node-edge-cmd",
		GameKey:        domain.GameTerraria,
		ProviderKey:    domain.ProviderTerrariaVanilla,
		Spec: domain.ServerSpec{
			Generation:   1,
			DesiredState: domain.DesiredRunning,
		},
		Status: domain.ServerRuntimeStatus{
			Phase:              domain.PhaseRunning,
			ActualState:        domain.ActualRunning,
			ObservedGeneration: 1,
			AppliedGeneration:  1,
			RuntimeID:          "container-running",
		},
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	cmdPayload := []byte(`{"command": "say Hello from Control Plane"}`)
	cmdRec := httptest.NewRecorder()
	cmdReq := httptest.NewRequest(stdhttp.MethodPost, "/api/servers/running-cmd-server/command", bytes.NewReader(cmdPayload))
	cmdReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(cmdRec, cmdReq)

	if cmdRec.Code != stdhttp.StatusOK {
		t.Fatalf("expected command dispatch 200, got %d: %s", cmdRec.Code, cmdRec.Body.String())
	}

	// 验证生成了 NodeTask 供边缘 Agent 拉取
	tasks, err := db.ListPendingNodeTasks(context.Background(), "node-edge-cmd")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 pending node task, got %d", len(tasks))
	}
	if tasks[0].Action != "exec_command" || tasks[0].Payload != "say Hello from Control Plane" {
		t.Fatalf("unexpected node task payload: %+v", tasks[0])
	}
	if tasks[0].ServerID != "running-cmd-server" {
		t.Fatalf("expected task server ID running-cmd-server, got %s", tasks[0].ServerID)
	}
}
