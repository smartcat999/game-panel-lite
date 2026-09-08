package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestSchedulerRetainsDeletingCapacityAndRejectsUnknownCapacity(t *testing.T) {
	now := time.Now()
	db := &mockStore{nodes: []domain.ComputeNode{
		{ID: "unknown", IsLocal: true, MemoryTotalMB: 65536},
		{ID: "CPU-full", IsLocal: true, CPUCores: 1, MemoryTotalMB: 32768},
		{ID: "available", IsLocal: true, CPUCores: 2, MemoryTotalMB: 4096},
	}, servers: []domain.GameServer{{ID: "deleting", NodeID: "CPU-full", Spec: domain.ServerSpec{DesiredState: domain.DesiredDeleted, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}}}}}
	s := New(db, testRequirements{}).WithClock(func() time.Time { return now })
	request := domain.GameServer{ID: "new", Spec: domain.ServerSpec{Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}}}
	chosen, err := s.Schedule(context.Background(), request)
	if err != nil || chosen.ID != "available" {
		t.Fatalf("node=%s err=%v", chosen.ID, err)
	}
	for _, id := range []string{"unknown", "CPU-full"} {
		if err := s.ValidateNode(context.Background(), id, request); !errors.Is(err, ErrInsufficientCapacity) {
			t.Fatalf("node %s accepted: %v", id, err)
		}
	}
}

func TestSchedulerRetainsDeletingDefaultHostPort(t *testing.T) {
	node := domain.ComputeNode{ID: "node"}
	request := domain.GameServer{ID: "new", Spec: domain.ServerSpec{Network: domain.ServerNetworkSpec{HostPort: 7777}}}
	old := domain.GameServer{ID: "old", NodeID: node.ID, Spec: domain.ServerSpec{DesiredState: domain.DesiredDeleted, Network: domain.ServerNetworkSpec{Port: 7777}}}
	if err := filterPortAvailable(node, request, []domain.GameServer{old}); !errors.Is(err, ErrPortConflict) {
		t.Fatalf("deletion released default host binding: %v", err)
	}
}
