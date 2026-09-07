package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

// CreateAccountWithPersonalOrganization commits the account, workspace,
// owner membership and initial quota together. Session creation follows commit.
func (s *Store) CreateAccountWithPersonalOrganization(ctx context.Context, account *domain.AdminAccount) error {
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.CreateAdminAccount(ctx, account); err != nil {
			return err
		}
		id := uuid.NewString()
		org := domain.Organization{
			ID:        id,
			Name:      account.Username + "'s workspace",
			Slug:      "personal-" + id,
			Plan:      "starter",
			CreatedAt: account.CreatedAt,
			UpdatedAt: account.UpdatedAt,
		}
		return tx.CreateOrganization(ctx, &org, account.ID)
	})
}

// User-scoped queries join current membership in the same SQL statement.
// They never grant a platform administrator an implicit membership bypass.
func (s *Store) userOrganizations(ctx context.Context, userID string) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.Organization{}).
		Joins("JOIN organization_members ON organization_members.organization_id = organizations.id").
		Where("organization_members.user_id = ? AND organization_members.role IN ?", userID, []domain.Role{domain.RoleOwner, domain.RoleAdmin, domain.RoleMember, domain.RoleViewer}).
		Where("organization_members.user_id <> ''")
}

func (s *Store) ListUserOrganizations(ctx context.Context, userID string) ([]domain.Organization, error) {
	orgs := []domain.Organization{}
	err := s.userOrganizations(ctx, userID).Select("organizations.*").Order("organizations.created_at ASC, organizations.id ASC").Find(&orgs).Error
	return orgs, err
}

func (s *Store) GetUserOrganization(ctx context.Context, userID, orgID string) (domain.Organization, error) {
	var org domain.Organization
	err := s.userOrganizations(ctx, userID).Select("organizations.*").Where("organizations.id = ?", orgID).Take(&org).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return org, ErrNotFound
	}
	return org, err
}
