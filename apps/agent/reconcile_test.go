package main

import (
	"io"
	"log/slog"
	"net/http"
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
