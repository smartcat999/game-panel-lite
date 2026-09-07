package http

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"reflect"
)

func TestAgentAssignmentsRequireOwningNodeToken(t *testing.T) {
	router, db, _ := newTestRouter(t)
	now := time.Now().UTC()
	for _, node := range []domain.ComputeNode{
		{ID: "node-a", Name: "A", Token: "token-a", Status: "online", LastHeartbeat: now, CreatedAt: now, UpdatedAt: now},
		{ID: "node-b", Name: "B", Token: "token-b", Status: "online", LastHeartbeat: now, CreatedAt: now, UpdatedAt: now},
	} {
		copy := node
		if err := db.CreateComputeNode(context.Background(), &copy); err != nil {
			t.Fatalf("create node: %v", err)
		}
	}
	assignment := domain.WorkloadAssignment{
		ID: "assignment-1", UID: "uid-1", ServerID: "server-1", NodeID: "node-a",
		Generation: 1, DesiredState: domain.DesiredRunning,
		Spec:      domain.WorkloadSpec{ServerID: "server-1", Image: "game:1", Resources: domain.WorkloadResources{CPULimitCores: 1.5, MemoryLimitMB: 2048}, Network: domain.WorkloadNetwork{Port: 7777, HostPort: 47777, AdditionalPorts: []domain.WorkloadPort{{Port: 8888, HostPort: 48888, Protocol: "udp"}}}},
		CreatedAt: now, UpdatedAt: now,
	}
	instance := domain.GameServer{ID: assignment.ServerID, NodeID: assignment.NodeID, Spec: domain.ServerSpec{Generation: assignment.Generation, DesiredState: assignment.DesiredState}}
	if err := db.CreateGameServer(context.Background(), &instance); err != nil {
		t.Fatal(err)
	}
	if err := db.PublishWorkloadAssignment(context.Background(), instance, &assignment); err != nil {
		t.Fatalf("create assignment: %v", err)
	}

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(stdhttp.MethodGet, "/api/agent/assignments", nil))
	if unauthorized.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("expected missing token 401, got %d", unauthorized.Code)
	}

	listRequest := httptest.NewRequest(stdhttp.MethodGet, "/api/agent/assignments", nil)
	listRequest.Header.Set("X-Node-Token", "token-a")
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != stdhttp.StatusOK {
		t.Fatalf("expected assignment list 200, got %d: %s", listResponse.Code, listResponse.Body.String())
	}
	var assignments []workload.Assignment
	if err := json.Unmarshal(listResponse.Body.Bytes(), &assignments); err != nil || len(assignments) != 1 || assignments[0].UID != assignment.UID {
		t.Fatalf("unexpected assignments: %v %+v", err, assignments)
	}

	if !reflect.DeepEqual(assignments[0].Spec, assignment.Spec) {
		t.Fatalf("worker protocol dropped fields: %+v", assignments[0].Spec)
	}
	statusBody := []byte(`{"observedGeneration":1,"actualState":"running","reconcileDurationSeconds":0.25}`)
	forbiddenRequest := httptest.NewRequest(stdhttp.MethodPost, "/api/agent/assignments/uid-1/status", bytes.NewReader(statusBody))
	forbiddenRequest.Header.Set("X-Node-Token", "token-b")
	forbiddenResponse := httptest.NewRecorder()
	router.ServeHTTP(forbiddenResponse, forbiddenRequest)
	if forbiddenResponse.Code != stdhttp.StatusForbidden {
		t.Fatalf("expected other node status report 403, got %d", forbiddenResponse.Code)
	}

	statusRequest := httptest.NewRequest(stdhttp.MethodPost, "/api/agent/assignments/uid-1/status", bytes.NewReader(statusBody))
	statusRequest.Header.Set("X-Node-Token", "token-a")
	statusResponse := httptest.NewRecorder()
	router.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != stdhttp.StatusOK {
		t.Fatalf("expected owner status report 200, got %d: %s", statusResponse.Code, statusResponse.Body.String())
	}
	observation, err := db.GetWorkloadObservation(context.Background(), assignment.UID)
	if err != nil || observation.NodeID != "node-a" || observation.ActualState != domain.ActualRunning {
		t.Fatalf("unexpected stored observation: %v %+v", err, observation)
	}

	metricsResponse := httptest.NewRecorder()
	router.ServeHTTP(metricsResponse, httptest.NewRequest(stdhttp.MethodGet, "/metrics", nil))
	metricsBody := metricsResponse.Body.String()
	for _, expected := range []string{
		`gamepanel_worker_reconcile_backlog{node_id="node-a"} 1`,
		`gamepanel_worker_reconcile_duration_seconds_count{node_id="node-a"} 1`,
		`gamepanel_worker_generation_lag{node_id="node-a",server_id="server-1"} 0`,
	} {
		if !strings.Contains(metricsBody, expected) {
			t.Fatalf("expected agent metric %q, got:\n%s", expected, metricsBody)
		}
	}
	refreshReq := httptest.NewRequest(stdhttp.MethodGet, "/api/agent/assignments", nil)
	refreshReq.Header.Set("X-Node-Token", "token-a")
	refreshed := httptest.NewRecorder()
	router.ServeHTTP(refreshed, refreshReq)
	if err := json.Unmarshal(refreshed.Body.Bytes(), &assignments); err != nil || len(assignments) != 1 || assignments[0].ObservationToken != observation.ID {
		t.Fatalf("missing observation token: %s", refreshed.Body.String())
	}
	body, err := json.Marshal(workload.Observation{ObservationToken: assignments[0].ObservationToken, ObservedGeneration: 1, ActualState: "stopped"})
	if err != nil {
		t.Fatal(err)
	}
	for i, expected := range []int{stdhttp.StatusOK, stdhttp.StatusConflict} {
		req := httptest.NewRequest(stdhttp.MethodPost, "/api/agent/assignments/uid-1/status", bytes.NewReader(body))
		req.Header.Set("X-Node-Token", "token-a")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != expected {
			t.Fatalf("report attempt %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}

}

func TestAgentDockerCommandMountsWorkerDataDirectory(t *testing.T) {
	command := agentDockerCommand("https://panel.example.com", "node-token")
	if !strings.Contains(command, "-v /var/lib/gamepanel:/var/lib/gamepanel") {
		t.Fatalf("expected worker data mount in agent command, got %q", command)
	}
}

func TestAgentArtifactCapabilityNegotiation(t *testing.T) {
	router, db, _ := newTestRouter(t)
	node := domain.ComputeNode{ID: "artifact-node", Token: "artifact-token"}
	if err := db.CreateComputeNode(context.Background(), &node); err != nil {
		t.Fatal(err)
	}
	instance := domain.GameServer{ID: "artifact-server", NodeID: node.ID, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
	if err := db.CreateGameServer(context.Background(), &instance); err != nil {
		t.Fatal(err)
	}
	assignment := domain.WorkloadAssignment{ID: "artifact-assignment", UID: "artifact-uid", ServerID: instance.ID, NodeID: node.ID, Generation: 1, DesiredState: domain.DesiredRunning, Spec: workload.Spec{Options: workload.Options{Artifacts: []workload.Artifact{{ID: "source", Path: "Mods/mod.bin", SizeBytes: 1, SHA256: strings.Repeat("a", 64)}}}}}
	if err := db.PublishWorkloadAssignment(context.Background(), instance, &assignment); err != nil {
		t.Fatal(err)
	}
	for _, capability := range []string{"", "other-feature", "artifacts-v10", "other-feature, artifacts-v1"} {
		request := httptest.NewRequest(stdhttp.MethodGet, "/api/agent/assignments", nil)
		request.Header.Set("X-Node-Token", node.Token)
		request.Header.Set("X-Workload-Capabilities", capability)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if capability == "other-feature, artifacts-v1" {
			var items []workload.Assignment
			if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &items) != nil || len(items) != 1 || len(items[0].Spec.Options.Artifacts) != 1 {
				t.Fatalf("capable poll: %d %s", response.Code, response.Body.String())
			}
		} else if response.Code != 409 {
			t.Fatalf("legacy poll %q: %d", capability, response.Code)
		}
	}
}
