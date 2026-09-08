package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
)

func (w *EncryptedIntentWriter) ReplayCreate(ctx context.Context, actor string, r instances.CreateRequest, config []byte) (instances.IntentResult, bool, error) {
	if r.ValidateMetadata() != nil || r.Specification.Configuration.KeyID != "" || len(r.Specification.Configuration.Ciphertext) != 0 || len(config) == 0 {
		return instances.IntentResult{}, false, instances.ErrInvalidIntent
	}
	encoded, err := encodeProtectedCreate(r, config)
	if err != nil {
		return instances.IntentResult{}, false, err
	}
	defer clear(encoded)
	return w.replay(ctx, actor, r.OrganizationID, "create", r.IdempotencyKey, encoded)
}
func (w *EncryptedIntentWriter) ReplayRevise(ctx context.Context, actor string, r instances.ReviseRequest, config []byte) (instances.IntentResult, bool, error) {
	if r.ValidateMetadata() != nil || r.Specification.Configuration.KeyID != "" || len(r.Specification.Configuration.Ciphertext) != 0 || len(config) == 0 {
		return instances.IntentResult{}, false, instances.ErrInvalidIntent
	}
	encoded, err := encodeProtectedRevise(r, config)
	if err != nil {
		return instances.IntentResult{}, false, err
	}
	defer clear(encoded)
	return w.replay(ctx, actor, r.OrganizationID, "revise", r.IdempotencyKey, encoded)
}

// replay is an authorized lookup, never a grant for a later write. The write
// transaction still repeats authorization and idempotency under the same lock.
func (w *EncryptedIntentWriter) replay(ctx context.Context, actor, organization, kind, key string, encoded []byte) (instances.IntentResult, bool, error) {
	var result instances.IntentResult
	found := false
	err := w.store.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspaceWriter(ctx, organization, actor); err != nil {
			return err
		}
		var operation globalOperationRow
		err := tx.db.WithContext(ctx).Table("server_operations").Where("organization_id = ? AND kind = ? AND idempotency_key = ?", organization, kind, key).Take(&operation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		matched, err := w.fingerprinter.Matches(operation.RequestHash, encoded)
		if err != nil {
			return err
		}
		if !matched {
			return instances.ErrIdempotencyConflict
		}
		result, err = tx.readGlobalIntent(ctx, operation)
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	if err != nil {
		return instances.IntentResult{}, false, err
	}
	return result, found, nil
}
