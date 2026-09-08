package main

import (
	"context"
	"errors"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"testing"
)

type infoRuntime struct {
	agentRuntime
	info workload.RuntimeInfo
	err  error
}

func (r infoRuntime) Info(context.Context) (workload.RuntimeInfo, error) { return r.info, r.err }
func TestRuntimeArchitectureComesFromDaemon(t *testing.T) {
	got := getDockerInfo(context.Background(), infoRuntime{info: workload.RuntimeInfo{Architecture: "aarch64"}})
	if got.Architecture != "arm64" {
		t.Fatalf("architecture=%q", got.Architecture)
	}
	got = getDockerInfo(context.Background(), infoRuntime{info: workload.RuntimeInfo{Architecture: "amd64"}, err: errors.New("unavailable")})
	if got.Architecture != "" {
		t.Fatalf("failed probe retained architecture %q", got.Architecture)
	}
}
