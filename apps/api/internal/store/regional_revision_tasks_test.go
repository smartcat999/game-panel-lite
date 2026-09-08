package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type revisionSourceFunc func(context.Context, instances.RevisionAvailable) (regional.RevisionSnapshot, error)

func (f revisionSourceFunc) GetRevision(ctx context.Context, event instances.RevisionAvailable) (regional.RevisionSnapshot, error) {
	return f(ctx, event)
}

func testRegionalRevisionTasks(t *testing.T, db *RegionalStore, dsn string) {
	ctx := context.Background()
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "fetch-event", OperationID: "fetch-operation", OrganizationID: "org", ServerID: "server", RevisionID: "revision", RegionID: "fetch-test", PlacementEpoch: 1, SpecGeneration: 1}
	if err := db.RecordRevisionNotification(ctx, event); err != nil {
		t.Fatal(err)
	}
	claims := make(chan *regional.RevisionClaim, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := db.ClaimRevision(ctx, time.Minute)
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
		t.Fatalf("concurrent claims=%d", len(claims))
	}
	old := <-claims
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", event.OperationID).Update("lease_until_ms", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.RetryRevision(ctx, *old, time.Second); !errors.Is(err, regional.ErrRevisionClaimLost) {
		t.Fatalf("expired claim accepted: %v", err)
	}
	reopened, err := OpenRegionalPostgres(dsn, "fetch-test", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	current, err := reopened.ClaimRevision(ctx, time.Minute)
	if err != nil || current == nil {
		t.Fatalf("reclaim: %v", err)
	}
	if current.Token == old.Token {
		t.Fatal("reused token")
	}
	if err := db.RetryRevision(ctx, *old, time.Second); !errors.Is(err, regional.ErrRevisionClaimLost) {
		t.Fatalf("stale token accepted: %v", err)
	}
	if err := db.RetryRevision(ctx, *current, time.Hour); err != nil {
		t.Fatal(err)
	}
	if claim, err := db.ClaimRevision(ctx, time.Minute); err != nil || claim != nil {
		t.Fatalf("retry delay ignored: %v", err)
	}
	ready := func() {
		t.Helper()
		if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", event.OperationID).Update("next_attempt_ms", 0).Error; err != nil {
			t.Fatal(err)
		}
	}
	ready()
	fetcher := regional.Fetcher{Tasks: db, Lease: time.Minute, Timeout: time.Second, RetryDelay: time.Hour, Source: revisionSourceFunc(func(context.Context, instances.RevisionAvailable) (regional.RevisionSnapshot, error) {
		return regional.RevisionSnapshot{}, errors.New("private credentials")
	})}
	if done, err := fetcher.RunOnce(ctx); done || err == nil || err.Error() == "private credentials" {
		t.Fatalf("failed fetch: %v", err)
	}
	ready()
	snapshot := regional.RevisionSnapshot{Event: event, CurrentSpecGeneration: 2, DesiredState: "stopped", IntentVersion: 3, Revision: instances.Revision{ID: event.RevisionID, ServerID: event.ServerID, SpecGeneration: 1, Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 256}, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("opaque")}}}}
	fetcher.Source = revisionSourceFunc(func(context.Context, instances.RevisionAvailable) (regional.RevisionSnapshot, error) {
		return snapshot, nil
	})
	if done, err := fetcher.RunOnce(ctx); !done || err != nil {
		t.Fatalf("fetch: %v", err)
	}
	var row struct {
		Status, Snapshot string
		Attempts         int64
	}
	if err := db.db.Table("regional_revision_tasks").Where("operation_id = ?", event.OperationID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var saved regional.RevisionSnapshot
	if err := json.Unmarshal([]byte(row.Snapshot), &saved); err != nil || saved.ValidateFor(event) != nil || saved.DesiredState != "stopped" || row.Status != "revision_fetched" || row.Attempts != 4 {
		t.Fatalf("invalid persisted result: %+v %v", row, err)
	}
	if err := db.RecordRevisionNotification(ctx, event); err != nil {
		t.Fatal(err)
	}
	if claim, err := db.ClaimRevision(ctx, time.Minute); err != nil || claim != nil {
		t.Fatalf("completed task reset: %v", err)
	}
}
