package store

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
)

func testGlobalBackupCheck(t *testing.T, db *Store, request backup.Requested) {
	t.Helper()
	ctx := context.Background()
	if err := db.CheckRegionalBackup(ctx, request.RegionID, request); err != nil {
		t.Fatal(err)
	}
	for _, region := range []string{"", "foreign"} {
		if !errors.Is(db.CheckRegionalBackup(ctx, region, request), backup.ErrRequestUnavailable) {
			t.Fatal("foreign service accepted")
		}
	}
	changed := request
	changed.Scope = "instance"
	if !errors.Is(db.CheckRegionalBackup(ctx, request.RegionID, changed), backup.ErrRequestUnavailable) {
		t.Fatal("modified request accepted")
	}
	for _, tc := range []struct {
		table, key, id, column string
		value                  any
	}{
		{"global_backup_tasks", "id", request.BackupID, "status", "cancelled"},
		{"global_backup_tasks", "id", request.BackupID, "status", "succeeded"},
		{"server_operations", "id", request.OperationID, "status", "failed"},
		{"server_operations", "id", request.OperationID, "kind", "create"},
		{"logical_servers", "id", request.ServerID, "desired_state", "deleted"},
		{"logical_servers", "id", request.ServerID, "intent_version", request.IntentVersion + 1},
		{"logical_servers", "id", request.ServerID, "spec_generation", request.SpecGeneration + 1},
		{"server_placements", "server_id", request.ServerID, "region_id", "other"},
		{"server_placements", "server_id", request.ServerID, "placement_epoch", request.PlacementEpoch + 1},
	} {
		t.Run(tc.table+"/"+tc.column, func(t *testing.T) {
			rollback := errors.New("rollback fixture")
			err := db.Transaction(ctx, func(tx *Store) error {
				if err := tx.db.Table(tc.table).Where(tc.key+" = ?", tc.id).UpdateColumn(tc.column, tc.value).Error; err != nil {
					return err
				}
				if err := tx.CheckRegionalBackup(ctx, request.RegionID, request); !errors.Is(err, backup.ErrRequestUnavailable) {
					t.Errorf("stale request accepted: %v", err)
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatal(err)
			}
		})
	}
	if err := db.CheckRegionalBackup(ctx, request.RegionID, request); err != nil {
		t.Fatal("fixture rollback failed", err)
	}
}
