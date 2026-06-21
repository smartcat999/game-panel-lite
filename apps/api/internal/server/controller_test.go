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

	events := reconciliationActivityEvents(before, after, now)
	if len(events) != 2 {
		t.Fatalf("expected runtime created and server started events, got %+v", events)
	}
	if events[0].Type != "server.runtime.created" || events[1].Type != "server.started" {
		t.Fatalf("unexpected event types: %+v", events)
	}
	if events[0].Payload["serverName"] != "Friends" || events[0].Payload["runtimeId"] != "runtime-1" {
		t.Fatalf("expected structured server payload, got %+v", events[0].Payload)
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

	events := reconciliationActivityEvents(before, after, now)
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
