package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
)

func TestSubscriptionExpiryLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "expiry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	org := domain.Organization{
		ID:        "org-exp-1",
		Name:      "Exp Org",
		Credits:   100,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.CreateOrganization(ctx, &org, "user-owner"); err != nil {
		t.Fatal(err)
	}

	// Create running game server 1 (expired subscription)
	server1 := domain.GameServer{
		ID:             "srv-exp-1",
		OrganizationID: org.ID,
		Name:           "Expired Server",
		ProviderKey:    domain.ProviderTerrariaVanilla,
		GameKey:        domain.GameTerraria,
		Spec: domain.ServerSpec{
			DesiredState: domain.DesiredRunning,
			Generation:   1,
			Resources: domain.ServerResources{
				CPULimitCores: 2,
				MemoryLimitMB: 4096,
			},
		},
		Status: domain.ServerRuntimeStatus{
			Phase: domain.PhaseRunning,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.CreateAllocatedGameServer(ctx, "user-owner", &server1); err != nil {
		t.Fatal(err)
	}

	// Create running game server 2 (active subscription)
	server2 := domain.GameServer{
		ID:             "srv-active-2",
		OrganizationID: org.ID,
		Name:           "Active Server",
		ProviderKey:    domain.ProviderTerrariaVanilla,
		GameKey:        domain.GameTerraria,
		Spec: domain.ServerSpec{
			DesiredState: domain.DesiredRunning,
			Generation:   1,
			Resources: domain.ServerResources{
				CPULimitCores: 2,
				MemoryLimitMB: 4096,
			},
		},
		Status: domain.ServerRuntimeStatus{
			Phase: domain.PhaseRunning,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.CreateAllocatedGameServer(ctx, "user-owner", &server2); err != nil {
		t.Fatal(err)
	}

	// Seed subscription records in database
	nowMS := time.Now().UnixMilli()
	expiredEndsAtMS := nowMS - 5000   // 5 seconds ago
	activeEndsAtMS := nowMS + 3600000 // 1 hour in future

	sub1 := map[string]any{
		"id":              "sub-1",
		"organization_id": org.ID,
		"server_id":       server1.ID,
		"order_id":        "ord-1",
		"payment_id":      "pay-1",
		"revision_id":     "rev-1",
		"placement_epoch": 1,
		"quote":           "{}",
		"status":          "active",
		"created_at_ms":   nowMS - 10000,
	}
	if err := db.db.Table("service_subscriptions").Create(&sub1).Error; err != nil {
		t.Fatal(err)
	}

	sub2 := map[string]any{
		"id":              "sub-2",
		"organization_id": org.ID,
		"server_id":       server2.ID,
		"order_id":        "ord-2",
		"payment_id":      "pay-2",
		"revision_id":     "rev-2",
		"placement_epoch": 1,
		"quote":           "{}",
		"status":          "active",
		"created_at_ms":   nowMS,
	}
	if err := db.db.Table("service_subscriptions").Create(&sub2).Error; err != nil {
		t.Fatal(err)
	}

	ent1 := entitlements.Record{
		Policy: entitlements.Policy{
			OrganizationID: org.ID,
			ServerID:       server1.ID,
			CPU:            2,
			MemoryMB:       4096,
			StartsAtMS:     nowMS - 10000,
			EndsAtMS:       expiredEndsAtMS,
			Status:         "active",
		},
		Version:    1,
		SourceKind: "prepaid_subscription",
		SourceID:   "sub-1",
	}
	if err := db.db.Table("global_server_entitlements").Create(&ent1).Error; err != nil {
		t.Fatal(err)
	}

	ent2 := entitlements.Record{
		Policy: entitlements.Policy{
			OrganizationID: org.ID,
			ServerID:       server2.ID,
			CPU:            2,
			MemoryMB:       4096,
			StartsAtMS:     nowMS,
			EndsAtMS:       activeEndsAtMS,
			Status:         "active",
		},
		Version:    1,
		SourceKind: "prepaid_subscription",
		SourceID:   "sub-2",
	}
	if err := db.db.Table("global_server_entitlements").Create(&ent2).Error; err != nil {
		t.Fatal(err)
	}

	// 1. Verify ClaimExpiredSubscriptions claims only expired subscriptions
	claimed, err := db.ClaimExpiredSubscriptions(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimExpiredSubscriptions failed: %v", err)
	}
	if len(claimed) != 1 || claimed[0] != server1.ID {
		t.Fatalf("expected [srv-exp-1] claimed, got %v", claimed)
	}

	// 2. Verify CheckServerStartable blocks expired server and allows active server
	if err := db.CheckServerStartable(ctx, server1.ID); !errors.Is(err, commerce.ErrSubscriptionExpired) {
		t.Fatalf("expected ErrSubscriptionExpired for server1, got %v", err)
	}
	if err := db.CheckServerStartable(ctx, server2.ID); err != nil {
		t.Fatalf("expected server2 to be startable, got %v", err)
	}
	if err := db.CheckServerStartable(ctx, "srv-unbilled"); err != nil {
		t.Fatalf("expected unbilled server to be startable, got %v", err)
	}

	// 3. Expire subscription and stop server
	if err := db.ExpireSubscriptionAndStopServer(ctx, server1.ID); err != nil {
		t.Fatalf("ExpireSubscriptionAndStopServer failed: %v", err)
	}

	// 4. Verify DB changes
	var checkEnt entitlements.Record
	if err := db.db.Table("global_server_entitlements").Where("server_id = ?", server1.ID).Take(&checkEnt).Error; err != nil {
		t.Fatal(err)
	}
	if checkEnt.Status != "suspended" {
		t.Fatalf("expected entitlement status suspended, got %s", checkEnt.Status)
	}
	if checkEnt.Version != 2 {
		t.Fatalf("expected entitlement version incremented to 2, got %d", checkEnt.Version)
	}

	var checkSub struct{ Status string }
	if err := db.db.Table("service_subscriptions").Where("server_id = ?", server1.ID).Take(&checkSub).Error; err != nil {
		t.Fatal(err)
	}
	if checkSub.Status != "expired" {
		t.Fatalf("expected subscription status expired, got %s", checkSub.Status)
	}

	// Verify server spec desired_state transitioned to stopped
	updatedServer1, err := db.GetGameServer(ctx, server1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedServer1.Spec.DesiredState != domain.DesiredStopped {
		t.Fatalf("expected server1 DesiredState stopped, got %s", updatedServer1.Spec.DesiredState)
	}
	if updatedServer1.Status.Phase != domain.PhaseStopped {
		t.Fatalf("expected server1 Phase stopped, got %s", updatedServer1.Status.Phase)
	}

	// 5. Verify ClaimExpiredSubscriptions no longer claims server1 since its status is now suspended
	claimedAfter, err := db.ClaimExpiredSubscriptions(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimedAfter) != 0 {
		t.Fatalf("expected 0 claimed after expiration, got %v", claimedAfter)
	}

	// 6. Test idempotency: second call returns nil without error
	if err := db.ExpireSubscriptionAndStopServer(ctx, server1.ID); err != nil {
		t.Fatalf("expected idempotent re-call to succeed, got %v", err)
	}
}
