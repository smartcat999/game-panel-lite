package store

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/deploymentstatus"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func testDeploymentStatusProjection(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	org := domain.Organization{ID: "deployment-status-org", Slug: "deployment-status-org"}
	if err := db.CreateOrganization(ctx, &org, "deployment-status-owner"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateTenantQuota(ctx, domain.TenantQuota{OrganizationID: org.ID, MaxServers: 1, MaxCPUCores: 2, MaxMemoryMB: 2048, MaxStorageGB: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RegisterRegion(ctx, "deployment-status-east", "Deployment status east"); err != nil {
		t.Fatal(err)
	}
	request := instances.CreateRequest{OrganizationID: org.ID, Name: "status-server", RegionID: "deployment-status-east", IdempotencyKey: "create-status-server", Specification: instances.Specification{ProviderKey: "fixture", GameVersion: "1", ConfigSchemaVersion: 1, Configuration: instances.ProtectedConfiguration{KeyID: "key", Ciphertext: []byte("protected")}, Resources: instances.Resources{CPU: 1, MemoryMB: 512}}}
	created, err := db.CreateGlobalServer(ctx, "deployment-status-owner", request)
	if err != nil {
		t.Fatal(err)
	}
	event := deploymentstatus.Event{SchemaVersion: 1, EventID: "deployment-status-1", RegionID: created.Placement.RegionID, OrganizationID: org.ID, OperationID: created.Operation.ID, ServerID: created.Server.ID, RevisionID: created.Revision.ID, TaskID: "task-1", NodeID: "node-1", PlacementEpoch: created.Placement.PlacementEpoch, SpecGeneration: created.Server.SpecGeneration, IntentVersion: created.Server.IntentVersion, Fence: 1, ActualState: "running", Outcome: "succeeded", RuntimeID: "container-1", ObservedAtMS: 100}
	if err := db.RecordDeploymentStatus(ctx, event.RegionID, event); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordDeploymentStatus(ctx, event.RegionID, event); err != nil {
		t.Fatal("idempotent replay", err)
	}
	got, err := db.GetDeploymentStatus(ctx, org.ID, created.Server.ID)
	if err != nil || got != event {
		t.Fatal("deployment projection", err)
	}
	op, err := db.GetServerOperation(ctx, org.ID, created.Operation.ID)
	if err != nil || op.Status != "succeeded" {
		t.Fatal("successful execution did not complete operation", err)
	}
	conflict := event
	conflict.EventID = "deployment-status-conflict"
	conflict.RuntimeID = "other-container"
	if err := db.RecordDeploymentStatus(ctx, event.RegionID, conflict); !errors.Is(err, deploymentstatus.ErrEventConflict) {
		t.Fatal("same fence conflict accepted", err)
	}
	newer := event
	newer.EventID = "deployment-status-2"
	newer.Fence = 2
	newer.Outcome = "failed"
	newer.ActualState = "unknown"
	newer.RuntimeID = ""
	newer.ObservedAtMS = 200
	if err := db.RecordDeploymentStatus(ctx, event.RegionID, newer); err != nil {
		t.Fatal(err)
	}
	older := event
	older.EventID = "deployment-status-old"
	if err := db.RecordDeploymentStatus(ctx, event.RegionID, older); err != nil {
		t.Fatal("older fence should be acknowledged", err)
	}
	got, _ = db.GetDeploymentStatus(ctx, org.ID, created.Server.ID)
	if got != newer {
		t.Fatal("older fence replaced projection")
	}
	future := newer
	future.EventID = "deployment-status-future"
	future.SpecGeneration++
	if err := db.RecordDeploymentStatus(ctx, event.RegionID, future); !errors.Is(err, deploymentstatus.ErrEventConflict) {
		t.Fatal("future generation accepted", err)
	}
	if _, err := db.GetDeploymentStatus(ctx, "another-org", created.Server.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-tenant status read accepted", err)
	}
}
