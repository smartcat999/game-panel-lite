package store

import "testing"

// The fixture is inserted before migration, without the new column. Use a SQL
// projection so this test exercises migration semantics independently of GORM's
// current model defaults and SQLite AutoMigrate.
func testNodeSchedulingMigration(t *testing.T, db *Store) {
	t.Helper()
	var node struct {
		Name, Token, Status string
		Unschedulable       bool
	}
	if err := db.db.Table("compute_nodes").Select("name, token, status, unschedulable").Where("id = ?", "legacy-scheduling-node").Take(&node).Error; err != nil {
		t.Fatal(err)
	}
	if node.Name != "preserved-name" || node.Token != "preserved-token" || node.Status != "offline" || node.Unschedulable {
		t.Fatal("migration altered existing node configuration or scheduling default")
	}
	if err := db.db.Exec("INSERT INTO compute_nodes (id) VALUES (?)", "default-scheduling-node").Error; err != nil {
		t.Fatal(err)
	}
	var schedulable bool
	if err := db.db.Raw("SELECT NOT unschedulable FROM compute_nodes WHERE id = ?", "default-scheduling-node").Scan(&schedulable).Error; err != nil || !schedulable {
		t.Fatalf("new node default: %v", err)
	}
	if err := db.db.Exec("UPDATE compute_nodes SET unschedulable = NULL WHERE id = ?", "legacy-scheduling-node").Error; err == nil {
		t.Fatal("nullable scheduling flag accepted")
	}
	if err := db.db.Exec("UPDATE compute_nodes SET unschedulable = true WHERE id = ?", "legacy-scheduling-node").Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"legacy-scheduling-node", "default-scheduling-node"} {
		var architecture string
		if err := db.db.Raw("SELECT runtime_architecture FROM compute_nodes WHERE id = ?", id).Scan(&architecture).Error; err != nil || architecture != "" {
			t.Fatalf("unexpected architecture backfill for %s: %q %v", id, architecture, err)
		}
	}
	if err := db.db.Exec("UPDATE compute_nodes SET runtime_architecture = NULL WHERE id = ?", "legacy-scheduling-node").Error; err == nil {
		t.Fatal("nullable runtime architecture accepted")
	}
	if err := db.db.Exec("UPDATE compute_nodes SET runtime_architecture = 'arm64' WHERE id = ?", "legacy-scheduling-node").Error; err != nil {
		t.Fatal(err)
	}

}
