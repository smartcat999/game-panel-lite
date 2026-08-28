package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

func (s *Store) Transaction(ctx context.Context, fn func(*Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scoped := &Store{db: tx}
		return fn(scoped)
	})
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&domain.GameServer{}, &domain.Backup{}, &domain.World{}, &domain.ModFile{}, &domain.ModPack{}, &domain.ActivityEvent{}, &domain.GameUpdateJob{}, &domain.WorldRegenerationJob{}, &domain.AdminAccount{}, &domain.Session{}, &domain.Setting{}, &domain.ServerShare{}, &domain.ConfigPreset{}, &domain.Organization{}, &domain.OrganizationMember{}, &domain.TenantQuota{}, &domain.ComputeNode{}, &domain.NodeTask{}, &domain.WorkloadAssignment{}, &domain.WorkloadObservation{}); err != nil {
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

func (s *Store) ListAdminAccounts(ctx context.Context) ([]domain.AdminAccount, error) {
	var accounts []domain.AdminAccount
	err := s.db.WithContext(ctx).Order("created_at asc").Find(&accounts).Error
	return accounts, err
}

func (s *Store) DeleteAdminAccount(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.AdminAccount{}, "id = ?", id).Error
}

func (s *Store) CountAdminRoleAccounts(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&domain.AdminAccount{}).Where("role = ?", domain.RoleAdmin).Count(&count).Error
	return count, err
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

type GameServerListOptions struct {
	Page        int
	PageSize    int
	Search      string
	GameKey     string
	ProviderKey string
	Status      string
	Sort        string
	Direction   string
}

type GameServerPage struct {
	Items      []domain.GameServer `json:"items"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
	TotalPages int                 `json:"totalPages"`
}

func (s *Store) ListGameServersPage(ctx context.Context, options GameServerListOptions) (GameServerPage, error) {
	page := options.Page
	if page < 1 {
		page = 1
	}
	pageSize := options.PageSize
	if pageSize != 20 && pageSize != 50 && pageSize != 100 {
		pageSize = 20
	}

	query := s.db.WithContext(ctx).Model(&domain.GameServer{})
	if search := strings.TrimSpace(options.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Where("lower(name) LIKE ? OR lower(id) LIKE ?", like, like)
	}
	if gameKey := strings.TrimSpace(options.GameKey); gameKey != "" && gameKey != "all" {
		query = query.Where("game_key = ?", gameKey)
	}
	if providerKey := strings.TrimSpace(options.ProviderKey); providerKey != "" && providerKey != "all" {
		query = query.Where("provider_key = ?", providerKey)
	}
	switch strings.TrimSpace(options.Status) {
	case "running":
		query = query.Where("json_extract(status, '$.phase') = ?", domain.PhaseRunning)
	case "stopped":
		query = query.Where("json_extract(status, '$.phase') = ?", domain.PhaseStopped)
	case "errored":
		query = query.Where("json_extract(status, '$.phase') = ?", domain.PhaseFailed)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return GameServerPage{}, err
	}

	direction := "desc"
	if strings.EqualFold(options.Direction, "asc") {
		direction = "asc"
	}
	order := "updated_at " + direction
	switch options.Sort {
	case "name":
		order = "lower(name) " + direction
	case "status":
		order = "json_extract(status, '$.phase') " + direction
	case "createdAt":
		order = "created_at " + direction
	case "updatedAt", "":
		order = "updated_at " + direction
	}

	var servers []domain.GameServer
	if err := query.Order(order).Offset((page - 1) * pageSize).Limit(pageSize).Find(&servers).Error; err != nil {
		return GameServerPage{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return GameServerPage{Items: servers, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
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

func (s *Store) CreateGameUpdateJob(ctx context.Context, job *domain.GameUpdateJob) error {
	return s.db.WithContext(ctx).Create(job).Error
}

func (s *Store) SaveGameUpdateJob(ctx context.Context, job *domain.GameUpdateJob) error {
	return s.db.WithContext(ctx).Save(job).Error
}

func (s *Store) GetGameUpdateJobByID(ctx context.Context, id string) (domain.GameUpdateJob, error) {
	var job domain.GameUpdateJob
	err := s.db.WithContext(ctx).First(&job, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return job, ErrNotFound
	}
	return job, err
}

func (s *Store) GetLatestGameUpdateJobByInstance(ctx context.Context, instanceID string) (domain.GameUpdateJob, error) {
	var job domain.GameUpdateJob
	err := s.db.WithContext(ctx).
		Where("instance_id = ?", instanceID).
		Order("created_at desc, rowid desc").
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return job, ErrNotFound
	}
	return job, err
}

func (s *Store) GetLatestGameUpdateCheckByProvider(ctx context.Context, providerKey domain.ProviderKey) (domain.GameUpdateJob, error) {
	var job domain.GameUpdateJob
	err := s.db.WithContext(ctx).
		Where("provider_key = ? AND operation = ?", providerKey, domain.GameUpdateOperationCheck).
		Order("created_at desc, rowid desc").
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return job, ErrNotFound
	}
	return job, err
}

func (s *Store) GetActiveGameUpdateJobByInstance(ctx context.Context, instanceID string) (domain.GameUpdateJob, error) {
	var job domain.GameUpdateJob
	err := s.db.WithContext(ctx).
		Where("instance_id = ? AND status IN ?", instanceID, []domain.GameUpdateJobStatus{domain.GameUpdateJobQueued, domain.GameUpdateJobRunning}).
		Order("created_at desc, rowid desc").
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return job, ErrNotFound
	}
	return job, err
}

func (s *Store) ListActiveGameUpdateJobs(ctx context.Context) ([]domain.GameUpdateJob, error) {
	var jobs []domain.GameUpdateJob
	err := s.db.WithContext(ctx).
		Where("status IN ?", []domain.GameUpdateJobStatus{domain.GameUpdateJobQueued, domain.GameUpdateJobRunning}).
		Order("created_at asc, rowid asc").
		Find(&jobs).Error
	return jobs, err
}

func (s *Store) CreateWorldRegenerationJob(ctx context.Context, job *domain.WorldRegenerationJob) error {
	return s.db.WithContext(ctx).Create(job).Error
}

func (s *Store) SaveWorldRegenerationJob(ctx context.Context, job *domain.WorldRegenerationJob) error {
	return s.db.WithContext(ctx).Save(job).Error
}

func (s *Store) GetLatestWorldRegenerationJobByInstance(ctx context.Context, instanceID string) (domain.WorldRegenerationJob, error) {
	var job domain.WorldRegenerationJob
	err := s.db.WithContext(ctx).Where("instance_id = ?", instanceID).Order("created_at desc, rowid desc").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return job, ErrNotFound
	}
	return job, err
}

func (s *Store) GetActiveWorldRegenerationJobByInstance(ctx context.Context, instanceID string) (domain.WorldRegenerationJob, error) {
	var job domain.WorldRegenerationJob
	err := s.db.WithContext(ctx).
		Where("instance_id = ? AND status IN ?", instanceID, []domain.WorldRegenerationJobStatus{domain.WorldRegenerationJobQueued, domain.WorldRegenerationJobRunning}).
		Order("created_at desc, rowid desc").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return job, ErrNotFound
	}
	return job, err
}

func (s *Store) ListActiveWorldRegenerationJobs(ctx context.Context) ([]domain.WorldRegenerationJob, error) {
	var jobs []domain.WorldRegenerationJob
	err := s.db.WithContext(ctx).
		Where("status IN ?", []domain.WorldRegenerationJobStatus{domain.WorldRegenerationJobQueued, domain.WorldRegenerationJobRunning}).
		Order("created_at asc, rowid asc").Find(&jobs).Error
	return jobs, err
}

func hydratePresetConfigPayload(preset *domain.ConfigPreset) {
	if preset == nil {
		return
	}
	if preset.ConfigPayloadJSON != "" {
		var payload map[string]any
		if err := json.Unmarshal([]byte(preset.ConfigPayloadJSON), &payload); err == nil {
			preset.Config = payload
			preset.ConfigPayload = payload
		}
	}
	preset.ModIDs = []string{}
	if preset.ModIDsJSON != "" {
		_ = json.Unmarshal([]byte(preset.ModIDsJSON), &preset.ModIDs)
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

func (s *Store) EnsureDefaultOrganization(ctx context.Context) (*domain.Organization, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&domain.Organization{}).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		var first domain.Organization
		if err := s.db.WithContext(ctx).First(&first).Error; err != nil {
			return nil, err
		}
		return &first, nil
	}

	defaultOrg := domain.Organization{
		ID:        "default-org",
		Name:      "Default Workspace",
		Slug:      "default",
		Plan:      "pro",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&defaultOrg).Error; err != nil {
		return nil, err
	}

	defaultQuota := domain.TenantQuota{
		OrganizationID: defaultOrg.ID,
		MaxServers:     10,
		MaxCPUCores:    16.0,
		MaxMemoryMB:    32768,
		MaxStorageGB:   100,
	}
	_ = s.db.WithContext(ctx).Create(&defaultQuota).Error

	var admin domain.AdminAccount
	if err := s.db.WithContext(ctx).First(&admin).Error; err == nil {
		member := domain.OrganizationMember{
			ID:             "default-member-" + admin.ID,
			OrganizationID: defaultOrg.ID,
			UserID:         admin.ID,
			Role:           domain.RoleOwner,
			CreatedAt:      time.Now().UTC(),
		}
		_ = s.db.WithContext(ctx).Create(&member).Error
	}

	// Update existing unassigned servers to default organization
	_ = s.db.WithContext(ctx).Model(&domain.GameServer{}).Where("organization_id = '' OR organization_id IS NULL").Update("organization_id", defaultOrg.ID).Error

	return &defaultOrg, nil
}

func (s *Store) ListOrganizations(ctx context.Context) ([]domain.Organization, error) {
	var orgs []domain.Organization
	if err := s.db.WithContext(ctx).Order("created_at asc").Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

func (s *Store) GetOrganization(ctx context.Context, id string) (domain.Organization, error) {
	var org domain.Organization
	err := s.db.WithContext(ctx).First(&org, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return org, ErrNotFound
	}
	return org, err
}

func (s *Store) CreateOrganization(ctx context.Context, org *domain.Organization, ownerUserID string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.db.WithContext(ctx).Create(org).Error; err != nil {
			return err
		}
		member := domain.OrganizationMember{
			ID:             "member-" + org.ID + "-" + ownerUserID,
			OrganizationID: org.ID,
			UserID:         ownerUserID,
			Role:           domain.RoleOwner,
			CreatedAt:      time.Now().UTC(),
		}
		if err := tx.db.WithContext(ctx).Create(&member).Error; err != nil {
			return err
		}
		quota := domain.TenantQuota{
			OrganizationID: org.ID,
			MaxServers:     5,
			MaxCPUCores:    8.0,
			MaxMemoryMB:    16384,
			MaxStorageGB:   50,
		}
		return tx.db.WithContext(ctx).Create(&quota).Error
	})
}

func (s *Store) ListOrganizationMembers(ctx context.Context, orgID string) ([]domain.OrganizationMember, error) {
	var members []domain.OrganizationMember
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Find(&members).Error; err != nil {
		return nil, err
	}
	return members, nil
}

func (s *Store) GetOrganizationMember(ctx context.Context, orgID, userID string) (domain.OrganizationMember, error) {
	var member domain.OrganizationMember
	err := s.db.WithContext(ctx).First(&member, "organization_id = ? AND user_id = ?", orgID, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return member, ErrNotFound
	}
	return member, err
}

func (s *Store) AddOrganizationMember(ctx context.Context, member *domain.OrganizationMember) error {
	return s.db.WithContext(ctx).Create(member).Error
}

func (s *Store) RemoveOrganizationMember(ctx context.Context, orgID, userID string) error {
	return s.db.WithContext(ctx).Delete(&domain.OrganizationMember{}, "organization_id = ? AND user_id = ?", orgID, userID).Error
}

func (s *Store) GetTenantQuota(ctx context.Context, orgID string) (domain.TenantQuota, error) {
	var quota domain.TenantQuota
	err := s.db.WithContext(ctx).First(&quota, "organization_id = ?", orgID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.TenantQuota{
			OrganizationID: orgID,
			MaxServers:     10,
			MaxCPUCores:    16.0,
			MaxMemoryMB:    32768,
			MaxStorageGB:   100,
		}, nil
	}
	return quota, err
}

func (s *Store) UpdateTenantQuota(ctx context.Context, quota domain.TenantQuota) error {
	return s.db.WithContext(ctx).Save(&quota).Error
}

func (s *Store) GetTenantUsage(ctx context.Context, orgID string) (domain.TenantUsage, error) {
	quota, _ := s.GetTenantQuota(ctx, orgID)
	var servers []domain.GameServer
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Find(&servers).Error; err != nil {
		return domain.TenantUsage{Quota: quota}, err
	}

	running := 0
	var usedCpu float64
	var usedMemory int

	for _, srv := range servers {
		if srv.Status.ActualState == domain.ActualRunning {
			running++
			usedCpu += srv.Spec.Resources.CPULimitCores
			usedMemory += srv.Spec.Resources.MemoryLimitMB
		}
	}

	return domain.TenantUsage{
		TotalServers:   len(servers),
		RunningServers: running,
		UsedCPUCores:   usedCpu,
		UsedMemoryMB:   usedMemory,
		Quota:          quota,
	}, nil
}

func detectHostHardware() (int, int64) {
	cores := runtime.NumCPU()
	if cores <= 0 {
		cores = 4
	}

	memMB := int64(16384)
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, parseErr := strconv.ParseInt(fields[1], 10, 64); parseErr == nil && kb > 0 {
						memMB = kb / 1024
					}
				}
				break
			}
		}
	}
	return cores, memMB
}

func (s *Store) EnsureDefaultLocalNode(ctx context.Context) (*domain.ComputeNode, error) {
	hostCores, hostMemMB := detectHostHardware()

	var first domain.ComputeNode
	err := s.db.WithContext(ctx).Where("is_local = ?", true).First(&first).Error
	if err == nil {
		first.CPUCores = hostCores
		first.MemoryTotalMB = hostMemMB
		_ = s.db.WithContext(ctx).Save(&first).Error
		return &first, nil
	}

	localNode := domain.ComputeNode{
		ID:            "node-local",
		Name:          "Local Host Daemon",
		Host:          "127.0.0.1",
		Port:          4000,
		PublicIP:      "127.0.0.1",
		Region:        "Local",
		Status:        "online",
		IsLocal:       true,
		CPUCores:      hostCores,
		MemoryTotalMB: hostMemMB,
		MemoryUsedMB:  1024,
		RunningCount:  0,
		LastHeartbeat: time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&localNode).Error; err != nil {
		return nil, err
	}

	// Associate existing unassigned servers to local node
	_ = s.db.WithContext(ctx).Model(&domain.GameServer{}).Where("node_id = '' OR node_id IS NULL").Update("node_id", localNode.ID).Error

	return &localNode, nil
}

func (s *Store) ListComputeNodes(ctx context.Context) ([]domain.ComputeNode, error) {
	var nodes []domain.ComputeNode
	if err := s.db.WithContext(ctx).Order("is_local desc, created_at asc").Find(&nodes).Error; err != nil {
		return nil, err
	}

	hostCores, hostMemMB := detectHostHardware()

	// Lightweight projection for node server stats instead of full table deserialize
	type nodeServerStat struct {
		NodeID string `gorm:"column:node_id"`
	}
	var serverStats []nodeServerStat
	_ = s.db.WithContext(ctx).Model(&domain.GameServer{}).
		Select("node_id").
		Find(&serverStats).Error

	nodeServerCounts := make(map[string]int, len(nodes))
	for _, stat := range serverStats {
		nID := stat.NodeID
		if nID == "" {
			nID = "node-local"
		}
		nodeServerCounts[nID]++
	}

	for i := range nodes {
		if nodes[i].IsLocal {
			nodes[i].CPUCores = hostCores
			nodes[i].MemoryTotalMB = hostMemMB
		}

		if !nodes[i].IsLocal {
			if time.Since(nodes[i].LastHeartbeat) > 45*time.Second {
				nodes[i].Status = "offline"
			}
		}
		nodes[i].RunningCount = nodeServerCounts[nodes[i].ID]
	}

	return nodes, nil
}

func (s *Store) GetComputeNode(ctx context.Context, id string) (domain.ComputeNode, error) {
	var node domain.ComputeNode
	err := s.db.WithContext(ctx).First(&node, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return node, ErrNotFound
	}
	if node.IsLocal {
		cores, mem := detectHostHardware()
		node.CPUCores = cores
		node.MemoryTotalMB = mem
	} else if time.Since(node.LastHeartbeat) > 45*time.Second {
		node.Status = "offline"
	}
	return node, err
}

func (s *Store) GetComputeNodeByToken(ctx context.Context, token string) (domain.ComputeNode, error) {
	var node domain.ComputeNode
	err := s.db.WithContext(ctx).First(&node, "token = ?", token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return node, ErrNotFound
	}
	return node, err
}

func (s *Store) CreateComputeNode(ctx context.Context, node *domain.ComputeNode) error {
	return s.db.WithContext(ctx).Create(node).Error
}

func (s *Store) UpdateComputeNode(ctx context.Context, node *domain.ComputeNode) error {
	return s.db.WithContext(ctx).Save(node).Error
}

func (s *Store) DeleteComputeNode(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&domain.ComputeNode{}, "id = ? AND is_local = ?", id, false).Error
}

func (s *Store) CreateNodeTask(ctx context.Context, task *domain.NodeTask) error {
	return s.db.WithContext(ctx).Create(task).Error
}

func (s *Store) ListPendingNodeTasks(ctx context.Context, nodeID string) ([]domain.NodeTask, error) {
	var tasks []domain.NodeTask
	err := s.db.WithContext(ctx).
		Where("node_id = ? AND status = ?", nodeID, domain.TaskPending).
		Order("created_at asc").
		Find(&tasks).Error
	return tasks, err
}

func (s *Store) UpdateNodeTaskStatus(ctx context.Context, taskID string, status domain.NodeTaskStatus, errMsg string) error {
	updates := map[string]interface{}{
		"status":     status,
		"error":      errMsg,
		"updated_at": time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Model(&domain.NodeTask{}).Where("id = ?", taskID).Updates(updates).Error
}

func (s *Store) GetNodeTask(ctx context.Context, taskID string) (domain.NodeTask, error) {
	var task domain.NodeTask
	err := s.db.WithContext(ctx).First(&task, "id = ?", taskID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return task, ErrNotFound
	}
	return task, err
}

func (s *Store) UpsertWorkloadAssignment(ctx context.Context, assignment *domain.WorkloadAssignment) error {
	var current domain.WorkloadAssignment
	err := s.db.WithContext(ctx).First(&current, "server_id = ?", assignment.ServerID).Error
	if err == nil {
		assignment.ID = current.ID
		assignment.CreatedAt = current.CreatedAt
		return s.db.WithContext(ctx).Save(assignment).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.WithContext(ctx).Create(assignment).Error
}

func (s *Store) GetWorkloadAssignmentByServer(ctx context.Context, serverID string) (domain.WorkloadAssignment, error) {
	var assignment domain.WorkloadAssignment
	err := s.db.WithContext(ctx).First(&assignment, "server_id = ?", serverID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return assignment, ErrNotFound
	}
	return assignment, err
}

func (s *Store) GetWorkloadAssignmentByUID(ctx context.Context, uid string) (domain.WorkloadAssignment, error) {
	var assignment domain.WorkloadAssignment
	err := s.db.WithContext(ctx).First(&assignment, "uid = ?", uid).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return assignment, ErrNotFound
	}
	return assignment, err
}

func (s *Store) ListWorkloadAssignmentsByNode(ctx context.Context, nodeID string) ([]domain.WorkloadAssignment, error) {
	var assignments []domain.WorkloadAssignment
	err := s.db.WithContext(ctx).Where("node_id = ?", nodeID).Order("created_at asc").Find(&assignments).Error
	return assignments, err
}

func (s *Store) DeleteWorkloadAssignment(ctx context.Context, serverID string) error {
	return s.db.WithContext(ctx).Where("server_id = ?", serverID).Delete(&domain.WorkloadAssignment{}).Error
}

func (s *Store) UpsertWorkloadObservation(ctx context.Context, observation *domain.WorkloadObservation) error {
	var current domain.WorkloadObservation
	err := s.db.WithContext(ctx).First(&current, "assignment_uid = ?", observation.AssignmentUID).Error
	if err == nil {
		if current.ObservedGeneration > observation.ObservedGeneration {
			return nil
		}
		observation.ID = current.ID
		observation.CreatedAt = current.CreatedAt
		return s.db.WithContext(ctx).Save(observation).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.WithContext(ctx).Create(observation).Error
}

func (s *Store) GetWorkloadObservation(ctx context.Context, assignmentUID string) (domain.WorkloadObservation, error) {
	var observation domain.WorkloadObservation
	err := s.db.WithContext(ctx).First(&observation, "assignment_uid = ?", assignmentUID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return observation, ErrNotFound
	}
	return observation, err
}
