package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

// A source is either an already installed mod of this instance or a library
// record in its workspace. Empty ownership is only the legacy unowned scope.
func (s *Store) modSources(ctx context.Context, target domain.GameServer) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.ModFile{}).Where("(instance_id = ? OR (instance_id = ? AND organization_id = ?))", target.ID, "unassigned", target.OrganizationID)
}

// CheckModTarget rejects an obsolete planning snapshot. It is not a lease and
// does not fence subsequent filesystem operations against another process.
func (s *Store) CheckModTarget(ctx context.Context, target domain.GameServer) error {
	spec, err := json.Marshal(target.Spec)
	if err != nil {
		return err
	}
	var count int64
	err = s.db.WithContext(ctx).Model(&domain.GameServer{}).Where("id = ? AND provider_key = ? AND COALESCE(organization_id, '') = ? AND COALESCE(node_id, '') = ? AND spec = ?", target.ID, target.ProviderKey, target.OrganizationID, target.NodeID, string(spec)).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrReconciliationSuperseded
	}
	return nil
}

func (s *Store) GetModForServer(ctx context.Context, target domain.GameServer, id string) (domain.ModFile, error) {
	var item domain.ModFile
	if err := s.CheckModTarget(ctx, target); err != nil {
		return item, err
	}
	err := s.modSources(ctx, target).Where("id = ?", id).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, err
	}
	if key := item.ProviderKey; key != target.ProviderKey && !(key == "" && target.ProviderKey == domain.ProviderTerrariaTModLoader) {
		return domain.ModFile{}, ErrInvalidModLibrary
	}
	return item, s.CheckModTarget(ctx, target)
}

func (s *Store) ListLibraryModsForServer(ctx context.Context, target domain.GameServer) ([]domain.ModFile, error) {
	if err := s.CheckModTarget(ctx, target); err != nil {
		return nil, err
	}
	items := []domain.ModFile{}
	err := s.db.WithContext(ctx).Where("instance_id = ? AND organization_id = ?", "unassigned", target.OrganizationID).Order("created_at DESC, id ASC").Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, s.CheckModTarget(ctx, target)
}

// Called inside the allocation transaction and workspace lock. New desired
// references must already exist in the same workspace/provider; installed IDs
// may only refer to this instance. Legacy blank provider keys stay compatible
// with the existing tModLoader record migration rule.
func (s *Store) validateServerModReferences(ctx context.Context, target domain.GameServer) error {
	ids := target.Spec.ModIDs
	if len(ids) == 0 {
		return nil
	}
	var items []domain.ModFile
	if err := s.modSources(ctx, target).Where("id IN ?", ids).Find(&items).Error; err != nil {
		return err
	}
	found := make(map[string]bool, len(items))
	for _, item := range items {
		key := item.ProviderKey
		if key == "" {
			key = domain.ProviderTerrariaTModLoader
		}
		if key != target.ProviderKey {
			return ErrInvalidModLibrary
		}
		found[item.ID] = true
	}
	for _, id := range ids {
		if !found[id] {
			return ErrInvalidModLibrary
		}
	}
	return nil
}
