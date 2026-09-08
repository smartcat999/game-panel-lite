package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestNodePortRelease(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "release.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testNodePortRelease(t, db)
}
func testNodePortRelease(t *testing.T, db *Store) {
	ctx := context.Background()
	node := domain.ComputeNode{ID: "port-release-node", Token: "port-release-token"}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	server := domain.GameServer{ID: "port-release-server", NodeID: node.ID, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	a := domain.WorkloadAssignment{ID: server.ID, UID: server.ID, ServerID: server.ID, NodeID: node.ID, Generation: 1, DesiredState: domain.DesiredRunning, Spec: domain.WorkloadSpec{Network: domain.WorkloadNetwork{Port: 7777, HostPort: 19000}}}
	if err := db.PublishWorkloadAssignment(ctx, server, &a); err != nil {
		t.Fatal(err)
	}
	assertClaims := func(want int64) {
		t.Helper()
		var count int64
		if err := db.db.Model(&nodePortReservation{}).Where("server_id = ?", server.ID).Count(&count).Error; err != nil || count != want {
			t.Fatalf("claims=%d want=%d err=%v", count, want, err)
		}
	}
	report := domain.WorkloadObservation{AssignmentUID: a.UID, NodeID: node.ID, ServerID: server.ID, ObservedGeneration: 1, ActualState: domain.ActualMissing}
	request := ExecutionLeaseRequest{AssignmentUID: a.UID, NodeID: node.ID, NodeToken: node.Token, HolderID: "port-owner", Generation: 1}
	lease, err := db.AcquireExecutionLease(ctx, request, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAgentWorkloadObservation(ctx, request, lease.Fence, &report); err != nil {
		t.Fatal(err)
	}
	assertClaims(1) // Missing while desired-running is not deletion confirmation.
	if err := db.ReleaseExecutionLease(ctx, request, lease.Fence); err != nil {
		t.Fatal(err)
	}
	server.Spec.Generation++
	server.Spec.DesiredState = domain.DesiredDeleted
	if err := db.SaveGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	a.Generation++
	a.DesiredState = domain.DesiredDeleted
	if err := db.PublishWorkloadAssignment(ctx, server, &a); err != nil {
		t.Fatal(err)
	}
	request.Generation++
	lease, err = db.AcquireExecutionLease(ctx, request, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	report.ObservedGeneration = 2
	report.ObservationToken = lease.ObservationToken
	bad := request
	bad.HolderID = "stale-owner"
	if err := db.SaveAgentWorkloadObservation(ctx, bad, lease.Fence, &report); !errors.Is(err, ErrExecutionLeaseUnavailable) {
		t.Fatalf("stale authority accepted: %v", err)
	}
	assertClaims(1)
	report.LastError = "runtime deletion failed"
	if err := db.SaveAgentWorkloadObservation(ctx, request, lease.Fence, &report); err != nil {
		t.Fatal(err)
	}
	assertClaims(1)
	report.LastError = ""
	report.ObservationToken = "stale-observation"
	if err := db.SaveAgentWorkloadObservation(ctx, request, lease.Fence, &report); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("stale observation accepted: %v", err)
	}
	assertClaims(1)
	current, err := db.GetWorkloadObservation(ctx, a.UID)
	if err != nil {
		t.Fatal(err)
	}
	report.ObservationToken = current.ID
	report.RuntimeID = "container-still-reported"
	if err := db.SaveAgentWorkloadObservation(ctx, request, lease.Fence, &report); err != nil {
		t.Fatal(err)
	}
	assertClaims(1) // Contradictory runtime evidence must retain the reservation.
	current, err = db.GetWorkloadObservation(ctx, a.UID)
	if err != nil {
		t.Fatal(err)
	}
	report.ObservationToken = current.ID
	report.RuntimeID = ""
	if err := db.SaveAgentWorkloadObservation(ctx, request, lease.Fence, &report); err != nil {
		t.Fatal(err)
	}
	assertClaims(0)
}
