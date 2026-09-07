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
	if grant.AssignmentUID != assignment.UID || grant.ServerID != server.ID || grant.Fence != 1 || grant.ValidForMS != agentExecutionLeaseTTL.Milliseconds() || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("invalid grant: %+v", grant)
	}
	request("lease-token", acquire, 409)
	request("lease-token", `{"action":"renew","holderId":"process","generation":1,"fence":1}`, 200)
	request("lease-token", `{"action":"release","holderId":"other","generation":1,"fence":1}`, 409)
	request("lease-token", `{"action":"release","holderId":"process","generation":1,"fence":1}`, 204)
	request("lease-token", acquire, 200)
	request("lease-token", `{"action":"release","holderId":"process","generation":1,"fence":1}`, 409)
}
