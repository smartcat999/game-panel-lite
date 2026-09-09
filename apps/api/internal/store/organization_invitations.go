package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

var (
	ErrInvitationRevoked = errors.New("invitation has been revoked")
	ErrInvitationExpired = errors.New("invitation has expired")
	ErrInvitationMaxUses = errors.New("invitation has reached its maximum uses")
)

func generateInvitationToken() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "inv_" + uuid.NewString()[:16]
	}
	return "inv_" + hex.EncodeToString(bytes)
}

func (s *Store) CreateOrganizationInvitation(ctx context.Context, invite *domain.OrganizationInvitation) error {
	if invite.ID == "" {
		invite.ID = uuid.NewString()
	}
	if invite.Token == "" {
		invite.Token = generateInvitationToken()
	}
	if invite.Role == "" {
		invite.Role = domain.RoleMember
	}
	if invite.ExpiresAt.IsZero() {
		invite.ExpiresAt = time.Now().UTC().Add(7 * 24 * time.Hour)
	}
	invite.CreatedAt = time.Now().UTC()
	return s.db.WithContext(ctx).Create(invite).Error
}

func (s *Store) GetOrganizationInvitationByToken(ctx context.Context, token string) (domain.OrganizationInvitation, error) {
	var invite domain.OrganizationInvitation
	err := s.db.WithContext(ctx).Where("token = ?", token).First(&invite).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.OrganizationInvitation{}, ErrNotFound
	}
	return invite, err
}

func (s *Store) ListOrganizationInvitations(ctx context.Context, orgID string) ([]domain.OrganizationInvitation, error) {
	var list []domain.OrganizationInvitation
	err := s.db.WithContext(ctx).
		Where("organization_id = ?", orgID).
		Order("created_at desc").
		Find(&list).Error
	return list, err
}

func (s *Store) RevokeOrganizationInvitation(ctx context.Context, orgID, inviteID string) error {
	result := s.db.WithContext(ctx).
		Model(&domain.OrganizationInvitation{}).
		Where("organization_id = ? AND id = ?", orgID, inviteID).
		Update("revoked", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AcceptOrganizationInvitation(ctx context.Context, token, userID string) (domain.OrganizationMember, error) {
	var member domain.OrganizationMember
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invite domain.OrganizationInvitation
		if err := tx.Where("token = ?", token).First(&invite).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		if invite.Revoked {
			return ErrInvitationRevoked
		}
		if !invite.ExpiresAt.IsZero() && invite.ExpiresAt.Before(time.Now().UTC()) {
			return ErrInvitationExpired
		}
		if invite.MaxUses > 0 && invite.UsedCount >= invite.MaxUses {
			return ErrInvitationMaxUses
		}

		// Check if user is already a member
		var existing domain.OrganizationMember
		if err := tx.Where("organization_id = ? AND user_id = ?", invite.OrganizationID, userID).First(&existing).Error; err == nil {
			member = existing
			return nil
		}

		// Increment used count
		if err := tx.Model(&domain.OrganizationInvitation{}).
			Where("id = ?", invite.ID).
			UpdateColumn("used_count", gorm.Expr("used_count + 1")).Error; err != nil {
			return err
		}

		member = domain.OrganizationMember{
			ID:             uuid.NewString(),
			OrganizationID: invite.OrganizationID,
			UserID:         userID,
			Role:           invite.Role,
			CreatedAt:      time.Now().UTC(),
		}
		return tx.Create(&member).Error
	})
	return member, err
}

func (s *Store) GetOrganizationInvitationSummary(ctx context.Context, token string) (domain.InvitationSummary, error) {
	invite, err := s.GetOrganizationInvitationByToken(ctx, token)
	if err != nil {
		return domain.InvitationSummary{}, err
	}

	summary := domain.InvitationSummary{
		Token:          invite.Token,
		OrganizationID: invite.OrganizationID,
		Role:           invite.Role,
		ExpiresAt:      invite.ExpiresAt,
		IsExpired:      invite.Revoked || (!invite.ExpiresAt.IsZero() && invite.ExpiresAt.Before(time.Now().UTC())) || (invite.MaxUses > 0 && invite.UsedCount >= invite.MaxUses),
	}

	org, err := s.GetOrganization(ctx, invite.OrganizationID)
	if err == nil {
		summary.OrganizationName = org.Name
	}
	if summary.OrganizationName == "" {
		summary.OrganizationName = invite.OrganizationID
	}

	inviter, err := s.GetAdminAccount(ctx, invite.InviterUserID)
	if err == nil {
		summary.InviterName = inviter.Username
	}

	return summary, nil
}
