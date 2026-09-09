package controlapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/nodeapi"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type nodeStateFixture struct{ t *testing.T }

type nodeExecutionFixture struct {
	t                                 *testing.T
	acquired, renewed, seen, released bool
}

func (f *nodeExecutionFixture) Acquire(_ context.Context, node string, epoch int64, holder string) (*workload.AuthorizedAssignment, error) {
	if node != "node-a" || epoch != 1 || holder != "process-a" {
		f.t.Error("invalid acquire identity")
	}
	f.acquired = true
	token := ""
	assignment := workload.Assignment{ID: "task-a", UID: "task-a", ServerID: "server-a", NodeID: node, Generation: 1, DesiredState: "running", Spec: workload.Spec{ServerID: "server-a", Name: "server-a", Image: "registry.example/game@sha256:fixture", Network: workload.Network{Port: 7777, HostPort: 30001}}}
	lease := workload.LeaseGrant{ObservationToken: &token, AssignmentUID: assignment.UID, ServerID: assignment.ServerID, NodeID: node, Generation: 1, HolderID: holder, Fence: 4, ValidForMS: 120000}
	return &workload.AuthorizedAssignment{Assignment: assignment, Lease: lease}, nil
}
func (f *nodeExecutionFixture) Renew(_ context.Context, node, task string, epoch int64, holder string, fence int64) (workload.LeaseGrant, error) {
	if node != "node-a" || task != "task-a" || epoch != 1 || holder != "process-a" || fence != 4 {
		f.t.Error("invalid renewal identity")
	}
	f.renewed = true
	return workload.LeaseGrant{AssignmentUID: task, ServerID: "server-a", NodeID: node, Generation: 1, HolderID: holder, Fence: fence, ValidForMS: 120000}, nil
}
func (f *nodeExecutionFixture) Release(_ context.Context, node, task string, epoch int64, holder string, fence int64) error {
	if node != "node-a" || task != "task-a" || epoch != 1 || holder != "process-a" || fence != 4 {
		f.t.Error("invalid release identity")
	}
	f.released = true
	return nil
}
func (f *nodeExecutionFixture) Observe(_ context.Context, node, task string, epoch int64, observation workload.Observation) error {
	if node != "node-a" || task != "task-a" || epoch != 1 || observation.LeaseHolderID != "process-a" || observation.LeaseFence != 4 {
		f.t.Error("invalid observation identity")
	}
	f.seen = true
	return nil
}

func (s nodeStateFixture) RegionID() string { return "east" }
func (s nodeStateFixture) StartRegionalNodeSession(_ context.Context, node string) (regional.NodeSession, error) {
	if node != "node-a" {
		s.t.Error("unverified session node")
	}
	return regional.NodeSession{Epoch: 1}, nil
}
func (s nodeStateFixture) RecordRegionalNodeHeartbeat(_ context.Context, node string, h regional.NodeHeartbeat) error {
	if node != "node-a" {
		s.t.Error("unverified heartbeat node")
	}
	if h.Sequence == 99 {
		return errors.New("private database details")
	}
	if h.SessionEpoch != 1 {
		return regional.ErrNodeHeartbeatStale
	}
	return nil
}

// Reuse this package's real certificate fixture; the Node API has its own port
// and allowlist. Persistence and ordering are exercised with PostgreSQL in Store.
func testNodeAPI(t *testing.T, serverTLS *tls.Config, pool *x509.CertPool, known, unknown tls.Certificate) {
	mapping := map[string]string{"spiffe://test/region/east": "node-a"}
	auth, err := serviceauth.NewNodes("east", mapping)
	if err != nil {
		t.Fatal(err)
	}
	mapping["spiffe://test/region/east"] = "forged"
	execution := &nodeExecutionFixture{t: t}
	handler, err := nodeapi.NewExecutionHandler(nodeStateFixture{t}, execution, auth, 512)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := serviceauth.NewNodes("west", mapping)
	if _, err := nodeapi.NewHandler(nodeStateFixture{t}, other, 512); err == nil {
		t.Fatal("wrong regional binding accepted")
	}
	plain := httptest.NewRequest("POST", "/internal/node/session", nil)
	plain.Header.Set("X-Node-ID", "node-a")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, plain)
	if w.Code != 401 {
		t.Fatal("header spoof accepted")
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = serverTLS.Clone()
	server.StartTLS()
	defer server.Close()
	valid := `{"sessionEpoch":1,"sequence":1,"architecture":"amd64","runtimeReady":true}`
	for _, tc := range []struct {
		path, body, media string
		cert              tls.Certificate
		status            int
	}{
		{"session", "", "", known, 200}, {"session", "{}", "application/json", known, 400}, {"session", "", "", unknown, 401},
		{"heartbeat", valid, "application/json", known, 204}, {"heartbeat?node=other", valid, "application/json", known, 400},
		{"heartbeat", strings.Replace(valid, `"sessionEpoch":1`, `"sessionEpoch":2`, 1), "application/json", known, 409},
		{"heartbeat", strings.Replace(valid, `"sequence":1`, `"sequence":99`, 1), "application/json", known, 503},
		{"heartbeat", strings.TrimSuffix(valid, "}") + `,"nodeId":"other"}`, "application/json", known, 400},
		{"heartbeat", strings.TrimSuffix(valid, "}") + `,"sequence":2}`, "application/json", known, 400},
		{"heartbeat", strings.Replace(valid, `"runtimeReady":true`, `"runtimeReady":null`, 1), "application/json", known, 400},
		{"heartbeat", valid + ` {}`, "application/json", known, 400}, {"heartbeat", valid, "text/plain", known, 415},
		{"heartbeat", strings.Repeat(" ", 513) + valid, "application/json", known, 413},
	} {
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{tc.cert}}}
		client := &http.Client{Transport: transport, Timeout: time.Second}
		req, _ := http.NewRequest("POST", server.URL+"/internal/node/"+tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.media)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		transport.CloseIdleConnections()
		if resp.StatusCode != tc.status || resp.Header.Get("Cache-Control") != "no-store" || strings.Contains(string(body), "private database") {
			t.Fatalf("%s: got %d want %d", tc.path, resp.StatusCode, tc.status)
		}
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{known}}}
	client := &http.Client{Transport: transport, Timeout: time.Second}
	defer transport.CloseIdleConnections()
	post := func(path, body string, want int) []byte {
		t.Helper()
		req, _ := http.NewRequest("POST", server.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		encoded, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: got %d want %d", path, resp.StatusCode, want)
		}
		return encoded
	}
	claim := post("/internal/node/assignments/claim", `{"sessionEpoch":1,"holderId":"process-a"}`, 200)
	var granted workload.AuthorizedAssignment
	if json.Unmarshal(claim, &granted) != nil || granted.Assignment.UID != "task-a" || granted.Lease.Fence != 4 {
		t.Fatal("invalid assignment response")
	}
	post("/internal/node/assignments/task-a/lease", `{"sessionEpoch":1,"action":"renew","holderId":"process-a","fence":4}`, 200)
	post("/internal/node/assignments/task-a/observation", `{"sessionEpoch":1,"observation":{"leaseHolderId":"process-a","leaseFence":4,"observedGeneration":1,"actualState":"running","reconcileDurationSeconds":0,"observedAt":"2026-09-10T00:00:00Z","observationToken":""}}`, 204)
	post("/internal/node/assignments/task-a/lease", `{"sessionEpoch":1,"action":"release","holderId":"process-a","fence":4}`, 204)
	post("/internal/node/assignments/claim", `{"sessionEpoch":1,"holderId":"process-a","unknown":true}`, 400)
	if !execution.acquired || !execution.renewed || !execution.seen || !execution.released {
		t.Fatal("execution route did not reach its module")
	}
}
