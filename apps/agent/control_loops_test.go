package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type slowControlRuntime struct {
	agentRuntime
	entered, canceled, release chan struct{}
	inspections                atomic.Int32
}

func (r *slowControlRuntime) Info(context.Context) (workload.RuntimeInfo, error) {
	return workload.RuntimeInfo{Version: "test", RunningContainers: 1}, nil
}
func (r *slowControlRuntime) Inspect(ctx context.Context, _ string) (worker.State, error) {
	if r.inspections.Add(1) == 1 {
		close(r.entered)
		<-ctx.Done()
		close(r.canceled)
		<-r.release // Model cleanup that must finish before closing the runtime.
	}
	return worker.State{}, ctx.Err()
}

func TestAgentHeartbeatContinuesDuringWorkAndShutdownWaits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &slowControlRuntime{entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	defer func() {
		select {
		case <-runtime.release:
		default:
			close(runtime.release)
		}
	}()
	heartbeats := make(chan struct{}, 16)
	var polls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		body := `{}`
		switch req.URL.Path {
		case "/api/agent/assignments":
			polls.Add(1)
			body = `[{"uid":"first","nodeId":"node","serverId":"one","generation":1,"desiredState":"running"},{"uid":"second","nodeId":"node","serverId":"two","generation":1,"desiredState":"running"}]`
		case "/api/agent/assignments/first/lease":
			var lease workload.LeaseRequest
			if err := json.NewDecoder(req.Body).Decode(&lease); err != nil {
				t.Error(err)
			}
			grant, _ := json.Marshal(workload.LeaseGrant{AssignmentUID: "first", NodeID: "node", ServerID: "one", Generation: 1, HolderID: lease.HolderID, Fence: 1, ValidForMS: 120000})
			body = string(grant)
		case "/api/agent/heartbeat":
			var payload HeartbeatPayload
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || payload.Token != "token" || payload.RunningCount != 1 {
				t.Errorf("invalid heartbeat: %+v %v", payload, err)
			}
			select {
			case <-runtime.entered:
				select {
				case heartbeats <- struct{}{}:
				default:
				}
			default:
			}
		default:
			t.Errorf("unexpected request: %s", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	cfg := AgentConfig{MasterURL: "http://master", Token: "token", Interval: time.Millisecond}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan struct{})
	go func() {
		defer close(done)
		runAgentControlLoops(ctx, cfg.Interval, func(ctx context.Context) {
			reportAgentHeartbeat(ctx, client, cfg, logger, runtime)
		}, func(ctx context.Context) {
			reconcileAssignments(ctx, client, cfg, logger, runtime)
		})
	}()
	await := func(ch <-chan struct{}, name string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out: %s", name)
		}
	}
	await(runtime.entered, "work started")
	for i := 0; i < 3; i++ {
		await(heartbeats, "heartbeat during work")
	}
	if polls.Load() != 1 || runtime.inspections.Load() != 1 {
		t.Fatal("slow work overlapped another poll or assignment")
	}
	cancel()
	await(runtime.canceled, "work canceled")
	select {
	case <-done:
		t.Fatal("returned before worker cleanup finished")
	default:
	}
	close(runtime.release)
	await(done, "shutdown joined")
	if runtime.inspections.Load() != 1 || polls.Load() != 1 {
		t.Fatal("started more work after cancellation")
	}
}

func TestAgentControlLoopsDoNotStartWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	action := func(context.Context) { calls.Add(1) }
	runAgentControlLoops(ctx, time.Nanosecond, action, action)
	if calls.Load() != 0 {
		t.Fatal("started work after cancellation")
	}
}
