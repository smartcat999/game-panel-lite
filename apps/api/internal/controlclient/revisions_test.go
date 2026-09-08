package controlclient

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

func TestRevisionResponses(t *testing.T) {
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "org", ServerID: "server", RevisionID: "revision", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	valid := regional.RevisionSnapshot{Event: event, CurrentSpecGeneration: 2, DesiredState: "stopped", IntentVersion: 3, Revision: instances.Revision{ID: "revision", ServerID: "server", SpecGeneration: 1, Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 256}, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("opaque")}}}}
	encoded, _ := json.Marshal(valid)
	for _, test := range []struct {
		name     string
		status   int
		body     string
		expected error
	}{
		{"historical stopped", 200, string(encoded), nil},
		{"wrong revision", 200, strings.Replace(string(encoded), `"id":"revision"`, `"id":"different"`, 1), ErrInvalidResponse},
		{"stale generation", 200, strings.Replace(string(encoded), `"currentSpecGeneration":2`, `"currentSpecGeneration":0`, 1), ErrInvalidResponse},
		{"invalid state", 200, strings.Replace(string(encoded), `"stopped"`, `"deleted"`, 1), ErrInvalidResponse},
		{"trailing JSON", 200, string(encoded) + "{}", ErrInvalidResponse},
		{"oversized", 200, string(encoded) + strings.Repeat(" ", 4096), ErrInvalidResponse},
		{"unavailable", 404, "private details", regional.ErrRevisionUnavailable},
		{"forbidden", 403, "private details", ErrAccessDenied},
		{"retry", 503, "private details", ErrRequestFailed},
		{"redirect", 307, "private details", ErrRequestFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/internal/region/revisions/resolve" || r.Method != http.MethodPost {
					t.Error("wrong request route")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			pool := x509.NewCertPool()
			pool.AddCert(server.Certificate())
			client, err := New(Options{Endpoint: server.URL, RegionID: "east", Certificate: server.TLS.Certificates[0], ServerCAs: pool, Timeout: time.Second, MaxResponseBytes: 4096})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			got, err := client.GetRevision(context.Background(), event)
			if !errors.Is(err, test.expected) {
				t.Fatalf("got %v wanted %v", err, test.expected)
			}
			if err == nil && (got.DesiredState != "stopped" || got.CurrentSpecGeneration != 2) {
				t.Fatal("lost current intent")
			}
			if err != nil && got.Event.EventID != "" {
				t.Fatal("partial snapshot returned")
			}
		})
	}
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	options := Options{Endpoint: server.URL, RegionID: "east", Certificate: server.TLS.Certificates[0], ServerCAs: pool, Timeout: 50 * time.Millisecond, MaxResponseBytes: 4096}
	client, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	started := time.Now()
	if _, err := client.GetRevision(context.Background(), event); !errors.Is(err, ErrRequestFailed) || time.Since(started) > time.Second {
		t.Fatalf("timeout not bounded: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.GetRevision(ctx, event); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	options.Endpoint = "http://control.example"
	if _, err := New(options); err == nil {
		t.Fatal("plaintext endpoint accepted")
	}
}
