package http

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestCreateServer_UserExplicitNodeSelection(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_ = cfg
	now := time.Now().UTC()

	// Seed two compute nodes: node-good (online) and node-offline (offline)
	goodNode := domain.ComputeNode{
		ID:            "node-good",
		Name:          "Good Node",
		Status:        "online",
		Host:          "192.168.1.10",
		MemoryTotalMB: 8192,
		CPUCores:      8,
		LastHeartbeat: now,
	}
	offlineNode := domain.ComputeNode{
		ID:            "node-offline",
		Name:          "Offline Node",
		Status:        "offline",
		Host:          "192.168.1.11",
		MemoryTotalMB: 8192,
		CPUCores:      8,
		LastHeartbeat: now.Add(-2 * time.Minute),
	}
	if err := db.CreateComputeNode(context.Background(), &goodNode); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateComputeNode(context.Background(), &offlineNode); err != nil {
		t.Fatal(err)
	}

	// 1. Explicitly choosing offline node should be rejected with 400 Bad Request
	reqBody, _ := json.Marshal(map[string]any{
		"name":        "Test Reject Server",
		"resources":   map[string]any{"cpuLimitCores": 1, "memoryLimitMB": 512},
		"providerKey": domain.ProviderTerrariaVanilla,
		"nodeId":      "node-offline",
		"hostPort":    7778,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewReader(reqBody)))
	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected 400 when user explicitly picks offline node, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Explicitly choosing good node should succeed and bind directly to node-good
	reqGoodBody, _ := json.Marshal(map[string]any{
		"name":        "Test Good Server",
		"resources":   map[string]any{"cpuLimitCores": 1, "memoryLimitMB": 512},
		"providerKey": domain.ProviderTerrariaVanilla,
		"nodeId":      "node-good",
		"hostPort":    7779,
	})
	recGood := httptest.NewRecorder()
	router.ServeHTTP(recGood, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewReader(reqGoodBody)))
	if recGood.Code != stdhttp.StatusCreated {
		t.Fatalf("expected 201 when user explicitly picks healthy node, got %d: %s", recGood.Code, recGood.Body.String())
	}
	var created domain.GameServer
	if err := json.Unmarshal(recGood.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.NodeID != "node-good" {
		t.Fatalf("expected server to be bound to user-selected node-good, got %s", created.NodeID)
	}
}

func TestCreateServer_AutoScheduleFallback_WhenNodeOmitted(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_ = cfg
	now := time.Now().UTC()

	// Seed node-1 (already has a server) and node-2 (fresh, 0 servers)
	node1 := domain.ComputeNode{
		ID:            "node-1",
		Name:          "Node One",
		Status:        "online",
		Host:          "192.168.1.20",
		MemoryTotalMB: 8192,
		CPUCores:      8,
		LastHeartbeat: now,
	}
	node2 := domain.ComputeNode{
		ID:            "node-2",
		Name:          "Node Two",
		Status:        "online",
		Host:          "192.168.1.21",
		MemoryTotalMB: 8192,
		CPUCores:      8,
		LastHeartbeat: now,
	}
	if err := db.CreateComputeNode(context.Background(), &node1); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateComputeNode(context.Background(), &node2); err != nil {
		t.Fatal(err)
	}

	// Place an existing server on node-1 with port 7780
	existingServer := domain.GameServer{
		ID:          "existing-srv-1",
		NodeID:      "node-1",
		Name:        "Existing",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec: domain.ServerSpec{
			Network:   domain.ServerNetworkSpec{HostPort: 7780},
			Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512},
		},
		Status: domain.ServerRuntimeStatus{Phase: domain.PhaseRunning},
	}
	if err := db.CreateGameServer(context.Background(), &existingServer); err != nil {
		t.Fatal(err)
	}

	// Auto-schedule server (nodeId omitted / empty): should select node-2 due to port collision or least load
	reqBody, _ := json.Marshal(map[string]any{
		"name":        "Auto Scheduled Server",
		"resources":   map[string]any{"cpuLimitCores": 1, "memoryLimitMB": 512},
		"providerKey": domain.ProviderTerrariaVanilla,
		"nodeId":      "",
		"hostPort":    7780,
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewReader(reqBody)))
	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("expected 201 for auto-schedule, got %d: %s", rec.Code, rec.Body.String())
	}
	var created domain.GameServer
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.NodeID != "node-2" {
		t.Fatalf("expected auto-scheduler to pick node-2, got %s", created.NodeID)
	}
}

func TestMigrateServer_Endpoint(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_ = cfg
	now := time.Now().UTC()

	nodeA := domain.ComputeNode{
		ID:            "node-a",
		Name:          "Node A",
		Status:        "online",
		Host:          "192.168.1.30",
		MemoryTotalMB: 8192,
		CPUCores:      8,
		LastHeartbeat: now,
	}
	nodeB := domain.ComputeNode{
		ID:            "node-b",
		Name:          "Node B",
		Status:        "online",
		Host:          "192.168.1.31",
		MemoryTotalMB: 8192,
		CPUCores:      8,
		LastHeartbeat: now,
	}
	if err := db.CreateComputeNode(context.Background(), &nodeA); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateComputeNode(context.Background(), &nodeB); err != nil {
		t.Fatal(err)
	}

	server := domain.GameServer{
		ID:          "server-to-migrate",
		NodeID:      "node-a",
		Name:        "Migratable Server",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec: domain.ServerSpec{
			Generation: 1,
			Resources:  domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512},
			Network:    domain.ServerNetworkSpec{HostPort: 7790},
		},
		Status: domain.ServerRuntimeStatus{
			Phase: domain.PhaseStopped,
		},
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}

	// 1. Migrate to same node should fail with 400
	reqSame, _ := json.Marshal(map[string]any{"targetNodeId": "node-a"})
	recSame := httptest.NewRecorder()
	router.ServeHTTP(recSame, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/server-to-migrate/migrate", bytes.NewReader(reqSame)))
	if recSame.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected 400 migrating to same node, got %d: %s", recSame.Code, recSame.Body.String())
	}

	// 2. Migrate to node-b should succeed
	reqTarget, _ := json.Marshal(map[string]any{"targetNodeId": "node-b"})
	recTarget := httptest.NewRecorder()
	router.ServeHTTP(recTarget, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/server-to-migrate/migrate", bytes.NewReader(reqTarget)))
	if recTarget.Code != stdhttp.StatusOK {
		t.Fatalf("expected 200 migrating to node-b, got %d: %s", recTarget.Code, recTarget.Body.String())
	}

	var migrated domain.GameServer
	if err := json.Unmarshal(recTarget.Body.Bytes(), &migrated); err != nil {
		t.Fatal(err)
	}
	if migrated.NodeID != "node-b" {
		t.Fatalf("expected migrated.NodeID to be node-b, got %s", migrated.NodeID)
	}
	if migrated.Spec.Generation != 2 {
		t.Fatalf("expected generation to increment to 2, got %d", migrated.Spec.Generation)
	}
}
