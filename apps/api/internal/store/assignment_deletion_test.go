package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

func TestAssignmentReferenceDeletion(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "deletion.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	testAssignmentReferenceDeletion(t, db)
}

func testAssignmentReferenceDeletion(t *testing.T, db *Store) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []string{"delete-refs-target", "delete-refs-other"} {
		if err := db.db.Create(&domain.WorkloadAssignment{ID: id, UID: id, ServerID: id}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.db.Create(&artifactReference{AssignmentID: id, ArtifactID: id, OrganizationID: id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	const callback = "test:assignment-delete-failure"
	if err := db.db.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "workload_assignments" {
			tx.AddError(errors.New("injected assignment deletion failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failure := db.DeleteWorkloadAssignment(ctx, "delete-refs-target")
	if err := db.db.Callback().Delete().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if failure == nil {
		t.Fatal("deletion failure was hidden")
	}
	for _, id := range []string{"delete-refs-target", "delete-refs-other"} {
		var count int64
		if err := db.db.Model(&artifactReference{}).Where("assignment_id = ?", id).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("failed deletion lost references for %s: %d %v", id, count, err)
		}
	}
	if err := db.DeleteWorkloadAssignment(ctx, "delete-refs-target"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteWorkloadAssignment(ctx, "delete-refs-target"); err != nil {
		t.Fatalf("repeat deletion: %v", err)
	}
	for _, table := range []string{"workload_assignments", "workload_artifact_references"} {
		column := "id"
		if table == "workload_artifact_references" {
			column = "assignment_id"
		}
		var count int64
		if err := db.db.Table(table).Where(column+" = ?", "delete-refs-target").Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("target remained in %s: %d %v", table, count, err)
		}
		if err := db.db.Table(table).Where(column+" = ?", "delete-refs-other").Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("foreign data removed from %s: %d %v", table, count, err)
		}
	}
}
