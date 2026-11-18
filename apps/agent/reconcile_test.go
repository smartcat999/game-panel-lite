package main

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestReconcileStoppedAssignmentUsesObservedDockerState(t *testing.T) {
	inspectCount := 0
	stopCount := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := ""
		status := http.StatusOK
		switch {
		case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/json"):
			inspectCount++
			running := inspectCount == 1
			body = `{"Id":"container-1","State":{"Running":` + map[bool]string{true: "true", false: "false"}[running] + `},"Config":{"Labels":{"io.gamepanel.assignment-uid":"uid-1","io.gamepanel.generation":"2"}}}`
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/stop"):
			stopCount++
			status = http.StatusNoContent
		default:
			t.Fatalf("unexpected docker request: %s %s", req.Method, req.URL.String())
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	assignment := workloadAssignment{UID: "uid-1", ServerID: "server-1", NodeID: "node-1", Generation: 2, DesiredState: "stopped"}
	observation := reconcileWorkloadWithClient(assignment, slog.Default(), client)
	if observation.LastError != "" || observation.ActualState != "stopped" {
		t.Fatalf("unexpected observation: %+v", observation)
	}
	if stopCount != 1 || inspectCount != 2 {
		t.Fatalf("expected inspect-stop-inspect reconcile, inspect=%d stop=%d", inspectCount, stopCount)
	}
}

func TestSafeAgentInstancePathRejectsTraversal(t *testing.T) {
	if _, err := safeAgentInstancePath("/var/lib/gamepanel/instances/server-1", "../../etc/passwd"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}

func TestAgentDataBindPathsSupportsWholeDataDirectoryMount(t *testing.T) {
	host, container, err := agentDataBindPaths("/var/lib/gamepanel/instances/server-1", "/data")
	if err != nil {
		t.Fatalf("resolve bind: %v", err)
	}
	if host != "/var/lib/gamepanel/instances/server-1" || container != "/data" {
		t.Fatalf("unexpected bind paths: %s:%s", host, container)
	}
}

func TestNormalizeAgentInstancePermissions(t *testing.T) {
	root := t.TempDir()
	worlds := filepath.Join(root, "Worlds")
	if err := os.Mkdir(worlds, 0o700); err != nil {
		t.Fatal(err)
	}
	world := filepath.Join(worlds, "world.wld")
	if err := os.WriteFile(world, []byte("world"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := normalizeAgentInstancePermissions(root); err != nil {
		t.Fatal(err)
	}

	worldsInfo, err := os.Stat(worlds)
	if err != nil {
		t.Fatal(err)
	}
	if got := worldsInfo.Mode().Perm(); got != 0o777 {
		t.Fatalf("expected Worlds mode 0777, got %04o", got)
	}
	worldInfo, err := os.Stat(world)
	if err != nil {
		t.Fatal(err)
	}
	if got := worldInfo.Mode().Perm(); got != 0o666 {
		t.Fatalf("expected world mode 0666, got %04o", got)
	}
}

func TestServerIDFromContainerNamesIgnoresAgentContainers(t *testing.T) {
	for _, names := range [][]string{
		{"/gamepanel-agent"},
		{"/gamepanel-agent-backup-20260828"},
	} {
		if got := serverIDFromContainerNames(names); got != "" {
			t.Fatalf("expected agent container %v to be ignored, got %q", names, got)
		}
	}
	const serverID = "f2b4c3b6-86bb-448a-b6b1-0745cf1c12c7"
	if got := serverIDFromContainerNames([]string{"/gamepanel-" + serverID}); got != serverID {
		t.Fatalf("expected game server id %q, got %q", serverID, got)
	}
}

func TestReportWorkloadObservationRecoversOnNextAttempt(t *testing.T) {
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("master unavailable")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"acceptedGeneration":2}`)), Header: make(http.Header)}, nil
	})}
	assignment := workloadAssignment{UID: "uid-1", ServerID: "server-1", NodeID: "node-1", Generation: 2}
	observation := workloadObservation{ObservedGeneration: 2, ActualState: "running"}
	if err := reportWorkloadObservation(client, AgentConfig{MasterURL: "http://master", Token: "token"}, assignment, observation); err == nil {
		t.Fatal("expected first report to fail while master is unavailable")
	}
	if err := reportWorkloadObservation(client, AgentConfig{MasterURL: "http://master", Token: "token"}, assignment, observation); err != nil {
		t.Fatalf("expected next reconcile report to recover: %v", err)
	}
}

func TestReconcileRunningAssignmentIsIdempotentAfterAgentRestart(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != http.MethodGet || !strings.HasSuffix(req.URL.Path, "/json") {
			t.Fatalf("unexpected mutation during steady-state reconcile: %s %s", req.Method, req.URL.Path)
		}
		body := `{"Id":"container-1","State":{"Running":true},"Config":{"Labels":{"io.gamepanel.assignment-uid":"uid-1","io.gamepanel.generation":"2"}}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	assignment := workloadAssignment{UID: "uid-1", ServerID: "server-1", NodeID: "node-1", Generation: 2, DesiredState: "running"}
	for i := 0; i < 2; i++ {
		observation := reconcileWorkloadWithClient(assignment, slog.Default(), client)
		if observation.LastError != "" || observation.ActualState != "running" {
			t.Fatalf("unexpected observation after restart pass %d: %+v", i+1, observation)
		}
	}
	if requests != 4 {
		t.Fatalf("expected two read-only inspections per pass, got %d requests", requests)
	}
}
