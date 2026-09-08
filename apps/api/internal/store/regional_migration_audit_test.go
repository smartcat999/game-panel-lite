package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRegionalMigrationAudit(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testRegionalMigrationAudit(t, db)
}

func testRegionalMigrationAudit(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	baseline, err := db.AuditRegionalMigration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO organizations(id,name) VALUES ('audit-org','Audit')`,
		`INSERT INTO compute_nodes(id,region,token) VALUES ('audit-east','east','private-node-token'),('audit-west','west',''),('audit-unset','',''),('audit-space',' east ','')`,
		// Invalid JSON in runtime/config columns is intentional: ownership
		// auditing must not deserialize or expose those unrelated payloads.
		`INSERT INTO game_servers(id,organization_id,node_id,spec,status) VALUES ('audit-valid','audit-org','audit-east','private-config','private-runtime'),('audit-local','audit-org','','',''),('audit-orphan','missing-org','missing-node','',''),('audit-unknown-region','audit-org','audit-unset','','')`,
		`INSERT INTO workload_assignments(id,uid,server_id,node_id,spec) VALUES ('audit-retiring','audit-retiring','audit-valid','audit-west','private-task'),('audit-lost','audit-lost','missing-server','missing-node','')`,
	} {
		if err := db.db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	first, err := db.AuditRegionalMigration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.AuditRegionalMigration(ctx)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("audit is not stable: %v", err)
	}
	if first.Servers != baseline.Servers+4 || first.Nodes != baseline.Nodes+4 || first.Assignments != baseline.Assignments+2 {
		t.Fatalf("audit omitted source rows: %+v", first)
	}
	wantIssues := map[string]string{
		"audit-unset/region_required":            "node",
		"audit-space/region_not_canonical":       "node",
		"audit-local/node_ambiguous":             "server",
		"audit-orphan/organization_unresolved":   "server",
		"audit-orphan/node_missing":              "server",
		"audit-unknown-region/region_unresolved": "server",
		"audit-retiring/placement_diverged":      "assignment",
		"audit-lost/server_missing":              "assignment",
		"audit-lost/node_missing":                "assignment",
	}
	for _, issue := range first.Issues {
		if !strings.HasPrefix(issue.ID, "audit-") {
			continue
		}
		key := issue.ID + "/" + issue.Code
		if wantIssues[key] != issue.Kind {
			t.Fatalf("unexpected issue: %+v", issue)
		}
		delete(wantIssues, key)
	}
	if len(wantIssues) != 0 {
		t.Fatalf("missed ownership problems: %v", wantIssues)
	}
	var placements []LegacyRegionalPlacement
	for _, placement := range first.Placements {
		if strings.HasPrefix(placement.ServerID, "audit-") {
			placements = append(placements, placement)
		}
	}
	want := []LegacyRegionalPlacement{{"audit-valid", "audit-org", "east", "audit-east"}}
	if !reflect.DeepEqual(placements, want) {
		t.Fatalf("guessed or lost ownership: %+v", placements)
	}
	encoded, err := json.Marshal(first)
	if err != nil || strings.Contains(string(encoded), "private-") {
		t.Fatalf("audit exposed private payloads: %v", err)
	}
	var rows []struct{ Spec, Status string }
	if err := db.db.Table("game_servers").Select("spec,status").Where("id = ?", "audit-valid").Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Spec != "private-config" || rows[0].Status != "private-runtime" {
		t.Fatal("audit modified source payloads")
	}
	// Remove only this test's fixtures; malformed payloads must not affect the
	// other PostgreSQL integration cases that share the schema.
	for _, table := range []string{"workload_assignments", "game_servers", "compute_nodes", "organizations"} {
		if err := db.db.Table(table).Where("id LIKE ?", "audit-%").Delete(map[string]any{}).Error; err != nil {
			t.Fatal(err)
		}
	}
}
