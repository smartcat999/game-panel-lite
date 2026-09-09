package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	summaries, err := s.ListUserOrganizationMemberships(ctx, userID)
	orgs := make([]domain.Organization, 0, len(summaries))
	for _, summary := range summaries {
		orgs = append(orgs, summary.Organization)
	}
	return orgs, err
}

// ListUserOrganizationMemberships returns workspace data together with the
// caller's role on each membership. It intentionally reads memberships and
// organizations separately and composes them in memory.
func (s *Store) ListUserOrganizationMemberships(ctx context.Context, userID string) ([]domain.OrganizationMembershipSummary, error) {
	summaries := []domain.OrganizationMembershipSummary{}
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var memberships []domain.OrganizationMember
		if err := tx.readableMemberships(ctx, userID).Find(&memberships).Error; err != nil {
			return err
		}
		ids := make([]string, 0, len(memberships))
		roles := make(map[string]domain.Role, len(memberships))
		for _, membership := range memberships {
			ids = append(ids, membership.OrganizationID)
			roles[membership.OrganizationID] = membership.Role
		}
		organizations := make(map[string]domain.Organization, len(ids))
		for start := 0; start < len(ids); start += idLookupBatchSize {
			var batch []domain.Organization
			if err := tx.db.WithContext(ctx).Where("id IN ?", ids[start:min(start+idLookupBatchSize, len(ids))]).Find(&batch).Error; err != nil {
				return err
			}
			for _, organization := range batch {
				organizations[organization.ID] = organization
			}
		}
		for _, id := range ids {
			organization, ok := organizations[id]
			if !ok {
				continue
			}
			summaries = append(summaries, domain.OrganizationMembershipSummary{
				Organization:   organization,
				MembershipRole: roles[id],
			})
		}
		return nil
	})
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].CreatedAt.Equal(summaries[j].CreatedAt) {
			return summaries[i].ID < summaries[j].ID
		}
		return summaries[i].CreatedAt.Before(summaries[j].CreatedAt)
	})
	return summaries, err
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
	return s.whereIDs(query, "organization_id", ids)
}

// The column is an SQL identifier, not an expression supplied by a client.
func (s *Store) whereIDs(query *gorm.DB, column string, ids []string) *gorm.DB {
	identifier := clause.Column{Name: column}
	if len(ids) > idLookupBatchSize {
		// A materialized ID set is a single parameter, without querying another
		// table. Keeping one resource query preserves global ordering/limits.
		encoded, err := json.Marshal(ids)
		if err != nil {
			query.AddError(err)
			return query
		}
		if s.db.Dialector.Name() == "postgres" {
			return query.Where("? IN (SELECT jsonb_array_elements_text(?::jsonb))", identifier, string(encoded))
		}
		return query.Where("? IN (SELECT value FROM json_each(?))", identifier, string(encoded))
	}
	return query.Where("? IN ?", identifier, ids)
}
