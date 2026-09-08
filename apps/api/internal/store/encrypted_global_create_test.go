package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/configprotection"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
)

type countingSealer struct {
	inner ConfigurationSealer
	calls int
	fail  bool
}

func (s *countingSealer) Seal(ctx context.Context, b instances.ConfigurationBinding, p []byte) (instances.ProtectedConfiguration, error) {
	s.calls++
	if s.fail {
		return instances.ProtectedConfiguration{}, errors.New("sealing failed")
	}
	return s.inner.Seal(ctx, b, p)
}

func testEncryptedGlobalCreate(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.RegisterRegion(ctx, "east", "East"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetRegionAcceptingCreates(ctx, "east", 1, true); err != nil {
		t.Fatal(err)
	}
	key := func() []byte {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	p, err := configprotection.New("encryption", map[string][]byte{"encryption": key()}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	digestKeys := map[string][]byte{"old": key(), "new": key()}
	f, err := configprotection.NewFingerprinter("old", digestKeys)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := configprotection.NewFingerprinter("new", digestKeys)
	if err != nil {
		t.Fatal(err)
	}
	org := domain.Organization{ID: "encrypted-tenant", Slug: "encrypted-tenant"}
	if err := db.CreateOrganization(ctx, &org, "encrypted-owner"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateTenantQuota(ctx, domain.TenantQuota{OrganizationID: org.ID, MaxServers: 3, MaxCPUCores: 3, MaxMemoryMB: 2048, MaxStorageGB: 10}); err != nil {
		t.Fatal(err)
	}
	request := instances.CreateRequest{OrganizationID: org.ID, Name: "protected", RegionID: "east", IdempotencyKey: "encrypted-create", Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Resources: instances.Resources{CPU: 1, MemoryMB: 256}}}
	plaintext := []byte(`{"password":"must-never-be-stored-plain"}`)
	sealer := &countingSealer{inner: p}
	created, err := db.CreateEncryptedGlobalServer(ctx, "encrypted-owner", request, plaintext, sealer, f)
	if err != nil {
		t.Fatal(err)
	}
	binding := instances.ConfigurationBinding{OrganizationID: org.ID, ServerID: created.Server.ID, RevisionID: created.Revision.ID, SpecGeneration: 1, ProviderKey: "test", ConfigSchemaVersion: 1}
	opened, err := p.Open(ctx, binding, created.Revision.Specification.Configuration)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("stored encrypted revision: %v", err)
	}
	sealer.fail = true
	replayed, err := db.CreateEncryptedGlobalServer(ctx, "encrypted-owner", request, plaintext, sealer, rotated)
	if err != nil || replayed.Operation.ID != created.Operation.ID || sealer.calls != 1 {
		t.Fatalf("retry resealed or lost identity: %v", err)
	}
	if _, err := db.CreateEncryptedGlobalServer(ctx, "encrypted-owner", request, []byte("changed"), sealer, rotated); !errors.Is(err, instances.ErrIdempotencyConflict) {
		t.Fatalf("changed input accepted: %v", err)
	}
	if _, err := db.CreateEncryptedGlobalServer(ctx, "intruder", request, plaintext, sealer, f); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("unauthorized retry: %v", err)
	}
	fresh := request
	fresh.IdempotencyKey = "failed-seal"
	if _, err := db.CreateEncryptedGlobalServer(ctx, "encrypted-owner", fresh, plaintext, sealer, f); err == nil {
		t.Fatal("seal failure ignored")
	}
	var count int64
	if err := db.db.Table("server_operations").Where("organization_id = ?", org.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("partial operation: %d %v", count, err)
	}
	if err := db.db.Table("logical_servers").Where("organization_id = ?", org.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("partial server: %d %v", count, err)
	}
	var row globalRevisionRow
	if err := db.db.Table("server_revisions").Where("id = ?", created.Revision.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	var op globalOperationRow
	if err := db.db.Table("server_operations").Where("id = ?", created.Operation.ID).Take(&op).Error; err != nil {
		t.Fatal(err)
	}
	var payload string
	if err := db.db.Table("server_outbox").Select("payload").Where("operation_id = ?", op.ID).Scan(&payload).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(row.Specification, "must-never") || strings.Contains(payload, "must-never") || strings.Contains(op.RequestHash, "must-never") || !strings.HasPrefix(op.RequestHash, "h1.") {
		t.Fatal("plaintext or unversioned digest persisted")
	}
	testEncryptedGlobalRevision(t, db, created, request.Specification, p, f, rotated)
	writer, err := NewEncryptedIntentWriter(db, p, rotated)
	if err != nil {
		t.Fatal(err)
	}
	replay, found, err := writer.ReplayCreate(ctx, "encrypted-owner", request, plaintext)
	if err != nil || !found || replay.Operation.ID != created.Operation.ID || replay.Server.SpecGeneration != 3 {
		t.Fatalf("authorized replay lookup: %v", err)
	}
	if _, _, err := writer.ReplayCreate(ctx, "intruder", request, plaintext); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("unauthorized replay lookup: %v", err)
	}
	if _, _, err := writer.ReplayCreate(ctx, "encrypted-owner", request, []byte("changed")); !errors.Is(err, instances.ErrIdempotencyConflict) {
		t.Fatalf("replay conflict: %v", err)
	}
	missing := request
	missing.IdempotencyKey = "never-created"
	if _, found, err := writer.ReplayCreate(ctx, "encrypted-owner", missing, plaintext); err != nil || found {
		t.Fatalf("missing replay: %v", err)
	}
	if err := db.SetRegionAcceptingCreates(ctx, "east", 2, false); err != nil {
		t.Fatal(err)
	}
	if replay, err := db.CreateEncryptedGlobalServer(ctx, "encrypted-owner", request, plaintext, sealer, f); err != nil || replay.Operation.ID != created.Operation.ID {
		t.Fatalf("closed region blocked existing operation: %v", err)
	}
	for _, region := range []string{"east", "not-registered"} {
		fresh := request
		fresh.RegionID = region
		fresh.IdempotencyKey = "rejected-" + region
		if _, err := db.CreateEncryptedGlobalServer(ctx, "encrypted-owner", fresh, plaintext, sealer, f); !errors.Is(err, regions.ErrRegionUnavailable) {
			t.Fatalf("unavailable region accepted: %v", err)
		}
	}
	if db.db.Dialector.Name() == "postgres" {
		testRegionCloseDuringCreate(t, db, request, plaintext, p, f)
	}
}

type blockingRegionSealer struct {
	inner            ConfigurationSealer
	entered, release chan struct{}
}

func (s blockingRegionSealer) Seal(ctx context.Context, b instances.ConfigurationBinding, p []byte) (instances.ProtectedConfiguration, error) {
	close(s.entered)
	select {
	case <-s.release:
		return s.inner.Seal(ctx, b, p)
	case <-ctx.Done():
		return instances.ProtectedConfiguration{}, ctx.Err()
	}
}

func testRegionCloseDuringCreate(t *testing.T, db *Store, request instances.CreateRequest, plaintext []byte, p ConfigurationSealer, f RequestFingerprinter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.RegisterRegion(ctx, "race-region", "Race Region"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetRegionAcceptingCreates(ctx, "race-region", 1, true); err != nil {
		t.Fatal(err)
	}
	request.RegionID = "race-region"
	request.IdempotencyKey = "race-region-create"
	blocking := blockingRegionSealer{inner: p, entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := db.CreateEncryptedGlobalServer(ctx, "encrypted-owner", request, plaintext, blocking, f)
		done <- err
	}()
	select {
	case <-blocking.entered:
	case err := <-done:
		t.Fatalf("create failed before region lock: %v", err)
	case <-ctx.Done():
		t.Fatal("create did not reach encryption")
	}
	closeCtx, stop := context.WithTimeout(ctx, 150*time.Millisecond)
	err := db.SetRegionAcceptingCreates(closeCtx, "race-region", 2, false)
	stop()
	close(blocking.release)
	if err == nil || !errors.Is(closeCtx.Err(), context.DeadlineExceeded) {
		t.Fatalf("expected region close to wait for admitted transaction: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("create did not finish")
	}
	if err := db.SetRegionAcceptingCreates(ctx, "race-region", 2, false); err != nil {
		t.Fatalf("close after commit: %v", err)
	}
	request.IdempotencyKey = "after-region-close"
	if _, err := db.CreateEncryptedGlobalServer(ctx, "encrypted-owner", request, plaintext, p, f); !errors.Is(err, regions.ErrRegionUnavailable) {
		t.Fatalf("post-close creation accepted: %v", err)
	}
}
