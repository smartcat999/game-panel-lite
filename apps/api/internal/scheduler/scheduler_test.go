package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type mockStore struct {
	nodes   []domain.ComputeNode
	servers []domain.GameServer
}

func (m *mockStore) ListComputeNodes(ctx context.Context) ([]domain.ComputeNode, error) {
	return m.nodes, nil
}

func (m *mockStore) GetComputeNode(ctx context.Context, id string) (domain.ComputeNode, error) {
	for _, n := range m.nodes {
		if n.ID == id {
			return n, nil
		}
	}
	return domain.ComputeNode{}, errors.New("not found")
}

func (m *mockStore) ListGameServers(ctx context.Context) ([]domain.GameServer, error) {
	return m.servers, nil
}

func TestSchedulerNoNodesAvailable(t *testing.T) {
	s := New(&mockStore{}, testRequirements{})
	_, err := s.Schedule(context.Background(), domain.GameServer{})
	if !errors.Is(err, ErrNoNodesAvailable) {
		t.Fatalf("expected ErrNoNodesAvailable, got %v", err)
	}
}

func TestSchedulerFiltersOfflineAndStaleNodes(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		nodes: []domain.ComputeNode{
			{ID: "node-offline", Status: "offline", LastHeartbeat: now},
			{ID: "node-stale", Status: "online", LastHeartbeat: now.Add(-60 * time.Second)},
			{ID: "node-fresh", Status: "online", LastHeartbeat: now.Add(-5 * time.Second), CPUCores: 8, MemoryTotalMB: 8192},
		},
	}
	s := New(store, testRequirements{}).WithClock(func() time.Time { return now })

	chosen, err := s.Schedule(context.Background(), domain.GameServer{
		Spec: domain.ServerSpec{
			Network:   domain.ServerNetworkSpec{HostPort: 7777},
			Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 1024},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen.ID != "node-fresh" {
		t.Fatalf("expected node-fresh, got %s", chosen.ID)
	}
}

func TestSchedulerFiltersPortConflict(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		nodes: []domain.ComputeNode{
			{ID: "node-1", Status: "online", LastHeartbeat: now, CPUCores: 8, MemoryTotalMB: 8192},
			{ID: "node-2", Status: "online", LastHeartbeat: now, CPUCores: 8, MemoryTotalMB: 8192},
		},
		servers: []domain.GameServer{
			{
				ID:     "existing-1",
				NodeID: "node-1",
				Name:   "Existing",
				Spec: domain.ServerSpec{
					Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512},
					Network:   domain.ServerNetworkSpec{HostPort: 7777},
				},
			},
		},
	}
	s := New(store, testRequirements{}).WithClock(func() time.Time { return now })

	chosen, err := s.Schedule(context.Background(), domain.GameServer{
		ID: "new-server",
		Spec: domain.ServerSpec{
			Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512},
			Network:   domain.ServerNetworkSpec{HostPort: 7777},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen.ID != "node-2" {
		t.Fatalf("expected node-2 due to port 7777 collision on node-1, got %s", chosen.ID)
	}
}

func TestSchedulerFiltersArchitectureMismatch(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		nodes: []domain.ComputeNode{
			{ID: "arm-node", Status: "online", LastHeartbeat: now, RuntimeArchitecture: "arm64", CPUCores: 8, MemoryTotalMB: 16384},
			{ID: "amd-node", Status: "online", LastHeartbeat: now, RuntimeArchitecture: "amd64", CPUCores: 8, MemoryTotalMB: 8192},
		},
	}
	s := New(store, testRequirements{}).WithClock(func() time.Time { return now })

	chosen, err := s.Schedule(context.Background(), domain.GameServer{
		ProviderKey: domain.ProviderKey("test-amd64-game"),
		Spec: domain.ServerSpec{
			Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 1024},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen.ID != "amd-node" {
		t.Fatalf("expected amd-node for DST provider, got %s", chosen.ID)
	}
}

func TestSchedulerFiltersCapacityExceeded(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		nodes: []domain.ComputeNode{
			{ID: "small-node", Status: "online", LastHeartbeat: now, CPUCores: 8, MemoryTotalMB: 2048},
			{ID: "large-node", Status: "online", LastHeartbeat: now, CPUCores: 8, MemoryTotalMB: 8192},
		},
		servers: []domain.GameServer{
			{
				ID:     "occupier",
				NodeID: "small-node",
				Spec: domain.ServerSpec{
					Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 1536},
				},
			},
		},
	}
	s := New(store, testRequirements{}).WithClock(func() time.Time { return now })

	chosen, err := s.Schedule(context.Background(), domain.GameServer{
		Spec: domain.ServerSpec{
			Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 1024},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen.ID != "large-node" {
		t.Fatalf("expected large-node, got %s", chosen.ID)
	}
}

func TestSchedulerScoresLeastAllocated(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		nodes: []domain.ComputeNode{
			{ID: "busy-node", Status: "online", LastHeartbeat: now, CPUCores: 8, MemoryTotalMB: 8192},
			{ID: "idle-node", Status: "online", LastHeartbeat: now, CPUCores: 8, MemoryTotalMB: 8192},
		},
		servers: []domain.GameServer{
			{
				ID:     "s1",
				NodeID: "busy-node",
				Spec: domain.ServerSpec{
					Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 2048},
				},
			},
			{
				ID:     "s2",
				NodeID: "busy-node",
				Spec: domain.ServerSpec{
					Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 2048},
				},
			},
		},
	}
	s := New(store, testRequirements{}).WithClock(func() time.Time { return now })

	chosen, err := s.Schedule(context.Background(), domain.GameServer{
		Spec: domain.ServerSpec{
			Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 1024},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen.ID != "idle-node" {
		t.Fatalf("expected idle-node with more free memory, got %s", chosen.ID)
	}
}

func TestSchedulerValidateNodeExplicitSelection(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		nodes: []domain.ComputeNode{
			{ID: "node-1", Status: "online", LastHeartbeat: now, CPUCores: 8, MemoryTotalMB: 8192},
		},
		servers: []domain.GameServer{
			{
				ID:     "existing",
				NodeID: "node-1",
				Spec: domain.ServerSpec{
					Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512},
					Network:   domain.ServerNetworkSpec{HostPort: 7777},
				},
			},
		},
	}
	s := New(store, testRequirements{}).WithClock(func() time.Time { return now })

	err := s.ValidateNode(context.Background(), "node-1", domain.GameServer{
		Spec: domain.ServerSpec{
			Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512},
			Network:   domain.ServerNetworkSpec{HostPort: 7777},
		},
	})
	if !errors.Is(err, ErrPortConflict) {
		t.Fatalf("expected ErrPortConflict, got %v", err)
	}

	err = s.ValidateNode(context.Background(), "node-1", domain.GameServer{
		Spec: domain.ServerSpec{
			Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512},
			Network:   domain.ServerNetworkSpec{HostPort: 8888},
		},
	})
	if err != nil {
		t.Fatalf("expected no error for available port, got %v", err)
	}
}

func TestSchedulerFiltersUnschedulableNodes(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStore{
		nodes: []domain.ComputeNode{
			{ID: "cordoned-node", Status: "online", Unschedulable: true, LastHeartbeat: now, CPUCores: 8, MemoryTotalMB: 16384},
			{ID: "active-node", Status: "online", Unschedulable: false, LastHeartbeat: now, CPUCores: 8, MemoryTotalMB: 8192},
		},
	}
	s := New(store, testRequirements{}).WithClock(func() time.Time { return now })

	// Auto-schedule should bypass the cordoned node and pick active-node
	chosen, err := s.Schedule(context.Background(), domain.GameServer{
		Spec: domain.ServerSpec{
			Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512},
			Network:   domain.ServerNetworkSpec{HostPort: 7777},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen.ID != "active-node" {
		t.Fatalf("expected active-node, got %s", chosen.ID)
	}

	// ValidateNode explicit selection of cordoned node should fail
	err = s.ValidateNode(context.Background(), "cordoned-node", domain.GameServer{
		Spec: domain.ServerSpec{
			Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512},
			Network:   domain.ServerNetworkSpec{HostPort: 7777},
		},
	})
	if !errors.Is(err, ErrNodeUnschedulable) {
		t.Fatalf("expected ErrNodeUnschedulable, got %v", err)
	}
}

type testRequirements struct{}

func (testRequirements) NodeRequirements(key domain.ProviderKey) (domain.NodeRequirements, error) {
	if key == domain.ProviderKey("test-amd64-game") {
		return domain.NodeRequirements{Architectures: []string{"amd64"}}, nil
	}
	return domain.NodeRequirements{}, nil
}
