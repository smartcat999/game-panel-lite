package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
)

type customArmProvider struct{ dst.Provider }

func (customArmProvider) Key() domain.ProviderKey { return "custom-arm-game" }
func (customArmProvider) NodeRequirements() domain.NodeRequirements {
	return domain.NodeRequirements{Architectures: []string{"arm64"}}
}

func TestRegisteredProviderControlsPlacement(t *testing.T) {
	registry, err := provider.NewRegistry(dst.NewProvider(), customArmProvider{dst.NewProvider()})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	nodes := &mockStore{nodes: []domain.ComputeNode{
		{ID: "unknown", Status: "online", LastHeartbeat: now, CPUCores: 8, MemoryTotalMB: 32768},
		{ID: "arm", Status: "online", LastHeartbeat: now, RuntimeArchitecture: "arm64", OSInfo: "linux/amd64 (agent)", CPUCores: 8, MemoryTotalMB: 16384},
		{ID: "amd", Status: "online", LastHeartbeat: now, RuntimeArchitecture: "amd64", OSInfo: "linux/arm64 (agent)", CPUCores: 8, MemoryTotalMB: 8192},
	}}
	scheduler := New(nodes, registry)
	for _, tc := range []struct {
		key  domain.ProviderKey
		want string
	}{{domain.ProviderDST, "amd"}, {"custom-arm-game", "arm"}} {
		server := domain.GameServer{ProviderKey: tc.key, Spec: domain.ServerSpec{Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}}}
		chosen, err := scheduler.Schedule(context.Background(), server)
		if err != nil || chosen.ID != tc.want {
			t.Fatalf("provider=%s node=%s want=%s err=%v", tc.key, chosen.ID, tc.want, err)
		}
		if err := scheduler.ValidateNode(context.Background(), "unknown", server); !errors.Is(err, ErrArchIncompatible) {
			t.Fatalf("unknown architecture accepted: %v", err)
		}
	}
	if _, err := scheduler.Schedule(context.Background(), domain.GameServer{ProviderKey: "unregistered"}); !errors.Is(err, ErrNoFeasibleNodes) {
		t.Fatalf("unknown provider scheduled: %v", err)
	}
}

func TestLocalPlacementUsesLiveDaemonProbe(t *testing.T) {
	registry, err := provider.NewRegistry(dst.NewProvider())
	if err != nil {
		t.Fatal(err)
	}
	nodes := &mockStore{nodes: []domain.ComputeNode{{ID: "local", IsLocal: true, RuntimeArchitecture: "amd64", CPUCores: 8, MemoryTotalMB: 8192}}}
	scheduler := New(nodes, registry)
	instance := domain.GameServer{ProviderKey: domain.ProviderDST, Spec: domain.ServerSpec{Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}}}
	if err := scheduler.ValidateNode(context.Background(), "local", instance); !errors.Is(err, ErrArchIncompatible) {
		t.Fatalf("accepted without daemon evidence: %v", err)
	}
	scheduler.WithLocalArchitecture(func() string { return "x86_64" })
	if err := scheduler.ValidateNode(context.Background(), "local", instance); err != nil {
		t.Fatal(err)
	}
	scheduler.WithLocalArchitecture(func() string { return "" })
	if err := scheduler.ValidateNode(context.Background(), "local", instance); !errors.Is(err, ErrArchIncompatible) {
		t.Fatalf("accepted after probe failure: %v", err)
	}
}
