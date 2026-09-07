package main

import (
	"context"
	"errors"
	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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
	assignment := workload.Assignment{UID: "uid-1", ServerID: "server-1", NodeID: "node-1", Generation: 2}
	observation := workload.Observation{ObservedGeneration: 2, ActualState: "running"}
	if err := reportWorkloadObservation(context.Background(), client, AgentConfig{MasterURL: "http://master", Token: "token"}, assignment, observation); err == nil {
		t.Fatal("expected first report to fail while master is unavailable")
	}
	if err := reportWorkloadObservation(context.Background(), client, AgentConfig{MasterURL: "http://master", Token: "token"}, assignment, observation); err != nil {
		t.Fatalf("expected next reconcile report to recover: %v", err)
	}
}

func TestRuntimeTaskRejectsLegacyLifecycle(t *testing.T) {
	for _, action := range []string{"start", "stop", "restart", "create", "delete"} {
		if err := executeRuntimeTask(context.Background(), NodeTask{Action: action}, nil); err == nil {
			t.Fatalf("accepted obsolete action %s", action)
		}
	}
}

type consoleRuntime struct {
	agentRuntime
	state worker.State
	input string
}

func (r *consoleRuntime) Inspect(context.Context, string) (worker.State, error) { return r.state, nil }
func (r *consoleRuntime) Console(_ context.Context, _ string, input string) error {
	r.input = input
	return nil
}
func TestConsoleTaskChecksRuntimeOwnership(t *testing.T) {
	runtime := &consoleRuntime{state: worker.State{Exists: true, Managed: true, ServerID: "server", NodeID: "node"}}
	task := NodeTask{ServerID: "server", NodeID: "other", Action: "exec_command", Payload: "help"}
	if err := executeRuntimeTask(context.Background(), task, runtime); err == nil || runtime.input != "" {
		t.Fatal("foreign node command accepted")
	}
	task.NodeID = "node"
	if err := executeRuntimeTask(context.Background(), task, runtime); err != nil || runtime.input != "help" {
		t.Fatalf("command failed: %v", err)
	}
}
