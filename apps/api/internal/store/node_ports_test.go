package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

func TestNodePortReservations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ports.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testNodePortReservations(t, db)
}
func testNodePortReservations(t *testing.T, db *Store) {
	ctx := context.Background()
	node := domain.ComputeNode{ID: "port-pool-node", CPUCores: 16, MemoryTotalMB: 16384}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	var instances []domain.GameServer
	var assignments []domain.WorkloadAssignment
	for i := 0; i < 8; i++ {
		instance := domain.GameServer{ID: fmt.Sprintf("port-owner-%d", i), NodeID: "port-pool-node", Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}}}
		if err := db.CreateGameServer(ctx, &instance); err != nil {
			t.Fatal(err)
		}
		a := domain.WorkloadAssignment{ID: instance.ID, UID: instance.ID, ServerID: instance.ID, NodeID: instance.NodeID, Generation: 1, DesiredState: domain.DesiredRunning, Spec: domain.WorkloadSpec{ServerID: instance.ID, Network: domain.WorkloadNetwork{Port: 7777, HostPort: 10000 + i, AdditionalPorts: []domain.WorkloadPort{{Port: 7778, HostPort: 11000}}}}}
		instances = append(instances, instance)
		assignments = append(assignments, a)
	}
	type result struct {
		index int
		err   error
	}
	results := make(chan result, 8)
	for i := range instances {
		go func(i int) { results <- result{i, db.PublishWorkloadAssignment(ctx, instances[i], &assignments[i])} }(i)
	}
	winner := -1
	for i := 0; i < 8; i++ {
		r := <-results
		if r.err == nil {
			if winner != -1 {
				t.Fatal("multiple owners acquired shared additional port")
			}
			winner = r.index
		} else if !errors.Is(r.err, ErrNodeAllocationUnavailable) {
			t.Fatal(r.err)
		}
	}
	if winner < 0 {
		t.Fatal("no assignment acquired ports")
	}
	var claims int64
	if err := db.db.Model(&nodePortReservation{}).Where("node_id = ?", "port-pool-node").Count(&claims).Error; err != nil || claims != 2 {
		t.Fatalf("failed publications leaked claims: %d %v", claims, err)
	}
	conflict := assignments[winner]
	conflict.Spec.Network.HostPort = 13000
	if err := db.PublishWorkloadAssignment(ctx, instances[winner], &conflict); !errors.Is(err, ErrReconciliationSuperseded) {
		t.Fatalf("same-generation replacement accepted: %v", err)
	}
	if err := db.db.Model(&nodePortReservation{}).Where("node_id = ? AND host_port = 13000", node.ID).Count(&claims).Error; err != nil || claims != 0 {
		t.Fatalf("failed update leaked port claim: %d %v", claims, err)
	}
	before := instances[winner]
	next := before
	next.Spec.Generation++
	if err := db.SaveGameServer(ctx, &next); err != nil {
		t.Fatal(err)
	}
	replacement := assignments[winner]
	replacement.Generation++
	replacement.Spec.Network.AdditionalPorts = []domain.WorkloadPort{{Port: 7778, HostPort: 12000}}
	if err := db.PublishWorkloadAssignment(ctx, next, &replacement); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteWorkloadAssignment(ctx, before.ID); err != nil {
		t.Fatal(err)
	}
	for _, port := range []int{11000, 12000} {
		var retained int64
		if err := db.db.Model(&nodePortReservation{}).Where("node_id = ? AND host_port = ? AND server_id = ?", before.NodeID, port, before.ID).Count(&retained).Error; err != nil || retained != 1 {
			t.Fatalf("released old binding without runtime evidence: port=%d count=%d err=%v", port, retained, err)
		}
	}
	org := domain.Organization{ID: "port-new-tenant", Slug: "port-new-tenant"}
	if err := db.CreateOrganization(ctx, &org, "owner"); err != nil {
		t.Fatal(err)
	}
	candidate := domain.GameServer{ID: "port-main-candidate", OrganizationID: org.ID, NodeID: node.ID, Spec: domain.ServerSpec{Generation: 1, Resources: domain.ServerResources{CPULimitCores: 1, MemoryLimitMB: 512}, Network: domain.ServerNetworkSpec{HostPort: 11000}}}
	if err := db.CreateAllocatedGameServer(ctx, "owner", &candidate); !errors.Is(err, ErrNodeAllocationUnavailable) || !strings.Contains(err.Error(), "retained") {
		t.Fatalf("primary port ignored retained additional claim: %v", err)
	}
	loser := (winner + 1) % len(instances)
	if err := db.PublishWorkloadAssignment(ctx, instances[loser], &assignments[loser]); !errors.Is(err, ErrNodeAllocationUnavailable) {
		t.Fatalf("retired assignment released live port: %v", err)
	}
}

func seedLegacyNodePorts(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, sql := range []string{
		`INSERT INTO game_servers(id,node_id,spec) VALUES ('legacy-port-a','legacy-port-node','{"network":{"hostPort":12000}}')`,
		`INSERT INTO workload_assignments(id,uid,server_id,node_id,spec) VALUES ('legacy-port-task','legacy-port-task','legacy-port-b','legacy-port-node','{"network":{"port":7777,"hostPort":12000,"additionalPorts":[{"port":7778,"hostPort":12001}]}}')`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
}
func verifyLegacyNodePorts(t *testing.T, db *Store) {
	t.Helper()
	var count int64
	if err := db.db.Model(&nodePortReservation{}).Where("node_id = ?", "legacy-port-node").Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("backfill lost historical owners/additional ports: %d %v", count, err)
	}
	if err := db.db.Model(&nodePortReservation{}).Where("node_id = ? AND host_port = 12000", "legacy-port-node").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("migration chose a conflicting owner: %d %v", count, err)
	}
}
func TestSQLiteNodePortUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedLegacyNodePorts(t, db.db)
	for _, sql := range []string{"DROP TABLE node_port_reservations", "DROP TABLE node_port_pools", "DELETE FROM gamepanel_sqlite_migrations WHERE version = 2"} {
		if err := db.db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	for i := 0; i < 2; i++ {
		db, err = Open(path)
		if err != nil {
			t.Fatal(err)
		}
		verifyLegacyNodePorts(t, db)
		db.Close()
	}
}

func TestSQLiteNodePortBackfillBatchesAndRollback(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "port-batches.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows := make([]domain.WorkloadAssignment, idLookupBatchSize+1)
	for i := range rows {
		rows[i] = domain.WorkloadAssignment{ID: fmt.Sprintf("batch-%04d", i), UID: fmt.Sprintf("batch-%04d", i), ServerID: fmt.Sprintf("server-%04d", i), NodeID: "shared-node"}
	}
	if err := db.db.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.db.Model(&domain.WorkloadAssignment{}).Where("node_id = ?", "shared-node").UpdateColumn("spec", `{"network":{"port":7777,"additionalPorts":[{"hostPort":8888},{"hostPort":8888}]}}`).Error; err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"DROP TABLE node_port_reservations", "DROP TABLE node_port_pools", "DELETE FROM gamepanel_sqlite_migrations WHERE version = 2"} {
		if err := db.db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	// The invalid record is on the second page, after earlier writes occurred.
	if err := db.db.Model(&domain.WorkloadAssignment{}).Where("id = ?", rows[len(rows)-1].ID).UpdateColumn("spec", `{"network":{"hostPort":65536}}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLiteNodePorts(db.db); err == nil {
		t.Fatal("invalid historical port accepted")
	}
	var applied int64
	if err := db.db.Table("gamepanel_sqlite_migrations").Where("version = 2").Count(&applied).Error; err != nil || applied != 0 {
		t.Fatalf("failed migration recorded success: %d %v", applied, err)
	}
	if db.db.Migrator().HasTable(&nodePortReservation{}) {
		t.Fatal("failed migration retained partial schema")
	}
	if err := db.db.Model(&domain.WorkloadAssignment{}).Where("id = ?", rows[len(rows)-1].ID).UpdateColumn("spec", `{"network":{"port":7777,"additionalPorts":[{"hostPort":8888}]}}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.db.Model(&domain.WorkloadAssignment{}).Where("id = ?", rows[len(rows)-1].ID).UpdateColumn("server_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLiteNodePorts(db.db); err == nil {
		t.Fatal("NULL port owner silently converted to empty string")
	}
	if err := db.db.Model(&domain.WorkloadAssignment{}).Where("id = ?", rows[len(rows)-1].ID).UpdateColumn("server_id", rows[len(rows)-1].ServerID).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateSQLiteNodePorts(db.db); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.db.Model(&nodePortReservation{}).Count(&count).Error; err != nil || count != int64(2*len(rows)) {
		t.Fatalf("lost historical owners or retained duplicates: %d %v", count, err)
	}
	if err := db.db.Model(&nodePortPool{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("pool backfill: %d %v", count, err)
	}
}
