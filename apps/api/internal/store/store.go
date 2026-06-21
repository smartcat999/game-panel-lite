package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Store struct {
	db                  *gorm.DB
	activityMu          sync.Mutex
	activitySubscribers map[uint64]activitySubscriber
	nextActivitySubID   uint64
}

type activitySubscriber struct {
	instanceID string
	ch         chan domain.ActivityEvent
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&domain.GameServer{}, &domain.Backup{}, &domain.World{}, &domain.ModFile{}, &domain.ModPack{}, &domain.ActivityEvent{}, &domain.AdminAccount{}, &domain.Session{}, &domain.Setting{}, &domain.ServerShare{}, &domain.ConfigPreset{}); err != nil {
		return nil, err
	}
	return &Store{db: db, activitySubscribers: map[uint64]activitySubscriber{}}, nil
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

func (s *Store) CreateGameServer(ctx context.Context, server *domain.GameServer) error {
	return s.db.WithContext(ctx).Create(server).Error
}

func (s *Store) SaveGameServer(ctx context.Context, server *domain.GameServer) error {
	return s.db.WithContext(ctx).Save(server).Error
}

func (s *Store) ListGameServers(ctx context.Context) ([]domain.GameServer, error) {
	var servers []domain.GameServer
	if err := s.db.WithContext(ctx).Order("created_at desc").Find(&servers).Error; err != nil {
		return nil, err
	}
	return servers, nil
}

func (s *Store) GetGameServer(ctx context.Context, id string) (domain.GameServer, error) {
	server, err := s.getStoredGameServer(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return server, ErrNotFound
	}
	return server, err
}

func (s *Store) getStoredGameServer(ctx context.Context, id string) (domain.GameServer, error) {
	var server domain.GameServer
	err := s.db.WithContext(ctx).First(&server, "id = ?", id).Error
	return server, err
}

func (s *Store) DeleteGameServer(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.GameServer{}, "id = ?", id).Error
}

func hydratePresetConfigPayload(preset *domain.ConfigPreset) {
	if preset == nil || preset.ConfigPayloadJSON == "" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(preset.ConfigPayloadJSON), &payload); err == nil {
		preset.Config = payload
		preset.ConfigPayload = payload
	}
}

func hydrateWorldConfigPayload(world *domain.World) {
	if world == nil || world.ConfigPayloadJSON == "" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(world.ConfigPayloadJSON), &payload); err == nil {
		world.Config = payload
		world.ConfigPayload = payload
	}
}

func prepareWorldConfigPayload(world *domain.World) error {
	if world == nil {
		return nil
	}
	payload := world.ConfigPayload
	if len(payload) == 0 {
		payload = world.Config
	}
	if len(payload) == 0 {
		return nil
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	world.ConfigPayloadJSON = string(buf)
	world.Config = payload
	world.ConfigPayload = payload
	return nil
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
	if err := prepareWorldConfigPayload(world); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(world).Error
}

func (s *Store) ListWorlds(ctx context.Context) ([]domain.World, error) {
	var worlds []domain.World
	if err := s.db.WithContext(ctx).Order("created_at desc").Find(&worlds).Error; err != nil {
		return nil, err
	}
	for index := range worlds {
		hydrateWorldConfigPayload(&worlds[index])
	}
	return worlds, nil
}

func (s *Store) GetWorld(ctx context.Context, id string) (domain.World, error) {
	var world domain.World
	err := s.db.WithContext(ctx).First(&world, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return world, ErrNotFound
	}
	if err == nil {
		hydrateWorldConfigPayload(&world)
	}
	return world, err
}

func (s *Store) GetWorldByInstanceAndFile(ctx context.Context, instanceID string, fileName string) (domain.World, error) {
	var world domain.World
	err := s.db.WithContext(ctx).First(&world, "instance_id = ? AND file_name = ?", instanceID, fileName).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return world, ErrNotFound
	}
	if err == nil {
		hydrateWorldConfigPayload(&world)
	}
	return world, err
}

func (s *Store) SaveWorld(ctx context.Context, world *domain.World) error {
	if err := prepareWorldConfigPayload(world); err != nil {
		return err
	}
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
	if event != nil && len(event.Payload) > 0 {
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			return err
		}
		event.PayloadJSON = string(payload)
	}
	if err := s.db.WithContext(ctx).Create(event).Error; err != nil {
		return err
	}
	if event != nil {
		s.broadcastActivity(*event)
	}
	return nil
}

func (s *Store) SubscribeActivity(ctx context.Context, instanceID string) <-chan domain.ActivityEvent {
	ch := make(chan domain.ActivityEvent, 32)
	s.activityMu.Lock()
	if s.activitySubscribers == nil {
		s.activitySubscribers = map[uint64]activitySubscriber{}
	}
	s.nextActivitySubID++
	id := s.nextActivitySubID
	s.activitySubscribers[id] = activitySubscriber{instanceID: instanceID, ch: ch}
	s.activityMu.Unlock()

	go func() {
		<-ctx.Done()
		s.activityMu.Lock()
		delete(s.activitySubscribers, id)
		close(ch)
		s.activityMu.Unlock()
	}()
	return ch
}

func (s *Store) broadcastActivity(event domain.ActivityEvent) {
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	for _, subscriber := range s.activitySubscribers {
		if subscriber.instanceID != "" && subscriber.instanceID != event.InstanceID {
			continue
		}
		select {
		case subscriber.ch <- event:
		default:
		}
	}
}

func (s *Store) ListActivity(ctx context.Context, limit int) ([]domain.ActivityEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var events []domain.ActivityEvent
	if err := s.db.WithContext(ctx).Order("created_at desc, rowid desc").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	for index := range events {
		hydrateActivityPayload(&events[index])
	}
	return events, nil
}

func (s *Store) ListActivityByInstance(ctx context.Context, instanceID string, limit int) ([]domain.ActivityEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var events []domain.ActivityEvent
	if err := s.db.WithContext(ctx).Where("instance_id = ?", instanceID).Order("created_at desc, rowid desc").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	for index := range events {
		hydrateActivityPayload(&events[index])
	}
	return events, nil
}

var ErrNotFound = errors.New("not found")

func hydrateActivityPayload(event *domain.ActivityEvent) {
	if event == nil || event.PayloadJSON == "" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err == nil {
		event.Payload = payload
	}
}
