package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

var ErrInvalidModLibrary = errors.New("invalid workspace mod library resource or reference")

// These operations own workspace library metadata. Instance installation and
// file publication are separate operations, never implied by a metadata commit.
func (s *Store) userLibraryMods(ctx context.Context, userID string) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.ModFile{}).Where("instance_id = ? AND organization_id IN (?)", "unassigned", s.userOrganizations(ctx, userID).Select("organizations.id"))
}
func (s *Store) ListUserLibraryMods(ctx context.Context, userID string) ([]domain.ModFile, error) {
	items := []domain.ModFile{}
	err := s.userLibraryMods(ctx, userID).Order("created_at DESC, id ASC").Find(&items).Error
	return items, err
}
func (s *Store) GetUserLibraryMod(ctx context.Context, userID, id string) (domain.ModFile, error) {
	var item domain.ModFile
	err := s.userLibraryMods(ctx, userID).Where("id = ?", id).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrNotFound
	}
	return item, err
}
func (s *Store) userModPacks(ctx context.Context, userID string) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.ModPack{}).Where("organization_id IN (?)", s.userOrganizations(ctx, userID).Select("organizations.id"))
}
func (s *Store) ListUserModPacks(ctx context.Context, userID string) ([]domain.ModPack, error) {
	items := []domain.ModPack{}
	err := s.userModPacks(ctx, userID).Order("created_at DESC, id ASC").Find(&items).Error
	return items, err
}
func (s *Store) GetUserModPack(ctx context.Context, userID, id string) (domain.ModPack, error) {
	var item domain.ModPack
	err := s.userModPacks(ctx, userID).Where("id = ?", id).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrNotFound
	}
	return item, err
}
func (s *Store) lockLibraryWriter(ctx context.Context, userID, orgID string) error {
	if strings.TrimSpace(orgID) == "" {
		return ErrWorkspaceWriteDenied
	}
	return s.lockWorkspaceWriter(ctx, orgID, userID)
}
func (s *Store) CheckLibraryWriter(ctx context.Context, userID, orgID string) error {
	return s.Transaction(ctx, func(tx *Store) error { return tx.lockLibraryWriter(ctx, userID, orgID) })
}
func (s *Store) CreateOwnedLibraryMod(ctx context.Context, userID string, item *domain.ModFile) error {
	_, err := s.CommitLibraryUpload(ctx, userID, item)
	return err
}
func (s *Store) CommitLibraryUpload(ctx context.Context, userID string, item *domain.ModFile) (domain.LibraryCommitOutcome, error) {
	if item.InstanceID != "unassigned" || item.ID == "" || item.ProviderKey == "" || item.FileName == "" || item.Revision != 0 {
		return domain.LibraryCommitRejected, ErrInvalidModLibrary
	}
	outcome := domain.LibraryCommitRejected
	err := s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockLibraryWriter(ctx, userID, item.OrganizationID); err != nil {
			return err
		}
		if err := tx.libraryModCollision(ctx, *item); err != nil {
			return err
		}
		// From this point an insert or COMMIT failure may have an uncertain result.
		outcome = domain.LibraryCommitUncertain
		return tx.CreateMod(ctx, item)
	})
	if err == nil {
		outcome = domain.LibraryCommitApplied
	}
	return outcome, err
}

// One workspace lock also serializes duplicate detection, pack reference checks
// and deletion. All workspace library writers must use this interface.
func (s *Store) libraryModCollision(ctx context.Context, item domain.ModFile) error {
	q := s.db.WithContext(ctx).Model(&domain.ModFile{}).Where("organization_id = ? AND instance_id = ? AND provider_key = ? AND id <> ?", item.OrganizationID, "unassigned", item.ProviderKey, item.ID)
	if item.WorkshopID != "" {
		q = q.Where("(file_name = ? OR workshop_id = ?)", item.FileName, item.WorkshopID)
	} else {
		q = q.Where("file_name = ?", item.FileName)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return ErrInvalidModLibrary
	}
	return nil
}
func (s *Store) libraryModSnapshot(ctx context.Context, before domain.ModFile) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.ModFile{}).Where("id = ? AND organization_id = ? AND instance_id = ? AND provider_key = ? AND file_name = ? AND workshop_id = ? AND revision = ?", before.ID, before.OrganizationID, "unassigned", before.ProviderKey, before.FileName, before.WorkshopID, before.Revision)
}
func (s *Store) SaveOwnedLibraryMod(ctx context.Context, userID string, before, after domain.ModFile) error {
	if before.InstanceID != "unassigned" || after.InstanceID != before.InstanceID || after.ID != before.ID || after.OrganizationID != before.OrganizationID || after.ProviderKey != before.ProviderKey || after.GameKey != before.GameKey || after.FileName != before.FileName || after.WorkshopID != before.WorkshopID || after.Source != before.Source {
		return ErrInvalidModLibrary
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockLibraryWriter(ctx, userID, before.OrganizationID); err != nil {
			return err
		}
		if err := tx.artifactReferences(ctx, before.OrganizationID, before.ID); err != nil {
			return err
		}
		after.Revision = before.Revision + 1
		result := tx.libraryModSnapshot(ctx, before).Select("revision", "mod_name", "title", "mod_version", "t_mod_version", "creator_steam_id", "preview_url", "description", "content_hash", "tags_json", "subscriptions", "favorited", "views", "updated_at_steam", "size_bytes", "enabled", "dependencies_json").Updates(&after)
		return libraryWriteResult(result)
	})
}
func libraryWriteResult(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrReconciliationSuperseded
	}
	return nil
}
func (s *Store) CreateOwnedModPack(ctx context.Context, userID string, item *domain.ModPack) error {
	if item.ID == "" || item.Revision != 0 {
		return ErrInvalidModLibrary
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockLibraryWriter(ctx, userID, item.OrganizationID); err != nil {
			return err
		}
		if err := tx.validateLibraryPack(ctx, *item); err != nil {
			return err
		}
		return tx.CreateModPack(ctx, item)
	})
}
func (s *Store) validateLibraryPack(ctx context.Context, item domain.ModPack) error {
	var ids []string
	if strings.TrimSpace(item.Name) == "" || json.Unmarshal([]byte(item.ModIDsJSON), &ids) != nil || len(ids) == 0 {
		return ErrInvalidModLibrary
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			return ErrInvalidModLibrary
		}
		seen[id] = true
	}
	var mods []domain.ModFile
	if err := s.db.WithContext(ctx).Where("id IN ? AND organization_id = ? AND instance_id = ?", ids, item.OrganizationID, "unassigned").Find(&mods).Error; err != nil {
		return err
	}
	if len(mods) != len(ids) {
		return ErrInvalidModLibrary
	}
	var provider domain.ProviderKey
	for _, mod := range mods {
		if mod.ProviderKey == "" || (provider != "" && provider != mod.ProviderKey) {
			return ErrInvalidModLibrary
		}
		provider = mod.ProviderKey
	}
	var conflicts int64
	if err := s.db.WithContext(ctx).Model(&domain.ConfigPreset{}).Where("organization_id = ? AND mod_pack_id = ? AND provider_key <> ?", item.OrganizationID, item.ID, provider).Count(&conflicts).Error; err != nil {
		return err
	}
	if conflicts != 0 {
		return ErrInvalidModLibrary
	}
	return nil
}
func (s *Store) modPackSnapshot(ctx context.Context, before domain.ModPack) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.ModPack{}).Where("id = ? AND organization_id = ? AND revision = ?", before.ID, before.OrganizationID, before.Revision)
}
func (s *Store) SaveOwnedModPack(ctx context.Context, userID string, before, after domain.ModPack) error {
	if before.ID != after.ID || before.OrganizationID != after.OrganizationID {
		return ErrInvalidModLibrary
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockLibraryWriter(ctx, userID, before.OrganizationID); err != nil {
			return err
		}
		if err := tx.validateLibraryPack(ctx, after); err != nil {
			return err
		}
		after.Revision = before.Revision + 1
		return libraryWriteResult(tx.modPackSnapshot(ctx, before).Select("revision", "name", "description", "mod_ids_json", "updated_at").Updates(&after))
	})
}

// Referenced resources cannot be removed silently. The caller must explicitly
// update the instance/pack/preset first, under the same workspace writer lock.
func (s *Store) libraryReferences(ctx context.Context, orgID, modID, packID string) error {
	if modID != "" {
		if err := s.artifactReferences(ctx, orgID, modID); err != nil {
			return err
		}
		var servers []domain.GameServer
		if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Find(&servers).Error; err != nil {
			return err
		}
		for _, server := range servers {
			for _, id := range server.Spec.ModIDs {
				if id == modID {
					return ErrInvalidModLibrary
				}
			}
		}
		var packs []domain.ModPack
		if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Find(&packs).Error; err != nil {
			return err
		}
		for _, pack := range packs {
			var ids []string
			if json.Unmarshal([]byte(pack.ModIDsJSON), &ids) != nil {
				return ErrInvalidModLibrary
			}
			for _, id := range ids {
				if id == modID {
					return ErrInvalidModLibrary
				}
			}
		}
	}
	var presets []domain.ConfigPreset
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Find(&presets).Error; err != nil {
		return err
	}
	for _, preset := range presets {
		if packID != "" && preset.ModPackID == packID {
			return ErrInvalidModLibrary
		}
		if modID != "" && preset.ModIDsJSON != "" {
			var ids []string
			if json.Unmarshal([]byte(preset.ModIDsJSON), &ids) != nil {
				return ErrInvalidModLibrary
			}
			for _, id := range ids {
				if id == modID {
					return ErrInvalidModLibrary
				}
			}
		}
	}
	return nil
}
func (s *Store) DeleteOwnedLibraryMod(ctx context.Context, userID string, before domain.ModFile) error {
	if before.InstanceID != "unassigned" {
		return ErrInvalidModLibrary
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockLibraryWriter(ctx, userID, before.OrganizationID); err != nil {
			return err
		}
		if err := tx.libraryReferences(ctx, before.OrganizationID, before.ID, ""); err != nil {
			return err
		}
		return libraryWriteResult(tx.libraryModSnapshot(ctx, before).Delete(&domain.ModFile{}))
	})
}
func (s *Store) DeleteOwnedModPack(ctx context.Context, userID string, before domain.ModPack) error {
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockLibraryWriter(ctx, userID, before.OrganizationID); err != nil {
			return err
		}
		if err := tx.libraryReferences(ctx, before.OrganizationID, "", before.ID); err != nil {
			return err
		}
		return libraryWriteResult(tx.modPackSnapshot(ctx, before).Delete(&domain.ModPack{}))
	})
}

// Called while the preset's workspace lock is held, the same lock used by
// library deletion. A reference can neither race deletion nor cross a workspace.
func (s *Store) validatePresetLibrary(ctx context.Context, preset domain.ConfigPreset) error {
	if preset.OrganizationID == "" {
		return nil
	} // Legacy administrator presets.
	var ids []string
	if preset.ModIDsJSON != "" && json.Unmarshal([]byte(preset.ModIDsJSON), &ids) != nil {
		return ErrInvalidModLibrary
	}
	if preset.ModPackID != "" {
		var pack domain.ModPack
		err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", preset.ModPackID, preset.OrganizationID).Take(&pack).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidModLibrary
		}
		if err != nil {
			return err
		}
		var packIDs []string
		if json.Unmarshal([]byte(pack.ModIDsJSON), &packIDs) != nil || len(packIDs) == 0 {
			return ErrInvalidModLibrary
		}
		ids = append(ids, packIDs...)
	}
	if len(ids) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return ErrInvalidModLibrary
		}
		unique[id] = struct{}{}
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&domain.ModFile{}).Where("id IN ? AND organization_id = ? AND instance_id = ? AND provider_key = ?", ids, preset.OrganizationID, "unassigned", preset.ProviderKey).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(unique)) {
		return ErrInvalidModLibrary
	}
	return nil
}
