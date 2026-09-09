package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
)

type GlobalIntentAdmission struct{ store *Store }

func NewGlobalIntentAdmission(store *Store) (*GlobalIntentAdmission, error) {
	if store == nil {
		return nil, errors.New("global intent admission store is required")
	}
	return &GlobalIntentAdmission{store: store}, nil
}

func (a *GlobalIntentAdmission) CheckCreate(ctx context.Context, actor string, request instances.CreateRequest) error {
	if actor == "" || request.ValidateMetadata() != nil {
		return instances.ErrInvalidIntent
	}
	return a.store.readSnapshot(ctx, func(tx *Store) error {
		if err := checkIntentMember(tx.db.WithContext(ctx), request.OrganizationID, actor); err != nil {
			return err
		}
		if err := tx.checkRegionCreate(ctx, request.RegionID); err != nil {
			return err
		}
		return tx.checkGlobalAssets(ctx, request.OrganizationID, request.Specification.Assets)
	})
}

func (a *GlobalIntentAdmission) CheckRevise(ctx context.Context, actor string, request instances.ReviseRequest) error {
	if actor == "" || request.ValidateMetadata() != nil {
		return instances.ErrInvalidIntent
	}
	return a.store.readSnapshot(ctx, func(tx *Store) error {
		if err := checkIntentMember(tx.db.WithContext(ctx), request.OrganizationID, actor); err != nil {
			return err
		}
		var server instances.Server
		if err := tx.db.WithContext(ctx).Table("logical_servers").Select("id,spec_generation,desired_state").Where("id = ? AND organization_id = ?", request.ServerID, request.OrganizationID).Take(&server).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if server.SpecGeneration != request.ExpectedGeneration || server.DesiredState == "deleted" {
			return instances.ErrVersionConflict
		}
		return tx.checkGlobalAssets(ctx, request.OrganizationID, request.Specification.Assets)
	})
}

func checkIntentMember(db *gorm.DB, organizationID, actor string) error {
	var member domain.OrganizationMember
	err := db.Table("organization_members").Select("id").Where("organization_id = ? AND user_id = ? AND role IN ?", organizationID, actor, []domain.Role{domain.RoleOwner, domain.RoleAdmin, domain.RoleMember}).Take(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrWorkspaceWriteDenied
	}
	return err
}
