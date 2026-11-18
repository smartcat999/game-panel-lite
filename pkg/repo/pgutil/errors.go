// Package pgutil contains PostgreSQL transport semantics only. Domain SQL and
// inventory decisions stay in their owning stores.
package pgutil

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

func Error(err error, operationID string) error {
	if err == nil {
		return nil
	}
	var protocol *corev1.Error
	var validation *corev1.ValidationError
	if errors.As(err, &protocol) || errors.As(err, &validation) {
		return err
	}
	code := corev1.TransientUnavailable
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			code = corev1.IdentityConflict
		case "23514", "23503", "22003", "22P02":
			code = corev1.InvariantViolation
		}
	}
	return &corev1.Error{Code: code, OperationID: operationID, Detail: "PostgreSQL operation failed", Cause: err}
}

// WriteError is for autocommit statements only. A transport failure cannot
// establish whether that statement committed; server rejections remain definite.
func WriteError(err error, operationID string) error {
	if err == nil {
		return nil
	}
	var pg *pgconn.PgError
	var protocol *corev1.Error
	var validation *corev1.ValidationError
	if errors.As(err, &pg) || errors.As(err, &protocol) || errors.As(err, &validation) {
		return Error(err, operationID)
	}
	return &corev1.Error{Code: corev1.OutcomeUnknown, OperationID: operationID, Detail: "autocommit acknowledgement unavailable", Cause: err}
}

// Commit treats lost acknowledgements conservatively. Callers recover by the
// same immutable ID; they must never refund or create a replacement command.
func Commit(ctx context.Context, tx pgx.Tx, operationID string) error {
	err := tx.Commit(ctx)
	if errors.Is(err, pgx.ErrTxCommitRollback) {
		return Error(err, operationID)
	}
	if err != nil {
		return &corev1.Error{Code: corev1.OutcomeUnknown, OperationID: operationID, Detail: "commit acknowledgement unavailable", Cause: err}
	}
	if err := ctx.Err(); err != nil {
		return &corev1.Error{Code: corev1.OutcomeUnknown, OperationID: operationID, Detail: "commit completed after caller stopped waiting", Cause: err}
	}
	return nil
}

// CommitGORM commits a GORM transaction while preserving conservative outcome-unknown semantics.
func CommitGORM(ctx context.Context, tx *gorm.DB, operationID string) error {
	err := tx.Commit().Error
	if errors.Is(err, pgx.ErrTxCommitRollback) || errors.Is(err, sql.ErrTxDone) {
		return Error(err, operationID)
	}
	if err != nil {
		return &corev1.Error{Code: corev1.OutcomeUnknown, OperationID: operationID, Detail: "commit acknowledgement unavailable", Cause: err}
	}
	if err := ctx.Err(); err != nil {
		return &corev1.Error{Code: corev1.OutcomeUnknown, OperationID: operationID, Detail: "commit completed after caller stopped waiting", Cause: err}
	}
	return nil
}

func ResourceArgs(r corev1.Resources) []any {
	return []any{r.CPUMillicores, r.MemoryBytes, r.StorageBytes, r.InstanceCount, r.BackupsCount, r.BackupStorageBytes}
}

func LimitArgs(l corev1.Limits) []any {
	out := make([]any, 6)
	for i, limit := range []corev1.Limit{l.CPU, l.Memory, l.Storage, l.Instances, l.Backups, l.BackupStorage} {
		if !limit.Unlimited {
			out[i] = limit.Value
		}
	}
	return out
}

func Limits(values [6]*int64) corev1.Limits {
	var limits [6]corev1.Limit
	for i, value := range values {
		if value == nil {
			limits[i].Unlimited = true
		} else {
			limits[i].Value = *value
		}
	}
	return corev1.Limits{CPU: limits[0], Memory: limits[1], Storage: limits[2], Instances: limits[3], Backups: limits[4], BackupStorage: limits[5]}
}
