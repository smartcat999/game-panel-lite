package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

var ErrInsufficientCredits = errors.New("insufficient credits in organization")
var ErrCreditOverflow = errors.New("credit balance exceeds supported range")

// TopUpCredits adds credits to an organization and records an audit transaction.
func (s *Store) TopUpCredits(ctx context.Context, orgID string, amount int64, description, operatorUserID string) (*domain.CreditTransaction, error) {
	if amount <= 0 {
		return nil, errors.New("top-up amount must be positive")
	}

	var txRecord *domain.CreditTransaction
	err := s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspace(ctx, orgID); err != nil {
			return err
		}
		var org domain.Organization
		if err := tx.db.WithContext(ctx).Where("id = ?", orgID).Take(&org).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		if org.Credits > math.MaxInt64-amount {
			return ErrCreditOverflow
		}
		newBalance := org.Credits + amount
		if err := tx.db.WithContext(ctx).Model(&domain.Organization{}).Where("id = ?", orgID).Update("credits", newBalance).Error; err != nil {
			return err
		}

		txRecord = &domain.CreditTransaction{
			ID:             "ctx-" + uuid.NewString(),
			OrganizationID: orgID,
			Amount:         amount,
			BalanceAfter:   newBalance,
			Type:           "topup",
			Description:    description,
			CreatedBy:      operatorUserID,
			CreatedAt:      time.Now().UTC(),
		}
		return tx.db.WithContext(ctx).Create(txRecord).Error
	})
	if err != nil {
		return nil, err
	}
	return txRecord, nil
}

// DeductCredits atomically verifies and deducts credits from an organization for server provisioning/renewal.
func (s *Store) DeductCredits(ctx context.Context, orgID string, amount int64, txType, description, operatorUserID string) (*domain.CreditTransaction, error) {
	if amount <= 0 {
		return nil, errors.New("deduct amount must be positive")
	}

	var txRecord *domain.CreditTransaction
	err := s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspace(ctx, orgID); err != nil {
			return err
		}
		var org domain.Organization
		if err := tx.db.WithContext(ctx).Where("id = ?", orgID).Take(&org).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		if org.Credits < amount {
			return fmt.Errorf("%w: required %d, available %d", ErrInsufficientCredits, amount, org.Credits)
		}

		newBalance := org.Credits - amount
		result := tx.db.WithContext(ctx).Model(&domain.Organization{}).Where("id = ? AND credits >= ?", orgID, amount).Update("credits", newBalance)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInsufficientCredits
		}

		if txType == "" {
			txType = "server_create"
		}

		txRecord = &domain.CreditTransaction{
			ID:             "ctx-" + uuid.NewString(),
			OrganizationID: orgID,
			Amount:         -amount,
			BalanceAfter:   newBalance,
			Type:           txType,
			Description:    description,
			CreatedBy:      operatorUserID,
			CreatedAt:      time.Now().UTC(),
		}
		return tx.db.WithContext(ctx).Create(txRecord).Error
	})
	if err != nil {
		return nil, err
	}
	return txRecord, nil
}

// GetOrganizationCredits returns the current credit balance of an organization.
func (s *Store) GetOrganizationCredits(ctx context.Context, orgID string) (int64, error) {
	var org domain.Organization
	if err := s.db.WithContext(ctx).Select("credits").Where("id = ?", orgID).Take(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return org.Credits, nil
}

// ListCreditTransactions returns recent transactions for an organization.
func (s *Store) ListCreditTransactions(ctx context.Context, orgID string, limit int) ([]domain.CreditTransaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var records []domain.CreditTransaction
	err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Order("created_at desc").Limit(limit).Find(&records).Error
	return records, err
}

// CreateChargedGameServer commits allocation, balance and ledger together. The
// actor authorizes workspace access; operator identifies the account for audit.
func (s *Store) CreateChargedGameServer(ctx context.Context, actor, operator string, instance *domain.GameServer, cost int64) error {
	return s.Transaction(ctx, func(tx *Store) error {
		if err := tx.CreateAllocatedGameServer(ctx, actor, instance); err != nil {
			return err
		}
		_, err := tx.DeductCredits(ctx, instance.OrganizationID, cost, "server_create", fmt.Sprintf("Create game server %s", instance.Name), operator)
		return err
	})
}
