// Package quota owns tenant quota accounts, allocations, and immutable resource
// operations. All inventory mutations commit within this domain.
package quota

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB must receive a PRIMARY pool. Correctness reads must never use a
// replica/cache. Transactions are private, short and restricted to quota tables.
type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func limitPtr(l corev1.Limit) *int64 {
	if l.Unlimited {
		return nil
	}
	v := l.Value
	return &v
}

func (s *DB) GetAccount(ctx context.Context, id string) (Account, error) {
	return getAccountRow(s.orm.WithContext(ctx), id)
}

func (s *DB) OpenAllocation(ctx context.Context, intent pgutil.AllocationIntent) (pgutil.Holding, error) {
	hash, err := intent.Fingerprint()
	if err != nil {
		return pgutil.Holding{}, err
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return pgutil.Holding{}, pgutil.Error(tx.Error, "")
	}
	defer tx.Rollback()
	acct := QuotaAccountRecord{TenantUID: intent.Owner.TenantUID}
	if err := tx.Select("tenant_uid").Clauses(clause.OnConflict{DoNothing: true}).Create(&acct).Error; err != nil {
		return pgutil.Holding{}, pgutil.Error(err, "")
	}
	raw, err := json.Marshal(intent)
	if err != nil {
		return pgutil.Holding{}, err
	}
	alloc := QuotaAllocationRecord{
		AllocationID: intent.AllocationID, TenantUID: intent.Owner.TenantUID,
		InstanceUID: intent.Owner.InstanceUID, InstanceName: intent.Owner.InstanceName,
		InitialIntent: raw, IntentHash: hash, State: corev1.AllocationPending, Version: 1,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&alloc).Error; err != nil {
		return pgutil.Holding{}, pgutil.Error(err, "")
	}
	h, err := getHolding(tx, intent.AllocationID)
	if err != nil {
		return h, err
	}
	if err = matchIntent(h, intent, hash); err != nil {
		return h, err
	}
	return h, pgutil.CommitGORM(ctx, tx, "")
}

func (s *DB) RegisterCommand(ctx context.Context, c pgutil.Command) (Operation, error) {
	if err := validateCommand(c, ""); err != nil {
		return Operation{}, err
	}
	if !funding(c.Kind) {
		return Operation{}, failure(corev1.IdentityConflict, c.OperationID, "closing uses explicit close methods", nil)
	}
	if existing, err := s.GetOperation(ctx, c.OperationID); err == nil {
		return existing, pgutil.MatchCommand(existing.Command, c)
	} else if !errors.Is(err, ErrNotFound) {
		return Operation{}, err
	}
	h, err := s.GetAllocation(ctx, c.AllocationID)
	if err != nil {
		return Operation{}, err
	}
	if h.AccountID == c.AccountID && h.Allocation.Owner == c.Owner && (c.Kind == corev1.Increase || c.Kind == corev1.SettleDecrease) {
		cancelled, err := hasCancelledChange(s.orm.WithContext(ctx), c.AllocationID, c.ChangePlanID)
		if err != nil {
			return Operation{}, err
		}
		if cancelled {
			return Operation{}, failure(corev1.Closed, c.OperationID, "resource change has been cancelled", nil)
		}
	}
	if err := checkRegistration(h, c); err != nil {
		return Operation{}, err
	}
	if c.Kind == corev1.Increase || c.Kind == corev1.SettleDecrease {
		plan, err := getChange(s.orm.WithContext(ctx), c.ChangePlanID)
		if err != nil {
			return Operation{}, err
		}
		if err := checkChangeCommand(h, c, plan); err != nil {
			return Operation{}, err
		}
	}
	return insertOperation(s.orm.WithContext(ctx), c, true)
}

func insertOperation(tx *gorm.DB, c pgutil.Command, autocommit bool) (Operation, error) {
	hash, err := c.Fingerprint()
	if err != nil {
		return Operation{}, err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return Operation{}, err
	}
	rec := QuotaOperationRecord{
		OperationID: c.OperationID, AllocationID: c.AllocationID, Kind: c.Kind,
		ChangePlanID: c.ChangePlanID, CloseIntentID: c.CloseIntentID,
		RequestHash: hash, Command: raw, State: corev1.Accepted,
	}
	err = tx.Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "operation_id"}},
			DoUpdates: clause.Assignments(map[string]any{"operation_id": gorm.Expr("quota_operations.operation_id")}),
		},
		clause.Returning{},
	).Create(&rec).Error
	if err != nil {
		if autocommit {
			return Operation{}, pgutil.WriteError(err, c.OperationID)
		}
		return Operation{}, pgutil.Error(err, c.OperationID)
	}
	op, err := toOperation(rec)
	if err != nil {
		return op, err
	}
	return op, pgutil.MatchCommand(op.Command, c)
}

func (s *DB) TryGrant(ctx context.Context, id string) (pgutil.MutationResult, error) {
	return s.execute(ctx, id, corev1.Grant)
}
func (s *DB) TryIncrease(ctx context.Context, id string) (pgutil.MutationResult, error) {
	return s.execute(ctx, id, corev1.Increase)
}
func (s *DB) SettleDecrease(ctx context.Context, id string) (pgutil.MutationResult, error) {
	return s.execute(ctx, id, corev1.SettleDecrease)
}

func (s *DB) execute(ctx context.Context, id string, kind corev1.Kind) (pgutil.MutationResult, error) {
	op, err := s.GetOperation(ctx, id)
	if err != nil {
		return pgutil.MutationResult{}, err
	}
	if err := validateCommand(op.Command, kind); err != nil {
		return pgutil.MutationResult{}, err
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return pgutil.MutationResult{}, pgutil.Error(tx.Error, id)
	}
	defer tx.Rollback()
	result, err := mutate(tx, op)
	if err != nil {
		if isClosed(err) && result.State == "" {
			_ = tx.Rollback()
			cancelErr := s.orm.WithContext(ctx).Model(&QuotaOperationRecord{}).
				Where("operation_id = ? AND state = ?", id, corev1.Accepted).
				Updates(map[string]any{"state": corev1.Cancelled, "updated_at": gorm.Expr("NOW()")}).Error
			h, readErr := s.GetAllocation(ctx, op.Command.AllocationID)
			if cancelErr == nil && readErr == nil {
				return pgutil.MutationResult{OperationID: id, State: corev1.Cancelled, Allocation: h.Allocation}, err
			}
		}
		return result, err
	}
	if result.Replayed {
		return result, nil
	}
	return commitMutation(ctx, tx, result)
}

func (s *DB) RequestClose(ctx context.Context, c pgutil.Command) (pgutil.MutationResult, error) {
	return s.close(ctx, c, corev1.RequestClose)
}
func (s *DB) FinalizeClose(ctx context.Context, c pgutil.Command) (pgutil.MutationResult, error) {
	return s.close(ctx, c, corev1.FinalizeClose)
}

// CancelChange atomically cancels an unapproved resize plan and refunds any granted growth.
func (s *DB) CancelChange(ctx context.Context, c pgutil.Command) (pgutil.MutationResult, error) {
	if err := validateCommand(c, corev1.CancelChange); err != nil {
		return pgutil.MutationResult{}, err
	}
	if err := checkContext(ctx, c.OperationID); err != nil {
		return pgutil.MutationResult{}, err
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return pgutil.MutationResult{}, pgutil.Error(tx.Error, c.OperationID)
	}
	defer tx.Rollback()
	result, err := mutateCancelQuota(tx, c)
	if err != nil || result.Replayed {
		return result, err
	}
	return commitMutation(ctx, tx, result)
}

func mutateCancelQuota(tx *gorm.DB, c pgutil.Command) (pgutil.MutationResult, error) {
	touch := tx.Model(&QuotaAllocationRecord{}).Where("allocation_id = ?", c.AllocationID).UpdateColumn("updated_at", gorm.Expr("updated_at"))
	if touch.Error != nil {
		return pgutil.MutationResult{}, pgutil.Error(touch.Error, c.OperationID)
	}
	if touch.RowsAffected == 0 {
		return pgutil.MutationResult{}, notFound("allocation", c.AllocationID)
	}
	h, err := getHolding(tx, c.AllocationID)
	if err != nil {
		return pgutil.MutationResult{}, err
	}
	if h.AccountID != c.AccountID || h.Allocation.Owner != c.Owner {
		return pgutil.MutationResult{}, failure(corev1.IdentityConflict, c.OperationID, "command owner or account does not match allocation", nil)
	}
	op, err := insertOperation(tx, c, false)
	if err != nil {
		return pgutil.MutationResult{}, err
	}
	claim := tx.Model(&QuotaOperationRecord{}).Where("operation_id = ? AND state = ?", c.OperationID, corev1.Accepted).
		Updates(map[string]any{"state": corev1.Committed, "updated_at": gorm.Expr("NOW()")})
	if claim.Error != nil {
		return pgutil.MutationResult{}, pgutil.Error(claim.Error, c.OperationID)
	}
	if claim.RowsAffected == 0 {
		current, err := getOperation(tx, c.OperationID)
		if err != nil {
			return pgutil.MutationResult{}, err
		}
		return replay(current, h)
	}
	if h.Allocation.State == corev1.AllocationPending {
		return pgutil.MutationResult{}, failure(corev1.RevisionChanged, c.OperationID, "cancel change requires an Active or Closing allocation", nil)
	}
	return applyCancelQuota(tx, h, op.Command)
}

func applyCancelQuota(tx *gorm.DB, h pgutil.Holding, c pgutil.Command) (pgutil.MutationResult, error) {
	if err := tx.Model(&QuotaOperationRecord{}).
		Where("allocation_id = ? AND change_plan_id = ? AND kind IN ? AND state = ?", c.AllocationID, c.ChangePlanID, []corev1.Kind{corev1.Increase, corev1.SettleDecrease}, corev1.Accepted).
		Updates(map[string]any{"state": corev1.Cancelled, "updated_at": gorm.Expr("NOW()")}).Error; err != nil {
		return pgutil.MutationResult{}, pgutil.Error(err, c.OperationID)
	}
	proposal, err := cancelProposal(tx, h, c)
	if err != nil {
		return pgutil.MutationResult{}, err
	}
	if err := applyQuotaHoldingProposal(tx, h, c, proposal); err != nil {
		return pgutil.MutationResult{}, err
	}
	raw, err := json.Marshal(proposal.Holding.Allocation)
	if err != nil {
		return pgutil.MutationResult{}, err
	}
	if err := tx.Model(&QuotaOperationRecord{}).Where("operation_id = ?", c.OperationID).
		Updates(map[string]any{"result": raw, "updated_at": gorm.Expr("NOW()")}).Error; err != nil {
		return pgutil.MutationResult{}, pgutil.Error(err, c.OperationID)
	}
	return pgutil.MutationResult{OperationID: c.OperationID, State: corev1.Committed, Allocation: proposal.Holding.Allocation}, nil
}

func (s *DB) close(ctx context.Context, c pgutil.Command, kind corev1.Kind) (pgutil.MutationResult, error) {
	if err := validateCommand(c, kind); err != nil {
		return pgutil.MutationResult{}, err
	}
	if err := checkContext(ctx, c.OperationID); err != nil {
		return pgutil.MutationResult{}, err
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return pgutil.MutationResult{}, pgutil.Error(tx.Error, c.OperationID)
	}
	defer tx.Rollback()
	if c.Kind == corev1.RequestClose {
		acct := QuotaAccountRecord{TenantUID: c.AccountID}
		if err := tx.Select("tenant_uid").Clauses(clause.OnConflict{DoNothing: true}).Create(&acct).Error; err != nil {
			return pgutil.MutationResult{}, pgutil.Error(err, c.OperationID)
		}
		alloc := QuotaAllocationRecord{
			AllocationID: c.AllocationID, TenantUID: c.AccountID,
			InstanceUID: c.Owner.InstanceUID, InstanceName: c.Owner.InstanceName,
			State: corev1.AllocationClosing, CloseIntentID: c.CloseIntentID, Version: 1,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&alloc).Error; err != nil {
			return pgutil.MutationResult{}, pgutil.Error(err, c.OperationID)
		}
	} else if _, err := getHolding(tx, c.AllocationID); err != nil {
		return pgutil.MutationResult{}, err
	}
	op, err := insertOperation(tx, c, false)
	if err != nil {
		return pgutil.MutationResult{}, err
	}
	result, err := mutate(tx, op)
	if err != nil {
		return result, err
	}
	return commitMutation(ctx, tx, result)
}

func mutate(tx *gorm.DB, op Operation) (pgutil.MutationResult, error) {
	c := op.Command
	claim := tx.Model(&QuotaOperationRecord{}).
		Where("operation_id = ? AND state = ?", c.OperationID, corev1.Accepted).
		Updates(map[string]any{"state": corev1.Committed, "updated_at": gorm.Expr("NOW()")})
	if claim.Error != nil {
		return pgutil.MutationResult{}, pgutil.Error(claim.Error, c.OperationID)
	}
	if claim.RowsAffected == 0 {
		current, err := getOperation(tx, c.OperationID)
		if err != nil {
			return pgutil.MutationResult{}, err
		}
		h, err := getHolding(tx, c.AllocationID)
		if err != nil {
			return pgutil.MutationResult{}, err
		}
		return replay(current, h)
	}
	h, err := getHolding(tx, c.AllocationID)
	if err != nil {
		return pgutil.MutationResult{}, err
	}
	if c.Kind == corev1.Increase || c.Kind == corev1.SettleDecrease {
		plan, err := getChange(tx, c.ChangePlanID)
		if err != nil {
			return pgutil.MutationResult{}, err
		}
		if err := checkChangeExecution(h, c, plan); err != nil {
			return pgutil.MutationResult{}, err
		}
	}
	proposal, err := pgutil.PlanMutation(h, c)
	if err != nil {
		return pgutil.MutationResult{}, err
	}
	if err := applyQuotaHoldingProposal(tx, h, c, proposal); err != nil {
		return pgutil.MutationResult{}, err
	}
	if c.Kind == corev1.Increase || c.Kind == corev1.SettleDecrease {
		cancelled, err := hasCancelledChange(tx, c.AllocationID, c.ChangePlanID)
		if err != nil {
			return pgutil.MutationResult{}, err
		}
		if cancelled {
			return pgutil.MutationResult{}, failure(corev1.Closed, c.OperationID, "resource change has been cancelled", nil)
		}
	}
	raw, err := json.Marshal(proposal.Holding.Allocation)
	if err != nil {
		return pgutil.MutationResult{}, err
	}
	if err := tx.Model(&QuotaOperationRecord{}).Where("operation_id = ?", c.OperationID).
		Updates(map[string]any{"result": raw, "updated_at": gorm.Expr("NOW()")}).Error; err != nil {
		return pgutil.MutationResult{}, pgutil.Error(err, c.OperationID)
	}
	return pgutil.MutationResult{OperationID: c.OperationID, State: corev1.Committed, Allocation: proposal.Holding.Allocation}, nil
}

func applyQuotaHoldingProposal(tx *gorm.DB, h pgutil.Holding, c pgutil.Command, proposal pgutil.Mutation) error {
	if proposal.Changed {
		res := updateHoldingGORM(tx, h.Allocation.Version, proposal.Holding)
		if res.Error != nil {
			return pgutil.Error(res.Error, c.OperationID)
		}
		if res.RowsAffected != 1 {
			return failure(corev1.RevisionChanged, c.OperationID, "allocation changed before commit", nil)
		}
	}
	if !proposal.Increase.IsZero() {
		res := increaseAccountGORM(tx, h.AccountID, proposal.Increase)
		if res.Error != nil {
			return pgutil.Error(res.Error, c.OperationID)
		}
		if res.RowsAffected != 1 {
			account, err := getAccountRow(tx, h.AccountID)
			if err != nil {
				return failure(corev1.InvariantViolation, c.OperationID, "allocation has no account", err)
			}
			if account.PolicyVersion == 0 {
				return failure(corev1.AdmissionDenied, c.OperationID, "quota account policy is not initialized", nil)
			}
			if account.ExpiresAt != nil && !account.ExpiresAt.After(time.Now()) {
				return failure(corev1.AdmissionDenied, c.OperationID, "tenant quota subscription has expired", nil)
			}
			if _, err := account.Allocated.Add(proposal.Increase); err != nil {
				return err
			}
			return failure(corev1.AdmissionDenied, c.OperationID, "current quota cannot admit this increase", nil)
		}
	}
	if !proposal.Decrease.IsZero() {
		res := decreaseAccountGORM(tx, h.AccountID, proposal.Decrease)
		if res.Error != nil {
			return pgutil.Error(res.Error, c.OperationID)
		}
		if res.RowsAffected != 1 {
			return failure(corev1.InvariantViolation, c.OperationID, "account cannot refund the recorded holding", nil)
		}
	}
	return nil
}

func commitMutation(ctx context.Context, tx *gorm.DB, result pgutil.MutationResult) (pgutil.MutationResult, error) {
	if err := pgutil.CommitGORM(ctx, tx, result.OperationID); err != nil {
		return pgutil.MutationResult{OperationID: result.OperationID}, err
	}
	return result, nil
}

func getHolding(tx *gorm.DB, id string) (pgutil.Holding, error) {
	var rec QuotaAllocationRecord
	if err := tx.Where("allocation_id = ?", id).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pgutil.Holding{}, notFound("allocation", id)
		}
		return pgutil.Holding{}, pgutil.Error(err, "")
	}
	return toHolding(rec)
}

func getOperation(tx *gorm.DB, id string) (Operation, error) {
	var rec QuotaOperationRecord
	if err := tx.Where("operation_id = ?", id).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Operation{}, notFound("operation", id)
		}
		return Operation{}, pgutil.Error(err, "")
	}
	return toOperation(rec)
}

func (s *DB) GetAllocation(ctx context.Context, id string) (pgutil.Holding, error) {
	return getHolding(s.orm.WithContext(ctx), id)
}

func (s *DB) GetOperation(ctx context.Context, id string) (Operation, error) {
	return getOperation(s.orm.WithContext(ctx), id)
}
