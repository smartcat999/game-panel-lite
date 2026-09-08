package controlapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
)

type nodeStateFixture struct{ t *testing.T }

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
	handler, err := nodeapi.NewHandler(nodeStateFixture{t}, auth, 512)
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
}
