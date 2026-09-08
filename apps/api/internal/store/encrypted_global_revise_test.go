package store

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/configprotection"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func testEncryptedGlobalRevision(t *testing.T, db *Store, created instances.IntentResult, spec instances.Specification, protector *configprotection.Protector, f, rotated RequestFingerprinter) {
	t.Helper()
	ctx := context.Background()
	actor := "encrypted-owner"
	if err := db.db.Table("logical_servers").Where("id = ?", created.Server.ID).Updates(map[string]any{"desired_state": "stopped", "intent_version": 2}).Error; err != nil {
		t.Fatal(err)
	}
	request := instances.ReviseRequest{OrganizationID: created.Server.OrganizationID, ServerID: created.Server.ID, ExpectedGeneration: 1, IdempotencyKey: "encrypted-revise", Specification: spec}
	plaintext := []byte(`{"password":"changed-secret"}`)
	sealer := &countingSealer{inner: protector}
	updated, err := db.ReviseEncryptedGlobalServer(ctx, actor, request, plaintext, sealer, f)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Server.SpecGeneration != 2 || updated.Server.DesiredState != "stopped" || updated.Server.IntentVersion != 2 || updated.Placement != created.Placement {
		t.Fatal("revision changed intent or placement")
	}
	binding := instances.ConfigurationBinding{OrganizationID: request.OrganizationID, ServerID: request.ServerID, RevisionID: updated.Revision.ID, SpecGeneration: 2, ProviderKey: spec.ProviderKey, ConfigSchemaVersion: spec.ConfigSchemaVersion}
	opened, err := protector.Open(ctx, binding, updated.Revision.Specification.Configuration)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("new revision decrypt: %v", err)
	}
	oldBinding := binding
	oldBinding.RevisionID = created.Revision.ID
	oldBinding.SpecGeneration = 1
	if _, err := protector.Open(ctx, oldBinding, updated.Revision.Specification.Configuration); err == nil {
		t.Fatal("new ciphertext accepted under old revision")
	}
	sealer.fail = true
	replayed, err := db.ReviseEncryptedGlobalServer(ctx, actor, request, plaintext, sealer, rotated)
	if err != nil || replayed.Operation.ID != updated.Operation.ID || sealer.calls != 1 {
		t.Fatalf("revision replay resealed: %v", err)
	}
	if _, err := db.ReviseEncryptedGlobalServer(ctx, actor, request, []byte("other"), sealer, f); !errors.Is(err, instances.ErrIdempotencyConflict) {
		t.Fatalf("changed request accepted: %v", err)
	}
	if _, err := db.ReviseEncryptedGlobalServer(ctx, "intruder", request, plaintext, sealer, f); !errors.Is(err, ErrWorkspaceWriteDenied) {
		t.Fatalf("unauthorized replay: %v", err)
	}
	stale := request
	stale.IdempotencyKey = "stale"
	if _, err := db.ReviseEncryptedGlobalServer(ctx, actor, stale, plaintext, sealer, f); !errors.Is(err, instances.ErrVersionConflict) {
		t.Fatalf("old generation accepted: %v", err)
	}
	failed := request
	failed.IdempotencyKey = "failed-revision"
	failed.ExpectedGeneration = 2
	if _, err := db.ReviseEncryptedGlobalServer(ctx, actor, failed, plaintext, sealer, f); err == nil {
		t.Fatal("sealing failure ignored")
	}
	var count int64
	if err := db.db.Table("server_revisions").Where("server_id = ?", request.ServerID).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("partial revision: %d %v", count, err)
	}
	var current instances.Server
	if err := db.db.Table("logical_servers").Where("id = ?", request.ServerID).Take(&current).Error; err != nil || current.CurrentRevisionID != updated.Revision.ID || current.SpecGeneration != 2 {
		t.Fatalf("failed encryption advanced pointer: %v", err)
	}
	// A later successful edit must not prevent retrying the earlier operation,
	// and that retry must not move the latest pointer back.
	sealer.fail = false
	latest, err := db.ReviseEncryptedGlobalServer(ctx, actor, failed, []byte("third-version"), sealer, f)
	if err != nil {
		t.Fatal(err)
	}
	sealer.fail = true
	replayed, err = db.ReviseEncryptedGlobalServer(ctx, actor, request, plaintext, sealer, rotated)
	if err != nil || replayed.Revision.ID != updated.Revision.ID || replayed.Server.CurrentRevisionID != latest.Revision.ID || replayed.Server.DesiredState != "stopped" {
		t.Fatalf("old retry rolled state back: %v", err)
	}
}
