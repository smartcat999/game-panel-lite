package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
)

// ReviseGlobalServer appends a revision and advances its global pointer. It
// never modifies user desired state, placement epoch or any regional runtime.
func (s *Store) ReviseGlobalServer(ctx context.Context, actor string, request instances.ReviseRequest) (instances.IntentResult, error) {
	if actor == "" {
		return instances.IntentResult{}, ErrWorkspaceWriteDenied
	}
	if err := request.Validate(); err != nil {
		return instances.IntentResult{}, err
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return instances.IntentResult{}, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(encoded))
	specification, err := instances.EncodeSpecification(request.Specification)
	if err != nil {
		return instances.IntentResult{}, err
	}
	var result instances.IntentResult
	err = s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspaceWriter(ctx, request.OrganizationID, actor); err != nil {
			return err
		}
		var operation globalOperationRow
		err := tx.db.WithContext(ctx).Table("server_operations").Where("organization_id = ? AND kind = ? AND idempotency_key = ?", request.OrganizationID, "revise", request.IdempotencyKey).Take(&operation).Error
		if err == nil {
			if operation.RequestHash != hash {
				return instances.ErrIdempotencyConflict
			}
			result, err = tx.readGlobalIntent(ctx, operation)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var server instances.Server
		if err := tx.db.WithContext(ctx).Table("logical_servers").Where("id = ? AND organization_id = ?", request.ServerID, request.OrganizationID).Take(&server).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if server.SpecGeneration != request.ExpectedGeneration || server.DesiredState == "deleted" {
			return instances.ErrVersionConflict
		}
		quota, err := tx.GetTenantQuota(ctx, request.OrganizationID)
		if err != nil {
			return err
		}
		allocation := domain.GameServer{ID: server.ID, Spec: domain.ServerSpec{Resources: domain.ServerResources{CPULimitCores: request.Specification.Resources.CPU, MemoryLimitMB: int(request.Specification.Resources.MemoryMB)}}}
		if int64(allocation.Spec.Resources.MemoryLimitMB) != request.Specification.Resources.MemoryMB {
			return instances.ErrInvalidIntent
		}
		if err := tx.checkAllocation(ctx, quota, &allocation); err != nil {
			return err
		}
		var placement instances.Placement
		if err := tx.db.WithContext(ctx).Table("server_placements").Where("server_id = ?", server.ID).Take(&placement).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		revision := globalRevisionRow{ID: uuid.NewString(), ServerID: server.ID, SpecGeneration: server.SpecGeneration + 1, Specification: string(specification), CPU: request.Specification.Resources.CPU, MemoryMB: request.Specification.Resources.MemoryMB, CreatedAt: now}
		operation = globalOperationRow{ID: uuid.NewString(), OrganizationID: server.OrganizationID, ServerID: server.ID, RevisionID: revision.ID, Kind: "revise", Status: "pending", IdempotencyKey: request.IdempotencyKey, RequestHash: hash, CreatedAt: now}
		if err := tx.db.WithContext(ctx).Table("server_revisions").Create(&revision).Error; err != nil {
			return err
		}
		updated := tx.db.WithContext(ctx).Table("logical_servers").Where("id = ? AND organization_id = ? AND spec_generation = ?", server.ID, server.OrganizationID, request.ExpectedGeneration).
			Updates(map[string]any{"current_revision_id": revision.ID, "spec_generation": revision.SpecGeneration})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return instances.ErrVersionConflict
		}
		if err := tx.db.WithContext(ctx).Table("server_operations").Create(&operation).Error; err != nil {
			return err
		}
		event := instances.RevisionAvailable{SchemaVersion: 1, EventID: uuid.NewString(), OperationID: operation.ID, OrganizationID: server.OrganizationID, ServerID: server.ID, RevisionID: revision.ID, RegionID: placement.RegionID, PlacementEpoch: placement.PlacementEpoch, SpecGeneration: revision.SpecGeneration}
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if err := tx.db.WithContext(ctx).Table("server_outbox").Create(map[string]any{"id": event.EventID, "operation_id": operation.ID, "region_id": placement.RegionID, "payload": string(payload), "created_at": now}).Error; err != nil {
			return err
		}
		result, err = tx.readGlobalIntent(ctx, operation)
		return err
	})
	if err != nil {
		return instances.IntentResult{}, err
	}
	return result, nil
}
