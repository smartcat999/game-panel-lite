package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestTenantActivity(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testTenantActivity(t, db)
}
func testTenantActivity(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []string{"activity-a", "activity-b"} {
		org := domain.Organization{ID: id, Slug: id}
		if err := db.CreateOrganization(ctx, &org, id); err != nil {
			t.Fatal(err)
		}
		server := domain.GameServer{ID: id, OrganizationID: id}
		if err := db.CreateGameServer(ctx, &server); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 65; i++ {
		owner := "activity-b"
		if i < 5 {
			owner = "activity-a"
		}
		event := domain.ActivityEvent{ID: fmt.Sprintf("activity-owned-%d", i), InstanceID: owner, CreatedAt: time.Now().Add(time.Duration(i) * time.Second), Payload: map[string]any{"owner": owner}}
		if err := db.CreateActivity(ctx, &event); err != nil {
			t.Fatal(err)
		}
		if event.OrganizationID != owner {
			t.Fatalf("event owner not captured: %+v", event)
		}
	}
	events, err := db.ListUserActivity(ctx, "activity-a", "", 3)
	if err != nil || len(events) != 3 {
		t.Fatalf("filter before limit: %d %v", len(events), err)
	}
	for _, event := range events {
		if event.OrganizationID != "activity-a" || event.Payload["owner"] != "activity-a" {
			t.Fatalf("foreign payload: %+v", event)
		}
	}
	// Retain history after the instance is deleted; membership still controls it.
	if err := db.db.Delete(&domain.GameServer{}, "id = ?", "activity-a").Error; err != nil {
		t.Fatal(err)
	}
	events, err = db.ListUserActivity(ctx, "activity-a", "activity-a", 100)
	if err != nil || len(events) != 5 {
		t.Fatalf("deleted instance history: %d %v", len(events), err)
	}
	if err := db.RemoveOrganizationMember(ctx, "activity-a", "activity-a"); err != nil {
		t.Fatal(err)
	}
	events, err = db.ListUserActivity(ctx, "activity-a", "", 50)
	if err != nil || len(events) != 0 {
		t.Fatalf("revoked activity: %d %v", len(events), err)
	}
	events, err = db.ListUserActivity(ctx, "", "", 50)
	if err != nil || len(events) != 0 {
		t.Fatalf("empty principal activity: %d %v", len(events), err)
	}
	if err := db.db.Exec("INSERT INTO game_servers(id,organization_id) VALUES (?,NULL)", "activity-null-owner").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.db.Create(&domain.ActivityEvent{ID: "activity-null-history", InstanceID: "activity-null-owner", OrganizationID: ""}).Error; err != nil {
		t.Fatal(err)
	}
	events, err = db.ListCurrentInstanceActivity(ctx, "activity-null-owner", 10)
	if err != nil || len(events) != 0 {
		t.Fatalf("NULL ownership became an empty-string grant: %d %v", len(events), err)
	}
	events, err = db.ListCurrentInstanceActivity(ctx, "activity-missing-instance", 10)
	if err != nil || len(events) != 0 {
		t.Fatalf("missing instance returned activity: %d %v", len(events), err)
	}
}

func TestSQLiteBackfillsActivityOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org := domain.Organization{ID: "legacy-activity-org", Slug: "legacy-activity-org"}
	if err := db.CreateOrganization(ctx, &org, "legacy-user"); err != nil {
		t.Fatal(err)
	}
	server := domain.GameServer{ID: "legacy-activity-server", OrganizationID: org.ID}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	if err := db.db.Exec("INSERT INTO activity_events (id, instance_id) VALUES (?, ?)", "legacy-activity", server.ID).Error; err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	events, err := db.ListUserActivity(ctx, "legacy-user", server.ID, 50)
	if err != nil || len(events) != 1 || events[0].OrganizationID != org.ID {
		t.Fatalf("activity backfill: %+v %v", events, err)
	}
}
