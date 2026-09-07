package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type leaseTestRuntime struct {
	agentRuntime
	state      worker.State
	calls      []string
	failCreate bool
}

func (r *leaseTestRuntime) Inspect(context.Context, string) (worker.State, error) {
	r.calls = append(r.calls, "inspect")
	return r.state, nil
}
func (r *leaseTestRuntime) Create(_ context.Context, a workload.Assignment) error {
	r.calls = append(r.calls, "create")
	if r.failCreate {
		return errors.New("ambiguous create")
	}
	r.state = worker.State{Exists: true, ID: "runtime", Managed: true, UID: a.UID, ServerID: a.ServerID, NodeID: a.NodeID, Generation: a.Generation}
	return nil
}
func (r *leaseTestRuntime) Start(context.Context, worker.State) error {
	r.calls = append(r.calls, "start")
	r.state.Running = true
	return nil
}

func TestLeasedReconciliation(t *testing.T) {
	for _, mode := range []string{"success", "report-rejected", "busy", "old-api", "transport-error", "renewal-rejected", "wrong-renewal-fence", "ambiguous-create", "wrong-grant", "expired-local-window"} {
		t.Run(mode, func(t *testing.T) {
			a := workload.Assignment{UID: "uid", ServerID: "server", NodeID: "node", Generation: 1, DesiredState: "running"}
			var actions []string
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/api/agent/assignments/uid/status" {
					actions = append(actions, "report")
					var report workload.Observation
					if err := json.NewDecoder(r.Body).Decode(&report); err != nil || report.LeaseHolderID == "" || report.LeaseFence != 1 {
						t.Errorf("missing report lease: %+v %v", report, err)
					}
					status := 200
					if mode == "report-rejected" {
						status = 409
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				}
				if r.Header.Get("X-Node-Token") != "token" || r.URL.Path != "/api/agent/assignments/uid/lease" {
					t.Errorf("invalid lease request: %s", r.URL.Path)
				}
				var request workload.LeaseRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				actions = append(actions, request.Action)
				if mode == "transport-error" {
					return nil, errors.New("connection lost")
				}
				status := 200
				if mode == "old-api" {
					status = 404
				}
				if mode == "busy" || (mode == "renewal-rejected" && request.Action == "renew") {
					status = 409
				}
				if request.Action == "release" {
					status = 204
				}
				grant := workload.LeaseGrant{AssignmentUID: a.UID, ServerID: a.ServerID, NodeID: a.NodeID, Generation: a.Generation, HolderID: request.HolderID, Fence: 1, ValidForMS: 120000}
				if mode == "wrong-renewal-fence" && request.Action == "renew" {
					grant.Fence++
				}
				if mode == "wrong-grant" {
					grant.ServerID = "foreign"
				}
				if mode == "expired-local-window" {
					grant.ValidForMS = 1000
				}
				body, _ := json.Marshal(grant)
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}
			runtime := &leaseTestRuntime{failCreate: mode == "ambiguous-create"}
			observation, err := reconcileLeasedAssignment(context.Background(), client, AgentConfig{MasterURL: "http://master", Token: "token"}, slog.New(slog.NewTextHandler(io.Discard, nil)), a, runtime)
			switch mode {
			case "report-rejected":
				if err == nil || !reflect.DeepEqual(actions, []string{"acquire", "renew", "renew", "report"}) {
					t.Fatalf("released unconfirmed report: %v %v", err, actions)
				}
			case "success":
				if err != nil || observation.LastError != "" || observation.ActualState != "running" || !reflect.DeepEqual(actions, []string{"acquire", "renew", "renew", "report", "release"}) {
					t.Fatalf("success: %+v %v %v", observation, err, actions)
				}
			case "busy", "old-api", "transport-error", "wrong-grant", "expired-local-window":
				if err == nil || len(runtime.calls) != 0 {
					t.Fatalf("executed without lease: %v %v", err, runtime.calls)
				}
			case "renewal-rejected", "wrong-renewal-fence":
				if err != nil || observation.LastError == "" || !reflect.DeepEqual(runtime.calls, []string{"inspect"}) || !reflect.DeepEqual(actions, []string{"acquire", "renew", "report"}) {
					t.Fatalf("continued after revocation: %+v %v %v", observation, runtime.calls, actions)
				}
			case "ambiguous-create":
				if observation.LastError == "" || !reflect.DeepEqual(actions, []string{"acquire", "renew", "report"}) {
					t.Fatalf("released ambiguous execution: %+v %v", observation, actions)
				}
			}
		})
	}
}

func TestLeaseClientDoesNotFollowRedirects(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; w.WriteHeader(200) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	lease, err := newAgentLeaseClient(source.Client(), AgentConfig{MasterURL: source.URL, Token: "private"}, workload.Assignment{UID: "uid", NodeID: "node", ServerID: "server", Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lease.exchange(context.Background(), "acquire", 0); err == nil || reached {
		t.Fatal("redirect was accepted or followed")
	}
}
