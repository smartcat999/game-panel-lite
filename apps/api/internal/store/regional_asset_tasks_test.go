package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assetfiles"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type taskAssetSource func(context.Context, instances.RevisionAvailable, assets.PublishedVersion) (io.ReadCloser, error)

func (f taskAssetSource) Open(ctx context.Context, e instances.RevisionAvailable, v assets.PublishedVersion) (io.ReadCloser, error) {
	return f(ctx, e, v)
}

func testRegionalAssetTasks(t *testing.T, db *RegionalStore, dsn string) {
	t.Helper()
	ctx := context.Background()
	e := instances.RevisionAvailable{SchemaVersion: 1, EventID: "asset-event", OperationID: "asset-operation", OrganizationID: "org", ServerID: "server", RevisionID: "revision", RegionID: "asset-test", PlacementEpoch: 1, SpecGeneration: 1}
	if err := db.RecordRevisionNotification(ctx, e); err != nil {
		t.Fatal(err)
	}
	fetch, err := db.ClaimRevision(ctx, time.Minute)
	if err != nil || fetch == nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("world"))
	v := assets.PublishedVersion{AssetID: "world", OrganizationID: "org", Version: "v1", SHA256: hex.EncodeToString(sum[:]), SizeBytes: 5}
	snapshot := regional.RevisionSnapshot{Event: e, CurrentSpecGeneration: 1, DesiredState: "stopped", IntentVersion: 1, Assets: []assets.PublishedVersion{v}, Revision: instances.Revision{ID: e.RevisionID, ServerID: e.ServerID, SpecGeneration: 1, Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 128}, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("opaque")}, Assets: []instances.AssetVersion{{AssetID: "world", Version: "v1"}}}}}
	if err := db.SaveRevision(ctx, *fetch, snapshot); err != nil {
		t.Fatal(err)
	}
	claims := make(chan *regional.AssetClaim, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := db.ClaimAssets(ctx, time.Minute)
			if err != nil {
				t.Error(err)
			}
			if claim != nil {
				claims <- claim
			}
		}()
	}
	wg.Wait()
	close(claims)
	if len(claims) != 1 {
		t.Fatalf("asset claims: %d", len(claims))
	}
	old := <-claims
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", e.OperationID).Update("asset_lease_until_ms", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteAssets(ctx, *old); !errors.Is(err, regional.ErrAssetClaimLost) {
		t.Fatal("expired asset claim completed")
	}
	reopened, err := OpenRegionalPostgres(dsn, "asset-test", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	current, err := reopened.ClaimAssets(ctx, time.Minute)
	if err != nil || current == nil || current.Token == old.Token {
		t.Fatal("asset claim did not recover after reopen")
	}
	forged := *current
	forged.Snapshot.DesiredState = "running"
	if err := db.CompleteAssets(ctx, forged); !errors.Is(err, regional.ErrAssetClaimLost) {
		t.Fatal("changed snapshot completed")
	}
	// Simulate an authorized repair returning the task to revision fetching.
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", e.OperationID).Update("status", "awaiting_revision").Error; err != nil {
		t.Fatal(err)
	}
	fetchAgain, err := db.ClaimRevision(ctx, time.Minute)
	if err != nil || fetchAgain == nil {
		t.Fatal("refetch not claimable")
	}
	if err := db.SaveRevision(ctx, *fetchAgain, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteAssets(ctx, *current); !errors.Is(err, regional.ErrAssetClaimLost) {
		t.Fatal("old asset claim survived revision refetch")
	}
	current, err = db.ClaimAssets(ctx, time.Minute)
	if err != nil || current == nil {
		t.Fatal("asset task not claimable after refetch")
	}
	if err := db.RetryAssets(ctx, *current, time.Hour); err != nil {
		t.Fatal(err)
	}
	if claim, err := db.ClaimAssets(ctx, time.Minute); err != nil || claim != nil {
		t.Fatal("asset retry delay ignored")
	}
	ready := func() {
		t.Helper()
		if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", e.OperationID).Update("asset_next_attempt_ms", 0).Error; err != nil {
			t.Fatal(err)
		}
	}
	files, err := assetfiles.New(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	failed := true
	p := regional.AssetPreparer{RegionID: "asset-test", Files: files, MaxFiles: 1, MaxFileBytes: 10, MaxTotalBytes: 10, Timeout: time.Second, Source: taskAssetSource(func(_ context.Context, event instances.RevisionAvailable, version assets.PublishedVersion) (io.ReadCloser, error) {
		if event != e || version != v {
			t.Fatal("task lost authorization identity")
		}
		if failed {
			return nil, errors.New("source unavailable")
		}
		return io.NopCloser(strings.NewReader("world")), nil
	})}
	worker := regional.AssetWorker{Tasks: reopened, Preparation: p, Lease: time.Minute, Timeout: 2 * time.Second, RetryDelay: time.Hour}
	ready()
	if done, err := worker.RunOnce(ctx); done || err == nil {
		t.Fatal("failed preparation completed")
	}
	ready()
	failed = false
	if done, err := worker.RunOnce(ctx); !done || err != nil {
		t.Fatalf("asset preparation: %v", err)
	}
	var row struct {
		Status                  string
		Attempts, AssetAttempts int64
	}
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", e.OperationID).Take(&row).Error; err != nil || row.Status != "assets_prepared" || row.Attempts != 2 || row.AssetAttempts != 5 {
		t.Fatalf("asset lifecycle counters: %+v %v", row, err)
	}
	var deployment regional.Deployment
	if err := db.db.Table("regional_deployments").Where("server_id = ? AND placement_epoch = ?", e.ServerID, e.PlacementEpoch).Take(&deployment).Error; err != nil || deployment.ID == "" || deployment.RevisionOperationID != e.OperationID || deployment.RevisionID != e.RevisionID || deployment.Status != "awaiting_authority" {
		t.Fatalf("prepared files did not stage deployment: %+v %v", deployment, err)
	}
	if err := db.RecordRevisionNotification(ctx, e); err != nil {
		t.Fatal(err)
	}
	if claim, err := db.ClaimAssets(ctx, time.Minute); err != nil || claim != nil {
		t.Fatal("duplicate event reset preparation")
	}
	f, err := files.Open(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	bad := e
	bad.EventID = "bad-event"
	bad.OperationID = "bad-operation"
	if err := db.RecordRevisionNotification(ctx, bad); err != nil {
		t.Fatal(err)
	}
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", bad.OperationID).Updates(map[string]any{"status": "revision_fetched", "snapshot": "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	if claim, err := db.ClaimAssets(ctx, time.Minute); err == nil || claim != nil {
		t.Fatal("corrupt snapshot claimed")
	}
	if claim, err := db.ClaimAssets(ctx, time.Minute); err != nil || claim != nil {
		t.Fatal("corrupt snapshot cooldown not persisted")
	}
}
