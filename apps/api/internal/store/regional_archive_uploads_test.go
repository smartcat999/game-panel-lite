package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
)

func testRegionalArchiveUploads(t *testing.T, db *RegionalStore, dsn string) {
	t.Helper()
	ctx := context.Background()
	plan := backup.UploadPlan{ID: "upload", OperationID: "global-operation", RequestEventID: "global-event", RegionID: "upload-test", ServerID: "server", DeploymentID: "deployment", NodeID: "node", SnapshotID: "snapshot", PlacementEpoch: 1, StorageID: "store", ObjectKey: "object", Asset: assets.PublishedVersion{OrganizationID: "tenant", AssetID: "backup", Version: "one", SHA256: strings.Repeat("a", 64), SizeBytes: 10}}
	for i := 0; i < 2; i++ {
		if err := db.PrepareArchiveUpload(ctx, plan); err != nil {
			t.Fatal(err)
		}
	}
	for _, mutate := range []func(*backup.UploadPlan){
		func(p *backup.UploadPlan) { p.NodeID = "other" },
		func(p *backup.UploadPlan) { p.RegionID = "other" },
		func(p *backup.UploadPlan) { p.ID = "other"; p.OperationID = "other" },
		func(p *backup.UploadPlan) { p.ID = "other"; p.ObjectKey = "other" },
		func(p *backup.UploadPlan) { p.RequestEventID = "other" },
	} {
		changed := plan
		mutate(&changed)
		if !errors.Is(db.PrepareArchiveUpload(ctx, changed), backup.ErrUploadPlan) {
			t.Fatal("conflicting plan accepted")
		}
	}
	claims := make(chan *backup.UploadClaim, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c, err := db.ClaimArchiveUpload(ctx, time.Minute); claims <- c; errs <- err }()
	}
	wg.Wait()
	close(claims)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first *backup.UploadClaim
	for claim := range claims {
		if claim != nil {
			if first != nil {
				t.Fatal("duplicate active claim")
			}
			first = claim
		}
	}
	if first == nil || first.Plan != plan {
		t.Fatal("lost claim identity")
	}
	receipt := backup.StoredArchive{StorageID: plan.StorageID, ObjectKey: plan.ObjectKey, ObjectVersion: "version", Asset: plan.Asset}
	badReceipt := receipt
	badReceipt.Asset.OrganizationID = "other"
	if !errors.Is(db.CompleteArchiveUpload(ctx, *first, badReceipt), backup.ErrUploadPlan) {
		t.Fatal("cross-tenant receipt accepted")
	}
	if err := db.db.Table("regional_archive_uploads").Where("id = ?", plan.ID).Update("lease_until_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(db.CompleteArchiveUpload(ctx, *first, receipt), backup.ErrUploadClaimLost) {
		t.Fatal("expired completion accepted")
	}
	reopened, err := OpenRegionalPostgres(dsn, "upload-test", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	second, err := reopened.ClaimArchiveUpload(ctx, time.Minute)
	if err != nil || second == nil || second.Token == first.Token {
		t.Fatalf("reclaim %+v %v", second, err)
	}
	if !errors.Is(db.CompleteArchiveUpload(ctx, *first, receipt), backup.ErrUploadClaimLost) {
		t.Fatal("old completion accepted")
	}
	changed := *second
	changed.Plan.SnapshotID = "other"
	if !errors.Is(reopened.CompleteArchiveUpload(ctx, changed, receipt), backup.ErrUploadClaimLost) {
		t.Fatal("snapshot replacement accepted")
	}
	if err := reopened.RetryArchiveUpload(ctx, *second, time.Hour); err != nil {
		t.Fatal(err)
	}
	if c, err := db.ClaimArchiveUpload(ctx, time.Minute); c != nil || err != nil {
		t.Fatal("retry delay ignored")
	}
	if err := db.db.Table("regional_archive_uploads").Where("id = ?", plan.ID).Update("next_attempt_ms", 0).Error; err != nil {
		t.Fatal(err)
	}
	third, err := db.ClaimArchiveUpload(ctx, time.Minute)
	if err != nil || third == nil {
		t.Fatal("retry not claimable")
	}
	if err := db.db.Exec("ALTER TABLE regional_backup_result_outbox ADD CONSTRAINT test_block_result CHECK(false) NOT VALID").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteArchiveUpload(ctx, *third, receipt); err == nil {
		t.Fatal("completion ignored outbox failure")
	}
	var pending struct{ Status, Receipt, LeaseToken string }
	if err := db.db.Table("regional_archive_uploads").Where("id = ?", plan.ID).Take(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Status != "pending" || pending.Receipt != "" || pending.LeaseToken != third.Token {
		t.Fatal("failed outbox transaction partially completed upload")
	}
	if err := db.db.Exec("ALTER TABLE regional_backup_result_outbox DROP CONSTRAINT test_block_result").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteArchiveUpload(ctx, *third, receipt); err != nil {
		t.Fatal(err)
	}
	if err := db.PrepareArchiveUpload(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if c, err := db.ClaimArchiveUpload(ctx, time.Minute); c != nil || err != nil {
		t.Fatal("completed task reset")
	}
	var row struct {
		Status, Receipt string
		Attempts        int64
	}
	if err := db.db.Table("regional_archive_uploads").Where("id = ?", plan.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var saved backup.StoredArchive
	if row.Status != "uploaded" || row.Attempts != 3 || json.Unmarshal([]byte(row.Receipt), &saved) != nil || saved != receipt {
		t.Fatalf("durable result %+v", row)
	}
	var result struct{ OperationID, EventType, Payload string }
	if err := db.db.Table("regional_backup_result_outbox").Where("upload_id = ?", plan.ID).Take(&result).Error; err != nil {
		t.Fatal(err)
	}
	var event backup.ArchiveUploaded
	if json.Unmarshal([]byte(result.Payload), &event) != nil || event.EventID == "" || event.SchemaVersion != 1 || event.Plan != plan || event.Receipt != receipt || result.OperationID != plan.OperationID || result.EventType != "backup.archive.uploaded" {
		t.Fatalf("result event %+v", result)
	}
	testBackupResultDelivery(t, db, event)
	invalid := plan
	invalid.ID = "invalid"
	invalid.OperationID = "invalid-operation"
	invalid.ObjectKey = "invalid"
	if err := db.PrepareArchiveUpload(ctx, invalid); err != nil {
		t.Fatal(err)
	}
	if err := db.db.Table("regional_archive_uploads").Where("id = ?", invalid.ID).Update("plan", "{}").Error; err != nil {
		t.Fatal(err)
	}
	if c, err := db.ClaimArchiveUpload(ctx, time.Minute); c != nil || !errors.Is(err, backup.ErrUploadPlan) {
		t.Fatal("invalid task not isolated")
	}
	if c, err := db.ClaimArchiveUpload(ctx, time.Minute); c != nil || err != nil {
		t.Fatal("invalid task starves queue")
	}
}
