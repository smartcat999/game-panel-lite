package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/delivery"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

type outboxPublisherFunc func(context.Context, delivery.Message) error

func (f outboxPublisherFunc) Publish(ctx context.Context, m delivery.Message) error { return f(ctx, m) }

func TestDurableOutbox(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testDurableOutbox(t, db)
}

func testDurableOutbox(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	org := domain.Organization{ID: "outbox-owner", Slug: "outbox-owner"}
	if err := db.CreateOrganization(ctx, &org, "outbox-user"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateTenantQuota(ctx, domain.TenantQuota{OrganizationID: org.ID, MaxServers: 8, MaxCPUCores: 8, MaxMemoryMB: 4096, MaxStorageGB: 10}); err != nil {
		t.Fatal(err)
	}
	create := func(key, region string) instances.IntentResult {
		t.Helper()
		result, err := db.CreateGlobalServer(ctx, "outbox-user", instances.CreateRequest{OrganizationID: org.ID, Name: key, RegionID: region, IdempotencyKey: key, Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Configuration: instances.ProtectedConfiguration{KeyID: "test", Ciphertext: []byte("protected")}, Resources: instances.Resources{CPU: 1, MemoryMB: 128}}})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := create("first", "outbox-a")
	create("second", "outbox-b")
	claim := func(region string) []delivery.Message {
		t.Helper()
		rows, err := db.ClaimOutbox(ctx, region, 1, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	rows := claim("outbox-a")
	if len(rows) != 1 || rows[0].Attempts != 1 {
		t.Fatalf("initial claim: %+v", rows)
	}
	old := rows[0]
	if len(claim("outbox-a")) != 0 {
		t.Fatal("live claim stolen")
	}
	if len(claim("outbox-b")) != 1 {
		t.Fatal("regional backlog isolation failed")
	}
	if err := db.db.Table("server_outbox").Where("id = ?", old.ID).UpdateColumn("lease_until_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteOutbox(ctx, old.ID, old.Token); !errors.Is(err, delivery.ErrClaimLost) {
		t.Fatalf("expired worker acknowledged: %v", err)
	}
	rows = claim("outbox-a")
	if len(rows) != 1 || rows[0].ID != old.ID || rows[0].Token == old.Token || rows[0].Attempts != 2 {
		t.Fatalf("crash recovery: %+v", rows)
	}
	if err := db.RetryOutbox(ctx, old.ID, old.Token, time.Second); !errors.Is(err, delivery.ErrClaimLost) {
		t.Fatalf("stale worker overwrote retry: %v", err)
	}
	if err := db.RetryOutbox(ctx, rows[0].ID, rows[0].Token, time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(claim("outbox-a")) != 0 {
		t.Fatal("retry delay ignored")
	}
	ready := func() {
		t.Helper()
		if err := db.db.Table("server_outbox").Where("id = ?", old.ID).UpdateColumn("next_attempt_ms", 0).Error; err != nil {
			t.Fatal(err)
		}
	}
	ready()
	seen := []string{}
	d := delivery.Dispatcher{Outbox: db, RegionID: "outbox-a", Batch: 1, Lease: time.Minute, PublishTimeout: time.Second, RetryDelay: time.Second}
	d.Publisher = outboxPublisherFunc(func(_ context.Context, m delivery.Message) error {
		seen = append(seen, m.ID)
		return errors.New("broker accepted but confirmation lost")
	})
	if n, err := d.RunOnce(ctx); n != 0 || err == nil {
		t.Fatalf("uncertain publish counted complete: %d %v", n, err)
	}
	ready()
	d.Publisher = outboxPublisherFunc(func(_ context.Context, m delivery.Message) error { seen = append(seen, m.ID); return nil })
	if n, err := d.RunOnce(ctx); n != 1 || err != nil {
		t.Fatalf("confirmed retry: %d %v", n, err)
	}
	if len(seen) != 2 || seen[0] != seen[1] {
		t.Fatalf("retry changed event identity: %v", seen)
	}
	if n, err := d.RunOnce(ctx); n != 0 || err != nil {
		t.Fatalf("published event resent: %d %v", n, err)
	}
	var operation struct{ Status string }
	if err := db.db.Table("server_operations").Where("id = ?", first.Operation.ID).Take(&operation).Error; err != nil || operation.Status != "pending" {
		t.Fatalf("broker ACK changed business success: %+v %v", operation, err)
	}
	if db.db.Dialector.Name() != "postgres" {
		return
	}
	for i := 0; i < 4; i++ {
		create(fmt.Sprintf("parallel-%d", i), "outbox-parallel")
	}
	var wg sync.WaitGroup
	results := make(chan []delivery.Message, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := db.ClaimOutbox(ctx, "outbox-parallel", 1, time.Minute)
			if err != nil {
				t.Error(err)
			}
			results <- rows
		}()
	}
	wg.Wait()
	close(results)
	unique := map[string]bool{}
	for rows := range results {
		if len(rows) != 1 {
			t.Errorf("parallel batch: %+v", rows)
			continue
		}
		if unique[rows[0].ID] {
			t.Error("duplicate concurrent claim")
		}
		unique[rows[0].ID] = true
	}
	if len(unique) != 4 {
		t.Fatalf("parallel claims: %d", len(unique))
	}
}
