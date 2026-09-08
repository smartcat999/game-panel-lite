package controlclient

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func TestRunEntitlements(t *testing.T) {
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "org", ServerID: "server", RegionID: "east", RevisionID: "revision", SpecGeneration: 1, PlacementEpoch: 1}
	record := entitlements.Record{Policy: entitlements.Policy{OrganizationID: "org", ServerID: "server", CPU: 1, MemoryMB: 128, StartsAtMS: 1, EndsAtMS: 2, Status: "active"}, Version: 1, SourceID: "source", SourceKind: "operator"}
	encoded, _ := json.Marshal(record)
	var response atomic.Value
	response.Store(string(encoded))
	var status atomic.Int64
	status.Store(200)
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/internal/region/entitlements/resolve" || r.URL.RawQuery != "intentVersion=1" || r.Method != "POST" {
			t.Error("wrong request")
		}
		var got instances.RevisionAvailable
		if json.NewDecoder(r.Body).Decode(&got) != nil || got != event {
			t.Error("changed request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/redirected")
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(response.Load().(string)))
	}))
	defer server.Close()
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	client, err := New(Options{Endpoint: server.URL, RegionID: "east", Certificate: server.TLS.Certificates[0], ServerCAs: pool, Timeout: time.Second, MaxResponseBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, tc := range []struct {
		status int64
		want   error
	}{{200, nil}, {404, entitlements.ErrUnavailable}, {403, ErrAccessDenied}, {503, ErrRequestFailed}, {307, ErrRequestFailed}, {200, nil}} {
		status.Store(tc.status)
		before := calls.Load()
		got, err := client.GetRunEntitlement(context.Background(), event, 1)
		if !errors.Is(err, tc.want) || (err == nil && got != record) || (err != nil && got.Version != 0) {
			t.Fatalf("response %+v %v", got, err)
		}
		if calls.Load() != before+1 {
			t.Fatal("cached response or redirect followed")
		}
	}
	for _, body := range []string{`{}`, string(encoded) + ` {}`, `{"version":1}`, string(make([]byte, 1025))} {
		response.Store(body)
		got, err := client.GetRunEntitlement(context.Background(), event, 1)
		if !errors.Is(err, ErrInvalidResponse) || got.Version != 0 {
			t.Fatalf("invalid response: %+v %v", got, err)
		}
	}
	before := calls.Load()
	if _, err := client.GetRunEntitlement(context.Background(), event, 0); !errors.Is(err, entitlements.ErrInvalid) || before != calls.Load() {
		t.Fatal("invalid request sent")
	}
}
