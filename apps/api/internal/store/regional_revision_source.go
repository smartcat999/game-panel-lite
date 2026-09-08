package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
)

// GetRegionalRevision is an internal read adapter. authenticatedRegion must
// come from authenticated service identity, never from a tenant request field.
// Returning a revision neither authorizes execution nor proves ciphertext valid.
func (s *Store) GetRegionalRevision(ctx context.Context, authenticatedRegion string, event instances.RevisionAvailable) (regional.RevisionSnapshot, error) {
	var result regional.RevisionSnapshot
	err := s.withRegionalRevision(ctx, authenticatedRegion, event, func(tx *Store, snapshot regional.RevisionSnapshot) error {
		manifest, err := tx.resolveGlobalAssets(ctx, event.OrganizationID, snapshot.Revision.Specification.Assets)
		if err != nil {
			return err
		}
		snapshot.Assets = manifest
		result = snapshot
		return nil
	})
	if err != nil {
		return regional.RevisionSnapshot{}, err
	}
	return result, nil
}

// ResolveRegionalAsset authorizes exactly one asset reference for the current
// event/Region placement. It returns metadata, not a reusable download ticket,
// storage address, or execution grant. authenticatedRegion is a service identity.
func (s *Store) ResolveRegionalAsset(ctx context.Context, authenticatedRegion string, event instances.RevisionAvailable, reference instances.AssetVersion) (assets.PublishedVersion, error) {
	var result assets.PublishedVersion
	err := s.withRegionalRevision(ctx, authenticatedRegion, event, func(tx *Store, snapshot regional.RevisionSnapshot) error {
		found := false
		for _, ref := range snapshot.Revision.Specification.Assets {
			if ref == reference {
				found = true
				break
			}
		}
		if !found {
			return ErrNotFound
		}
		manifest, err := tx.resolveGlobalAssets(ctx, event.OrganizationID, []instances.AssetVersion{reference})
		if err != nil {
			return err
		}
		result = manifest[0]
		return nil
	})
	if err != nil {
		return assets.PublishedVersion{}, err
	}
	return result, nil
}

// withRegionalRevision keeps identity, placement and the caller's catalog read
// in one database snapshot. The callback must not perform network or file I/O.
func (s *Store) withRegionalRevision(ctx context.Context, authenticatedRegion string, event instances.RevisionAvailable, read func(*Store, regional.RevisionSnapshot) error) error {
	if event.Validate() != nil || !validRegion(authenticatedRegion) || event.RegionID != authenticatedRegion {
		return errors.Join(ErrNotFound, regional.ErrRevisionUnavailable)
	}
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
		snapshot := regional.RevisionSnapshot{Event: event, Revision: instances.Revision{ID: revision.ID, ServerID: server.ID, SpecGeneration: revision.SpecGeneration, Specification: specification, CreatedAt: revision.CreatedAt}, CurrentSpecGeneration: server.SpecGeneration, DesiredState: server.DesiredState, IntentVersion: server.IntentVersion}
		return read(tx, snapshot)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrNotFound) || errors.Is(err, assets.ErrUnavailable) {
			err = errors.Join(ErrNotFound, regional.ErrRevisionUnavailable)
		}
		return err
	}
	return nil
}
