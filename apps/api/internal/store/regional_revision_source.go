package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
)

// GetRegionalRevision is an internal read adapter. authenticatedRegion must
// come from authenticated service identity, never from a tenant request field.
// Returning a revision neither authorizes execution nor proves ciphertext valid.
func (s *Store) GetRegionalRevision(ctx context.Context, authenticatedRegion string, event instances.RevisionAvailable) (regional.RevisionSnapshot, error) {
	if event.Validate() != nil || !validRegion(authenticatedRegion) || event.RegionID != authenticatedRegion {
		return regional.RevisionSnapshot{}, ErrNotFound
	}
	var result regional.RevisionSnapshot
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var outbox struct{ Payload string }
		if err := tx.db.WithContext(ctx).Table("server_outbox").Select("payload").Where("id = ? AND operation_id = ? AND region_id = ?", event.EventID, event.OperationID, authenticatedRegion).Take(&outbox).Error; err != nil {
			return err
		}
		var published instances.RevisionAvailable
		if err := json.Unmarshal([]byte(outbox.Payload), &published); err != nil {
			return ErrNotFound
		}
		if published != event {
			return ErrNotFound
		}
		var server instances.Server
		if err := tx.db.WithContext(ctx).Table("logical_servers").Where("id = ? AND organization_id = ?", event.ServerID, event.OrganizationID).Take(&server).Error; err != nil {
			return err
		}
		if server.DesiredState == "deleted" {
			return ErrNotFound
		}
		var placement instances.Placement
		if err := tx.db.WithContext(ctx).Table("server_placements").Where("server_id = ?", server.ID).Take(&placement).Error; err != nil {
			return err
		}
		if placement.RegionID != authenticatedRegion || placement.PlacementEpoch != event.PlacementEpoch {
			return ErrNotFound
		}
		var revision globalRevisionRow
		if err := tx.db.WithContext(ctx).Table("server_revisions").Where("id = ? AND server_id = ? AND spec_generation = ?", event.RevisionID, server.ID, event.SpecGeneration).Take(&revision).Error; err != nil {
			return err
		}
		var specification instances.Specification
		if err := json.Unmarshal([]byte(revision.Specification), &specification); err != nil {
			return ErrNotFound
		}
		if specification.Validate() != nil || specification.Resources.CPU != revision.CPU || specification.Resources.MemoryMB != revision.MemoryMB || server.SpecGeneration < revision.SpecGeneration || server.IntentVersion < 1 {
			return ErrNotFound
		}
		result = regional.RevisionSnapshot{Event: event, Revision: instances.Revision{ID: revision.ID, ServerID: server.ID, SpecGeneration: revision.SpecGeneration, Specification: specification, CreatedAt: revision.CreatedAt}, CurrentSpecGeneration: server.SpecGeneration, DesiredState: server.DesiredState, IntentVersion: server.IntentVersion}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrNotFound
		}
		return regional.RevisionSnapshot{}, err
	}
	return result, nil
}
