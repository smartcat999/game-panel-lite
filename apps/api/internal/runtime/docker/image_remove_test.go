package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/client"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

func TestRemoveImageOnlyRemovesAuthorizedStoppedContainersAndKeepsVolumes(t *testing.T) {
	for _, tc := range []struct {
		name                                                                   string
		containers, allow, running, foreign, unsafeMount, mixed, race, wantErr bool
	}{
		{name: "unused"},
		{name: "managed stopped", containers: true, allow: true},
		{name: "not authorized", containers: true, wantErr: true},
		{name: "running despite stale stopped status", containers: true, allow: true, running: true, wantErr: true},
		{name: "foreign stopped container", containers: true, allow: true, foreign: true, wantErr: true},
		{name: "unknown volume cannot be reattached", containers: true, allow: true, unsafeMount: true, wantErr: true},
		{name: "running dependency prevents all cleanup", containers: true, allow: true, mixed: true, wantErr: true},
		{name: "external start race is not forced", containers: true, allow: true, race: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var deleted []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					if r.URL.Query().Get("all") != "1" || !strings.Contains(r.URL.Query().Get("filters"), "sha256:abc") {
						t.Error("must inspect all dependencies by image ID")
					}
					items := []map[string]string{}
					if tc.containers {
						items = append(items, map[string]string{"Id": "stopped", "ImageID": "sha256:abc"})
					}
					if tc.mixed {
						items = append(items, map[string]string{"Id": "running", "ImageID": "sha256:abc"})
					}
					_ = json.NewEncoder(w).Encode(items)
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/containers/"):
					running := tc.running || strings.Contains(r.URL.Path, "/running/")
					name := "/gamepanel-saved"
					if tc.foreign {
						name = "/outside-panel"
					}
					state := "exited"
					if running {
						state = "running"
					}
					mount := map[string]any{"Type": "bind", "Source": "/instances/saved", "Destination": "/data", "RW": true}
					if tc.unsafeMount {
						mount["Type"] = "volume"
						mount["Source"] = "/var/lib/docker/volumes/anonymous/_data"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": "stopped", "Name": name, "State": map[string]any{"Status": state, "Running": running}, "Mounts": []any{mount}})
				case r.Method == http.MethodDelete:
					for _, key := range []string{"force", "v"} {
						if v := r.URL.Query().Get(key); v == "true" || v == "1" {
							t.Errorf("must not enable %s", key)
						}
					}
					if tc.race && strings.Contains(r.URL.Path, "/containers/") {
						w.WriteHeader(http.StatusConflict)
						_, _ = w.Write([]byte(`{"message":"container is running"}`))
						return
					}
					deleted = append(deleted, r.URL.Path)
					if strings.Contains(r.URL.Path, "/containers/") {
						w.WriteHeader(http.StatusNoContent)
					} else {
						_, _ = w.Write([]byte(`[]`))
					}
				default:
					_, _ = w.Write([]byte(`{"Id":"sha256:abc"}`))
				}
			}))
			defer server.Close()
			cli, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithVersion("1.44"), client.WithHTTPClient(server.Client()))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			var targets []runtime.ImageRemovalTarget
			if tc.allow {
				targets = []runtime.ImageRemovalTarget{{ServerID: "saved", DataDir: "/instances/saved"}}
			}
			err = (&Adapter{client: cli}).RemoveImage(context.Background(), "example/game:1", targets)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected result: %v", err)
			}
			if tc.wantErr && len(deleted) != 0 {
				t.Fatalf("must preflight all containers before deleting: %v", deleted)
			}
			if !tc.wantErr {
				want := 1
				if tc.containers {
					want = 2
				}
				if len(deleted) != want || !strings.Contains(deleted[len(deleted)-1], "/images/") {
					t.Fatalf("wrong removal order: %v", deleted)
				}
			}
		})
	}
}

func TestInspectRemovedContainerReportsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No such container"}`))
	}))
	defer server.Close()
	cli, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithVersion("1.44"), client.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	status, err := (&Adapter{client: cli}).InspectWorkload(context.Background(), "removed-by-uninstall")
	if err != nil || status.State != domain.ActualMissing {
		t.Fatalf("expected missing workload, got %+v, %v", status, err)
	}
}
