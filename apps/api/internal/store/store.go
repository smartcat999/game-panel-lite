package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&domain.GameServerInstance{}, &domain.Backup{}, &domain.World{}, &domain.ModFile{}, &domain.ModPack{}, &domain.ActivityEvent{}, &domain.AdminAccount{}, &domain.Session{}, &domain.Setting{}, &domain.ServerShare{}, &domain.ConfigPreset{}); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) HasAdminAccount(ctx context.Context) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&domain.AdminAccount{}).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) CreateAdminAccount(ctx context.Context, account *domain.AdminAccount) error {
	return s.db.WithContext(ctx).Create(account).Error
}

func (s *Store) GetAdminAccountByUsername(ctx context.Context, username string) (domain.AdminAccount, error) {
	var account domain.AdminAccount
	err := s.db.WithContext(ctx).First(&account, "username = ?", username).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return account, ErrNotFound
	}
	return account, err
}

func (s *Store) GetAdminAccount(ctx context.Context, id string) (domain.AdminAccount, error) {
	var account domain.AdminAccount
	err := s.db.WithContext(ctx).First(&account, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return account, ErrNotFound
	}
	return account, err
}

func (s *Store) SaveAdminAccount(ctx context.Context, account *domain.AdminAccount) error {
	return s.db.WithContext(ctx).Save(account).Error
}

func (s *Store) CreateSession(ctx context.Context, session *domain.Session) error {
	return s.db.WithContext(ctx).Create(session).Error
}

func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error) {
	var session domain.Session
	err := s.db.WithContext(ctx).First(&session, "token_hash = ?", tokenHash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return session, ErrNotFound
	}
	return session, err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.Session{}, "id = ?", id).Error
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	return s.db.WithContext(ctx).Delete(&domain.Session{}, "expires_at <= ?", now).Error
}

func (s *Store) CreateServer(ctx context.Context, server *domain.GameServerInstance) error {
	return s.db.WithContext(ctx).Create(server).Error
}

func (s *Store) SaveServer(ctx context.Context, server *domain.GameServerInstance) error {
	return s.db.WithContext(ctx).Save(server).Error
}

func (s *Store) ListServers(ctx context.Context) ([]domain.GameServerInstance, error) {
	var servers []domain.GameServerInstance
	if err := s.db.WithContext(ctx).Order("created_at desc").Find(&servers).Error; err != nil {
		return nil, err
	}
	for index := range servers {
		hydrateServerConfigPayload(&servers[index])
	}
	return servers, nil
}

func (s *Store) GetServer(ctx context.Context, id string) (domain.GameServerInstance, error) {
	var server domain.GameServerInstance
	err := s.db.WithContext(ctx).First(&server, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return server, ErrNotFound
	}
	if err == nil {
		hydrateServerConfigPayload(&server)
	}
	return server, err
}

func (s *Store) DeleteServer(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.GameServerInstance{}, "id = ?", id).Error
}

func hydrateServerConfigPayload(server *domain.GameServerInstance) {
	if server == nil || server.ConfigPayloadJSON == "" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(server.ConfigPayloadJSON), &payload); err == nil {
		server.ConfigPayload = payload
	}
}

func hydratePresetConfigPayload(preset *domain.ConfigPreset) {
	if preset == nil || preset.ConfigPayloadJSON == "" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(preset.ConfigPayloadJSON), &payload); err == nil {
		preset.ConfigPayload = payload
	}
}

func (s *Store) CreateConfigPreset(ctx context.Context, preset *domain.ConfigPreset) error {
	return s.db.WithContext(ctx).Create(preset).Error
}

func (s *Store) ListConfigPresets(ctx context.Context) ([]domain.ConfigPreset, error) {
	var presets []domain.ConfigPreset
	if err := s.db.WithContext(ctx).Order("created_at desc").Find(&presets).Error; err != nil {
		return nil, err
	}
	for index := range presets {
		hydratePresetConfigPayload(&presets[index])
	}
	return presets, nil
}

func (s *Store) GetConfigPreset(ctx context.Context, id string) (domain.ConfigPreset, error) {
	var preset domain.ConfigPreset
	err := s.db.WithContext(ctx).First(&preset, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return preset, ErrNotFound
	}
	if err == nil {
		hydratePresetConfigPayload(&preset)
	}
	return preset, err
}

func (s *Store) SaveConfigPreset(ctx context.Context, preset *domain.ConfigPreset) error {
	return s.db.WithContext(ctx).Save(preset).Error
}

func (s *Store) DeleteConfigPreset(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.ConfigPreset{}, "id = ?", id).Error
}

func (s *Store) CreateWorld(ctx context.Context, world *domain.World) error {
	return s.db.WithContext(ctx).Create(world).Error
}

func (s *Store) ListWorlds(ctx context.Context) ([]domain.World, error) {
	var worlds []domain.World
	return worlds, s.db.WithContext(ctx).Order("created_at desc").Find(&worlds).Error
}

func (s *Store) GetWorld(ctx context.Context, id string) (domain.World, error) {
	var world domain.World
	err := s.db.WithContext(ctx).First(&world, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return world, ErrNotFound
	}
	return world, err
}

func (s *Store) GetWorldByInstanceAndFile(ctx context.Context, instanceID string, fileName string) (domain.World, error) {
	var world domain.World
	err := s.db.WithContext(ctx).First(&world, "instance_id = ? AND file_name = ?", instanceID, fileName).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return world, ErrNotFound
	}
	return world, err
}

func (s *Store) SaveWorld(ctx context.Context, world *domain.World) error {
	return s.db.WithContext(ctx).Save(world).Error
}

func (s *Store) DeleteWorld(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.World{}, "id = ?", id).Error
}

func (s *Store) CreateBackup(ctx context.Context, backup *domain.Backup) error {
	return s.db.WithContext(ctx).Create(backup).Error
}

func (s *Store) ListBackups(ctx context.Context) ([]domain.Backup, error) {
	var backups []domain.Backup
	return backups, s.db.WithContext(ctx).Order("created_at desc").Find(&backups).Error
}

func (s *Store) ListBackupsByInstance(ctx context.Context, instanceID string) ([]domain.Backup, error) {
	var backups []domain.Backup
	return backups, s.db.WithContext(ctx).Where("instance_id = ?", instanceID).Order("created_at desc").Find(&backups).Error
}

func (s *Store) GetBackup(ctx context.Context, id string) (domain.Backup, error) {
	var backup domain.Backup
	err := s.db.WithContext(ctx).First(&backup, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return backup, ErrNotFound
	}
	return backup, err
}

func (s *Store) GetBackupByInstanceAndFile(ctx context.Context, instanceID string, fileName string) (domain.Backup, error) {
	var backup domain.Backup
	err := s.db.WithContext(ctx).First(&backup, "instance_id = ? AND file_name = ?", instanceID, fileName).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return backup, ErrNotFound
	}
	return backup, err
}

func (s *Store) SaveBackup(ctx context.Context, backup *domain.Backup) error {
	return s.db.WithContext(ctx).Save(backup).Error
}

func (s *Store) DeleteBackup(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.Backup{}, "id = ?", id).Error
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var setting domain.Setting
	err := s.db.WithContext(ctx).First(&setting, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return setting.Value, err
}

func (s *Store) SetSetting(ctx context.Context, key string, value string) error {
	setting := domain.Setting{Key: key, Value: value}
	return s.db.WithContext(ctx).Save(&setting).Error
}

func (s *Store) SaveServerShare(ctx context.Context, share *domain.ServerShare) error {
	return s.db.WithContext(ctx).Save(share).Error
}

func (s *Store) GetServerShareByInstance(ctx context.Context, instanceID string) (domain.ServerShare, error) {
	var share domain.ServerShare
	err := s.db.WithContext(ctx).First(&share, "instance_id = ?", instanceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return share, ErrNotFound
	}
	return share, err
}

func (s *Store) GetServerShareByToken(ctx context.Context, token string) (domain.ServerShare, error) {
	var share domain.ServerShare
	err := s.db.WithContext(ctx).First(&share, "token = ?", token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return share, ErrNotFound
	}
	return share, err
}

func (s *Store) DeleteServerShareByInstance(ctx context.Context, instanceID string) error {
	return s.db.WithContext(ctx).Delete(&domain.ServerShare{}, "instance_id = ?", instanceID).Error
}

func (s *Store) CreateMod(ctx context.Context, mod *domain.ModFile) error {
	return s.db.WithContext(ctx).Create(mod).Error
}

func (s *Store) ListMods(ctx context.Context, instanceID string) ([]domain.ModFile, error) {
	var mods []domain.ModFile
	return mods, s.db.WithContext(ctx).Where("instance_id = ?", instanceID).Order("created_at desc").Find(&mods).Error
}

func (s *Store) GetMod(ctx context.Context, id string) (domain.ModFile, error) {
	var mod domain.ModFile
	err := s.db.WithContext(ctx).First(&mod, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return mod, ErrNotFound
	}
	return mod, err
}

func (s *Store) GetModByInstanceAndFile(ctx context.Context, instanceID string, fileName string) (domain.ModFile, error) {
	var mod domain.ModFile
	err := s.db.WithContext(ctx).First(&mod, "instance_id = ? AND file_name = ?", instanceID, fileName).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return mod, ErrNotFound
	}
	return mod, err
}

func (s *Store) GetModByInstanceAndWorkshopID(ctx context.Context, instanceID string, workshopID string) (domain.ModFile, error) {
	var mod domain.ModFile
	err := s.db.WithContext(ctx).First(&mod, "instance_id = ? AND workshop_id = ?", instanceID, workshopID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return mod, ErrNotFound
	}
	return mod, err
}

func (s *Store) SaveMod(ctx context.Context, mod *domain.ModFile) error {
	return s.db.WithContext(ctx).Save(mod).Error
}

func (s *Store) DeleteMod(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.ModFile{}, "id = ?", id).Error
}

func (s *Store) CreateModPack(ctx context.Context, pack *domain.ModPack) error {
	return s.db.WithContext(ctx).Create(pack).Error
}

func (s *Store) ListModPacks(ctx context.Context) ([]domain.ModPack, error) {
	var packs []domain.ModPack
	return packs, s.db.WithContext(ctx).Order("created_at desc").Find(&packs).Error
}

func (s *Store) GetModPack(ctx context.Context, id string) (domain.ModPack, error) {
	var pack domain.ModPack
	err := s.db.WithContext(ctx).First(&pack, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return pack, ErrNotFound
	}
	return pack, err
}

func (s *Store) SaveModPack(ctx context.Context, pack *domain.ModPack) error {
	return s.db.WithContext(ctx).Save(pack).Error
}

func (s *Store) DeleteModPack(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.ModPack{}, "id = ?", id).Error
}

func (s *Store) CreateActivity(ctx context.Context, event *domain.ActivityEvent) error {
	return s.db.WithContext(ctx).Create(event).Error
}

func (s *Store) ListActivity(ctx context.Context, limit int) ([]domain.ActivityEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var events []domain.ActivityEvent
	return events, s.db.WithContext(ctx).Order("created_at desc").Limit(limit).Find(&events).Error
}

var ErrNotFound = errors.New("not found")
