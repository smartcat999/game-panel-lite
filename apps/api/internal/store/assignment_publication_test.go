package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestAssignmentPublication(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "assignments.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testAssignmentPublication(t, db)
}

func testAssignmentPublication(t *testing.T, db *Store) {
	ctx := context.Background()
	before := domain.GameServer{ID: "publication-server", NodeID: "publication-node", Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
	if err := db.CreateGameServer(ctx, &before); err != nil {
		t.Fatal(err)
	}
	assignmentFor := func(i int) domain.WorkloadAssignment {
		return domain.WorkloadAssignment{ID: fmt.Sprintf("publication-%d", i), UID: fmt.Sprintf("publication-uid-%d", i), ServerID: before.ID, NodeID: before.NodeID, Generation: 1, DesiredState: domain.DesiredRunning, Spec: domain.WorkloadSpec{ServerID: before.ID, Image: "image:v1"}}
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			assignment := assignmentFor(i)
			results <- db.PublishWorkloadAssignment(ctx, before, &assignment)
		}(i)
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrReconciliationSuperseded) {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatalf("published %d competing identities, want 1", accepted)
	}
	published, err := db.GetWorkloadAssignmentByServer(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := published
	if err := db.PublishWorkloadAssignment(ctx, before, &duplicate); err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	changed := published
	changed.Spec.Image = "unexpected"
	if err := db.PublishWorkloadAssignment(ctx, before, &changed); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("same generation changed content: %v", err)
	}
	current := before
	current.Spec.Generation = 8
	if err := db.SaveGameServer(ctx, &current); err != nil {
		t.Fatal(err)
	}
	if err := db.PublishWorkloadAssignment(ctx, before, &published); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale publication: %v", err)
	}
	newer := published
	newer.Generation = 8
	newer.Spec.Image = "image:v8"
	if err := db.PublishWorkloadAssignment(ctx, current, &newer); err != nil {
		t.Fatal(err)
	}
	if newer.UID != published.UID || newer.ID != published.ID {
		t.Fatal("same-node publication changed identity")
	}

	reports := make(chan error, 8)
	for generation := 1; generation <= 8; generation++ {
		wg.Add(1)
		go func(generation int) {
			defer wg.Done()
			observation := domain.WorkloadObservation{ID: fmt.Sprintf("publication-report-%d", generation), AssignmentUID: newer.UID, ServerID: current.ID, NodeID: current.NodeID, ObservedGeneration: generation, RuntimeID: fmt.Sprintf("runtime-%d", generation)}
			var result error
			for attempt := 0; attempt < 16; attempt++ {
				observed, readErr := db.GetWorkloadObservation(ctx, newer.UID)
				if readErr != nil && !errors.Is(readErr, ErrNotFound) {
					result = readErr
					break
				}
				observation.ObservationToken = observed.ID
				result = db.UpsertWorkloadObservation(ctx, &observation)
				if !errors.Is(result, ErrReconciliationSuperseded) {
					break
				}
			}
			reports <- result
		}(generation)
	}
	wg.Wait()
	close(reports)
	for err := range reports {
		if err != nil {
			t.Fatal(err)
		}
	}
	latest, err := db.GetWorkloadObservation(ctx, newer.UID)
	if err != nil || latest.ObservedGeneration != 8 || latest.RuntimeID != "runtime-8" {
		t.Fatalf("report generation rolled back: %+v %v", latest, err)
	}

	next := latest
	next.ObservationToken = latest.ID
	next.ActualState = domain.ActualStopped
	if err := db.UpsertWorkloadObservation(ctx, &next); err != nil {
		t.Fatal(err)
	}
	stale := latest
	stale.ObservationToken = latest.ID
	stale.ActualState = domain.ActualRunning
	if err := db.UpsertWorkloadObservation(ctx, &stale); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale same-generation report: %v", err)
	}
	saved, err := db.GetWorkloadObservation(ctx, newer.UID)
	if err != nil || saved.ActualState != domain.ActualStopped || saved.ID == latest.ID {
		t.Fatalf("token did not protect latest report: %+v %v", saved, err)
	}
	latest = saved
	for _, invalid := range []string{"node", "server", "uid", "future", "negative"} {
		report := latest
		switch invalid {
		case "node":
			report.NodeID = "other"
		case "server":
			report.ServerID = "other"
		case "uid":
			report.AssignmentUID = "other"
		case "future":
			report.ObservedGeneration = 9
		case "negative":
			report.ObservedGeneration = -1
		}
		if err := db.UpsertWorkloadObservation(ctx, &report); !errors.Is(err, ErrReconciliationSuperseded) {
			t.Fatalf("accepted invalid %s report: %v", invalid, err)
		}
	}

	current.NodeID = "publication-next-node"
	current.Spec.Generation = 9
	if err := db.SaveGameServer(ctx, &current); err != nil {
		t.Fatal(err)
	}
	moved := newer
	moved.NodeID = current.NodeID
	moved.Generation = current.Spec.Generation
	moved.UID = "publication-next-uid"
	if err := db.PublishWorkloadAssignment(ctx, current, &moved); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertWorkloadObservation(ctx, &latest); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("old placement report accepted: %v", err)
	}
	newer = moved
	latest.AssignmentUID = moved.UID
	latest.NodeID = moved.NodeID
	if err := db.DeleteWorkloadAssignment(ctx, current.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertWorkloadObservation(ctx, &latest); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("report accepted after assignment deletion: %v", err)
	}
	if err := db.DeleteGameServer(ctx, current.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.PublishWorkloadAssignment(ctx, current, &newer); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("assignment resurrected deleted instance: %v", err)
	}
}
