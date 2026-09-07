package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestTenantServerQueries(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testTenantServerQueries(t, db)
}

func testTenantServerQueries(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	for _, user := range []string{"scope-a", "scope-b"} {
		org := domain.Organization{ID: user, Slug: user}
		if err := db.CreateOrganization(ctx, &org, user); err != nil {
			t.Fatal(err)
		}
		server := domain.GameServer{ID: user, Name: "search-me", OrganizationID: user, CreatedAt: time.Now(), UpdatedAt: time.Now(), Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}
		if err := db.CreateGameServer(ctx, &server); err != nil {
			t.Fatal(err)
		}
	}
	for _, status := range []string{"", "stopped", "running"} {
		page, err := db.ListUserGameServersPage(ctx, "scope-a", GameServerListOptions{Search: "search-me", Status: status, Sort: "status"})
		want := int64(1)
		if status == "running" {
			want = 0
		}
		if err != nil || page.Total != want || len(page.Items) != int(want) {
			t.Fatalf("scope page %s: %+v %v", status, page, err)
		}
		if want == 1 && page.Items[0].ID != "scope-a" {
			t.Fatalf("foreign item: %+v", page.Items)
		}
	}
	for _, user := range []string{"scope-a", "scope-b", ""} {
		servers, err := db.ListUserGameServers(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		if user == "" {
			if len(servers) != 0 {
				t.Fatal("empty principal read")
			}
			continue
		}
		if len(servers) != 1 || servers[0].ID != user {
			t.Fatalf("scope list: %+v", servers)
		}
	}
	if err := db.RemoveOrganizationMember(ctx, "scope-a", "scope-a"); err != nil {
		t.Fatal(err)
	}
	page, err := db.ListUserGameServersPage(ctx, "scope-a", GameServerListOptions{})
	if err != nil || page.Total != 0 {
		t.Fatalf("revoked scope: %+v %v", page, err)
	}
}
