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
