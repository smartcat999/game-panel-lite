package server

import (
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestReconciliationActivityEventsForRuntimeStart(t *testing.T) {
	now := time.Unix(1000, 0)
	before := domain.GameServer{
		ID:          "server-1",
		Name:        "Friends",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec:        domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning},
		Status: domain.ServerRuntimeStatus{
			Phase:       domain.PhasePending,
			ActualState: domain.ActualMissing,
		},
	}
	after := before
	after.Status.Phase = domain.PhaseRunning
	after.Status.ActualState = domain.ActualRunning
	after.Status.RuntimeID = "runtime-1"
	after.Status.ObservedGeneration = 1
	after.Status.AppliedGeneration = 1

	events := reconciliationActivityEvents(before, after, now, nil, "operation-1")
	if len(events) != 2 {
		t.Fatalf("expected runtime created and server started events, got %+v", events)
	}
	if events[0].Type != "server.runtime.created" || events[1].Type != "server.started" {
		t.Fatalf("unexpected event types: %+v", events)
	}
	if events[0].Payload["serverName"] != "Friends" || events[0].Payload["runtimeId"] != "runtime-1" {
		t.Fatalf("expected structured server payload, got %+v", events[0].Payload)
	}
	if events[0].Payload["operationId"] != "operation-1" || events[1].Payload["operationId"] != "operation-1" {
		t.Fatalf("expected operation id payload, got %+v %+v", events[0].Payload, events[1].Payload)
	}
}

func TestReconciliationActivityEventsSkipsSummaryWhenLifecycleAlreadyRecorded(t *testing.T) {
	now := time.Unix(1000, 0)
	before := domain.GameServer{
		ID:          "server-1",
		Name:        "Friends",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec:        domain.ServerSpec{Generation: 2, DesiredState: domain.DesiredRunning},
		Status: domain.ServerRuntimeStatus{
			Phase:             domain.PhaseRunning,
			ActualState:       domain.ActualRunning,
			RuntimeID:         "runtime-old",
			AppliedGeneration: 1,
		},
	}
	after := before
	after.Status.Phase = domain.PhaseRunning
	after.Status.RuntimeID = "runtime-new"
	after.Status.AppliedGeneration = 2

	events := reconciliationActivityEvents(before, after, now, []LifecycleEvent{
		{Type: "server.container.remove.succeeded"},
		{Type: "server.container.create.succeeded"},
		{Type: "server.container.start.succeeded"},
	}, "operation-1")
	if len(events) != 0 {
		t.Fatalf("expected no duplicate summary events when lifecycle events exist, got %+v", events)
	}
}

func TestReconciliationLifecycleActivityEventsIncludeRuntimeDetails(t *testing.T) {
	now := time.Unix(1000, 0)
	occurredAt := now.Add(5 * time.Second)
	server := domain.GameServer{
		ID:          "server-1",
		Name:        "Friends",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec:        domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning},
		Status: domain.ServerRuntimeStatus{
			Phase:       domain.PhaseRunning,
			ActualState: domain.ActualRunning,
			RuntimeID:   "runtime-1",
		},
	}
	events := reconciliationLifecycleActivityEvents(server, []LifecycleEvent{{
		Type:       "server.container.start.failed",
		Message:    "Start runtime container failed for server Friends: boom",
		OccurredAt: occurredAt,
		Payload: map[string]any{
			"runtimeId": "runtime-1",
			"error":     "boom",
		},
	}}, now, "operation-1")
	if len(events) != 1 {
		t.Fatalf("expected one lifecycle event, got %+v", events)
	}
	if events[0].Type != "server.container.start.failed" {
		t.Fatalf("unexpected event type: %+v", events[0])
	}
	if !events[0].CreatedAt.Equal(occurredAt) {
		t.Fatalf("expected lifecycle event occurrence time, got %s", events[0].CreatedAt)
	}
	if events[0].Payload["serverName"] != "Friends" || events[0].Payload["runtimeId"] != "runtime-1" || events[0].Payload["error"] != "boom" || events[0].Payload["operationId"] != "operation-1" {
		t.Fatalf("expected merged lifecycle payload, got %+v", events[0].Payload)
	}
}

func TestReconciliationActivityEventsSkipsInitialStoppedConvergence(t *testing.T) {
	now := time.Unix(1000, 0)
	before := domain.GameServer{
		ID:          "server-1",
		Name:        "Friends",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec:        domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped},
		Status: domain.ServerRuntimeStatus{
			Phase:       domain.PhasePending,
			ActualState: domain.ActualMissing,
		},
	}
	after := before
	after.Status.Phase = domain.PhaseStopped
	after.Status.ObservedGeneration = 1

	events := reconciliationActivityEvents(before, after, now, nil, "operation-1")
	if len(events) != 0 {
		t.Fatalf("expected no stopped event for initial stopped convergence, got %+v", events)
	}
}

func TestReconciliationActivityEventsForFailure(t *testing.T) {
	now := time.Unix(1000, 0)
	before := domain.GameServer{
		ID:          "server-1",
		Name:        "Friends",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec:        domain.ServerSpec{Generation: 2, DesiredState: domain.DesiredRunning},
		Status:      domain.ServerRuntimeStatus{Phase: domain.PhasePending},
	}
	after := before
	after.Status.Phase = domain.PhaseFailed
	after.Status.LastError = "bad config"

	events := reconciliationActivityEvents(before, after, now, nil, "operation-1")
	if len(events) != 1 {
		t.Fatalf("expected one failure event, got %+v", events)
	}
	if events[0].Type != "server.reconcile.failed" {
		t.Fatalf("expected reconcile failure event, got %q", events[0].Type)
	}
	if events[0].Payload["lastError"] != "bad config" {
		t.Fatalf("expected failure payload, got %+v", events[0].Payload)
	}
}
