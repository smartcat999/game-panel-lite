package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

var ErrNotFound = errors.New("quota record not found")

type Account struct {
	TenantUID       string
	PolicyVersion   int64
	UpstreamVersion int64
	ExpiresAt       *time.Time
	Limits          corev1.Limits
	Allocated       corev1.Resources
}

// Policy returns the account's normalized effective limits and expiration timestamp.
func (a Account) Policy() QuotaPolicy {
	return QuotaPolicy{Limits: a.Limits, ExpiresAt: a.ExpiresAt}.Normalize()
}

type Change struct {
	Plan          pgutil.ResourceChange
	SourceVersion int64
}

type Operation struct {
	Command   pgutil.Command
	State     corev1.OperationState
	Result    pgutil.Allocation
	HasResult bool
}

type (
	QuotaAccountRecord      = corev1.QuotaAccountRecord
	QuotaPolicyChangeRecord = corev1.QuotaPolicyChangeRecord
	QuotaAllocationRecord   = corev1.QuotaAllocationRecord
	QuotaOperationRecord    = corev1.QuotaOperationRecord
	QuotaChangeRecord       = corev1.QuotaChangeRecord
)

func failure(code corev1.Code, id, detail string, cause error) error {
	return &corev1.Error{Code: code, OperationID: id, Detail: detail, Cause: cause}
}

func validateCommand(c pgutil.Command, kind corev1.Kind) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Domain != corev1.QuotaDomain || kind != "" && c.Kind != kind {
		return failure(corev1.IdentityConflict, c.OperationID, "wrong domain or method for command", nil)
	}
	return nil
}

func funding(kind corev1.Kind) bool {
	return kind == corev1.Grant || kind == corev1.Increase || kind == corev1.SettleDecrease
}

func matchIntent(h pgutil.Holding, intent pgutil.AllocationIntent, hash string) error {
	if h.AccountID != intent.Owner.TenantUID || h.Allocation.Owner != intent.Owner {
		return failure(corev1.IdentityConflict, "", "allocation belongs to a different owner", nil)
	}
	if h.Allocation.State == corev1.AllocationClosing || h.Allocation.State == corev1.AllocationClosed {
		return failure(corev1.Closed, "", "allocation admission gate is closed", nil)
	}
	if h.IntentHash != hash {
		return failure(corev1.IdentityConflict, "", "initial allocation intent is immutable", nil)
	}
	return nil
}

func replay(op Operation, h pgutil.Holding) (pgutil.MutationResult, error) {
	result := pgutil.MutationResult{OperationID: op.Command.OperationID, State: op.State, Allocation: h.Allocation, Replayed: true}
	if op.State == corev1.Cancelled || funding(op.Command.Kind) && (h.Allocation.State == corev1.AllocationClosing || h.Allocation.State == corev1.AllocationClosed) {
		return result, failure(corev1.Closed, op.Command.OperationID, "historical result does not grant current entitlement", nil)
	}
	return result, nil
}

func notFound(kind, id string) error { return fmt.Errorf("%w: %s %s", ErrNotFound, kind, id) }

func isClosed(err error) bool { return errors.Is(err, corev1.Closed) }

func checkContext(ctx context.Context, op string) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return failure(corev1.OutcomeUnknown, op, "context ended before quota outcome was known", ctx.Err())
}

func checkRegistration(h pgutil.Holding, c pgutil.Command) error {
	if h.AccountID != c.AccountID || h.Allocation.Owner != c.Owner {
		return failure(corev1.IdentityConflict, c.OperationID, "command owner or account does not match allocation", nil)
	}
	if h.Allocation.State == corev1.AllocationClosing || h.Allocation.State == corev1.AllocationClosed {
		return failure(corev1.Closed, c.OperationID, "allocation admission gate is closed", nil)
	}
	if c.Kind == corev1.Grant {
		if h.Allocation.State != corev1.AllocationPending {
			return failure(corev1.RevisionChanged, c.OperationID, "initial grant requires a Pending allocation", nil)
		}
		if c.Quantity != h.InitialQuantity || c.ResourceRevision != h.InitialRevision {
			return failure(corev1.IdentityConflict, c.OperationID, "grant command does not match initial allocation intent", nil)
		}
	} else if h.Allocation.State != corev1.AllocationActive {
		return failure(corev1.RevisionChanged, c.OperationID, "resource change requires an Active allocation", nil)
	}
	if c.ExpectedVersion != h.Allocation.Version {
		return failure(corev1.RevisionChanged, c.OperationID, "allocation version changed", nil)
	}
	return nil
}

func toAccount(rec QuotaAccountRecord) (Account, error) {
	var expiresAt *time.Time
	if rec.ExpiresAt != nil {
		t := rec.ExpiresAt.UTC().Truncate(time.Microsecond)
		expiresAt = &t
	}
	a := Account{
		TenantUID:       rec.TenantUID,
		PolicyVersion:   rec.PolicyVersion,
		UpstreamVersion: rec.UpstreamVersion,
		ExpiresAt:       expiresAt,
		Limits: pgutil.Limits([6]*int64{
			rec.LimitCPU, rec.LimitMem, rec.LimitStorage,
			rec.LimitInstances, rec.LimitBackups, rec.LimitBackupStorage,
		}),
		Allocated: corev1.Resources{
			CPUMillicores:      rec.AllocatedCPU,
			MemoryBytes:        rec.AllocatedMem,
			StorageBytes:       rec.AllocatedStorage,
			InstanceCount:      rec.AllocatedInstances,
			BackupsCount:       rec.AllocatedBackups,
			BackupStorageBytes: rec.AllocatedBackupStorage,
		},
	}
	if err := a.Limits.Validate(); err != nil {
		return a, failure(corev1.InvariantViolation, "", "invalid persisted limits", err)
	}
	if err := a.Allocated.Validate(); err != nil {
		return a, failure(corev1.InvariantViolation, "", "invalid persisted account", err)
	}
	return a, nil
}

func toHolding(rec QuotaAllocationRecord) (pgutil.Holding, error) {
	h := pgutil.Holding{
		Domain:         corev1.QuotaDomain,
		AccountID:      rec.TenantUID,
		IntentHash:     rec.IntentHash,
		ChangePlanID:   rec.ChangePlanID,
		ChangeRevision: rec.ChangeRevision,
		Allocation: pgutil.Allocation{
			ID: rec.AllocationID,
			Owner: pgutil.Owner{
				TenantUID:    rec.TenantUID,
				InstanceUID:  rec.InstanceUID,
				InstanceName: rec.InstanceName,
			},
			State: rec.State,
			Current: corev1.Resources{
				CPUMillicores:      rec.CurrentCPU,
				MemoryBytes:        rec.CurrentMem,
				StorageBytes:       rec.CurrentStorage,
				InstanceCount:      rec.CurrentInstances,
				BackupsCount:       rec.CurrentBackups,
				BackupStorageBytes: rec.CurrentBackupStorage,
			},
			Version:          rec.Version,
			ResourceRevision: rec.ResourceRevision,
			CloseIntentID:    rec.CloseIntentID,
		},
	}
	if len(rec.InitialIntent) > 0 {
		var intent pgutil.AllocationIntent
		if err := json.Unmarshal(rec.InitialIntent, &intent); err != nil {
			return h, failure(corev1.InvariantViolation, "", "invalid persisted intent", err)
		}
		hash, err := intent.Fingerprint()
		if err != nil || hash != h.IntentHash || intent.AllocationID != h.Allocation.ID || intent.Owner != h.Allocation.Owner {
			return h, failure(corev1.InvariantViolation, "", "intent identity or digest is corrupt", err)
		}
		h.InitialQuantity, h.InitialRevision = intent.Quantity, intent.ResourceRevision
	}
	return h, h.Validate()
}

func toOperation(rec QuotaOperationRecord) (Operation, error) {
	op := Operation{State: rec.State}
	if err := json.Unmarshal(rec.Command, &op.Command); err != nil {
		return op, failure(corev1.InvariantViolation, rec.OperationID, "invalid persisted command", err)
	}
	actual, err := op.Command.Fingerprint()
	if err != nil || actual != rec.RequestHash || op.Command.Domain != corev1.QuotaDomain ||
		op.Command.OperationID != rec.OperationID || op.Command.AllocationID != rec.AllocationID ||
		op.Command.Kind != rec.Kind || op.Command.ChangePlanID != rec.ChangePlanID || op.Command.CloseIntentID != rec.CloseIntentID {
		return op, failure(corev1.InvariantViolation, rec.OperationID, "command identity or digest is corrupt", err)
	}
	if err := pgutil.ValidateOperationTransition(op.State, op.State); err != nil {
		return op, err
	}
	if len(rec.Result) > 0 {
		if err := json.Unmarshal(rec.Result, &op.Result); err != nil {
			return op, failure(corev1.InvariantViolation, rec.OperationID, "invalid historical result", err)
		}
		op.HasResult = true
		if err := op.Result.Validate(); err != nil {
			return op, failure(corev1.InvariantViolation, rec.OperationID, "invalid historical holding", err)
		}
		if op.Result.ID != rec.AllocationID || op.Result.Owner != op.Command.Owner {
			return op, failure(corev1.InvariantViolation, rec.OperationID, "historical result belongs to another allocation", nil)
		}
	}
	if op.HasResult != (op.State == corev1.Committed) {
		return op, failure(corev1.InvariantViolation, rec.OperationID, "result and operation state disagree", nil)
	}
	return op, nil
}

func getAccountRow(tx *gorm.DB, id string) (Account, error) {
	var rec QuotaAccountRecord
	if err := tx.Where("tenant_uid = ?", id).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Account{}, notFound("account", id)
		}
		return Account{}, pgutil.Error(err, "")
	}
	return toAccount(rec)
}

func increaseAccountGORM(tx *gorm.DB, tenantUID string, inc corev1.Resources) *gorm.DB {
	return tx.Model(&QuotaAccountRecord{}).
		Where(`tenant_uid = ?
 AND policy_version > 0
 AND (expires_at IS NULL OR expires_at > NOW())
 AND allocated_cpu <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_cpu IS NULL OR allocated_cpu <= limit_cpu - ?::bigint)
 AND allocated_mem <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_mem IS NULL OR allocated_mem <= limit_mem - ?::bigint)
 AND allocated_storage <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_storage IS NULL OR allocated_storage <= limit_storage - ?::bigint)
 AND allocated_instances <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_instances IS NULL OR allocated_instances <= limit_instances - ?::bigint)
 AND allocated_backups <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_backups IS NULL OR allocated_backups <= limit_backups - ?::bigint)
 AND allocated_backup_storage <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_backup_storage IS NULL OR allocated_backup_storage <= limit_backup_storage - ?::bigint)`,
			tenantUID,
			inc.CPUMillicores, inc.CPUMillicores, inc.CPUMillicores,
			inc.MemoryBytes, inc.MemoryBytes, inc.MemoryBytes,
			inc.StorageBytes, inc.StorageBytes, inc.StorageBytes,
			inc.InstanceCount, inc.InstanceCount, inc.InstanceCount,
			inc.BackupsCount, inc.BackupsCount, inc.BackupsCount,
			inc.BackupStorageBytes, inc.BackupStorageBytes, inc.BackupStorageBytes,
		).
		Updates(map[string]any{
			"allocated_cpu":            gorm.Expr("allocated_cpu + ?", inc.CPUMillicores),
			"allocated_mem":            gorm.Expr("allocated_mem + ?", inc.MemoryBytes),
			"allocated_storage":        gorm.Expr("allocated_storage + ?", inc.StorageBytes),
			"allocated_instances":      gorm.Expr("allocated_instances + ?", inc.InstanceCount),
			"allocated_backups":        gorm.Expr("allocated_backups + ?", inc.BackupsCount),
			"allocated_backup_storage": gorm.Expr("allocated_backup_storage + ?", inc.BackupStorageBytes),
			"updated_at":               gorm.Expr("NOW()"),
		})
}

func decreaseAccountGORM(tx *gorm.DB, tenantUID string, dec corev1.Resources) *gorm.DB {
	return tx.Model(&QuotaAccountRecord{}).
		Where(`tenant_uid = ? AND allocated_cpu >= ? AND allocated_mem >= ? AND allocated_storage >= ?
 AND allocated_instances >= ? AND allocated_backups >= ? AND allocated_backup_storage >= ?`,
			tenantUID,
			dec.CPUMillicores, dec.MemoryBytes, dec.StorageBytes,
			dec.InstanceCount, dec.BackupsCount, dec.BackupStorageBytes,
		).
		Updates(map[string]any{
			"allocated_cpu":            gorm.Expr("allocated_cpu - ?", dec.CPUMillicores),
			"allocated_mem":            gorm.Expr("allocated_mem - ?", dec.MemoryBytes),
			"allocated_storage":        gorm.Expr("allocated_storage - ?", dec.StorageBytes),
			"allocated_instances":      gorm.Expr("allocated_instances - ?", dec.InstanceCount),
			"allocated_backups":        gorm.Expr("allocated_backups - ?", dec.BackupsCount),
			"allocated_backup_storage": gorm.Expr("allocated_backup_storage - ?", dec.BackupStorageBytes),
			"updated_at":               gorm.Expr("NOW()"),
		})
}

func updateHoldingGORM(tx *gorm.DB, expectedVersion int64, next pgutil.Holding) *gorm.DB {
	a := next.Allocation
	return tx.Model(&QuotaAllocationRecord{}).
		Where("allocation_id = ? AND version = ?", a.ID, expectedVersion).
		Updates(map[string]any{
			"state":                  a.State,
			"current_cpu":            a.Current.CPUMillicores,
			"current_mem":            a.Current.MemoryBytes,
			"current_storage":        a.Current.StorageBytes,
			"current_instances":      a.Current.InstanceCount,
			"current_backups":        a.Current.BackupsCount,
			"current_backup_storage": a.Current.BackupStorageBytes,
			"version":                a.Version,
			"resource_revision":      a.ResourceRevision,
			"close_intent_id":        a.CloseIntentID,
			"change_plan_id":         next.ChangePlanID,
			"change_revision":        next.ChangeRevision,
			"updated_at":             gorm.Expr("NOW()"),
		})
}
