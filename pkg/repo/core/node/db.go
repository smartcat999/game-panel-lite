// Package node owns node capacity accounts, reservations, and host-port claims.
// All inventory mutations commit within this domain.
package node

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB must receive a PRIMARY pool. Correctness reads must never use a
// replica/cache. Transactions are private, short and restricted to node tables.
type DB struct {
	orm     *gorm.DB
	options Options
}

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	s, err := NewDBWithOptions(pool, defaultOptions())
	if err != nil {
		panic(err)
	}
	return s
}

func NewDBWithOptions(pool *pgxpool.Pool, options Options) (*DB, error) {
	if pool == nil {
		return nil, &corev1.ValidationError{Field: "pool", Reason: corev1.ValidationReasonPrimaryPoolRequired}
	}
	if err := options.validate(); err != nil {
		return nil, err
	}
	return &DB{orm: db.OpenGORM(pool), options: options}, nil
}

func limitPtr(l corev1.Limit) *int64 {
	if l.Unlimited {
		return nil
	}
	v := l.Value
	return &v
}

func (s *DB) SyncAccount(ctx context.Context, id string, policyVersion int64, limits corev1.Limits) (Account, error) {
	if err := checkContext(ctx, ""); err != nil {
		return Account{}, err
	}
	if policyVersion < 1 {
		return Account{}, &corev1.ValidationError{Field: "policyVersion", Reason: corev1.ValidationReasonMustBePositive}
	}
	if err := corev1.ValidateIdentifier("nodeUid", id); err != nil {
		return Account{}, err
	}
	if err := validateLimits(limits); err != nil {
		return Account{}, err
	}
	rec := NodeAccountRecord{
		NodeUID: id, PolicyVersion: policyVersion,
		LimitCPU: limitPtr(limits.CPU), LimitMem: limitPtr(limits.Memory), LimitStorage: limitPtr(limits.Storage),
		LimitInstances: limitPtr(limits.Instances), LimitBackups: limitPtr(limits.Backups), LimitBackupStorage: limitPtr(limits.BackupStorage),
	}
	res := s.orm.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns: []clause.Column{{Name: "node_uid"}},
			Where:   clause.Where{Exprs: []clause.Expression{gorm.Expr("node_accounts.policy_version < EXCLUDED.policy_version")}},
			DoUpdates: clause.Assignments(map[string]any{
				"policy_version":       gorm.Expr("EXCLUDED.policy_version"),
				"limit_cpu":            gorm.Expr("EXCLUDED.limit_cpu"),
				"limit_mem":            gorm.Expr("EXCLUDED.limit_mem"),
				"limit_storage":        gorm.Expr("EXCLUDED.limit_storage"),
				"limit_instances":      gorm.Expr("EXCLUDED.limit_instances"),
				"limit_backups":        gorm.Expr("EXCLUDED.limit_backups"),
				"limit_backup_storage": gorm.Expr("EXCLUDED.limit_backup_storage"),
				"updated_at":           gorm.Expr("NOW()"),
			}),
		},
		clause.Returning{},
	).Create(&rec)
	if res.Error != nil {
		return Account{}, pgutil.WriteError(res.Error, "")
	}
	if res.RowsAffected == 0 {
		a, err := s.GetAccount(ctx, id)
		if err != nil {
			return a, err
		}
		if a.PolicyVersion > policyVersion {
			return a, failure(corev1.RevisionChanged, "", "capacity policy version is stale", nil)
		}
		if a.Limits != limits {
			return a, failure(corev1.IdentityConflict, "", "same capacity version changed limits", nil)
		}
		return a, nil
	}
	return toAccount(rec)
}

func (s *DB) GetAccount(ctx context.Context, id string) (Account, error) {
	if err := corev1.ValidateIdentifier("nodeUid", id); err != nil {
		return Account{}, err
	}
	return getAccountRow(s.orm.WithContext(ctx), id)
}

func (s *DB) OpenReservation(ctx context.Context, intent pgutil.PlacementIntent) (Reservation, error) {
	intent, err := intent.Canonical()
	if err != nil {
		return Reservation{}, err
	}
	hash, err := intent.Fingerprint()
	if err != nil {
		return Reservation{}, err
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return Reservation{}, pgutil.Error(tx.Error, "")
	}
	defer tx.Rollback()
	acct := NodeAccountRecord{NodeUID: intent.Binding.NodeUID}
	if err := tx.Select("node_uid").Clauses(clause.OnConflict{DoNothing: true}).Create(&acct).Error; err != nil {
		return Reservation{}, pgutil.Error(err, "")
	}
	raw, err := json.Marshal(intent)
	if err != nil {
		return Reservation{}, err
	}
	resv := NodeReservationRecord{
		AllocationID: intent.Binding.AttemptID, NodeUID: intent.Binding.NodeUID,
		TenantUID: intent.Owner.TenantUID, InstanceUID: intent.Owner.InstanceUID,
		InstanceName: intent.Owner.InstanceName, InitialIntent: raw,
		IntentHash: hash, AssignedPorts: []byte("[]"), State: corev1.AllocationPending, Version: 1,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&resv).Error; err != nil {
		return Reservation{}, pgutil.Error(err, "")
	}
	h, err := getHolding(tx, intent.Binding.AttemptID)
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
	h, err := s.GetReservation(ctx, c.AllocationID)
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
	rec := NodeOperationRecord{
		OperationID: c.OperationID, AllocationID: c.AllocationID, Kind: c.Kind,
		ChangePlanID: c.ChangePlanID, CloseIntentID: c.CloseIntentID,
		RequestHash: hash, Command: raw, State: corev1.Accepted,
	}
	err = tx.Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "operation_id"}},
			DoUpdates: clause.Assignments(map[string]any{"operation_id": gorm.Expr("node_operations.operation_id")}),
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

func (s *DB) TryReserve(ctx context.Context, id string) (Result, error) {
	return s.execute(ctx, id, corev1.Grant)
}
func (s *DB) TryIncrease(ctx context.Context, id string) (Result, error) {
	return s.execute(ctx, id, corev1.Increase)
}
func (s *DB) SettleDecrease(ctx context.Context, id string) (Result, error) {
	return s.execute(ctx, id, corev1.SettleDecrease)
}

func (s *DB) execute(ctx context.Context, id string, kind corev1.Kind) (Result, error) {
	op, err := s.GetOperation(ctx, id)
	if err != nil {
		return Result{}, err
	}
	if err := validateCommand(op.Command, kind); err != nil {
		return Result{}, err
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return Result{}, pgutil.Error(tx.Error, id)
	}
	defer tx.Rollback()
	result, err := s.mutate(tx, op)
	if err != nil {
		if isClosed(err) && result.State == "" {
			_ = tx.Rollback()
			cancelErr := s.orm.WithContext(ctx).Model(&NodeOperationRecord{}).
				Where("operation_id = ? AND state = ?", id, corev1.Accepted).
				Updates(map[string]any{"state": corev1.Cancelled, "updated_at": gorm.Expr("NOW()")}).Error
			h, readErr := s.GetReservation(ctx, op.Command.AllocationID)
			if cancelErr == nil && readErr == nil {
				op.State = corev1.Cancelled
				return currentResult(op, h, false), err
			}
		}
		return result, err
	}
	if result.Replayed {
		return result, nil
	}
	return commitResult(ctx, tx, result)
}

func (s *DB) RequestClose(ctx context.Context, c pgutil.Command) (Result, error) {
	return s.close(ctx, c, corev1.RequestClose)
}
func (s *DB) FinalizeClose(ctx context.Context, c pgutil.Command) (Result, error) {
	return s.close(ctx, c, corev1.FinalizeClose)
}

// CancelChange atomically cancels an unapproved resize plan on a node reservation.
func (s *DB) CancelChange(ctx context.Context, c pgutil.Command) (Result, error) {
	if err := validateCommand(c, corev1.CancelChange); err != nil {
		return Result{}, err
	}
	if err := checkContext(ctx, c.OperationID); err != nil {
		return Result{}, err
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return Result{}, pgutil.Error(tx.Error, c.OperationID)
	}
	defer tx.Rollback()
	result, err := s.mutateCancelNode(tx, c)
	if err != nil || result.Replayed {
		return result, err
	}
	return commitResult(ctx, tx, result)
}

func (s *DB) mutateCancelNode(tx *gorm.DB, c pgutil.Command) (Result, error) {
	touch := tx.Model(&NodeReservationRecord{}).Where("allocation_id = ?", c.AllocationID).UpdateColumn("updated_at", gorm.Expr("updated_at"))
	if touch.Error != nil {
		return Result{}, pgutil.Error(touch.Error, c.OperationID)
	}
	if touch.RowsAffected == 0 {
		return Result{}, notFound("reservation", c.AllocationID)
	}
	h, err := getHolding(tx, c.AllocationID)
	if err != nil {
		return Result{}, err
	}
	if h.AccountID != c.AccountID || h.Allocation.Owner != c.Owner {
		return Result{}, failure(corev1.IdentityConflict, c.OperationID, "command owner or account does not match reservation", nil)
	}
	op, err := insertOperation(tx, c, false)
	if err != nil {
		return Result{}, err
	}
	claim := tx.Model(&NodeOperationRecord{}).Where("operation_id = ? AND state = ?", c.OperationID, corev1.Accepted).
		Updates(map[string]any{"state": corev1.Committed, "updated_at": gorm.Expr("NOW()")})
	if claim.Error != nil {
		return Result{}, pgutil.Error(claim.Error, c.OperationID)
	}
	if claim.RowsAffected == 0 {
		current, err := getOperation(tx, c.OperationID)
		if err != nil {
			return Result{}, err
		}
		return replay(current, h)
	}
	if h.Allocation.State == corev1.AllocationPending {
		return Result{}, failure(corev1.RevisionChanged, c.OperationID, "cancel change requires an Active or Closing reservation", nil)
	}
	return s.applyCancelNode(tx, h, op)
}

func (s *DB) applyCancelNode(tx *gorm.DB, h Reservation, op Operation) (Result, error) {
	c := op.Command
	if err := tx.Model(&NodeOperationRecord{}).
		Where("allocation_id = ? AND change_plan_id = ? AND kind IN ? AND state = ?", c.AllocationID, c.ChangePlanID, []corev1.Kind{corev1.Increase, corev1.SettleDecrease}, corev1.Accepted).
		Updates(map[string]any{"state": corev1.Cancelled, "updated_at": gorm.Expr("NOW()")}).Error; err != nil {
		return Result{}, pgutil.Error(err, c.OperationID)
	}
	proposal, err := cancelProposal(tx, h.Holding, c)
	if err != nil {
		return Result{}, err
	}
	if err := applyNodeHoldingProposal(tx, h, c, proposal); err != nil {
		return Result{}, err
	}
	return s.finalizeNodeMutation(tx, h, op, proposal)
}

func (s *DB) close(ctx context.Context, c pgutil.Command, kind corev1.Kind) (Result, error) {
	if err := validateCommand(c, kind); err != nil {
		return Result{}, err
	}
	if err := checkContext(ctx, c.OperationID); err != nil {
		return Result{}, err
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return Result{}, pgutil.Error(tx.Error, c.OperationID)
	}
	defer tx.Rollback()
	if c.Kind == corev1.RequestClose {
		acct := NodeAccountRecord{NodeUID: c.AccountID}
		if err := tx.Select("node_uid").Clauses(clause.OnConflict{DoNothing: true}).Create(&acct).Error; err != nil {
			return Result{}, pgutil.Error(err, c.OperationID)
		}
		resv := NodeReservationRecord{
			AllocationID: c.AllocationID, NodeUID: c.AccountID,
			TenantUID: c.Owner.TenantUID, InstanceUID: c.Owner.InstanceUID,
			InstanceName: c.Owner.InstanceName, AssignedPorts: []byte("[]"),
			State: corev1.AllocationClosing, CloseIntentID: c.CloseIntentID, Version: 1,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&resv).Error; err != nil {
			return Result{}, pgutil.Error(err, c.OperationID)
		}
	} else if _, err := getHolding(tx, c.AllocationID); err != nil {
		return Result{}, err
	}
	op, err := insertOperation(tx, c, false)
	if err != nil {
		return Result{}, err
	}
	result, err := s.mutate(tx, op)
	if err != nil {
		return result, err
	}
	return commitResult(ctx, tx, result)
}

func commitResult(ctx context.Context, tx *gorm.DB, result Result) (Result, error) {
	if err := pgutil.CommitGORM(ctx, tx, result.OperationID); err != nil {
		return Result{MutationResult: pgutil.MutationResult{OperationID: result.OperationID}}, err
	}
	return result, nil
}

func (s *DB) mutate(tx *gorm.DB, op Operation) (Result, error) {
	c := op.Command
	claim := tx.Model(&NodeOperationRecord{}).
		Where("operation_id = ? AND state = ?", c.OperationID, corev1.Accepted).
		Updates(map[string]any{"state": corev1.Committed, "updated_at": gorm.Expr("NOW()")})
	if claim.Error != nil {
		return Result{}, pgutil.Error(claim.Error, c.OperationID)
	}
	if claim.RowsAffected == 0 {
		current, err := getOperation(tx, c.OperationID)
		if err != nil {
			return Result{}, err
		}
		h, err := getHolding(tx, c.AllocationID)
		if err != nil {
			return Result{}, err
		}
		return replay(current, h)
	}
	h, err := getHolding(tx, c.AllocationID)
	if err != nil {
		return Result{}, err
	}
	if c.Kind == corev1.Increase || c.Kind == corev1.SettleDecrease {
		plan, err := getChange(tx, c.ChangePlanID)
		if err != nil {
			return Result{}, err
		}
		if err := checkChangeExecution(h, c, plan); err != nil {
			return Result{}, err
		}
	}
	proposal, err := pgutil.PlanMutation(h.Holding, c)
	if err != nil {
		return Result{}, err
	}
	if err := applyNodeHoldingProposal(tx, h, c, proposal); err != nil {
		return Result{}, err
	}
	if c.Kind == corev1.Increase || c.Kind == corev1.SettleDecrease {
		cancelled, err := hasCancelledChange(tx, c.AllocationID, c.ChangePlanID)
		if err != nil {
			return Result{}, err
		}
		if cancelled {
			return Result{}, failure(corev1.Closed, c.OperationID, "resource change has been cancelled", nil)
		}
	}
	return s.finalizeNodeMutation(tx, h, op, proposal)
}

func applyNodeHoldingProposal(tx *gorm.DB, h Reservation, c pgutil.Command, proposal pgutil.Mutation) error {
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
			if _, err := account.Allocated.Add(proposal.Increase); err != nil {
				return err
			}
			return failure(corev1.AdmissionDenied, c.OperationID, "current node capacity cannot admit this increase", nil)
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
	if proposal.Increase.IsZero() && proposal.Decrease.IsZero() {
		if err := tx.Model(&NodeAccountRecord{}).Where("node_uid = ?", h.AccountID).
			UpdateColumn("updated_at", gorm.Expr("updated_at")).Error; err != nil {
			return pgutil.Error(err, c.OperationID)
		}
	}
	return nil
}

func (s *DB) finalizeNodeMutation(tx *gorm.DB, h Reservation, op Operation, proposal pgutil.Mutation) (Result, error) {
	c := op.Command
	next := cloneReservation(h)
	next.Holding = proposal.Holding
	var err error
	if c.Kind == corev1.Grant {
		next.AssignedPorts, err = assignPorts(s.options, h.Allocation.ID, h.Ports, func(p corev1.PortRequest, port int32) (bool, error) {
			rec := NodePortRecord{NodeUID: h.AccountID, Protocol: p.Protocol, PublishedPort: port, AttemptID: h.Allocation.ID}
			res := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "node_uid"}, {Name: "protocol"}, {Name: "published_port"}},
				DoNothing: true,
			}).Create(&rec)
			return res.RowsAffected == 1, pgutil.Error(res.Error, c.OperationID)
		})
		if err != nil {
			return Result{}, err
		}
	}
	if c.Kind == corev1.FinalizeClose {
		if err := tx.Where("node_uid = ? AND attempt_id = ?", h.AccountID, h.Allocation.ID).
			Delete(&NodePortRecord{}).Error; err != nil {
			return Result{}, pgutil.Error(err, c.OperationID)
		}
		next.AssignedPorts = nil
	}
	if err := next.Validate(); err != nil {
		return Result{}, err
	}
	assigned, err := json.Marshal(next.AssignedPorts)
	if err != nil {
		return Result{}, err
	}
	if next.AssignedPorts == nil {
		assigned = []byte("[]")
	}
	if err := tx.Model(&NodeReservationRecord{}).Where("allocation_id = ?", h.Allocation.ID).
		UpdateColumn("assigned_ports", assigned).Error; err != nil {
		return Result{}, pgutil.Error(err, c.OperationID)
	}
	op.State = corev1.Committed
	out := currentResult(op, next, false)
	raw, err := json.Marshal(out)
	if err != nil {
		return Result{}, err
	}
	if err := tx.Model(&NodeOperationRecord{}).Where("operation_id = ?", c.OperationID).
		Updates(map[string]any{"result": raw, "updated_at": gorm.Expr("NOW()")}).Error; err != nil {
		return Result{}, pgutil.Error(err, c.OperationID)
	}
	return out, nil
}

func getHolding(tx *gorm.DB, id string) (Reservation, error) {
	var rec NodeReservationRecord
	if err := tx.Where("allocation_id = ?", id).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Reservation{}, notFound("allocation", id)
		}
		return Reservation{}, pgutil.Error(err, "")
	}
	return toHolding(rec)
}

func getOperation(tx *gorm.DB, id string) (Operation, error) {
	var rec NodeOperationRecord
	if err := tx.Where("operation_id = ?", id).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Operation{}, notFound("operation", id)
		}
		return Operation{}, pgutil.Error(err, id)
	}
	return toOperation(rec)
}

func (s *DB) GetReservation(ctx context.Context, id string) (Reservation, error) {
	if err := corev1.ValidateIdentifier("attemptId", id); err != nil {
		return Reservation{}, err
	}
	return getHolding(s.orm.WithContext(ctx), id)
}

func (s *DB) GetOperation(ctx context.Context, id string) (Operation, error) {
	if err := corev1.ValidateIdentifier("operationId", id); err != nil {
		return Operation{}, err
	}
	return getOperation(s.orm.WithContext(ctx), id)
}
