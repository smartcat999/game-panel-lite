package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
)

func testGlobalBackupTasks(t *testing.T, db *Store, created instances.IntentResult) {
	t.Helper()
	ctx := context.Background()
	request := backup.Request{OrganizationID: created.Server.OrganizationID, ServerID: created.Server.ID, Scope: "world", IdempotencyKey: "backup-request-one"}
	for _, actor := range []string{"", "unrelated-user"} {
		if _, err := db.RequestGlobalBackup(ctx, actor, request); !errors.Is(err, ErrWorkspaceWriteDenied) {
			t.Fatalf("unauthorized backup %v", err)
		}
	}
	other := domain.Organization{ID: "backup-other", Slug: "backup-other"}
	if err := db.CreateOrganization(ctx, &other, "backup-other-owner"); err != nil {
		t.Fatal(err)
	}
	foreign := request
	foreign.OrganizationID = other.ID
	if _, err := db.RequestGlobalBackup(ctx, "backup-other-owner", foreign); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign instance backup accepted")
	}
	invalid := request
	invalid.Scope = "host-path"
	if _, err := db.RequestGlobalBackup(ctx, "global-owner", invalid); !errors.Is(err, backup.ErrInvalidRequest) {
		t.Fatal("invalid scope accepted")
	}
	var wg sync.WaitGroup
	results := make(chan backup.Task, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, err := db.RequestGlobalBackup(ctx, "global-owner", request)
			results <- task
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var task backup.Task
	for result := range results {
		if task.Request.BackupID != "" && task != result {
			t.Fatal("duplicate backup operations")
		}
		task = result
	}
	event := task.Request
	if task.Status != "requested" || event.Validate() != nil || event.ServerID != created.Server.ID || event.RevisionID != created.Revision.ID || event.RegionID != created.Placement.RegionID || event.PlacementEpoch != created.Placement.PlacementEpoch || event.IntentVersion != created.Server.IntentVersion {
		t.Fatalf("wrong command %+v", task)
	}
	changed := request
	changed.Scope = "instance"
	if _, err := db.RequestGlobalBackup(ctx, "global-owner", changed); !errors.Is(err, instances.ErrIdempotencyConflict) {
		t.Fatal("changed retry accepted")
	}
	changed = request
	changed.IdempotencyKey = "missing-server"
	changed.ServerID = "not-owned"
	if _, err := db.RequestGlobalBackup(ctx, "global-owner", changed); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing/foreign server %v", err)
	}
	var outbox struct{ Payload, RegionID string }
	if err := db.db.Table("backup_request_outbox").Where("id = ?", event.EventID).Take(&outbox).Error; err != nil {
		t.Fatal(err)
	}
	var queued backup.Requested
	if json.Unmarshal([]byte(outbox.Payload), &queued) != nil || queued != event || outbox.RegionID != event.RegionID {
		t.Fatal("outbox lost task identity")
	}
	var wrongQueue int64
	if err := db.db.Table("server_outbox").Where("operation_id = ?", event.OperationID).Count(&wrongQueue).Error; err != nil || wrongQueue != 0 {
		t.Fatal("backup entered revision queue")
	}
	// Preserve future control-plane states when replaying the original request.
	if err := db.db.Table("global_backup_tasks").Where("id = ?", event.BackupID).Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	replayed, err := db.RequestGlobalBackup(ctx, "global-owner", request)
	if err != nil || replayed.Status != "running" || replayed.Request != event {
		t.Fatal("retry reset backup state")
	}
	callback := "test:reject_backup_outbox"
	if err := db.db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "backup_request_outbox" {
			tx.AddError(errors.New("injected outbox failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failed := request
	failed.IdempotencyKey = "failed-outbox"
	_, failure := db.RequestGlobalBackup(ctx, "global-owner", failed)
	if err := db.db.Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if failure == nil {
		t.Fatal("outbox failure ignored")
	}
	var count int64
	if err := db.db.Table("server_operations").Where("organization_id = ? AND kind = ? AND idempotency_key = ?", request.OrganizationID, "backup", failed.IdempotencyKey).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("failed transaction left operation")
	}
	if err := db.db.Table("global_backup_tasks").Where("organization_id = ?", request.OrganizationID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("failed transaction left backup task")
	}
	testBackupRequestDelivery(t, db, event)
}
