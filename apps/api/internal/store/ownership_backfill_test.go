package store

import (
	"fmt"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

func TestOwnershipBackfillBatches(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ownership.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	instances := make([]struct{ ID, OrganizationID string }, idLookupBatchSize+1)
	rows := make([]struct{ ID, InstanceID, OrganizationID string }, len(instances))
	for i := range instances {
		instances[i].ID = fmt.Sprintf("owner-server-%04d", i)
		instances[i].OrganizationID = fmt.Sprintf("owner-%04d", i)
		rows[i].ID, rows[i].InstanceID = instances[i].ID, instances[i].ID
	}
	if err := db.db.Table("game_servers").CreateInBatches(instances, 200).Error; err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"worlds", "activity_events"} {
		if err := db.db.Table(table).CreateInBatches(rows, 200).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.db.Table(table).Create(map[string]any{"id": "retained", "instance_id": instances[0].ID, "organization_id": "original-owner"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.db.Table(table).Create(map[string]any{"id": "orphan", "instance_id": "missing", "organization_id": ""}).Error; err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if err := db.db.Transaction(func(tx *gorm.DB) error { return backfillInstanceOwnership(tx, table) }); err != nil {
				t.Fatal(err)
			}
		}
		var got []struct {
			ID             string
			OrganizationID *string
		}
		if err := db.db.Table(table).Select("id,organization_id").Order("id").Find(&got).Error; err != nil {
			t.Fatal(err)
		}
		if len(got) != len(rows)+2 {
			t.Fatalf("%s lost rows: %d", table, len(got))
		}
		for _, row := range got {
			switch row.ID {
			case "orphan":
				if row.OrganizationID == nil || *row.OrganizationID != "" {
					t.Fatalf("%s orphan adopted", table)
				}
			case "retained":
				if row.OrganizationID == nil || *row.OrganizationID != "original-owner" {
					t.Fatalf("%s existing ownership overwritten", table)
				}
			default:
				var index int
				if _, err := fmt.Sscanf(row.ID, "owner-server-%04d", &index); err != nil {
					t.Fatal(err)
				}
				if row.OrganizationID == nil || *row.OrganizationID != instances[index].OrganizationID {
					t.Fatalf("%s wrong batch owner: %s", table, row.ID)
				}
			}
		}
	}
}
