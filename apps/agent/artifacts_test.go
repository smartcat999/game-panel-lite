package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func artifactFixture() (workload.Assignment, workload.Artifact) {
	sum := sha256.Sum256([]byte("data"))
	return workload.Assignment{UID: "assignment", Generation: 2}, workload.Artifact{ID: "source", Path: "Mods/source.bin", SizeBytes: 4, SHA256: hex.EncodeToString(sum[:])}
}

func TestArtifactHTTPSource(t *testing.T) {
	assignment, item := artifactFixture()
	for _, mode := range []string{"success", "unauthorized", "redirect", "short", "etag", "compressed"} {
		t.Run(mode, func(t *testing.T) {
			var redirected atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/panel/api/agent/assignments/assignment/artifacts/source" || r.URL.Query().Get("generation") != "2" || r.Header.Get("X-Node-Token") != "secret" || r.Header.Get("Accept-Encoding") != "identity" {
					t.Errorf("unexpected download request: %s", r.URL)
				}
				w.Header().Set("Content-Length", "4")
				w.Header().Set("ETag", strconv.Quote(item.SHA256))
				switch mode {
				case "unauthorized":
					w.WriteHeader(401)
					return
				case "redirect":
					w.Header().Set("Location", target.URL)
					w.WriteHeader(307)
					return
				case "short":
					w.Header().Set("Content-Length", "3")
				case "etag":
					w.Header().Set("ETag", `"other"`)
				case "compressed":
					w.Header().Set("Content-Encoding", "gzip")
				}
				io.WriteString(w, "data")
			}))
			defer server.Close()
			source, err := newArtifactSource(server.URL+"/panel", "secret", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			body, err := source.Open(context.Background(), assignment, item)
			if mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(body)
				body.Close()
				if err != nil || string(data) != "data" {
					t.Fatalf("download: %q %v", data, err)
				}
			} else if err == nil {
				body.Close()
				t.Fatal("accepted invalid response")
			}
			if redirected.Load() != 0 {
				t.Fatal("followed redirect with node credentials")
			}
		})
	}
}

func TestArtifactDownloadCancellation(t *testing.T) {
	assignment, item := artifactFixture()
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4")
		w.Header().Set("ETag", strconv.Quote(item.SHA256))
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source, err := newArtifactSource(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	body, err := source.Open(ctx, assignment, item)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	readDone := make(chan error, 1)
	go func() { _, err := io.ReadAll(body); readDone <- err }()
	cancel()
	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("canceled read succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("read did not cancel")
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("server request still active")
	}
}

func TestArtifactConfigurationAndCapabilities(t *testing.T) {
	for _, base := range []string{"file:///tmp", "http://user:pass@master", "https://master?token=x", "https://master#fragment", "//master"} {
		if _, err := newArtifactSource(base, "token", http.DefaultClient); err == nil {
			t.Fatalf("accepted %s", base)
		}
	}
	defaults, err := agentArtifactLimits(func(string) string { return "" })
	if err != nil || defaults.MaxFiles != 128 || defaults.MaxFileBytes != 256<<20 || defaults.MaxTotalBytes != 1<<30 {
		t.Fatalf("defaults: %+v %v", defaults, err)
	}
	for _, key := range []string{"AGENT_ARTIFACT_MAX_FILES", "AGENT_ARTIFACT_MAX_FILE_BYTES", "AGENT_ARTIFACT_MAX_TOTAL_BYTES"} {
		for _, value := range []string{"0", "-1", "NaN", "99999999999999999999999"} {
			if _, err := agentArtifactLimits(func(name string) string {
				if name == key {
					return value
				}
				return ""
			}); err == nil {
				t.Fatalf("accepted %s=%s", key, value)
			}
		}
	}
	limits, err := agentArtifactLimits(func(string) string { return "42" })
	if err != nil || limits.MaxFiles != 42 || limits.MaxFileBytes != 42 || limits.MaxTotalBytes != 42 {
		t.Fatalf("overrides: %+v %v", limits, err)
	}
	for _, enabled := range []bool{false, true} {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			expected := ""
			if enabled {
				expected = "artifacts-v1"
			}
			if r.Header.Get("X-Workload-Capabilities") != expected {
				t.Error("incorrect capabilities")
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[]`))}, nil
		})}
		reconcileAssignments(context.Background(), client, AgentConfig{MasterURL: "http://master", Token: "token", ArtifactsEnabled: enabled}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	}
}

func TestArtifactIdentityRejectedBeforeNetwork(t *testing.T) {
	source, _ := newArtifactSource("http://master", "token", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid identity reached network")
		return nil, nil
	})})
	a, item := artifactFixture()
	for _, uid := range []string{"", "..", "a/b", "a?b", "a%2fb"} {
		a.UID = uid
		if _, err := source.Open(context.Background(), a, item); err == nil {
			t.Fatalf("accepted UID %q", uid)
		}
	}
	a.UID = "assignment"
	item.ID = "../other"
	if _, err := source.Open(context.Background(), a, item); err == nil {
		t.Fatal("accepted artifact traversal")
	}
}

func TestAgentRuntimePreparesOnlyVerifiedHTTPBytes(t *testing.T) {
	for _, payload := range []string{"data", "evil"} {
		t.Run(payload, func(t *testing.T) {
			a, item := artifactFixture()
			a.ServerID = "server"
			a.NodeID = "node"
			a.DesiredState = "running"
			a.Spec.Options.Artifacts = []workload.Artifact{item}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "4")
				w.Header().Set("ETag", strconv.Quote(item.SHA256))
				io.WriteString(w, payload)
			}))
			defer server.Close()
			limits, err := agentArtifactLimits(func(string) string { return "" })
			if err != nil {
				t.Fatal(err)
			}
			adapter, err := newAgentRuntime("unix:///var/run/docker.sock", t.TempDir(), AgentConfig{MasterURL: server.URL, Token: "token"}, limits)
			if err != nil {
				t.Fatal(err)
			}
			defer adapter.Close()
			prepared, err := adapter.PrepareArtifacts(context.Background(), a)
			if payload == "evil" {
				if err == nil {
					prepared.Release()
					t.Fatal("trusted ETag without checking downloaded bytes")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := prepared.Release(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
