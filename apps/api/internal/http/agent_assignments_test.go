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
		Spec:      domain.WorkloadSpec{ServerID: "server-1", Image: "game:1"},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.UpsertWorkloadAssignment(context.Background(), &assignment); err != nil {
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
	var assignments []domain.WorkloadAssignment
	if err := json.Unmarshal(listResponse.Body.Bytes(), &assignments); err != nil || len(assignments) != 1 || assignments[0].UID != assignment.UID {
		t.Fatalf("unexpected assignments: %v %+v", err, assignments)
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
}
