package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
)

type globalBackupTaskRow struct {
	ID, OperationID, OrganizationID, ServerID, RegionID, Status, Command string
	CreatedAt                                                            time.Time
}

// RequestGlobalBackup atomically persists the user operation, backup task and
// dedicated command outbox. It performs no Node, file or broker I/O.
func (s *Store) RequestGlobalBackup(ctx context.Context, actor string, request backup.Request) (backup.Task, error) {
	if actor == "" {
		return backup.Task{}, ErrWorkspaceWriteDenied
	}
	if err := request.Validate(); err != nil {
		return backup.Task{}, err
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return backup.Task{}, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(encoded))
	var result backup.Task
	err = s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspaceWriter(ctx, request.OrganizationID, actor); err != nil {
			return err
		}
		var operation globalOperationRow
		err := tx.db.WithContext(ctx).Table("server_operations").Where("organization_id = ? AND kind = ? AND idempotency_key = ?", request.OrganizationID, "backup", request.IdempotencyKey).Take(&operation).Error
		if err == nil {
			if operation.RequestHash != hash {
				return instances.ErrIdempotencyConflict
			}
			var row globalBackupTaskRow
			if err := tx.db.Table("global_backup_tasks").Where("operation_id = ? AND organization_id = ? AND server_id = ?", operation.ID, request.OrganizationID, request.ServerID).Take(&row).Error; err != nil {
				return err
			}
			var command backup.Requested
			if json.Unmarshal([]byte(row.Command), &command) != nil || command.Validate() != nil || command.BackupID != row.ID || command.OperationID != operation.ID || command.OrganizationID != row.OrganizationID || command.ServerID != row.ServerID || command.RegionID != row.RegionID || command.Scope != request.Scope {
				return backup.ErrUploadPlan
			}
			result = backup.Task{Request: command, Status: row.Status}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		locked := tx.db.Table("logical_servers").Where("id = ? AND organization_id = ? AND desired_state <> ?", request.ServerID, request.OrganizationID, "deleted").UpdateColumn("intent_version", gorm.Expr("intent_version"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return ErrNotFound
		}
		var server instances.Server
		if err := tx.db.Table("logical_servers").Where("id = ?", request.ServerID).Take(&server).Error; err != nil {
			return err
		}
		var placement instances.Placement
		if err := tx.db.Table("server_placements").Where("server_id = ?", server.ID).Take(&placement).Error; err != nil {
			return err
		}
		var revision globalRevisionRow
		if err := tx.db.Table("server_revisions").Select("id,server_id,spec_generation").Where("id = ? AND server_id = ? AND spec_generation = ?", server.CurrentRevisionID, server.ID, server.SpecGeneration).Take(&revision).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		operation = globalOperationRow{ID: uuid.NewString(), OrganizationID: request.OrganizationID, ServerID: server.ID, RevisionID: revision.ID, Kind: "backup", Status: "pending", IdempotencyKey: request.IdempotencyKey, RequestHash: hash, CreatedAt: now}
		command := backup.Requested{SchemaVersion: 1, EventID: uuid.NewString(), OperationID: operation.ID, BackupID: uuid.NewString(), OrganizationID: request.OrganizationID, ServerID: server.ID, RegionID: placement.RegionID, RevisionID: revision.ID, SpecGeneration: revision.SpecGeneration, IntentVersion: server.IntentVersion, PlacementEpoch: placement.PlacementEpoch, Scope: request.Scope}
		if err := command.Validate(); err != nil {
			return err
		}
		payload, err := json.Marshal(command)
		if err != nil {
			return err
		}
		row := globalBackupTaskRow{ID: command.BackupID, OperationID: operation.ID, OrganizationID: request.OrganizationID, ServerID: server.ID, RegionID: placement.RegionID, Status: "requested", Command: string(payload), CreatedAt: now}
		outbox := map[string]any{"id": command.EventID, "operation_id": operation.ID, "region_id": placement.RegionID, "payload": string(payload), "created_at": now}
		if err := tx.db.Table("server_operations").Create(&operation).Error; err != nil {
			return err
		}
		if err := tx.db.Table("global_backup_tasks").Create(&row).Error; err != nil {
			return err
		}
		if err := tx.db.Table("backup_request_outbox").Create(outbox).Error; err != nil {
			return err
		}
		result = backup.Task{Request: command, Status: row.Status}
		return nil
	})
	if err != nil {
		return backup.Task{}, err
	}
	return result, nil
}

func migrateSQLiteGlobalBackupTasks(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 8").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(globalBackupTasksSQL).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(8)").Error
	})
}
