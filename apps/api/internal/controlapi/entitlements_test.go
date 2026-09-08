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

	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
)

type entitlementReaderFunc func(context.Context, string, instances.RevisionAvailable, int64) (entitlements.Record, error)

func (f entitlementReaderFunc) RegionalRunEntitlement(ctx context.Context, region string, e instances.RevisionAvailable, intent int64) (entitlements.Record, error) {
	return f(ctx, region, e, intent)
}

func testEntitlementAPI(t *testing.T, auth *serviceauth.Regions, serverTLS *tls.Config, pool *x509.CertPool, east, west tls.Certificate) {
	t.Helper()
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "org", ServerID: "server", RegionID: "east", RevisionID: "revision", SpecGeneration: 1, PlacementEpoch: 1}
	h, err := NewEntitlementHandler(entitlementReaderFunc(func(_ context.Context, region string, e instances.RevisionAvailable, intent int64) (entitlements.Record, error) {
		if region != "east" || intent != 1 {
			t.Error("wrong authenticated identity")
		}
		if e.EventID == "failure" {
			return entitlements.Record{}, errors.New("private database detail")
		}
		if e != event {
			return entitlements.Record{}, entitlements.ErrUnavailable
		}
		return entitlements.Record{Version: 1}, nil
	}), auth, 1024)
	if err != nil {
		t.Fatal(err)
	}
	plain := httptest.NewRequest("POST", "/internal/region/entitlements/resolve", strings.NewReader("{}"))
	plain.Header.Set("X-Region-ID", "east")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, plain)
	if w.Code != 401 {
		t.Fatal("spoofed identity accepted")
	}
	server := httptest.NewUnstartedServer(h)
	server.TLS = serverTLS.Clone()
	server.StartTLS()
	defer server.Close()
	body, _ := json.Marshal(event)
	raw := string(body)
	for _, tc := range []struct {
		body, query, media string
		cert               tls.Certificate
		status             int
	}{
		{raw, "?intentVersion=1", "application/json", east, 200},
		{raw, "?intentVersion=1", "application/json", west, 403},
		{raw, "?region=east", "application/json", east, 400},
		{raw, "?intentVersion=1", "text/plain", east, 415},
		{raw + ` {}`, "?intentVersion=1", "application/json", east, 400},
		{strings.TrimSuffix(raw, "}") + `,"RegionId":"east"}`, "?intentVersion=1", "application/json", east, 400},
		{strings.Repeat("x", 1025), "?intentVersion=1", "application/json", east, 413},
		{strings.Replace(raw, `"eventId":"event"`, `"eventId":"old"`, 1), "?intentVersion=1", "application/json", east, 404},
		{strings.Replace(raw, `"eventId":"event"`, `"eventId":"failure"`, 1), "?intentVersion=1", "application/json", east, 503},
	} {
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{tc.cert}}}
		client := &http.Client{Transport: transport, Timeout: time.Second}
		req, _ := http.NewRequest("POST", server.URL+"/internal/region/entitlements/resolve"+tc.query, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.media)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		transport.CloseIdleConnections()
		if response.StatusCode != tc.status || response.Header.Get("Cache-Control") != "no-store" || strings.Contains(string(data), "private database detail") {
			t.Fatalf("response %d want %d", response.StatusCode, tc.status)
		}
	}
}
