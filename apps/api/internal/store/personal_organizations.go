package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

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

func (s *Store) ListUserOrganizations(ctx context.Context, userID string) ([]domain.Organization, error) {
	orgs := []domain.Organization{}
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var ids []string
		if err := tx.readableMemberships(ctx, userID).Pluck("organization_id", &ids).Error; err != nil {
			return err
		}
		for start := 0; start < len(ids); start += idLookupBatchSize {
			var batch []domain.Organization
			if err := tx.db.WithContext(ctx).Where("id IN ?", ids[start:min(start+idLookupBatchSize, len(ids))]).Find(&batch).Error; err != nil {
				return err
			}
			orgs = append(orgs, batch...)
		}
		return nil
	})
	sort.Slice(orgs, func(i, j int) bool {
		if orgs[i].CreatedAt.Equal(orgs[j].CreatedAt) {
			return orgs[i].ID < orgs[j].ID
		}
		return orgs[i].CreatedAt.Before(orgs[j].CreatedAt)
	})
	return orgs, err
}

func (s *Store) GetUserOrganization(ctx context.Context, userID, orgID string) (domain.Organization, error) {
	var org domain.Organization
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var membership domain.OrganizationMember
		if err := tx.readableMemberships(ctx, userID).Where("organization_id = ?", orgID).Take(&membership).Error; err != nil {
			return err
		}
		return tx.db.WithContext(ctx).Where("id = ?", orgID).Take(&org).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return org, ErrNotFound
	}
	return org, err
}

func (s *Store) readableMemberships(ctx context.Context, userID string) *gorm.DB {
	return s.db.WithContext(ctx).Model(&domain.OrganizationMember{}).
		Where("user_id = ? AND user_id <> ''", userID).
		Where("role IN ?", []domain.Role{domain.RoleOwner, domain.RoleAdmin, domain.RoleMember, domain.RoleViewer})
}

// Transitional owner filter for existing resource readers. Callers compose
// this with their resource predicates, never with a nested organization query.
func (s *Store) userOwnedQuery(ctx context.Context, userID string, model any) *gorm.DB {
	query := s.db.WithContext(ctx).Model(model)
	organizations, err := s.ListUserOrganizations(ctx, userID)
	if err != nil {
		query.AddError(err)
		return query
	}
	ids := make([]string, 0, len(organizations))
	for _, organization := range organizations {
		ids = append(ids, organization.ID)
	}
	if len(ids) > idLookupBatchSize {
		// A materialized ID set is a single parameter, without querying another
		// table. Keeping one resource query preserves global ordering/limits.
		encoded, err := json.Marshal(ids)
		if err != nil {
			query.AddError(err)
			return query
		}
		if s.db.Dialector.Name() == "postgres" {
			return query.Where("organization_id IN (SELECT jsonb_array_elements_text(?::jsonb))", string(encoded))
		}
		return query.Where("organization_id IN (SELECT value FROM json_each(?))", string(encoded))
	}
	return query.Where("organization_id IN ?", ids)
}
