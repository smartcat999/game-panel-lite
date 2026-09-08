package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
)

func testRegionalBackupPreparation(t *testing.T, db *RegionalStore, dsn string) {
	ctx := context.Background()
	request := backup.Requested{SchemaVersion: 1, EventID: "event", OperationID: "operation", BackupID: "backup", OrganizationID: "tenant", ServerID: "server", RegionID: db.regionID, RevisionID: "revision", SpecGeneration: 1, IntentVersion: 1, PlacementEpoch: 1, Scope: "world"}
	if err := db.RecordBackupRequest(ctx, request); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	claims := make(chan *backup.PreparationClaim, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c, e := db.ClaimBackupPreparation(ctx, time.Minute); claims <- c; errs <- e }()
	}
	wg.Wait()
	close(claims)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first *backup.PreparationClaim
	for c := range claims {
		if c != nil {
			if first != nil {
				t.Fatal("duplicate claim")
			}
			first = c
		}
	}
	if first == nil || first.Request != request {
		t.Fatal("missing original request")
	}
	wrong := *first
	wrong.Request.IntentVersion++
	if !errors.Is(db.RetryBackupPreparation(ctx, wrong, time.Second), backup.ErrPreparationClaimLost) {
		t.Fatal("modified request accepted")
	}
	wrong = *first
	wrong.Request.RegionID = "other"
	if !errors.Is(db.RetryBackupPreparation(ctx, wrong, time.Second), backup.ErrPreparationClaimLost) {
		t.Fatal("foreign region accepted")
	}
	plan := backup.UploadPlan{ID: "upload", OperationID: request.OperationID, RequestEventID: request.EventID, RegionID: request.RegionID, ServerID: request.ServerID, DeploymentID: "deployment", NodeID: "node", SnapshotID: "snapshot", PlacementEpoch: 1, StorageID: "storage", ObjectKey: "object", Asset: assets.PublishedVersion{OrganizationID: "tenant", AssetID: "asset", Version: "one", SHA256: strings.Repeat("a", 64), SizeBytes: 10}}
	// Expire the actual database lease without waiting for a wall-clock sleep.
	if err := db.db.Table("regional_backup_requests").Where("operation_id = ?", request.OperationID).Update("lease_until_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(db.PrepareArchiveUpload(ctx, *first, plan), backup.ErrPreparationClaimLost) {
		t.Fatal("expired preparation committed")
	}
	var count int64
	if err := db.db.Table("regional_archive_uploads").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("expired claim left upload")
	}
	reopened, err := OpenRegionalPostgres(dsn, db.regionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	second, err := reopened.ClaimBackupPreparation(ctx, time.Minute)
	if err != nil || second == nil || second.Token == first.Token {
		t.Fatal("claim not recovered after reopening")
	}
	if !errors.Is(db.RetryBackupPreparation(ctx, *first, time.Second), backup.ErrPreparationClaimLost) {
		t.Fatal("old token accepted")
	}
	if err := db.RetryBackupPreparation(ctx, *second, time.Hour); err != nil {
		t.Fatal(err)
	}
	if c, err := db.ClaimBackupPreparation(ctx, time.Minute); err != nil || c != nil {
		t.Fatal("retry delay ignored")
	}
	if err := db.db.Table("regional_backup_requests").Where("operation_id = ?", request.OperationID).Update("next_attempt_ms", 0).Error; err != nil {
		t.Fatal(err)
	}
	third, err := db.ClaimBackupPreparation(ctx, time.Minute)
	if err != nil || third == nil {
		t.Fatal("retry not claimable")
	}
	if err := db.db.Exec("ALTER TABLE regional_backup_requests ADD CONSTRAINT test_preparation_commit CHECK(status <> 'preparing') NOT VALID").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.PrepareArchiveUpload(ctx, *third, plan); err == nil {
		t.Fatal("injected failure ignored")
	}
	if err := db.db.Table("regional_archive_uploads").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("partial upload committed")
	}
	if err := db.db.Exec("ALTER TABLE regional_backup_requests DROP CONSTRAINT test_preparation_commit").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.PrepareArchiveUpload(ctx, *third, plan); err != nil {
		t.Fatal(err)
	}
	if err := db.PrepareArchiveUpload(ctx, *third, plan); err != nil {
		t.Fatal("lost-response replay failed", err)
	}
	if !errors.Is(db.RetryBackupPreparation(ctx, *third, time.Second), backup.ErrPreparationClaimLost) {
		t.Fatal("prepared task requeued")
	}
	if c, err := db.ClaimBackupPreparation(ctx, time.Minute); err != nil || c != nil {
		t.Fatal("prepared task reclaimed")
	}
	request.OperationID = "corrupt"
	request.EventID = "corrupt"
	request.BackupID = "corrupt"
	if err := db.RecordBackupRequest(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := db.db.Table("regional_backup_requests").Where("operation_id = ?", request.OperationID).Update("payload", "{}").Error; err != nil {
		t.Fatal(err)
	}
	if c, err := db.ClaimBackupPreparation(ctx, time.Minute); c != nil || !errors.Is(err, backup.ErrInvalidRequest) {
		t.Fatal("corrupt request accepted")
	}
	if c, err := db.ClaimBackupPreparation(ctx, time.Minute); c != nil || err != nil {
		t.Fatal("corrupt request starves queue")
	}
}
