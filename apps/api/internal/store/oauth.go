package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

// FindOAuthIdentity retrieves an OAuth identity by provider and providerUserID.
func (s *Store) FindOAuthIdentity(ctx context.Context, provider, providerUserID string) (*domain.OAuthIdentity, error) {
	var identity domain.OAuthIdentity
	err := s.db.WithContext(ctx).Where("provider = ? AND provider_user_id = ?", provider, providerUserID).Take(&identity).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &identity, nil
}

// FindOAuthIdentityByUser retrieves all OAuth identities linked to a specific user ID.
func (s *Store) FindOAuthIdentityByUser(ctx context.Context, userID string) ([]domain.OAuthIdentity, error) {
	var list []domain.OAuthIdentity
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&list).Error
	return list, err
}

// CreateUserWithOAuth creates a new AdminAccount with a personal organization, gives 100 starter credits, and links the OAuth identity.
func (s *Store) CreateUserWithOAuth(ctx context.Context, identity *domain.OAuthIdentity, preferredUsername string) (*domain.AdminAccount, *domain.Organization, error) {
	var createdAccount *domain.AdminAccount
	var createdOrg *domain.Organization

	err := s.Transaction(ctx, func(tx *Store) error {
		now := time.Now().UTC()
		username := preferredUsername
		if username == "" {
			suffixLen := 6
			if len(identity.ProviderUserID) < suffixLen {
				suffixLen = len(identity.ProviderUserID)
			}
			username = fmt.Sprintf("%s_user_%s", identity.Provider, identity.ProviderUserID[:suffixLen])
		}

		// Check if username collision exists; if so, append random suffix
		var count int64
		_ = tx.db.WithContext(ctx).Model(&domain.AdminAccount{}).Where("username = ?", username).Count(&count).Error
		if count > 0 {
			username = fmt.Sprintf("%s_%s", username, uuid.NewString()[:4])
		}

		account := domain.AdminAccount{
			ID:           uuid.NewString(),
			Username:     username,
			PasswordHash: "oauth:" + identity.Provider + ":" + uuid.NewString(), // Not usable for password login
			Role:         domain.RoleMember,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		if err := tx.CreateAdminAccount(ctx, &account); err != nil {
			return err
		}

		identity.ID = "oauth-" + uuid.NewString()[:8]
		identity.UserID = account.ID
		identity.CreatedAt = now
		if err := tx.db.WithContext(ctx).Create(identity).Error; err != nil {
			return err
		}

		orgID := uuid.NewString()
		org := domain.Organization{
			ID:        orgID,
			Name:      account.Username + "'s workspace",
			Slug:      "personal-" + orgID,
			Plan:      "starter",
			Credits:   100, // 100 starter credits
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := tx.CreateOrganization(ctx, &org, account.ID); err != nil {
			return err
		}

		// Record initial welcome credits transaction
		welcomeTx := domain.CreditTransaction{
			ID:             "ctx-" + uuid.NewString()[:8],
			OrganizationID: orgID,
			Amount:         100,
			BalanceAfter:   100,
			Type:           "welcome_bonus",
			Description:    "新用户快捷注册赠送体验算力点",
			CreatedBy:      "system",
			CreatedAt:      now,
		}
		_ = tx.db.WithContext(ctx).Create(&welcomeTx)

		createdAccount = &account
		createdOrg = &org
		return nil
	})

	return createdAccount, createdOrg, err
}
