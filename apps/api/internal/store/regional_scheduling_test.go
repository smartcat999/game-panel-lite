package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type fixtureSchedulingScope struct {
	scope regional.SchedulingScope
	err   error
}

func (f fixtureSchedulingScope) SchedulingScope(context.Context, regional.RevisionSnapshot) (regional.SchedulingScope, error) {
	return f.scope, f.err
}

type fixtureCurrentRevision struct {
	snapshot regional.RevisionSnapshot
	err      error
}

func (f fixtureCurrentRevision) GetRevision(context.Context, instances.RevisionAvailable) (regional.RevisionSnapshot, error) {
	return f.snapshot, f.err
}

func testSchedulingClaims(t *testing.T, db *RegionalStore, dsn string, scheduler regional.Scheduler, scope regional.SchedulingScope, first, second regional.Allocation) {
	t.Helper()
	ctx := context.Background()
	// Both fixture deployments have reservations but no durable scheduling
	// completion yet, representing a crash after reserve and before completion.
	var wg sync.WaitGroup
	claims := make(chan *regional.SchedulingClaim, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := db.ClaimScheduling(ctx, time.Minute)
			if err != nil {
				failures <- err
			}
			if c != nil {
				claims <- c
			}
		}()
	}
	wg.Wait()
	close(claims)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	byID := map[string]regional.SchedulingClaim{}
	for claim := range claims {
		if _, ok := byID[claim.Deployment.ID]; ok {
			t.Fatal("duplicate claim")
		}
		byID[claim.Deployment.ID] = *claim
	}
	if len(byID) != 2 {
		t.Fatalf("exclusive claims: %d", len(byID))
	}
	a, b := byID[first.DeploymentID], byID[second.DeploymentID]
	wrong := first
	wrong.MemoryMB++
	if err := db.CompleteScheduling(ctx, a, wrong); !errors.Is(err, regional.ErrAllocationConflict) {
		t.Fatal("forged receipt", err)
	}
	if err := db.db.Table("regional_deployments").Where("id = ?", a.Deployment.ID).Update("scheduling_until_ms", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScheduling(ctx, a, first); !errors.Is(err, regional.ErrSchedulingClaimLost) {
		t.Fatal("expired completion", err)
	}
	reopened, err := OpenRegionalPostgres(dsn, db.regionID, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err := reopened.ClaimScheduling(ctx, time.Minute)
	if err != nil || recovered == nil || recovered.Deployment.ID != a.Deployment.ID || recovered.Token == a.Token {
		t.Fatal("restart reclaim", err)
	}
	if err := db.RetryScheduling(ctx, a, time.Second); !errors.Is(err, regional.ErrSchedulingClaimLost) {
		t.Fatal("old claimant retry", err)
	}
	// Force the final scheduling write to fail after the Node task insert.
	if err := db.db.Exec("ALTER TABLE regional_deployments ADD CONSTRAINT test_completion_failure CHECK (scheduling_status <> 'reserved')").Error; err != nil {
		t.Fatal(err)
	}
	if err := reopened.CompleteScheduling(ctx, *recovered, first); err == nil {
		t.Fatal("injected completion failure ignored")
	}
	var tasksAfterFailure int64
	if err := db.db.Table("regional_node_tasks").Where("allocation_id = ?", first.ID).Count(&tasksAfterFailure).Error; err != nil || tasksAfterFailure != 0 {
		t.Fatal("failed scheduling left node task", err)
	}
	if err := db.db.Exec("ALTER TABLE regional_deployments DROP CONSTRAINT test_completion_failure").Error; err != nil {
		t.Fatal(err)
	}
	if err := reopened.CompleteScheduling(ctx, *recovered, first); err != nil {
		t.Fatal(err)
	}
	if err := db.RetryScheduling(ctx, b, time.Hour); err != nil {
		t.Fatal(err)
	}
	if claim, err := db.ClaimScheduling(ctx, time.Minute); err != nil || claim != nil {
		t.Fatal("retry delay ignored", err)
	}
	if err := db.db.Table("regional_deployments").Where("id = ?", b.Deployment.ID).Update("scheduling_next_ms", 0).Error; err != nil {
		t.Fatal(err)
	}
	scheduler.Resources = reopened
	worker := regional.SchedulingWorker{Source: fixtureCurrentRevision{snapshot: b.Snapshot}, Tasks: reopened, Scheduler: scheduler, Scopes: fixtureSchedulingScope{scope: scope}, Lease: time.Minute, Timeout: 10 * time.Second, RetryDelay: time.Hour}
	// Authorization failure persists retry without modifying the reservation.
	worker.Scopes = fixtureSchedulingScope{err: errors.New("denied")}
	if done, err := worker.RunOnce(ctx); done || err == nil {
		t.Fatal("scope failure completed")
	}
	if claim, err := db.ClaimScheduling(ctx, time.Minute); err != nil || claim != nil {
		t.Fatal("worker retry not durable", err)
	}
	if err := db.db.Table("regional_deployments").Where("id = ?", b.Deployment.ID).Update("scheduling_next_ms", 0).Error; err != nil {
		t.Fatal(err)
	}
	worker.Scopes = fixtureSchedulingScope{scope: scope}
	if done, err := worker.RunOnce(ctx); !done || err != nil {
		t.Fatal("worker receipt recovery", err)
	}
	if done, err := worker.RunOnce(ctx); done || err != nil {
		t.Fatal("completed task reclaimed", err)
	}
	var nodeTasks []regional.NodeTask
	if err := db.db.Table("regional_node_tasks").Order("id").Find(&nodeTasks).Error; err != nil || len(nodeTasks) != 2 {
		t.Fatal("missing or duplicate node tasks", err)
	}
	for _, task := range nodeTasks {
		expected := first
		if task.AllocationID == second.ID {
			expected = second
		}
		if task.ID != expected.ID || task.CapacityRequest != expected.CapacityRequest || task.Kind != "run" || task.Status != "awaiting_authority" {
			t.Fatal("incorrect node task binding")
		}
	}
	if err := db.db.Transaction(func(tx *gorm.DB) error { return stageRegionalNodeTask(tx, first) }); err != nil {
		t.Fatal("duplicate staging changed task", err)
	}
	// New intent invalidates an old claim and queues only running deployments.
	update := a.Snapshot
	update.IntentVersion++
	if err := db.db.Transaction(func(tx *gorm.DB) error { return stageRegionalDeployment(tx, update) }); err != nil {
		t.Fatal(err)
	}
	active, err := db.ClaimScheduling(ctx, time.Minute)
	if err != nil || active == nil {
		t.Fatal("new intent not claimed", err)
	}
	update.IntentVersion++
	update.DesiredState = "stopped"
	if err := db.db.Transaction(func(tx *gorm.DB) error { return stageRegionalDeployment(tx, update) }); err != nil {
		t.Fatal(err)
	}
	if err := db.RetryScheduling(ctx, *active, time.Second); !errors.Is(err, regional.ErrSchedulingClaimLost) {
		t.Fatal("superseded intent claim accepted", err)
	}
	var state regionalSchedulingRow
	if err := db.db.Table("regional_deployments").Where("id = ?", a.Deployment.ID).Take(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.SchedulingStatus != "pending" || state.SchedulingToken != "" || state.IntentVersion != update.IntentVersion {
		t.Fatal("intent did not invalidate scheduling")
	}
	if c, err := db.ClaimScheduling(ctx, time.Minute); c != nil || err != nil {
		t.Fatal("stopped deployment claimed", err)
	}
	var obsolete regional.NodeTask
	if err := db.db.Table("regional_node_tasks").Where("allocation_id = ?", first.ID).Take(&obsolete).Error; err != nil || obsolete.Status != "superseded" {
		t.Fatal("old run task remains eligible", err)
	}
	if err := db.db.Transaction(func(tx *gorm.DB) error { return stageRegionalNodeTask(tx, first) }); !errors.Is(err, regional.ErrNodeTaskConflict) {
		t.Fatal("superseded run task revived", err)
	}
	var count int64
	if err := db.db.Table("regional_allocations").Where("deployment_id IN ?", []string{first.DeploymentID, second.DeploymentID}).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("claim lifecycle changed reservations", err)
	}
	t.Log("PostgreSQL scheduling claims: concurrent exclusion, expiry/reopen, durable retry, worker replay and intent invalidation verified")
}
