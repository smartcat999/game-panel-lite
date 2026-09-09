package store

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type prepaidPlanRow struct {
	PlanID             string
	Version            int64
	Terms, PublishedBy string
}
type prepaidSaleRow struct {
	PlanID      string
	PlanVersion int64
	Enabled     bool
	Version     int64
	UpdatedBy   string
}

func requireCatalogOperator(tx *gorm.DB, actor string) error {
	var account struct {
		Role         domain.Role
		PlatformRole domain.PlatformRole
	}
	q := tx.Table("admin_accounts").Select("role", "platform_role").Where("id = ?", actor)
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "SHARE"})
	}
	err := q.Take(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && domain.NormalizePlatformRole(account.PlatformRole, account.Role) != domain.PlatformRoleAdmin) {
		return commerce.ErrOperatorRequired
	}
	return err
}

// PublishPrepaidPlan atomically creates immutable terms and a disabled sale row.
// Publishing identical terms is idempotent and never re-enables a retired plan.
func (s *Store) PublishPrepaidPlan(ctx context.Context, actor string, plan commerce.PlanVersion) error {
	if plan.Validate() != nil {
		return commerce.ErrInvalidPlan
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return commerce.ErrInvalidPlan
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := requireCatalogOperator(tx.db, actor); err != nil {
			return err
		}
		row := prepaidPlanRow{PlanID: plan.PlanID, Version: plan.Version, Terms: string(encoded), PublishedBy: actor}
		if err := tx.db.Table("prepaid_plan_versions").Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		var existing prepaidPlanRow
		if err := tx.db.Table("prepaid_plan_versions").Where("plan_id = ? AND version = ?", plan.PlanID, plan.Version).Take(&existing).Error; err != nil {
			return err
		}
		if existing.Terms != row.Terms {
			return commerce.ErrPlanConflict
		}
		sale := prepaidSaleRow{PlanID: plan.PlanID, PlanVersion: plan.Version, Version: 1, UpdatedBy: actor}
		return tx.db.Table("prepaid_plan_sales").Clauses(clause.OnConflict{DoNothing: true}).Create(&sale).Error
	})
}

// SetPrepaidPlanSale controls future checkout eligibility, not existing orders.
func (s *Store) SetPrepaidPlanSale(ctx context.Context, actor, planID string, planVersion, expectedVersion int64, enabled bool) error {
	if planID == "" || planVersion < 1 || expectedVersion < 1 || expectedVersion == math.MaxInt64 {
		return commerce.ErrInvalidPlan
	}
	return s.Transaction(ctx, func(tx *Store) error {
		if err := requireCatalogOperator(tx.db, actor); err != nil {
			return err
		}
		result := tx.db.Table("prepaid_plan_sales").Where("plan_id = ? AND plan_version = ? AND version = ?", planID, planVersion, expectedVersion).Updates(map[string]any{"enabled": enabled, "version": expectedVersion + 1, "updated_by": actor})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return commerce.ErrCatalogVersionConflict
		}
		return nil
	})
}

// QuoteListedPrepaidPlan obtains terms from the server catalog. Checkout must
// recheck sale eligibility in its order transaction; a quote is not a reservation.
func (s *Store) QuoteListedPrepaidPlan(ctx context.Context, planID string, version, periods int64) (commerce.Quote, error) {
	var quote commerce.Quote
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var sale prepaidSaleRow
		if err := tx.db.Table("prepaid_plan_sales").Where("plan_id = ? AND plan_version = ? AND enabled = ?", planID, version, true).Take(&sale).Error; err != nil {
			return err
		}
		var row prepaidPlanRow
		if err := tx.db.Table("prepaid_plan_versions").Where("plan_id = ? AND version = ?", planID, version).Take(&row).Error; err != nil {
			return err
		}
		var plan commerce.PlanVersion
		if json.Unmarshal([]byte(row.Terms), &plan) != nil || plan.PlanID != planID || plan.Version != version {
			return commerce.ErrInvalidPlan
		}
		var err error
		quote, err = commerce.QuotePrepaid(plan, periods)
		return err
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = commerce.ErrPlanUnavailable
		}
		return commerce.Quote{}, err
	}
	return quote, nil
}

// ListAvailablePrepaidPlans returns all currently enabled prepaid plans.
func (s *Store) ListAvailablePrepaidPlans(ctx context.Context) ([]commerce.PlanVersion, error) {
	var plans []commerce.PlanVersion
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var sales []prepaidSaleRow
		if err := tx.db.Table("prepaid_plan_sales").Where("enabled = ?", true).Find(&sales).Error; err != nil {
			return err
		}
		for _, sale := range sales {
			var row prepaidPlanRow
			if err := tx.db.Table("prepaid_plan_versions").Where("plan_id = ? AND version = ?", sale.PlanID, sale.PlanVersion).Take(&row).Error; err != nil {
				continue
			}
			var plan commerce.PlanVersion
			if json.Unmarshal([]byte(row.Terms), &plan) == nil && plan.PlanID == sale.PlanID && plan.Version == sale.PlanVersion {
				plans = append(plans, plan)
			}
		}
		return nil
	})
	return plans, err
}

// SeedDefaultPrepaidPlans ensures a system catalog operator, default region, and baseline
// commercial plans are published and enabled for sale.
func (s *Store) SeedDefaultPrepaidPlans(ctx context.Context) error {
	var admin domain.AdminAccount
	err := s.db.WithContext(ctx).Table("admin_accounts").Where("platform_role = ?", domain.PlatformRoleAdmin).First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		admin = domain.AdminAccount{
			ID:           "system-catalog-admin",
			Username:     "system_admin",
			Role:         domain.RoleAdmin,
			PlatformRole: domain.PlatformRoleAdmin,
			PasswordHash: "$2a$10$defaultseedhashplaceholder",
		}
		if createErr := s.db.WithContext(ctx).Table("admin_accounts").Create(&admin).Error; createErr != nil {
			return createErr
		}
	} else if err != nil {
		return err
	}

	defaultRegion := struct {
		ID               string
		Name             string
		AcceptingCreates bool
		Version          int64
	}{
		ID:               "default",
		Name:             "默认区域 (Default Region)",
		AcceptingCreates: true,
		Version:          1,
	}
	_ = s.db.WithContext(ctx).Table("global_regions").Clauses(clause.OnConflict{DoNothing: true}).Create(&defaultRegion).Error

	defaultPlans := []commerce.PlanVersion{
		{
			PlanID:               "terraria-starter",
			Version:              1,
			ProviderKey:          "terraria-vanilla",
			RegionID:             "default",
			CPU:                  2,
			MemoryMB:             4096,
			StorageBytes:         20 * 1024 * 1024 * 1024,
			BackupRetentionCount: 5,
			Currency:             "CNY",
			UnitAmountMinor:      1900,
			PeriodSeconds:        86400 * 30,
		},
		{
			PlanID:               "terraria-pro",
			Version:              1,
			ProviderKey:          "terraria-vanilla",
			RegionID:             "default",
			CPU:                  4,
			MemoryMB:             8192,
			StorageBytes:         50 * 1024 * 1024 * 1024,
			BackupRetentionCount: 10,
			Currency:             "CNY",
			UnitAmountMinor:      3900,
			PeriodSeconds:        86400 * 30,
		},
		{
			PlanID:               "tmodloader-expert",
			Version:              1,
			ProviderKey:          "terraria-tmodloader",
			RegionID:             "default",
			CPU:                  4,
			MemoryMB:             8192,
			StorageBytes:         50 * 1024 * 1024 * 1024,
			BackupRetentionCount: 10,
			Currency:             "CNY",
			UnitAmountMinor:      4900,
			PeriodSeconds:        86400 * 30,
		},
		{
			PlanID:               "tmodloader-flagship",
			Version:              1,
			ProviderKey:          "terraria-tmodloader",
			RegionID:             "default",
			CPU:                  8,
			MemoryMB:             16384,
			StorageBytes:         100 * 1024 * 1024 * 1024,
			BackupRetentionCount: 20,
			Currency:             "CNY",
			UnitAmountMinor:      8900,
			PeriodSeconds:        86400 * 30,
		},
		{
			PlanID:               "palworld-standard",
			Version:              1,
			ProviderKey:          "palworld",
			RegionID:             "default",
			CPU:                  4,
			MemoryMB:             16384,
			StorageBytes:         80 * 1024 * 1024 * 1024,
			BackupRetentionCount: 10,
			Currency:             "CNY",
			UnitAmountMinor:      6900,
			PeriodSeconds:        86400 * 30,
		},
		{
			PlanID:               "minecraft-paper",
			Version:              1,
			ProviderKey:          "minecraft",
			RegionID:             "default",
			CPU:                  4,
			MemoryMB:             8192,
			StorageBytes:         50 * 1024 * 1024 * 1024,
			BackupRetentionCount: 10,
			Currency:             "CNY",
			UnitAmountMinor:      3900,
			PeriodSeconds:        86400 * 30,
		},
		{
			PlanID:               "dst-wilderness",
			Version:              1,
			ProviderKey:          "dst",
			RegionID:             "default",
			CPU:                  2,
			MemoryMB:             4096,
			StorageBytes:         30 * 1024 * 1024 * 1024,
			BackupRetentionCount: 5,
			Currency:             "CNY",
			UnitAmountMinor:      2900,
			PeriodSeconds:        86400 * 30,
		},
	}

	for _, plan := range defaultPlans {
		var existing prepaidPlanRow
		if err := s.db.WithContext(ctx).Table("prepaid_plan_versions").Where("plan_id = ? AND version = ?", plan.PlanID, plan.Version).Take(&existing).Error; err == nil {
			_ = s.db.WithContext(ctx).Table("prepaid_plan_sales").Where("plan_id = ? AND plan_version = ?", plan.PlanID, plan.Version).Update("enabled", true).Error
			continue
		}
		if err := s.PublishPrepaidPlan(ctx, admin.ID, plan); err != nil && !errors.Is(err, commerce.ErrPlanConflict) {
			continue
		}
		_ = s.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, plan.Version, 1, true)
	}
	return nil
}

func migrateSQLitePrepaidCatalog(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 11").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(prepaidCatalogSQL).Error; err != nil {
			return err
		}
		for _, statement := range []string{
			"CREATE TRIGGER prepaid_plan_versions_no_update BEFORE UPDATE ON prepaid_plan_versions BEGIN SELECT RAISE(ABORT, 'published plan version is immutable'); END",
			"CREATE TRIGGER prepaid_plan_versions_no_delete BEFORE DELETE ON prepaid_plan_versions BEGIN SELECT RAISE(ABORT, 'published plan version is immutable'); END",
			"INSERT INTO gamepanel_sqlite_migrations(version) VALUES(11)",
		} {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
