package store

import (
	"context"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
)

var ErrCredentialsChanged = errors.New("credentials changed; authenticate again")

// CreateSessionForPassword serializes with password rotation on the account
// row. A login validated against an older hash cannot outlive a completed reset.
func (s *Store) CreateSessionForPassword(ctx context.Context, session *domain.Session, verifiedHash string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		result := tx.db.WithContext(ctx).Model(&domain.AdminAccount{}).Where("id = ? AND password_hash = ?", session.AccountID, verifiedHash).UpdateColumn("password_hash", gorm.Expr("password_hash"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCredentialsChanged
		}
		return tx.CreateSession(ctx, session)
	})
}

// RotatePassword updates only credential fields, revokes every old session and
// optionally issues the caller's replacement session, all in one transaction.
func (s *Store) RotatePassword(ctx context.Context, accountID, expectedHash, newHash string, replacement *domain.Session) error {
	if replacement != nil && replacement.AccountID != accountID {
		return errors.New("replacement session account mismatch")
	}
	return s.Transaction(ctx, func(tx *Store) error {
		result := tx.db.WithContext(ctx).Model(&domain.AdminAccount{}).Where("id = ? AND password_hash = ?", accountID, expectedHash).Updates(map[string]any{"password_hash": newHash, "updated_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCredentialsChanged
		}
		if err := tx.db.WithContext(ctx).Where("account_id = ?", accountID).Delete(&domain.Session{}).Error; err != nil {
			return err
		}
		if replacement != nil {
			return tx.CreateSession(ctx, replacement)
		}
		return nil
	})
}

// UpdateAccountRole must not persist a previously read password hash.
func (s *Store) UpdateAccountRole(ctx context.Context, id string, role domain.Role) error {
	result := s.db.WithContext(ctx).Model(&domain.AdminAccount{}).Where("id = ?", id).Updates(map[string]any{"role": role, "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrNotFound
	}
	return nil
}
