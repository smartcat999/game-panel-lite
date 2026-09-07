package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestLeasedObservations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "reports.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testLeasedObservations(t, db)
}
func testLeasedObservations(t *testing.T, db *Store) {
	ctx := context.Background()
	node := domain.ComputeNode{ID: "lease-report-node", Token: "lease-report-token"}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	server := domain.GameServer{ID: "lease-report-server", NodeID: node.ID, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	assignment := domain.WorkloadAssignment{ID: "lease-report-assignment", UID: "lease-report-uid", ServerID: server.ID, NodeID: node.ID, Generation: 1, DesiredState: domain.DesiredRunning}
	if err := db.PublishWorkloadAssignment(ctx, server, &assignment); err != nil {
		t.Fatal(err)
	}
	request := ExecutionLeaseRequest{NodeID: node.ID, NodeToken: node.Token, AssignmentUID: assignment.UID, HolderID: "reporter", Generation: 1}
	report := domain.WorkloadObservation{AssignmentUID: assignment.UID, ServerID: server.ID, NodeID: node.ID, ObservedGeneration: 1, ActualState: domain.ActualRunning}
	unavailable := func(err error) {
		t.Helper()
		if !errors.Is(err, ErrExecutionLeaseUnavailable) {
			t.Fatalf("expected unavailable: %v", err)
		}
	}
	unavailable(db.SaveAgentWorkloadObservation(ctx, request, 1, &report))
	lease, err := db.AcquireExecutionLease(ctx, request, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAgentWorkloadObservation(ctx, request, lease.Fence, &report); err != nil {
		t.Fatal(err)
	}
	current, err := db.GetWorkloadObservation(ctx, assignment.UID)
	if err != nil {
		t.Fatal(err)
	}
	report.ObservationToken = current.ID
	bad := request
	bad.HolderID = "other"
	unavailable(db.SaveAgentWorkloadObservation(ctx, bad, lease.Fence, &report))
	bad = request
	bad.NodeToken = "wrong"
	unavailable(db.SaveAgentWorkloadObservation(ctx, bad, lease.Fence, &report))
	var after ExecutionLease
	if err := db.db.First(&after, "server_id = ?", server.ID).Error; err != nil || after.ExpiresAtMS != lease.ExpiresAtMS {
		t.Fatalf("report renewed lease: %+v %v", after, err)
	}
	if err := db.ReleaseExecutionLease(ctx, request, lease.Fence); err != nil {
		t.Fatal(err)
	}
	unavailable(db.SaveAgentWorkloadObservation(ctx, request, lease.Fence, &report))
	next, err := db.AcquireExecutionLease(ctx, request, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	unavailable(db.SaveAgentWorkloadObservation(ctx, request, lease.Fence, &report))
	if err := db.SaveAgentWorkloadObservation(ctx, request, next.Fence, &report); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAgentWorkloadObservation(ctx, request, next.Fence, &report); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("replayed report accepted: %v", err)
	}
	current, err = db.GetWorkloadObservation(ctx, assignment.UID)
	if err != nil {
		t.Fatal(err)
	}
	report.ObservationToken = current.ID
	if err := db.db.Model(&ExecutionLease{}).Where("server_id = ?", server.ID).UpdateColumn("expires_at_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	unavailable(db.SaveAgentWorkloadObservation(ctx, request, next.Fence, &report))
	next, err = db.AcquireExecutionLease(ctx, request, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// Whichever operation obtains the transaction locks first defines the result.
	// A report after release must always fail and never replace the accepted row.
	start := make(chan struct{})
	saved, released := make(chan error, 1), make(chan error, 1)
	go func() { <-start; saved <- db.SaveAgentWorkloadObservation(ctx, request, next.Fence, &report) }()
	go func() { <-start; released <- db.ReleaseExecutionLease(ctx, request, next.Fence) }()
	close(start)
	if err := <-saved; err != nil {
		unavailable(err)
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	current, err = db.GetWorkloadObservation(ctx, assignment.UID)
	if err != nil {
		t.Fatal(err)
	}
	report.ObservationToken = current.ID
	unavailable(db.SaveAgentWorkloadObservation(ctx, request, next.Fence, &report))
	active, err := db.AcquireExecutionLease(ctx, request, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	server.Spec.Generation++
	if err := db.db.Model(&domain.GameServer{}).Where("id = ?", server.ID).Select("spec").Updates(&server).Error; err != nil {
		t.Fatal(err)
	}
	unavailable(db.SaveAgentWorkloadObservation(ctx, request, active.Fence, &report))
	unchanged, err := db.GetWorkloadObservation(ctx, assignment.UID)
	if err != nil || unchanged.ID != current.ID {
		t.Fatalf("rejected report changed state: %v", err)
	}
}
