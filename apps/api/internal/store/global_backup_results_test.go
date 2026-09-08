package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"gorm.io/gorm"
)

func testGlobalBackupResults(t *testing.T, db *Store, request backup.Requested) {
	t.Helper()
	ctx := context.Background()
	asset := assets.PublishedVersion{AssetID: "backup-result-asset", OrganizationID: request.OrganizationID, Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 100}
	plan := backup.UploadPlan{OperationID: request.OperationID, RequestEventID: request.EventID, ID: "upload", RegionID: request.RegionID, ServerID: request.ServerID, DeploymentID: "deployment", NodeID: "node", SnapshotID: "snapshot", PlacementEpoch: request.PlacementEpoch, StorageID: "storage", ObjectKey: "object", Asset: asset}
	event := backup.ArchiveUploaded{SchemaVersion: 1, EventID: "backup-result", Plan: plan, Receipt: backup.StoredArchive{StorageID: plan.StorageID, ObjectKey: plan.ObjectKey, Asset: asset}}
	for name, mutate := range map[string]func(*backup.ArchiveUploaded){
		"request": func(e *backup.ArchiveUploaded) { e.Plan.RequestEventID = "wrong" },
		"epoch":   func(e *backup.ArchiveUploaded) { e.Plan.PlacementEpoch++ },
		"server":  func(e *backup.ArchiveUploaded) { e.Plan.ServerID = "wrong" },
		"tenant":  func(e *backup.ArchiveUploaded) { e.Plan.Asset.OrganizationID = "wrong"; e.Receipt.Asset = e.Plan.Asset },
	} {
		t.Run(name, func(t *testing.T) {
			e := event
			mutate(&e)
			if !errors.Is(db.RecordBackupResult(ctx, request.RegionID, e), ErrNotificationConflict) {
				t.Fatal("invalid result accepted")
			}
		})
	}
	if !errors.Is(db.RecordBackupResult(ctx, "wrong", event), ErrRegionMismatch) {
		t.Fatal("wrong source accepted")
	}
	callback := "test:reject_backup_result"
	if err := db.db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "global_backup_results" {
			tx.AddError(errors.New("injected result failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failure := db.RecordBackupResult(ctx, request.RegionID, event)
	if err := db.db.Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if failure == nil {
		t.Fatal("result failure ignored")
	}
	var count int64
	if err := db.db.Table("global_assets").Where("id = ?", asset.AssetID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("asset escaped rollback")
	}
	var row globalBackupTaskRow
	if err := db.db.Table("global_backup_tasks").Where("id = ?", request.BackupID).Take(&row).Error; err != nil || row.Status != "running" {
		t.Fatal("task escaped rollback")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- db.RecordBackupResult(ctx, request.RegionID, event) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	duplicate := event
	duplicate.EventID = "duplicate-result"
	if err := db.RecordBackupResult(ctx, request.RegionID, duplicate); err != nil {
		t.Fatal(err)
	}
	conflict := event
	conflict.Plan.SnapshotID = "other-snapshot"
	if !errors.Is(db.RecordBackupResult(ctx, request.RegionID, conflict), ErrNotificationConflict) {
		t.Fatal("conflicting retry accepted")
	}
	if err := db.db.Table("global_backup_results").Where("operation_id = ?", request.OperationID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("duplicate result rows")
	}
	if err := db.db.Table("global_backup_tasks").Where("id = ?", request.BackupID).Take(&row).Error; err != nil || row.Status != "succeeded" {
		t.Fatal("task not completed")
	}
	var stored struct{ Payload, Disposition string }
	if err := db.db.Table("global_backup_results").Where("operation_id = ?", request.OperationID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	var receipt backup.ArchiveUploaded
	if json.Unmarshal([]byte(stored.Payload), &receipt) != nil || receipt != event || stored.Disposition != "published" {
		t.Fatal("published receipt changed")
	}
	var version globalAssetVersionRow
	if err := db.db.Table("global_asset_versions").Where("asset_id = ? AND version = ?", asset.AssetID, asset.Version).Take(&version).Error; err != nil || version.SHA256 != asset.SHA256 || version.SizeBytes != asset.SizeBytes {
		t.Fatal("asset metadata not published")
	}
	var op globalOperationRow
	if err := db.db.Table("server_operations").Where("id = ?", request.OperationID).Take(&op).Error; err != nil || op.Status != "succeeded" {
		t.Fatal("operation not completed")
	}
	for _, status := range []string{"cancelled", "failed"} {
		task, err := db.RequestGlobalBackup(ctx, "global-owner", backup.Request{OrganizationID: request.OrganizationID, ServerID: request.ServerID, Scope: request.Scope, IdempotencyKey: "result-" + status})
		if err != nil {
			t.Fatal(err)
		}
		if err := db.db.Table("global_backup_tasks").Where("id = ?", task.Request.BackupID).Update("status", status).Error; err != nil {
			t.Fatal(err)
		}
		e := event
		e.EventID = "result-" + status
		e.Plan.OperationID = task.Request.OperationID
		e.Plan.RequestEventID = task.Request.EventID
		e.Plan.Asset.AssetID = "discarded-" + status
		e.Receipt.Asset = e.Plan.Asset
		if err := db.RecordBackupResult(ctx, request.RegionID, e); err != nil {
			t.Fatal(err)
		}
		row = globalBackupTaskRow{}
		if err := db.db.Table("global_backup_tasks").Where("id = ?", task.Request.BackupID).Take(&row).Error; err != nil || row.Status != status {
			t.Fatal("terminal task revived")
		}
		if err := db.db.Table("global_assets").Where("id = ?", e.Plan.Asset.AssetID).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("discarded asset published")
		}
	}
	testGlobalBackupBroker(t, db, event, request)
}
