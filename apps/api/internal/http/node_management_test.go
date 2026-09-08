package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/scheduler"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestNodeManagementPreservesAgentObservation(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	node := domain.ComputeNode{ID: "managed-node", Token: "managed-token", Name: "old", Region: "old-region", Status: "online", LastHeartbeat: time.Now().UTC().Add(-time.Minute), PingLatencyMS: 73, WorkloadCapabilities: []string{workload.ArtifactCapability}}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	h := &Handler{store: db}
	router := chi.NewRouter()
	router.Post("/nodes/{id}/ping", h.pingNode)
	router.Patch("/nodes/{id}", h.updateNode)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/nodes/managed-node/ping", nil))
	var response pingNodeResponse
	if w.Code != 200 {
		t.Fatalf("ping: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "offline" || response.LatencyMS != 73 {
		t.Fatalf("invented observation: %+v", response)
	}
	if ready, err := db.RemoteArtifactsAvailable(ctx, node.ID); err != nil || ready {
		t.Fatalf("ping enabled offline node: %v", err)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/nodes/managed-node", strings.NewReader(`{"name":" new ","region":"","token":"attacker","status":"online"}`)))
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	current, err := db.GetComputeNodeByToken(ctx, node.Token)
	if err != nil {
		t.Fatal(err)
	}
	if current.Name != "new" || current.Region != "" || !current.LastHeartbeat.Equal(node.LastHeartbeat) || current.PingLatencyMS != 73 || len(current.WorkloadCapabilities) != 1 {
		t.Fatalf("observation changed: %+v", current)
	}
	if err := db.DeleteComputeNode(ctx, node.ID); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/nodes/managed-node", strings.NewReader(`{"name":"new"}`)))
	if w.Code != 404 {
		t.Fatalf("deleted update: %d %s", w.Code, w.Body.String())
	}
}

func TestNodeCordonUncordonAndDrain(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "drain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC()
	nodeA := domain.ComputeNode{
		ID:            "node-a",
		Token:         "token-a",
		Name:          "Node Alpha",
		Status:        "online",
		LastHeartbeat: now,
		CPUCores:      4,
		MemoryTotalMB: 8192,
		MemoryUsedMB:  1024,
	}
	nodeB := domain.ComputeNode{
		ID:            "node-b",
		Token:         "token-b",
		Name:          "Node Beta",
		Status:        "online",
		LastHeartbeat: now,
		CPUCores:      4,
		MemoryTotalMB: 8192,
		MemoryUsedMB:  1024,
	}
	if err := db.CreateComputeNode(ctx, &nodeA); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateComputeNode(ctx, &nodeB); err != nil {
		t.Fatal(err)
	}

	server := domain.GameServer{
		ID:          "srv-1",
		Name:        "Palworld Server",
		GameKey:     domain.GameKey("palworld"),
		ProviderKey: domain.ProviderPalworld,
		NodeID:      "node-a",
		Spec: domain.ServerSpec{
			Resources: domain.ServerResources{
				CPULimitCores: 1,
				MemoryLimitMB: 2048,
			},
		},
		Status: domain.ServerRuntimeStatus{
			Phase: domain.PhaseRunning,
		},
	}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}

	registry, err := provider.NewRegistry(palworld.NewProvider())
	if err != nil {
		t.Fatal(err)
	}
	sched := scheduler.New(db, registry)
	h := &Handler{
		store:     db,
		scheduler: sched,
	}

	router := chi.NewRouter()
	router.Post("/nodes/{id}/cordon", h.cordonNode)
	router.Post("/nodes/{id}/uncordon", h.uncordonNode)
	router.Post("/nodes/{id}/drain", h.drainNode)
	router.Patch("/nodes/{id}", h.updateNode)

	// 1. Test Cordon Node A
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/nodes/node-a/cordon", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("cordon failed: %d %s", w.Code, w.Body.String())
	}
	var cordoned domain.ComputeNode
	if err := json.Unmarshal(w.Body.Bytes(), &cordoned); err != nil {
		t.Fatal(err)
	}
	if !cordoned.Unschedulable {
		t.Fatalf("expected unschedulable = true after cordon")
	}

	// 2. Test Uncordon Node A
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/nodes/node-a/uncordon", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("uncordon failed: %d %s", w.Code, w.Body.String())
	}
	var uncordoned domain.ComputeNode
	if err := json.Unmarshal(w.Body.Bytes(), &uncordoned); err != nil {
		t.Fatal(err)
	}
	if uncordoned.Unschedulable {
		t.Fatalf("expected unschedulable = false after uncordon")
	}

	// 3. Test updateNode with unschedulable
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, "/nodes/node-a", strings.NewReader(`{"unschedulable":true}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("patch unschedulable failed: %d %s", w.Code, w.Body.String())
	}
	updated, err := db.GetComputeNode(ctx, "node-a")
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Unschedulable {
		t.Fatalf("expected unschedulable = true after patch")
	}

	// 4. Test Drain Node A -> server should migrate to Node B
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/nodes/node-a/drain", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("drain failed: %d %s", w.Code, w.Body.String())
	}
	var drainResp drainNodeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &drainResp); err != nil {
		t.Fatal(err)
	}
	if drainResp.TotalServers != 1 || drainResp.MigratedCount != 1 || drainResp.FailedCount != 0 {
		t.Fatalf("unexpected drain response: %+v", drainResp)
	}
	if len(drainResp.Details) != 1 || drainResp.Details[0].TargetNodeID != "node-b" {
		t.Fatalf("expected migration to node-b, got: %+v", drainResp.Details)
	}

	// Verify server in store is now assigned to node-b
	migratedSrv, err := db.GetGameServer(ctx, "srv-1")
	if err != nil {
		t.Fatal(err)
	}
	if migratedSrv.NodeID != "node-b" {
		t.Fatalf("expected server on node-b, got %s", migratedSrv.NodeID)
	}
	if migratedSrv.Status.Phase != domain.PhasePending {
		t.Fatalf("expected server phase pending after drain migration, got %s", migratedSrv.Status.Phase)
	}
}
