package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestAgentExecutionLeaseEndpoint(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "leases.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, node := range []domain.ComputeNode{{ID: "lease-node", Token: "lease-token"}, {ID: "foreign-node", Token: "foreign-token"}} {
		if err := db.CreateComputeNode(ctx, &node); err != nil {
			t.Fatal(err)
		}
	}
	server := domain.GameServer{ID: "lease-server", NodeID: "lease-node", Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	assignment := domain.WorkloadAssignment{ID: "lease-assignment", UID: "lease-uid", NodeID: server.NodeID, ServerID: server.ID, Generation: 1, DesiredState: domain.DesiredRunning}
	if err := db.PublishWorkloadAssignment(ctx, server, &assignment); err != nil {
		t.Fatal(err)
	}
	h := &Handler{store: db}
	router := chi.NewRouter()
	router.Post("/assignments/{uid}/lease", h.changeAgentLease)
	request := func(token, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/assignments/lease-uid/lease", strings.NewReader(body))
		r.Header.Set("X-Node-Token", token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
		}
		return w
	}
	acquire := `{"action":"acquire","holderId":"process","generation":1}`
	request("", acquire, 401)
	request("foreign-token", acquire, 409)
	request("lease-token", acquire+`{}`, 400)
	request("lease-token", `{"action":"renew","holderId":"process","generation":1}`, 400)
	w := request("lease-token", acquire, 200)
	var grant workload.LeaseGrant
	if err := json.Unmarshal(w.Body.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}
	if grant.ObservationToken == nil || *grant.ObservationToken != "" || grant.AssignmentUID != assignment.UID || grant.ServerID != server.ID || grant.Fence != 1 || grant.ValidForMS != agentExecutionLeaseTTL.Milliseconds() || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("invalid grant: %+v", grant)
	}
	request("lease-token", acquire, 409)
	request("lease-token", `{"action":"renew","holderId":"process","generation":1,"fence":1}`, 200)
	request("lease-token", `{"action":"release","holderId":"other","generation":1,"fence":1}`, 409)
	request("lease-token", `{"action":"release","holderId":"process","generation":1,"fence":1}`, 204)
	request("lease-token", acquire, 200)
	request("lease-token", `{"action":"release","holderId":"process","generation":1,"fence":1}`, 409)
}

func TestReportAgentAssignmentStatusWithArtifacts(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "status_artifacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	node := domain.ComputeNode{ID: "worker-node", Token: "worker-token"}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	server := domain.GameServer{ID: "worker-server", NodeID: "worker-node", Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	assignment := domain.WorkloadAssignment{ID: "worker-assignment", UID: "worker-uid", NodeID: server.NodeID, ServerID: server.ID, Generation: 1, DesiredState: domain.DesiredRunning}
	if err := db.PublishWorkloadAssignment(ctx, server, &assignment); err != nil {
		t.Fatal(err)
	}
	h := &Handler{store: db}
	router := chi.NewRouter()
	router.Post("/assignments/{uid}/lease", h.changeAgentLease)
	router.Post("/assignments/{uid}/status", h.reportAgentAssignmentStatus)

	// Acquire lease
	rAcquire := httptest.NewRequest(http.MethodPost, "/assignments/worker-uid/lease", strings.NewReader(`{"action":"acquire","holderId":"worker-proc","generation":1}`))
	rAcquire.Header.Set("X-Node-Token", "worker-token")
	wAcquire := httptest.NewRecorder()
	router.ServeHTTP(wAcquire, rAcquire)
	if wAcquire.Code != http.StatusOK {
		t.Fatalf("failed to acquire lease: %s", wAcquire.Body.String())
	}
	var grant workload.LeaseGrant
	if err := json.Unmarshal(wAcquire.Body.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}

	// Report status with Artifacts
	report := workload.Observation{
		LeaseHolderID:      "worker-proc",
		LeaseFence:         grant.Fence,
		ObservationToken:   "",
		ObservedGeneration: 1,
		RuntimeID:          "container-1",
		ActualState:        "running",
		Conditions: []workload.Condition{{
			Type:   workload.ConditionArtifactsReady,
			Status: workload.ConditionStatusTrue,
			Reason: "Ready",
		}},
		Artifacts: []workload.ArtifactObservation{{
			ID:     "mod-1",
			Path:   "Mods/mod1.tmod",
			Status: workload.ArtifactStatusReady,
		}},
	}
	body, _ := json.Marshal(report)
	rStatus := httptest.NewRequest(http.MethodPost, "/assignments/worker-uid/status", strings.NewReader(string(body)))
	rStatus.Header.Set("X-Node-Token", "worker-token")
	wStatus := httptest.NewRecorder()
	router.ServeHTTP(wStatus, rStatus)
	if wStatus.Code != http.StatusOK {
		t.Fatalf("status report failed: %s", wStatus.Body.String())
	}

	// Verify persisted observation
	obs, err := db.GetWorkloadObservation(ctx, "worker-uid")
	if err != nil {
		t.Fatalf("failed to get workload observation: %v", err)
	}
	if len(obs.Artifacts) != 1 || obs.Artifacts[0].ID != "mod-1" || obs.Artifacts[0].Status != "ready" {
		t.Fatalf("unexpected persisted artifacts: %+v", obs.Artifacts)
	}
	if len(obs.Conditions) != 1 || obs.Conditions[0].Type != workload.ConditionArtifactsReady || obs.Conditions[0].Status != "True" {
		t.Fatalf("unexpected persisted conditions: %+v", obs.Conditions)
	}
}

func TestRuntimeModPresentRemote(t *testing.T) {
	mod := domain.ModFile{FileName: "example.tmod"}

	// Local server checks disk (missing here -> false)
	localServer := domain.GameServer{
		NodeID:      "",
		ProviderKey: domain.ProviderTerrariaTModLoader,
		Spec:        domain.ServerSpec{Runtime: domain.ServerRuntimeSpec{DataDir: "/tmp/non-existent"}},
	}
	if runtimeModPresent(localServer, mod) {
		t.Fatal("expected local missing file to return false")
	}

	// Remote server with ArtifactsReady: True -> true
	remoteReady := domain.GameServer{
		NodeID:      "remote-node-1",
		ProviderKey: domain.ProviderTerrariaTModLoader,
		Spec:        domain.ServerSpec{Generation: 2, Runtime: domain.ServerRuntimeSpec{DataDir: "/srv/data"}},
		Status: domain.ServerRuntimeStatus{
			Conditions: []domain.ServerCondition{{
				Type:   workload.ConditionArtifactsReady,
				Status: workload.ConditionStatusTrue,
			}},
		},
	}
	if !runtimeModPresent(remoteReady, mod) {
		t.Fatal("expected remote server with ArtifactsReady True to return true")
	}

	// Remote server with ArtifactsReady: False -> false
	remoteFailed := domain.GameServer{
		NodeID:      "remote-node-1",
		ProviderKey: domain.ProviderTerrariaTModLoader,
		Spec:        domain.ServerSpec{Generation: 2, Runtime: domain.ServerRuntimeSpec{DataDir: "/srv/data"}},
		Status: domain.ServerRuntimeStatus{
			Conditions: []domain.ServerCondition{{
				Type:   workload.ConditionArtifactsReady,
				Status: workload.ConditionStatusFalse,
			}},
		},
	}
	if runtimeModPresent(remoteFailed, mod) {
		t.Fatal("expected remote server with ArtifactsReady False to return false")
	}

	// Remote server with no condition: converged running -> true
	remoteConverged := domain.GameServer{
		NodeID:      "remote-node-1",
		ProviderKey: domain.ProviderTerrariaTModLoader,
		Spec:        domain.ServerSpec{Generation: 2, Runtime: domain.ServerRuntimeSpec{DataDir: "/srv/data"}},
		Status: domain.ServerRuntimeStatus{
			ActualState:       domain.ActualRunning,
			AppliedGeneration: 2,
		},
	}
	if !runtimeModPresent(remoteConverged, mod) {
		t.Fatal("expected remote converged server to return true")
	}

	// Remote server with no condition: reconciling -> false
	remoteReconciling := domain.GameServer{
		NodeID:      "remote-node-1",
		ProviderKey: domain.ProviderTerrariaTModLoader,
		Spec:        domain.ServerSpec{Generation: 2, Runtime: domain.ServerRuntimeSpec{DataDir: "/srv/data"}},
		Status: domain.ServerRuntimeStatus{
			ActualState:       domain.ActualUnknown,
			AppliedGeneration: 1,
		},
	}
	if runtimeModPresent(remoteReconciling, mod) {
		t.Fatal("expected remote reconciling server to return false")
	}
}
