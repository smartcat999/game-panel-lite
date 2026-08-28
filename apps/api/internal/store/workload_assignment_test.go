package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestWorkloadAssignmentAndObservationRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "gamepanel.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	assignment := domain.WorkloadAssignment{
		ID:           "assignment-1",
		UID:          "uid-1",
		ServerID:     "server-1",
		NodeID:       "node-1",
		Generation:   3,
		DesiredState: domain.DesiredRunning,
		Spec: domain.WorkloadSpec{
			ServerID: "server-1",
			Image:    "example/game:1",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.UpsertWorkloadAssignment(ctx, &assignment); err != nil {
		t.Fatalf("upsert assignment: %v", err)
	}

	assignments, err := db.ListWorkloadAssignmentsByNode(ctx, "node-1")
	if err != nil || len(assignments) != 1 {
		t.Fatalf("list assignments: %v, %+v", err, assignments)
	}
	if assignments[0].Spec.Image != "example/game:1" || assignments[0].Generation != 3 {
		t.Fatalf("unexpected assignment: %+v", assignments[0])
	}

	observation := domain.WorkloadObservation{
		ID:                 "observation-1",
		AssignmentUID:      assignment.UID,
		ServerID:           assignment.ServerID,
		NodeID:             assignment.NodeID,
		ObservedGeneration: assignment.Generation,
		RuntimeID:          "container-1",
		ActualState:        domain.ActualRunning,
		ObservedAt:         time.Now().UTC(),
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	if err := db.UpsertWorkloadObservation(ctx, &observation); err != nil {
		t.Fatalf("upsert observation: %v", err)
	}
	stored, err := db.GetWorkloadObservation(ctx, assignment.UID)
	if err != nil {
		t.Fatalf("get observation: %v", err)
	}
	if stored.ActualState != domain.ActualRunning || stored.ObservedGeneration != assignment.Generation {
		t.Fatalf("unexpected observation: %+v", stored)
	}
}
