package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/configprotection"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
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
}
