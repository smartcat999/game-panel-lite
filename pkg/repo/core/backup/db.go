// Package backup persists immutable archive commands and their execution facts.
// Quota holdings belong exclusively to the quota DB; no inventory lives here.
package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB receives the primary pool and uses GORM over backup_commands.
type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func toRecord(row corev1.BackupCommandRecord) (Record, error) {
	r := Record{
		Phase:           Phase(row.Phase),
		Version:         row.Version,
		CreatedAt:       row.CreatedAt,
		UploadExpiresAt: row.UploadExpiresAt,
	}
	if err := json.Unmarshal(row.Command, &r.Command); err != nil {
		return r, err
	}
	actual, err := r.Command.fingerprint()
	if err != nil || actual != row.RequestHash || row.OperationID != r.Command.OperationID ||
		row.InstanceUID != r.Command.Owner.InstanceUID || row.ObjectKey != r.Command.ObjectKey || r.Version < 1 {
		return Record{}, fmt.Errorf("backup command identity is corrupt: %w", ErrConflict)
	}
	if len(row.Result) > 0 {
		if err := json.Unmarshal(row.Result, &r.Result); err != nil {
			return Record{}, err
		}
	}
	return r, nil
}

func getRecordWhere(tx *gorm.DB, query string, args ...any) (Record, error) {
	var row corev1.BackupCommandRecord
	if err := tx.Where(query, args...).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Record{}, ErrNotFound
		}
		return Record{}, err
	}
	return toRecord(row)
}

func (s *DB) Register(ctx context.Context, c Command) (Record, error) {
	hash, err := c.fingerprint()
	if err != nil {
		return Record{}, err
	}
	if r, err := s.Get(ctx, c.OperationID); err == nil {
		return r, match(r, c)
	} else if !errors.Is(err, ErrNotFound) {
		return r, err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return Record{}, err
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return Record{}, pgutil.Error(tx.Error, c.OperationID)
	}
	defer tx.Rollback()
	if err := ensureInstanceGateOpen(tx, c.Owner.InstanceUID, c.OperationID); err != nil {
		return Record{}, err
	}
	rec := corev1.BackupCommandRecord{
		OperationID: c.OperationID, InstanceUID: c.Owner.InstanceUID, ObjectKey: c.ObjectKey,
		RequestHash: hash, Command: raw, Phase: string(PendingQuota), Version: 1,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rec).Error; err != nil {
		return Record{}, pgutil.Error(err, c.OperationID)
	}
	r, err := getRecordWhere(tx, "operation_id = ?", c.OperationID)
	if errors.Is(err, ErrNotFound) {
		return Record{}, ErrConflict
	}
	if err != nil {
		return Record{}, pgutil.Error(err, c.OperationID)
	}
	if err := match(r, c); err != nil {
		return r, err
	}
	if err := pgutil.CommitGORM(ctx, tx, c.OperationID); err != nil {
		return Record{}, err
	}
	return r, nil
}

func ensureInstanceGateOpen(tx *gorm.DB, instanceUID, opID string) error {
	gate := corev1.BackupGateRecord{InstanceUID: instanceUID, Closed: false}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&gate).Error; err != nil {
		return pgutil.Error(err, opID)
	}
	res := tx.Model(&corev1.BackupGateRecord{}).
		Where("instance_uid = ? AND closed = FALSE", instanceUID).
		UpdateColumn("closed", false)
	if res.Error != nil {
		return pgutil.Error(res.Error, opID)
	}
	if res.RowsAffected != 1 {
		return ErrConflict
	}
	var activeCount int64
	if err := tx.Model(&corev1.BackupCommandRecord{}).
		Where("instance_uid = ? AND operation_id <> ? AND phase IN ?", instanceUID, opID, []string{string(PendingQuota), string(Ready), string(Uploading)}).
		Count(&activeCount).Error; err != nil {
		return pgutil.Error(err, opID)
	}
	if activeCount > 0 {
		return fmt.Errorf("%w: another backup operation is already in progress", ErrConflict)
	}
	return nil
}

func (s *DB) CloseInstance(ctx context.Context, uid string) error {
	if err := corev1.ValidateIdentifier("instanceUID", uid); err != nil {
		return err
	}
	gate := corev1.BackupGateRecord{InstanceUID: uid, Closed: true}
	err := s.orm.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "instance_uid"}},
		DoUpdates: clause.Assignments(map[string]any{"closed": true}),
	}).Create(&gate).Error
	return pgutil.WriteError(err, "")
}

func (s *DB) Get(ctx context.Context, id string) (Record, error) {
	return getRecordWhere(s.orm.WithContext(ctx), "operation_id = ?", id)
}

func (s *DB) GetByKey(ctx context.Context, uid, key string) (Record, error) {
	return getRecordWhere(s.orm.WithContext(ctx), "instance_uid = ? AND object_key = ?", uid, key)
}

func (s *DB) ListInstance(ctx context.Context, uid string) ([]Record, error) {
	var rows []corev1.BackupCommandRecord
	err := s.orm.WithContext(ctx).Where("instance_uid = ?", uid).Order("operation_id").Find(&rows).Error
	if err != nil {
		return nil, pgutil.Error(err, "")
	}
	out := make([]Record, 0, len(rows))
	for _, row := range rows {
		r, err := toRecord(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *DB) Transition(ctx context.Context, id string, expected int64, u Update) (Record, error) {
	r, err := s.Get(ctx, id)
	if err != nil {
		return r, err
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return Record{}, pgutil.Error(tx.Error, id)
	}
	defer tx.Rollback()
	if u.Phase == Ready || u.Phase == Uploading {
		res := tx.Model(&corev1.BackupGateRecord{}).
			Where("instance_uid = ? AND closed = FALSE", r.Command.Owner.InstanceUID).
			UpdateColumn("closed", false)
		if res.Error != nil {
			return Record{}, pgutil.Error(res.Error, id)
		}
		if res.RowsAffected != 1 {
			return Record{}, ErrConflict
		}
	}
	next, err := apply(r, expected, u)
	if err != nil {
		return r, err
	}
	var resultBytes []byte
	if next.Result != nil {
		resultBytes, err = json.Marshal(next.Result)
		if err != nil {
			return r, err
		}
	}
	var updatedRow corev1.BackupCommandRecord
	res := tx.Model(&updatedRow).Clauses(clause.Returning{}).
		Where("operation_id = ? AND version = ?", id, expected).
		Updates(map[string]any{
			"phase":             string(next.Phase),
			"version":           gorm.Expr("version + 1"),
			"result":            nullableJSON(resultBytes),
			"upload_expires_at": next.UploadExpiresAt,
			"updated_at":        gorm.Expr("NOW()"),
		})
	if res.Error != nil {
		return Record{}, pgutil.Error(res.Error, id)
	}
	if res.RowsAffected == 0 {
		return Record{}, ErrConflict
	}
	r, err = toRecord(updatedRow)
	if err != nil {
		return Record{}, pgutil.Error(err, id)
	}
	if err := pgutil.CommitGORM(ctx, tx, id); err != nil {
		return Record{}, err
	}
	return r, nil
}

func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
