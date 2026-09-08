package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type evictionFakeScheduler struct {
	targetNode domain.ComputeNode
	err        error
}

func (f *evictionFakeScheduler) Schedule(context.Context, domain.GameServer) (domain.ComputeNode, error) {
	if f.err != nil {
		return domain.ComputeNode{}, f.err
	}
	return f.targetNode, nil
}

func (f *evictionFakeScheduler) ValidateNode(context.Context, string, domain.GameServer) error {
	return nil
}

type evictionTestStore struct {
	servers    []domain.GameServer
	nodes      map[string]domain.ComputeNode
	assignment *domain.WorkloadAssignment
	activities []domain.ActivityEvent
}

func (s *evictionTestStore) ListGameServers(context.Context) ([]domain.GameServer, error) {
	return append([]domain.GameServer{}, s.servers...), nil
}

func (s *evictionTestStore) SaveReconciledGameServer(_ context.Context, before domain.GameServer, server domain.GameServer) error {
	for i := range s.servers {
		if s.servers[i].ID == server.ID {
			s.servers[i] = server
			return nil
		}
	}
	return nil
}

func (s *evictionTestStore) PublishWorkloadAssignment(_ context.Context, _ domain.GameServer, assignment *domain.WorkloadAssignment) error {
	copy := *assignment
	s.assignment = &copy
	return nil
}

func (s *evictionTestStore) GetWorkloadAssignmentByServer(context.Context, string) (domain.WorkloadAssignment, error) {
	if s.assignment == nil {
		return domain.WorkloadAssignment{}, errors.New("not found")
	}
	return *s.assignment, nil
}

func (s *evictionTestStore) DeleteWorkloadAssignment(context.Context, string) error {
	s.assignment = nil
	return nil
}

func (s *evictionTestStore) GetWorkloadObservation(context.Context, string) (domain.WorkloadObservation, error) {
	return domain.WorkloadObservation{}, errors.New("not found")
}

func (s *evictionTestStore) GetComputeNode(_ context.Context, id string) (domain.ComputeNode, error) {
	if node, ok := s.nodes[id]; ok {
		return node, nil
	}
	return domain.ComputeNode{}, errors.New("node not found")
}

func (s *evictionTestStore) CreateActivity(_ context.Context, event *domain.ActivityEvent) error {
	s.activities = append(s.activities, *event)
	return nil
}

func TestNodeHealth_TransientStale_BufferingInPhaseReconciling(t *testing.T) {
	now := time.Now().UTC()
	staleNode := domain.ComputeNode{
		ID:            "node-stale",
		Name:          "Stale Node",
		Status:        "online",
		LastHeartbeat: now.Add(-50 * time.Second), // Stale (> 45s), but < 2m eviction timeout
	}
	store := &evictionTestStore{
		nodes: map[string]domain.ComputeNode{staleNode.ID: staleNode},
		servers: []domain.GameServer{{
			ID:     "server-1",
			NodeID: "node-stale",
			Name:   "Game 1",
			Spec:   domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning},
			Status: domain.ServerRuntimeStatus{Phase: domain.PhaseRunning},
		}},
	}
	controller := NewController(store, NewRuntimeReconciler(&fakeBuilder{}, nil), nil).
		WithRecoveryTimeout(2 * time.Minute)

	controller.RunOnce(context.Background())

	// Should not be evicted; remains on node-stale but marked PhaseReconciling
	if store.servers[0].NodeID != "node-stale" {
		t.Fatalf("expected server to stay on node-stale during transient stale window, got %s", store.servers[0].NodeID)
	}
	if store.servers[0].Status.Phase != domain.PhaseReconciling {
		t.Fatalf("expected PhaseReconciling, got %s", store.servers[0].Status.Phase)
	}

	foundCondition := false
	for _, c := range store.servers[0].Status.Conditions {
		if c.Type == "AgentReachable" && c.Status == "Unknown" && c.Reason == "HeartbeatStale" {
			foundCondition = true
			break
		}
	}
	if !foundCondition {
		t.Fatalf("expected AgentReachable: Unknown condition, got %+v", store.servers[0].Status.Conditions)
	}
}

func TestNodeHealth_ExtendedStale_RequiresRecoveryEvidence(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(fmt.Sprintf("alternative=%t", available), func(t *testing.T) {
			now := time.Now().UTC()
			dead := domain.ComputeNode{ID: "node-dead", Status: "offline", LastHeartbeat: now.Add(-3 * time.Minute)}
			store := &evictionTestStore{
				nodes:   map[string]domain.ComputeNode{dead.ID: dead},
				servers: []domain.GameServer{{ID: "server-recovery", NodeID: dead.ID, Spec: domain.ServerSpec{Generation: 2, DesiredState: domain.DesiredRunning}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseRunning}}},
			}
			scheduler := &evictionFakeScheduler{targetNode: domain.ComputeNode{ID: "node-healthy"}}
			if !available {
				scheduler.err = errors.New("no nodes available")
			}
			controller := NewController(store, NewRuntimeReconciler(&fakeBuilder{}, nil), nil).WithScheduler(scheduler)
			for i := 0; i < 2; i++ {
				controller.RunOnce(context.Background())
				server := store.servers[0]
				if server.NodeID != dead.ID || server.Spec.Generation != 2 || server.Spec.DesiredState != domain.DesiredRunning {
					t.Fatalf("recovery changed execution intent: %+v", server)
				}
				if store.assignment == nil || store.assignment.NodeID != dead.ID {
					t.Fatalf("lost source assignment: %+v", store.assignment)
				}
				if server.Status.Phase != domain.PhaseReconciling || server.Status.ActualState != domain.ActualUnknown {
					t.Fatalf("unconfirmed runtime: %+v", server.Status)
				}
				found := false
				for _, condition := range server.Status.Conditions {
					if condition.Type == "RecoveryReady" && condition.Status == "False" && condition.Reason == "FencingAndCheckpointRequired" && condition.ObservedGeneration == 2 {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing recovery requirement: %+v", server.Status.Conditions)
				}
				for _, event := range store.activities {
					if event.Type == "server.failover" || event.Type == "server.evicted" {
						t.Fatalf("false recovery success: %+v", event)
					}
				}
			}
		})
	}
}

func TestNodeHealth_ExtendedStale_DesiredStopped_NotEvicted(t *testing.T) {
	now := time.Now().UTC()
	deadNode := domain.ComputeNode{
		ID:            "node-dead",
		Name:          "Dead Node",
		Status:        "offline",
		LastHeartbeat: now.Add(-3 * time.Minute),
	}
	store := &evictionTestStore{
		nodes: map[string]domain.ComputeNode{deadNode.ID: deadNode},
		servers: []domain.GameServer{{
			ID:     "server-stopped",
			NodeID: "node-dead",
			Name:   "Stopped Game",
			Spec:   domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped},
			Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped},
		}},
	}
	scheduler := &evictionFakeScheduler{targetNode: domain.ComputeNode{ID: "node-healthy"}}
	controller := NewController(store, NewRuntimeReconciler(&fakeBuilder{}, nil), nil).
		WithScheduler(scheduler).
		WithRecoveryTimeout(2 * time.Minute)

	controller.RunOnce(context.Background())

	server := store.servers[0]
	// DesiredStopped server does not need to be migrated or failed over
	if server.NodeID != "node-dead" {
		t.Fatalf("expected stopped server to remain on node-dead, got %s", server.NodeID)
	}
}

func TestRecoveryRequirementPersistsWithRealStore(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "recovery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	node := domain.ComputeNode{ID: "recovery-source", Status: "offline", LastHeartbeat: time.Now().Add(-3 * time.Minute)}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	instance := domain.GameServer{ID: "recovery-instance", NodeID: node.ID, Spec: domain.ServerSpec{Generation: 4, DesiredState: domain.DesiredRunning}}
	if err := db.CreateGameServer(ctx, &instance); err != nil {
		t.Fatal(err)
	}
	controller := NewController(db, NewRuntimeReconciler(&fakeBuilder{}, nil), nil).WithScheduler(&evictionFakeScheduler{targetNode: domain.ComputeNode{ID: "target"}})
	var uid string
	for i := 0; i < 2; i++ {
		controller.RunOnce(ctx)
		saved, err := db.GetGameServer(ctx, instance.ID)
		if err != nil {
			t.Fatal(err)
		}
		if saved.NodeID != node.ID || saved.Spec.Generation != 4 {
			t.Fatalf("changed source placement: %+v", saved)
		}
		assignment, err := db.GetWorkloadAssignmentByServer(ctx, instance.ID)
		if err != nil {
			t.Fatal(err)
		}
		if assignment.NodeID != node.ID || (uid != "" && assignment.UID != uid) {
			t.Fatalf("changed source assignment: %+v", assignment)
		}
		uid = assignment.UID
		found := false
		for _, condition := range saved.Status.Conditions {
			if condition.Type == "RecoveryReady" && condition.Status == "False" && condition.Reason == "FencingAndCheckpointRequired" {
				found = true
			}
		}
		if !found {
			t.Fatalf("recovery requirement not persisted: %+v", saved.Status)
		}
	}
}
