package instance

import (
	"context"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// ExecutionFactEntry describes an immutable instance execution history fact to commit atomically
// alongside an observation update (or recorded directly for stale/deleted transitions).
type ExecutionFactEntry struct {
	FactID       string
	Tenant       string
	TenantUID    string
	InstanceUID  string
	InstanceName string
	FactType     string
	OccurredAt   time.Time
	Attributes   map[string]string
}

// ObservationWriteHooks allows deterministic fault injection in repository transaction tests.
type ObservationWriteHooks struct {
	BeforeCommit         func(ctx context.Context) error
	CommitOverride       func(ctx context.Context, tx *gorm.DB, operationID string) error
	AfterCommitBeforeGet func(ctx context.Context) error
}

type observationWriteHooksKey struct{}

// WithObservationWriteHooks attaches optional transaction fault-injection hooks to ctx.
func WithObservationWriteHooks(ctx context.Context, hooks ObservationWriteHooks) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, observationWriteHooksKey{}, hooks)
}

func observationWriteHooksFrom(ctx context.Context) (ObservationWriteHooks, bool) {
	if ctx == nil {
		return ObservationWriteHooks{}, false
	}
	hooks, ok := ctx.Value(observationWriteHooksKey{}).(ObservationWriteHooks)
	return hooks, ok
}

// RecordExecutionFacts idempotently persists immutable execution history facts in a short transaction.
func (s *DB) RecordExecutionFacts(ctx context.Context, entries []ExecutionFactEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if s == nil || s.orm == nil {
		return errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return pgutil.Error(tx.Error, entries[0].FactID)
	}
	if err := insertExecutionFactsTx(ctx, tx, entries); err != nil {
		_ = tx.Rollback()
		return err
	}
	return pgutil.CommitGORM(ctx, tx, entries[0].FactID)
}

// GetExecutionFact retrieves a single execution fact record by fact ID.
func (s *DB) GetExecutionFact(ctx context.Context, factID string) (*corev1.InstanceExecutionFactRecord, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec corev1.InstanceExecutionFactRecord
	err := s.orm.WithContext(ctx).Where("fact_id = ?", factID).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListWindowExecutionFacts returns the latest prior checkpoint per instance before start
// plus all execution facts within [start, now) in chronological order.
func (s *DB) ListWindowExecutionFacts(ctx context.Context, tenant, tenantUID string, start time.Time) ([]corev1.InstanceExecutionFactRecord, error) {
	if s == nil || s.orm == nil || tenant == "" || tenantUID == "" {
		return nil, nil
	}
	if start.IsZero() {
		return nil, errors.New("execution fact window requires a non-zero start time")
	}
	startUTC := start.UTC()
	var prior []corev1.InstanceExecutionFactRecord
	if err := s.orm.WithContext(ctx).
		Model(&corev1.InstanceExecutionFactRecord{}).
		Select("DISTINCT ON (instance_uid) fact_id, tenant, tenant_uid, instance_uid, instance_name, fact_type, occurred_at, attributes, created_at").
		Where("tenant = ? AND tenant_uid = ? AND occurred_at < ?", tenant, tenantUID, startUTC).
		Order("instance_uid ASC, occurred_at DESC, fact_id DESC").
		Find(&prior).Error; err != nil {
		return nil, err
	}
	sort.Slice(prior, func(i, j int) bool {
		if !prior[i].OccurredAt.Equal(prior[j].OccurredAt) {
			return prior[i].OccurredAt.Before(prior[j].OccurredAt)
		}
		return prior[i].FactID < prior[j].FactID
	})
	var inWindow []corev1.InstanceExecutionFactRecord
	if err := s.orm.WithContext(ctx).
		Model(&corev1.InstanceExecutionFactRecord{}).
		Where("tenant = ? AND tenant_uid = ? AND occurred_at >= ?", tenant, tenantUID, startUTC).
		Order("occurred_at ASC, fact_id ASC").
		Find(&inWindow).Error; err != nil {
		return nil, err
	}
	return append(prior, inWindow...), nil
}

// ListInstanceExecutionFactsBefore returns execution facts for an instance at or before before,
// ordered from most recent to oldest.
func (s *DB) ListInstanceExecutionFactsBefore(ctx context.Context, tenant, tenantUID, instanceUID string, before time.Time, limit int) ([]corev1.InstanceExecutionFactRecord, error) {
	if s == nil || s.orm == nil || tenant == "" || tenantUID == "" || instanceUID == "" {
		return nil, nil
	}
	if before.IsZero() {
		before = time.Now().UTC()
	} else {
		before = before.UTC()
	}
	q := s.orm.WithContext(ctx).
		Model(&corev1.InstanceExecutionFactRecord{}).
		Where("tenant = ? AND tenant_uid = ? AND instance_uid = ? AND occurred_at <= ?", tenant, tenantUID, instanceUID, before).
		Order("occurred_at DESC, fact_id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []corev1.InstanceExecutionFactRecord
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func insertExecutionFactsTx(ctx context.Context, tx *gorm.DB, entries []ExecutionFactEntry) error {
	now := time.Now().UTC()
	for _, entry := range entries {
		rec, err := toExecutionFactRecord(entry, now)
		if err != nil {
			return err
		}
		res := tx.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "fact_id"}},
				DoNothing: true,
			}).
			Create(&rec)
		if res.Error != nil {
			return pgutil.Error(res.Error, entry.FactID)
		}
		if res.RowsAffected == 0 {
			if err := verifyExistingExecutionFactIdentity(ctx, tx, rec); err != nil {
				return err
			}
		}
	}
	return nil
}

func toExecutionFactRecord(entry ExecutionFactEntry, now time.Time) (corev1.InstanceExecutionFactRecord, error) {
	if entry.FactID == "" || entry.Tenant == "" || entry.InstanceUID == "" || entry.InstanceName == "" || entry.FactType == "" || entry.OccurredAt.IsZero() {
		return corev1.InstanceExecutionFactRecord{}, &corev1.Error{
			Code:        corev1.InvariantViolation,
			OperationID: entry.FactID,
			Detail:      "invalid instance execution fact entry",
		}
	}
	attrs := make(map[string]string, len(entry.Attributes))
	for k, v := range entry.Attributes {
		attrs[k] = v
	}
	return corev1.InstanceExecutionFactRecord{
		FactID:       entry.FactID,
		Tenant:       entry.Tenant,
		TenantUID:    entry.TenantUID,
		InstanceUID:  entry.InstanceUID,
		InstanceName: entry.InstanceName,
		FactType:     entry.FactType,
		OccurredAt:   entry.OccurredAt.UTC(),
		Attributes:   attrs,
		CreatedAt:    now,
	}, nil
}

var frozenFactAttrKeys = []string{
	"uid",
	"tenantUID",
	"phase",
	"cpu",
	"memory",
	"storage",
	"attemptID",
	"epoch",
	"resourceRevision",
}

func verifyExistingExecutionFactIdentity(ctx context.Context, tx *gorm.DB, rec corev1.InstanceExecutionFactRecord) error {
	var existing corev1.InstanceExecutionFactRecord
	if err := tx.WithContext(ctx).Where("fact_id = ?", rec.FactID).First(&existing).Error; err != nil {
		return pgutil.Error(err, rec.FactID)
	}
	if existing.Tenant != rec.Tenant ||
		existing.TenantUID != rec.TenantUID ||
		existing.InstanceUID != rec.InstanceUID ||
		existing.InstanceName != rec.InstanceName ||
		existing.FactType != rec.FactType ||
		!sameFrozenFactAttrs(existing.Attributes, rec.Attributes) {
		return &corev1.Error{
			Code:        corev1.IdentityConflict,
			OperationID: rec.FactID,
			Detail:      "instance execution fact identity or payload differs",
		}
	}
	return nil
}

func sameFrozenFactAttrs(a, b map[string]string) bool {
	for _, k := range frozenFactAttrKeys {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}
