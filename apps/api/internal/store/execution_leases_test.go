package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestExecutionLeases(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "leases.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testExecutionLeases(t, db)
}

func testExecutionLeases(t *testing.T, db *Store) {
	ctx := context.Background()
	node := domain.ComputeNode{ID: "lease-node", Token: "lease-token"}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	server := domain.GameServer{ID: "lease-server", NodeID: node.ID, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning}}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	assignment := domain.WorkloadAssignment{ID: "lease-assignment", UID: "lease-uid", ServerID: server.ID, NodeID: node.ID, Generation: 1, DesiredState: domain.DesiredRunning}
	if err := db.PublishWorkloadAssignment(ctx, server, &assignment); err != nil {
		t.Fatal(err)
	}
	request := ExecutionLeaseRequest{NodeID: node.ID, NodeToken: node.Token, AssignmentUID: assignment.UID, HolderID: "process-a", Generation: 1}
	assertUnavailable := func(err error) {
		t.Helper()
		if !errors.Is(err, ErrExecutionLeaseUnavailable) {
			t.Fatalf("expected unavailable, got %v", err)
		}
	}
	bad := request
	bad.NodeToken = "wrong"
	_, err := db.AcquireExecutionLease(ctx, bad, time.Minute)
	assertUnavailable(err)
	bad = request
	bad.Generation = 2
	_, err = db.AcquireExecutionLease(ctx, bad, time.Minute)
	assertUnavailable(err)
	_, err = db.AcquireExecutionLease(ctx, request, 0)
	if err == nil {
		t.Fatal("zero TTL accepted")
	}
	// Two independent process incarnations cannot receive overlapping grants.
	type result struct {
		lease   ExecutionLease
		err     error
		request ExecutionLeaseRequest
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, holder := range []string{"process-a", "process-b"} {
		candidate := request
		candidate.HolderID = holder
		go func() {
			<-start
			lease, err := db.AcquireExecutionLease(ctx, candidate, time.Minute)
			results <- result{lease, err, candidate}
		}()
	}
	close(start)
	var winner result
	accepted := 0
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err == nil {
			accepted++
			winner = r
		} else {
			assertUnavailable(r.err)
		}
	}
	if accepted != 1 || winner.lease.Fence != 1 {
		t.Fatalf("claims: %d %+v", accepted, winner.lease)
	}
	request = winner.request
	_, err = db.AcquireExecutionLease(ctx, request, time.Minute)
	assertUnavailable(err)
	renewed, err := db.RenewExecutionLease(ctx, request, winner.lease.Fence, time.Second)
	if err != nil || renewed.ExpiresAtMS < winner.lease.ExpiresAtMS {
		t.Fatalf("renew shortened lease: %+v %v", renewed, err)
	}
	bad = request
	bad.HolderID = "other"
	_, err = db.RenewExecutionLease(ctx, bad, renewed.Fence, time.Minute)
	assertUnavailable(err)
	assertUnavailable(db.ReleaseExecutionLease(ctx, bad, renewed.Fence))
	if err := db.ReleaseExecutionLease(ctx, request, renewed.Fence); err != nil {
		t.Fatal(err)
	}
	_, err = db.RenewExecutionLease(ctx, request, renewed.Fence, time.Minute)
	assertUnavailable(err)
	next, err := db.AcquireExecutionLease(ctx, request, time.Minute)
	if err != nil || next.Fence != renewed.Fence+1 {
		t.Fatalf("fence did not advance: %+v %v", next, err)
	}
	assertUnavailable(db.ReleaseExecutionLease(ctx, request, renewed.Fence))
	// Configuration changes invalidate renewal before the next manifest is built.
	server.Spec.Generation++
	if err := db.db.Model(&domain.GameServer{}).Where("id = ?", server.ID).Select("spec").Updates(&server).Error; err != nil {
		t.Fatal(err)
	}
	_, err = db.RenewExecutionLease(ctx, request, next.Fence, time.Minute)
	assertUnavailable(err)
	assignment.Generation = 2
	if err := db.PublishWorkloadAssignment(ctx, server, &assignment); err != nil {
		t.Fatal(err)
	}
	request.Generation = 2
	_, err = db.AcquireExecutionLease(ctx, request, time.Minute)
	assertUnavailable(err)
	// Expire in the database without sleeps; an expired grant cannot be revived.
	if err := db.db.Model(&ExecutionLease{}).Where("server_id = ?", server.ID).UpdateColumn("expires_at_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	_, err = db.RenewExecutionLease(ctx, request, next.Fence, time.Minute)
	assertUnavailable(err)
	next, err = db.AcquireExecutionLease(ctx, request, time.Minute)
	if err != nil || next.Fence != 3 {
		t.Fatalf("new generation: %+v %v", next, err)
	}
	if err := db.db.Model(&domain.ComputeNode{}).Where("id = ?", node.ID).UpdateColumn("token", "rotated-lease-token").Error; err != nil {
		t.Fatal(err)
	}
	_, err = db.RenewExecutionLease(ctx, request, next.Fence, time.Minute)
	assertUnavailable(err)
	request.NodeToken = "rotated-lease-token"
	if err := db.DeleteWorkloadAssignment(ctx, server.ID); err != nil {
		t.Fatal(err)
	}
	_, err = db.RenewExecutionLease(ctx, request, next.Fence, time.Minute)
	assertUnavailable(err)
	// A new assignment UID must still wait out the old grant and preserve fencing.
	replacementNode := domain.ComputeNode{ID: "lease-replacement-node", Token: "replacement-lease-token"}
	if err := db.CreateComputeNode(ctx, &replacementNode); err != nil {
		t.Fatal(err)
	}
	server.NodeID = replacementNode.ID
	server.Spec.Generation++
	if err := db.db.Model(&domain.GameServer{}).Where("id = ?", server.ID).Select("node_id", "spec").Updates(&server).Error; err != nil {
		t.Fatal(err)
	}
	assignment.UID = "lease-replacement-uid"
	assignment.NodeID = replacementNode.ID
	assignment.Generation = server.Spec.Generation
	if err := db.PublishWorkloadAssignment(ctx, server, &assignment); err != nil {
		t.Fatal(err)
	}
	request.AssignmentUID = assignment.UID
	request.NodeID = replacementNode.ID
	request.NodeToken = replacementNode.Token
	request.Generation = assignment.Generation
	_, err = db.AcquireExecutionLease(ctx, request, time.Minute)
	assertUnavailable(err)
	if err := db.db.Model(&ExecutionLease{}).Where("server_id = ?", server.ID).UpdateColumn("expires_at_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	next, err = db.AcquireExecutionLease(ctx, request, time.Minute)
	if err != nil || next.Fence != 4 {
		t.Fatalf("replacement lost fence: %+v %v", next, err)
	}
	if err := db.DeleteComputeNode(ctx, replacementNode.ID); err != nil {
		t.Fatal(err)
	}
	_, err = db.RenewExecutionLease(ctx, request, next.Fence, time.Minute)
	assertUnavailable(err)
}
