package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestBulkOwnerQueries(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "owners.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testBulkOwnerQueries(t, db)
}

func testBulkOwnerQueries(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	var organizations []domain.Organization
	var memberships []domain.OrganizationMember
	var events []domain.ActivityEvent
	for i := 0; i < idLookupBatchSize+1; i++ {
		id := fmt.Sprintf("bulk-org-%04d", i)
		organizations = append(organizations, domain.Organization{ID: id, Slug: id, CreatedAt: time.Unix(0, 0).UTC()})
		memberships = append(memberships, domain.OrganizationMember{ID: id, OrganizationID: id, UserID: "bulk-reader", Role: domain.RoleViewer})
		events = append(events, domain.ActivityEvent{ID: fmt.Sprintf("bulk-event-%04d", i), OrganizationID: id, CreatedAt: time.Unix(int64(i), 0).UTC()})
	}
	for _, value := range []any{&organizations, &memberships, &events} {
		if err := db.db.CreateInBatches(value, 100).Error; err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListUserOrganizations(ctx, "bulk-reader")
	if err != nil || len(got) != len(organizations) || got[0].ID != organizations[0].ID || got[len(got)-1].ID != organizations[len(organizations)-1].ID {
		t.Fatalf("batched organization query lost ordering or records: count=%d err=%v", len(got), err)
	}
	latest, err := db.ListUserActivity(ctx, "bulk-reader", "", 2)
	if err != nil || len(latest) != 2 || latest[0].ID != events[len(events)-1].ID || latest[1].ID != events[len(events)-2].ID {
		t.Fatalf("ID-set filter limited the wrong batch: %+v %v", latest, err)
	}
	foreign, err := db.ListUserActivity(ctx, "foreign-reader", "", 2)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("ID-set filter leaked another tenant: %+v %v", foreign, err)
	}
}
