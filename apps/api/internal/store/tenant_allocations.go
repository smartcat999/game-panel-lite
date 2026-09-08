package store

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidQuota            = errors.New("quota limits must be finite non-negative numbers")
	ErrQuotaExceeded           = errors.New("workspace resource quota exceeded")
	ErrFiniteResourcesRequired = errors.New("workspace instances require positive finite CPU and memory limits")
	ErrWorkspaceWriteDenied    = errors.New("workspace write permission required")
)

// All allocation writers lock the workspace before reading reservations. This
// serializes writers across API processes, including quota reductions.
func (s *Store) lockWorkspace(ctx context.Context, orgID string) error {
	result := s.db.WithContext(ctx).Model(&domain.Organization{}).Where("id = ?", orgID).
		UpdateColumn("updated_at", gorm.Expr("updated_at"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) lockWorkspaceWriter(ctx context.Context, orgID, userID string) error {
	if err := s.lockWorkspace(ctx, orgID); err != nil {
		return err
	}
	// An empty actor is reserved for platform administration by the caller.
	if userID == "" {
		return nil
	}
	result := s.db.WithContext(ctx).Model(&domain.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ? AND role IN ?", orgID, userID, []domain.Role{domain.RoleOwner, domain.RoleAdmin, domain.RoleMember}).
		UpdateColumn("role", gorm.Expr("role"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWorkspaceWriteDenied
	}
	return nil
}

func finiteResources(resources domain.ServerResources) bool {
	return resources.CPULimitCores > 0 && !math.IsNaN(resources.CPULimitCores) && !math.IsInf(resources.CPULimitCores, 0) && resources.MemoryLimitMB > 0
}

func (s *Store) checkAllocation(ctx context.Context, quota domain.TenantQuota, replacement *domain.GameServer) error {
	if quota.MaxServers < 0 || quota.MaxCPUCores < 0 || math.IsNaN(quota.MaxCPUCores) || math.IsInf(quota.MaxCPUCores, 0) || quota.MaxMemoryMB < 0 || quota.MaxStorageGB < 0 {
		return ErrInvalidQuota
	}

	var instances []domain.GameServer
	if err := s.db.WithContext(ctx).Where("organization_id = ?", quota.OrganizationID).Find(&instances).Error; err != nil {
		return err
	}
	logical, err := s.globalReservedResources(ctx, quota.OrganizationID)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(instances))
	for _, inst := range instances {
		seen[inst.ID] = true
	}
	for _, l := range logical {
		if !seen[l.ID] {
			seen[l.ID] = true
			instances = append(instances, l)
		}
	}
	count, cpu, memory := 0, 0.0, int64(0)
	for _, instance := range instances {
		if replacement != nil && instance.ID == replacement.ID {
			continue
		}
		// Legacy unlimited allocations must be assigned limits before admitting more.
		if !finiteResources(instance.Spec.Resources) {
			return ErrFiniteResourcesRequired
		}
		count++
		cpu += instance.Spec.Resources.CPULimitCores
		if int64(instance.Spec.Resources.MemoryLimitMB) > int64(quota.MaxMemoryMB)-memory {
			return ErrQuotaExceeded
		}
		memory += int64(instance.Spec.Resources.MemoryLimitMB)
	}
	if replacement != nil {
		if !finiteResources(replacement.Spec.Resources) {
			return ErrFiniteResourcesRequired
		}
		count++
		cpu += replacement.Spec.Resources.CPULimitCores
		if int64(replacement.Spec.Resources.MemoryLimitMB) > int64(quota.MaxMemoryMB)-memory {
			return ErrQuotaExceeded
		}
		memory += int64(replacement.Spec.Resources.MemoryLimitMB)
	}
	if count > quota.MaxServers || cpu > quota.MaxCPUCores || memory > int64(quota.MaxMemoryMB) {
		return ErrQuotaExceeded
	}
	return nil
}

func (s *Store) CreateAllocatedGameServer(ctx context.Context, userID string, instance *domain.GameServer) error {
	if instance.OrganizationID == "" {
		return ErrWorkspaceWriteDenied
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspaceWriter(ctx, instance.OrganizationID, userID); err != nil {
			return err
		}
		quota, err := tx.GetTenantQuota(ctx, instance.OrganizationID)
		if err != nil {
			return err
		}
		if err := tx.validateServerModReferences(ctx, *instance); err != nil {
			return err
		}
		if err := tx.checkAllocation(ctx, quota, instance); err != nil {
			return err
		}
		if err := tx.lockNodeAllocation(ctx, *instance); err != nil {
			return err
		}
		if err := tx.CreateGameServer(ctx, instance); err != nil {
			return err
		}
		return tx.ensureLogicalServerSync(ctx, instance)
	})
}

func (s *Store) SaveAllocatedGameServer(ctx context.Context, userID string, before, after domain.GameServer) error {
	if before.ID != after.ID || before.OrganizationID == "" || after.OrganizationID != before.OrganizationID {
		return ErrWorkspaceWriteDenied
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspaceWriter(ctx, before.OrganizationID, userID); err != nil {
			return err
		}
		quota, err := tx.GetTenantQuota(ctx, before.OrganizationID)
		if err != nil {
			return err
		}
		if err := tx.validateServerModReferences(ctx, after); err != nil {
			return err
		}
		if err := tx.checkAllocation(ctx, quota, &after); err != nil {
			return err
		}
		if before.Spec.Resources != after.Spec.Resources || before.Spec.Network != after.Spec.Network {
			if err := tx.lockNodeAllocation(ctx, after); err != nil {
				return err
			}
		}
		return tx.saveServerIntent(ctx, before, after)
	})
}

// Compare the complete previous intent; checking only generation would miss
// writers that have not yet migrated to generation-aware commands.
func (s *Store) saveServerIntent(ctx context.Context, before, after domain.GameServer) error {
	spec, err := json.Marshal(before.Spec)
	if err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Model(&domain.GameServer{}).
		Where("id = ? AND spec = ? AND organization_id = ? AND node_id = ?", before.ID, string(spec), before.OrganizationID, before.NodeID).
		Select("spec", "updated_at").Updates(&after)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrReconciliationSuperseded
	}
	return nil
}

func (s *Store) ensureLogicalServerSync(ctx context.Context, instance *domain.GameServer) error {
	if instance == nil || instance.ID == "" || instance.OrganizationID == "" {
		return nil
	}
	var existing struct{ ID string }
	err := s.db.WithContext(ctx).Table("logical_servers").Select("id").Where("id = ?", instance.ID).Take(&existing).Error
	if err == nil {
		return nil
	}

	now := instance.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	revID := "rev-" + instance.ID

	cpu := instance.Spec.Resources.CPULimitCores
	if cpu <= 0 {
		cpu = 2
	}
	mem := int64(instance.Spec.Resources.MemoryLimitMB)
	if mem <= 0 {
		mem = 4096
	}

	gameVer := instance.Spec.Version
	if gameVer == "" {
		gameVer = "default"
	}

	spec := instances.Specification{
		ProviderKey:         string(instance.ProviderKey),
		GameVersion:         gameVer,
		ConfigSchemaVersion: 1,
		Configuration:       instances.ProtectedConfiguration{KeyID: "plain", Ciphertext: []byte("{}")},
		Resources: instances.Resources{
			CPU:      cpu,
			MemoryMB: mem,
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		return err
	}

	serverRow := struct {
		ID                string
		OrganizationID    string
		Name              string
		CurrentRevisionID string
		SpecGeneration    int64
		DesiredState      string
		IntentVersion     int64
		CreatedAt         time.Time
	}{
		ID:                instance.ID,
		OrganizationID:    instance.OrganizationID,
		Name:              instance.Name,
		CurrentRevisionID: revID,
		SpecGeneration:    1,
		DesiredState:      "running",
		IntentVersion:     1,
		CreatedAt:         now,
	}

	revisionRow := struct {
		ID             string
		ServerID       string
		SpecGeneration int64
		Specification  string
		CPU            float64
		MemoryMB       int64
		CreatedAt      time.Time
	}{
		ID:             revID,
		ServerID:       instance.ID,
		SpecGeneration: 1,
		Specification:  string(specBytes),
		CPU:            cpu,
		MemoryMB:       mem,
		CreatedAt:      now,
	}

	placementRow := struct {
		ServerID       string
		RegionID       string
		PlacementEpoch int64
	}{
		ServerID:       instance.ID,
		RegionID:       "default",
		PlacementEpoch: 1,
	}

	if err := s.db.WithContext(ctx).Table("logical_servers").Clauses(clause.OnConflict{DoNothing: true}).Create(&serverRow).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Table("server_revisions").Clauses(clause.OnConflict{DoNothing: true}).Create(&revisionRow).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Table("server_placements").Clauses(clause.OnConflict{DoNothing: true}).Create(&placementRow).Error; err != nil {
		return err
	}
	return nil
}

