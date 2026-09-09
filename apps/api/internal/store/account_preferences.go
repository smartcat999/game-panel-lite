package store

import (
	"context"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func DefaultAccountPreferences(accountID string) domain.AccountPreferences {
	return domain.AccountPreferences{AccountID: accountID, Locale: "zh", Theme: "system"}
}

func (s *Store) GetAccountPreferences(ctx context.Context, accountID string) (domain.AccountPreferences, error) {
	preferences := DefaultAccountPreferences(accountID)
	err := s.db.WithContext(ctx).First(&preferences, "account_id = ?", accountID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return preferences, nil
	}
	return preferences, err
}

func (s *Store) SaveAccountPreferences(ctx context.Context, preferences domain.AccountPreferences) error {
	now := time.Now().UTC()
	preferences.UpdatedAt = now
	if preferences.CreatedAt.IsZero() {
		preferences.CreatedAt = now
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "account_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"locale":     preferences.Locale,
			"theme":      preferences.Theme,
			"updated_at": now,
		}),
	}).Create(&preferences).Error
}
