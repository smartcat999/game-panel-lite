package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

func (s *Store) userConfigPresets(ctx context.Context, userID string) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.ConfigPreset{}).Where("organization_id IN (?)", s.userOrganizations(ctx, userID).Select("organizations.id"))
}
func (s *Store) ListUserConfigPresets(ctx context.Context, userID string) ([]domain.ConfigPreset, error) {
	presets := []domain.ConfigPreset{}
	err := s.userConfigPresets(ctx, userID).Order("created_at DESC, id ASC").Find(&presets).Error
	for i := range presets {
		hydratePresetConfigPayload(&presets[i])
	}
	return presets, err
}
func (s *Store) GetUserConfigPreset(ctx context.Context, userID, id string) (domain.ConfigPreset, error) {
	var preset domain.ConfigPreset
	err := s.userConfigPresets(ctx, userID).Where("id = ?", id).Take(&preset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return preset, ErrNotFound
	}
	if err == nil {
		hydratePresetConfigPayload(&preset)
	}
	return preset, err
}

func (s *Store) lockPresetWriter(ctx context.Context, userID, orgID string) error {
	if orgID != "" {
		return s.lockWorkspaceWriter(ctx, orgID, userID)
	}
	if userID != "" {
		return ErrWorkspaceWriteDenied
	}
	return nil
}
func (s *Store) CreateOwnedConfigPreset(ctx context.Context, userID string, preset *domain.ConfigPreset) error {
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockPresetWriter(ctx, userID, preset.OrganizationID); err != nil {
			return err
		}
		return tx.CreateConfigPreset(ctx, preset)
	})
}

// Ownership is immutable here. Conditional updates never resurrect deleted
// presets and reject stale edits; membership is rechecked inside the transaction.
func (s *Store) SaveOwnedConfigPreset(ctx context.Context, userID string, before, after domain.ConfigPreset) error {
	if before.ID != after.ID || before.OrganizationID != after.OrganizationID {
		return ErrReconciliationSuperseded
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockPresetWriter(ctx, userID, before.OrganizationID); err != nil {
			return err
		}
		after.Revision = before.Revision + 1
		result := tx.presetSnapshot(ctx, before).Select("revision", "name", "game_key", "provider_key", "version", "config_payload_json", "cpu_limit_cores", "memory_limit_mb", "mod_pack_id", "mod_ids_json", "updated_at").Updates(&after)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrReconciliationSuperseded
		}
		return nil
	})
}
func (s *Store) DeleteOwnedConfigPreset(ctx context.Context, userID string, before domain.ConfigPreset) error {
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockPresetWriter(ctx, userID, before.OrganizationID); err != nil {
			return err
		}
		result := tx.presetSnapshot(ctx, before).Delete(&domain.ConfigPreset{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrReconciliationSuperseded
		}
		return nil
	})
}
func (s *Store) presetSnapshot(ctx context.Context, before domain.ConfigPreset) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.ConfigPreset{}).Where("id = ? AND COALESCE(organization_id, '') = ? AND revision = ?", before.ID, before.OrganizationID, before.Revision)
}
