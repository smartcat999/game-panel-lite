package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/deploymentstatus"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type globalDeploymentStatusRow struct {
	ServerID, OrganizationID, RegionID            string
	OperationID, RevisionID, TaskID, NodeID       string
	ActualState, Outcome, RuntimeID               string
	EventID, PayloadHash                          string
	PlacementEpoch, SpecGeneration, IntentVersion int64
	Fence, ObservedAtMS                           int64
}

func (r globalDeploymentStatusRow) event() deploymentstatus.Event {
	return deploymentstatus.Event{SchemaVersion: 1, EventID: r.EventID, RegionID: r.RegionID, OrganizationID: r.OrganizationID, OperationID: r.OperationID, ServerID: r.ServerID, RevisionID: r.RevisionID, TaskID: r.TaskID, NodeID: r.NodeID, PlacementEpoch: r.PlacementEpoch, SpecGeneration: r.SpecGeneration, IntentVersion: r.IntentVersion, Fence: r.Fence, ActualState: r.ActualState, Outcome: r.Outcome, RuntimeID: r.RuntimeID, ObservedAtMS: r.ObservedAtMS}
}

func deploymentStatusRow(event deploymentstatus.Event, hash string) globalDeploymentStatusRow {
	return globalDeploymentStatusRow{ServerID: event.ServerID, OrganizationID: event.OrganizationID, RegionID: event.RegionID, OperationID: event.OperationID, RevisionID: event.RevisionID, TaskID: event.TaskID, NodeID: event.NodeID, ActualState: event.ActualState, Outcome: event.Outcome, RuntimeID: event.RuntimeID, EventID: event.EventID, PayloadHash: hash, PlacementEpoch: event.PlacementEpoch, SpecGeneration: event.SpecGeneration, IntentVersion: event.IntentVersion, Fence: event.Fence, ObservedAtMS: event.ObservedAtMS}
}

// RecordDeploymentStatus projects one authenticated Region observation. All
// global identities are re-read by ID in the same transaction; stale events are
// acknowledged without changing current state and no query joins tables.
func (s *Store) RecordDeploymentStatus(ctx context.Context, source string, event deploymentstatus.Event) error {
	if event.Validate() != nil || source == "" || source != event.RegionID {
		return deploymentstatus.ErrInvalidEvent
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(payload))
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var region regions.Entry
		if err := tx.Table("global_regions").Where("id = ?", source).Clauses(clause.Locking{Strength: "SHARE"}).Take(&region).Error; err != nil {
			return deploymentstatus.ErrEventConflict
		}
		var server instances.Server
		if err := tx.Table("logical_servers").Where("id = ?", event.ServerID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&server).Error; err != nil {
			return deploymentstatus.ErrEventConflict
		}
		if server.OrganizationID != event.OrganizationID {
			return deploymentstatus.ErrEventConflict
		}
		var placement instances.Placement
		if err := tx.Table("server_placements").Where("server_id = ?", event.ServerID).Take(&placement).Error; err != nil {
			return deploymentstatus.ErrEventConflict
		}
		older := event.PlacementEpoch <= placement.PlacementEpoch && event.SpecGeneration <= server.SpecGeneration && event.IntentVersion <= server.IntentVersion &&
			(event.PlacementEpoch < placement.PlacementEpoch || event.SpecGeneration < server.SpecGeneration || event.IntentVersion < server.IntentVersion)
		if older {
			return nil
		}
		if event.PlacementEpoch != placement.PlacementEpoch || event.RegionID != placement.RegionID || event.SpecGeneration != server.SpecGeneration || event.IntentVersion != server.IntentVersion || event.RevisionID != server.CurrentRevisionID {
			return deploymentstatus.ErrEventConflict
		}
		var operation globalOperationRow
		if err := tx.Table("server_operations").Where("id = ? AND organization_id = ? AND server_id = ? AND revision_id = ?", event.OperationID, event.OrganizationID, event.ServerID, event.RevisionID).Take(&operation).Error; err != nil {
			return deploymentstatus.ErrEventConflict
		}
		var current globalDeploymentStatusRow
		err := tx.Table("global_deployment_statuses").Where("server_id = ?", event.ServerID).Take(&current).Error
		if err == nil {
			sameVersion := current.RegionID == event.RegionID && current.PlacementEpoch == event.PlacementEpoch && current.RevisionID == event.RevisionID && current.SpecGeneration == event.SpecGeneration && current.IntentVersion == event.IntentVersion
			if sameVersion {
				if event.Fence < current.Fence {
					return nil
				}
				if event.Fence == current.Fence {
					if event.EventID == current.EventID && hash == current.PayloadHash {
						return nil
					}
					return deploymentstatus.ErrEventConflict
				}
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row := deploymentStatusRow(event, hash)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Table("global_deployment_statuses").Create(&row).Error; err != nil {
				return err
			}
		} else {
			updated := tx.Table("global_deployment_statuses").Where("server_id = ? AND fence = ?", event.ServerID, current.Fence).Updates(map[string]any{
				"organization_id": event.OrganizationID, "region_id": event.RegionID, "operation_id": event.OperationID, "revision_id": event.RevisionID,
				"task_id": event.TaskID, "node_id": event.NodeID, "placement_epoch": event.PlacementEpoch, "spec_generation": event.SpecGeneration,
				"intent_version": event.IntentVersion, "fence": event.Fence, "actual_state": event.ActualState, "outcome": event.Outcome,
				"runtime_id": event.RuntimeID, "observed_at_ms": event.ObservedAtMS, "event_id": event.EventID, "payload_hash": hash, "received_at": gorm.Expr("CURRENT_TIMESTAMP"),
			})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return deploymentstatus.ErrEventConflict
			}
		}
		if event.Outcome == "succeeded" && event.ActualState == "running" {
			updated := tx.Table("server_operations").Where("id = ? AND status = ?", operation.ID, "pending").Update("status", "succeeded")
			if updated.Error != nil {
				return updated.Error
			}
		}
		return nil
	})
}

func (s *Store) GetDeploymentStatus(ctx context.Context, organizationID, serverID string) (deploymentstatus.Event, error) {
	if organizationID == "" || serverID == "" {
		return deploymentstatus.Event{}, ErrNotFound
	}
	var row globalDeploymentStatusRow
	err := s.db.WithContext(ctx).Table("global_deployment_statuses").Where("server_id = ? AND organization_id = ?", serverID, organizationID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return deploymentstatus.Event{}, ErrNotFound
	}
	if err != nil {
		return deploymentstatus.Event{}, err
	}
	return row.event(), nil
}

func migrateSQLiteDeploymentStatus(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 19").Count(&count).Error; err != nil || count > 0 {
			return err
		}
		if err := tx.Exec(`CREATE TABLE global_deployment_statuses (
server_id text PRIMARY KEY REFERENCES logical_servers(id), organization_id text NOT NULL REFERENCES organizations(id), region_id text NOT NULL REFERENCES global_regions(id),
operation_id text NOT NULL REFERENCES server_operations(id), revision_id text NOT NULL REFERENCES server_revisions(id), task_id text NOT NULL, node_id text NOT NULL,
placement_epoch integer NOT NULL CHECK(placement_epoch > 0), spec_generation integer NOT NULL CHECK(spec_generation > 0), intent_version integer NOT NULL CHECK(intent_version > 0),
fence integer NOT NULL CHECK(fence > 0), actual_state text NOT NULL CHECK(actual_state IN ('running','stopped','missing','unknown')), outcome text NOT NULL CHECK(outcome IN ('succeeded','failed')),
runtime_id text NOT NULL DEFAULT '', observed_at_ms integer NOT NULL CHECK(observed_at_ms > 0), event_id text NOT NULL UNIQUE, payload_hash text NOT NULL CHECK(length(payload_hash) = 64), received_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP)`).Error; err != nil {
			return err
		}
		if err := tx.Exec("CREATE INDEX idx_global_deployment_status_owner ON global_deployment_statuses(organization_id,server_id)").Error; err != nil {
			return err
		}
		if err := tx.Exec("CREATE INDEX idx_global_deployment_status_region ON global_deployment_statuses(region_id,server_id)").Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(19)").Error
	})
}
