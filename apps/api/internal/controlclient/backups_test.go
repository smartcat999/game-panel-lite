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

	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
)

func TestBackupChecks(t *testing.T) {
	event := backup.Requested{SchemaVersion: 1, EventID: "event", OperationID: "operation", BackupID: "backup", OrganizationID: "org", ServerID: "server", RegionID: "east", RevisionID: "revision", SpecGeneration: 1, IntentVersion: 1, PlacementEpoch: 1, Scope: "world"}
	var status atomic.Int64
	status.Store(204)
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/internal/region/backups/check" || r.URL.RawQuery != "" || r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong request")
		}
		var got backup.Requested
		if json.NewDecoder(r.Body).Decode(&got) != nil || got != event {
			t.Error("request identity changed")
		}
		if status.Load() == -1 {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Location", "/redirected")
		w.WriteHeader(int(status.Load()))
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
	}{{204, nil}, {404, backup.ErrRequestUnavailable}, {401, ErrAccessDenied}, {403, ErrAccessDenied}, {503, ErrRequestFailed}, {307, ErrRequestFailed}, {200, ErrRequestFailed}, {204, nil}} {
		status.Store(tc.status)
		before := calls.Load()
		if err := client.CheckBackup(context.Background(), event); !errors.Is(err, tc.want) {
			t.Fatalf("status %d: %v", tc.status, err)
		}
		if calls.Load() != before+1 {
			t.Fatal("cached result or followed redirect")
		}
	}
	before := calls.Load()
	bad := event
	bad.RegionID = "west"
	if !errors.Is(client.CheckBackup(context.Background(), bad), backup.ErrInvalidRequest) || calls.Load() != before {
		t.Fatal("invalid local request sent")
	}
	status.Store(-1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := client.CheckBackup(ctx, event); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
