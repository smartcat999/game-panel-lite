package controlclient

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

func TestAssetResponses(t *testing.T) {
	e := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "org", ServerID: "server", RevisionID: "revision", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	v := assets.PublishedVersion{AssetID: "world", OrganizationID: "org", Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 1}
	encoded, _ := json.Marshal(v)
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"valid", string(encoded), 200, nil},
		{"tenant", strings.Replace(string(encoded), `"org"`, `"foreign"`, 1), 200, ErrInvalidResponse},
		{"version", strings.Replace(string(encoded), `"v1"`, `"v2"`, 1), 200, ErrInvalidResponse},
		{"asset", strings.Replace(string(encoded), `"world"`, `"other"`, 1), 200, ErrInvalidResponse},
		{"trailing", string(encoded) + "{}", 200, ErrInvalidResponse},
		{"oversize", string(encoded) + strings.Repeat(" ", 1024), 200, ErrInvalidResponse},
		{"missing", "private details", 404, regional.ErrRevisionUnavailable},
		{"denied", "private details", 403, ErrAccessDenied},
		{"redirect", "private details", 307, ErrRequestFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/internal/region/assets/resolve" || r.Method != "POST" || r.URL.Query().Get("assetId") != "world" || r.URL.Query().Get("version") != "v1" {
					t.Error("invalid asset route")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			pool := x509.NewCertPool()
			pool.AddCert(server.Certificate())
			client, err := New(Options{Endpoint: server.URL, RegionID: "east", Certificate: server.TLS.Certificates[0], ServerCAs: pool, Timeout: time.Second, MaxResponseBytes: 1024})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			got, err := client.ResolveAsset(context.Background(), e, instances.AssetVersion{AssetID: "world", Version: "v1"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("asset response: %v want %v", err, tc.want)
			}
			if err == nil && got != v {
				t.Fatal("asset metadata changed")
			}
			if err != nil && got.AssetID != "" {
				t.Fatal("partial asset returned")
			}
		})
	}
}
