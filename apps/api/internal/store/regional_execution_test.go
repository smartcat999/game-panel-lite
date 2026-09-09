package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/deploymentstatus"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"gorm.io/gorm"
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
	const callback = "test:deployment-status-outbox-failure"
	if err := reopened.db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "regional_deployment_status_outbox" {
			tx.AddError(errors.New("injected status outbox failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failed := reopened.SaveRegionalExecutionObservation(ctx, current, next.HolderID, next.Fence, observation)
	_ = reopened.db.Callback().Create().Remove(callback)
	if failed == nil {
		t.Fatal("status outbox failure did not roll back observation")
	}
	for _, table := range []string{"regional_workload_observations", "regional_deployment_status_outbox"} {
		var count int64
		if err := reopened.db.Table(table).Where("task_id = ?", current.Task.ID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("%s partial observation: %d %v", table, count, err)
		}
	}
	if err := reopened.SaveRegionalExecutionObservation(ctx, current, next.HolderID, next.Fence, observation); err != nil {
		t.Fatal("save observation", err)
	}
	messages, err := reopened.DeploymentStatusOutbox().ClaimOutbox(ctx, reopened.regionID, 2, time.Minute)
	if err != nil || len(messages) != 1 || messages[0].Type != "deployment.status.observed" {
		t.Fatal("deployment status outbox", err)
	}
	var statusEvent deploymentstatus.Event
	if json.Unmarshal([]byte(messages[0].Payload), &statusEvent) != nil || statusEvent.Validate() != nil || statusEvent.ServerID != current.Task.ServerID || statusEvent.TaskID != current.Task.ID || statusEvent.Fence != next.Fence || statusEvent.Outcome != "succeeded" || statusEvent.ActualState != "running" {
		t.Fatal("invalid deployment status event")
	}
	if err := reopened.SaveRegionalExecutionObservation(ctx, current, next.HolderID, next.Fence, observation); err != nil {
		t.Fatal("idempotent observation replay", err)
	}
	var events int64
	if err := reopened.db.Table("regional_deployment_status_outbox").Where("task_id = ?", current.Task.ID).Count(&events).Error; err != nil || events != 1 {
		t.Fatal("idempotent observation duplicated status event", err)
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
