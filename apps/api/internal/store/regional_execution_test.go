package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func testRegionalExecution(t *testing.T, db *RegionalStore, dsn string, snapshot regional.RevisionSnapshot, allocation regional.Allocation) {
	t.Helper()
	ctx := context.Background()
	candidate, err := db.NextExecutionCandidate(ctx, allocation.NodeID, allocation.SessionEpoch)
	if err != nil || candidate == nil || candidate.Allocation.ID != allocation.ID || candidate.Snapshot.Event != snapshot.Event {
		t.Fatal("execution candidate", err)
	}
	now, err := outboxNow(db.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.db.Table("regional_node_sessions").Where("node_id = ?", allocation.NodeID).Update("last_seen_ms", now-time.Minute.Milliseconds()-1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := db.AcquireRegionalExecutionLease(ctx, *candidate, "stale-heartbeat", time.Minute, time.Minute); !errors.Is(err, regional.ErrExecutionUnavailable) {
		t.Fatal("stale heartbeat obtained execution authority", err)
	}
	if err := db.db.Table("regional_node_sessions").Where("node_id = ?", allocation.NodeID).Update("last_seen_ms", now).Error; err != nil {
		t.Fatal(err)
	}

	type acquisition struct {
		holder string
		lease  regional.ExecutionLease
		err    error
	}
	start := make(chan struct{})
	results := make(chan acquisition, 8)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			holder := "regional-worker-" + string(rune('a'+index))
			<-start
			lease, err := db.AcquireRegionalExecutionLease(ctx, *candidate, holder, time.Minute, time.Minute)
			results <- acquisition{holder: holder, lease: lease, err: err}
		}(i)
	}
	close(start)
	workers.Wait()
	close(results)
	var winner acquisition
	succeeded := 0
	for result := range results {
		if result.err == nil {
			winner = result
			succeeded++
		} else if !errors.Is(result.err, regional.ErrExecutionLeaseLost) {
			t.Fatal(result.err)
		}
	}
	if succeeded != 1 || winner.lease.Fence != 1 || winner.lease.TaskID != candidate.Task.ID {
		t.Fatalf("execution lease was not exclusive: %d %+v", succeeded, winner)
	}

	reopened, err := OpenRegionalPostgres(dsn, db.regionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	current, err := reopened.ExecutionCandidate(ctx, candidate.Task.ID, allocation.NodeID, allocation.SessionEpoch)
	if err != nil {
		t.Fatal("candidate after restart", err)
	}
	renewed, err := reopened.RenewRegionalExecutionLease(ctx, current, winner.holder, winner.lease.Fence, time.Minute, time.Minute)
	if err != nil || renewed.Fence != winner.lease.Fence || renewed.ExpiresAtMS <= renewed.GrantedAtMS {
		t.Fatal("renew after restart", err)
	}

	if err := db.db.Table("regional_execution_leases").Where("server_id = ?", candidate.Task.ServerID).Update("expires_at_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.RenewRegionalExecutionLease(ctx, current, winner.holder, renewed.Fence, time.Minute, time.Minute); !errors.Is(err, regional.ErrExecutionLeaseLost) {
		t.Fatal("expired fence renewed", err)
	}
	next, err := reopened.AcquireRegionalExecutionLease(ctx, current, "regional-worker-next", time.Minute, time.Minute)
	if err != nil || next.Fence != winner.lease.Fence+1 {
		t.Fatal("expired lease did not advance fence", err)
	}

	observation := workload.Observation{LeaseHolderID: next.HolderID, LeaseFence: next.Fence, ObservationToken: next.ObservationToken, ObservedGeneration: next.Generation, RuntimeID: "container-a", ActualState: "running", ObservedAt: time.Now().UTC()}
	if err := reopened.SaveRegionalExecutionObservation(ctx, current, next.HolderID, next.Fence, observation); err != nil {
		t.Fatal("save observation", err)
	}
	if err := reopened.SaveRegionalExecutionObservation(ctx, current, next.HolderID, next.Fence, observation); err != nil {
		t.Fatal("idempotent observation replay", err)
	}
	changed := observation
	changed.RuntimeID = "different"
	if err := reopened.SaveRegionalExecutionObservation(ctx, current, next.HolderID, next.Fence, changed); !errors.Is(err, regional.ErrExecutionLeaseLost) {
		t.Fatal("conflicting observation replay", err)
	}
	if err := reopened.ReleaseRegionalExecutionLease(ctx, current, next.HolderID, next.Fence); err != nil {
		t.Fatal("release successful execution", err)
	}
	if _, err := reopened.ExecutionCandidate(ctx, current.Task.ID, allocation.NodeID, allocation.SessionEpoch); !errors.Is(err, regional.ErrExecutionUnavailable) {
		t.Fatal("completed task remained executable", err)
	}
	var status string
	if err := db.db.Table("regional_node_tasks").Select("status").Where("id = ?", current.Task.ID).Scan(&status).Error; err != nil || status != "succeeded" {
		t.Fatal("successful execution not persisted", err)
	}
	t.Log("regional execution: concurrent acquire, restart renewal, expiry fencing, observation CAS and completion verified")
}
