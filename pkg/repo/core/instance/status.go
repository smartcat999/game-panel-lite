package instance

import (
	"context"
	"errors"
	"reflect"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

const (
	usageCacheTTL        = 5 * time.Minute
	usagePersistInterval = 60 * time.Second
)

// UpdateObservation updates runtime observation columns in instance_observations with StatusVersion CAS,
// leaving instances.resource_version, schedule_statuses, and backups untouched.
// Pure telemetry ticks within usagePersistInterval are served from Redis to avoid MVCC row churn.
func (s *DB) UpdateObservation(ctx context.Context, in *corev1.Instance) (*corev1.Instance, error) {
	return s.updateObservationColumns(ctx, in, false, nil)
}

// UpdateObservationWithFacts atomically updates runtime observation columns in instance_observations
// and inserts immutable execution history facts within a single short PostgreSQL transaction.
func (s *DB) UpdateObservationWithFacts(ctx context.Context, in *corev1.Instance, facts []ExecutionFactEntry) (*corev1.Instance, error) {
	return s.updateObservationColumns(ctx, in, false, facts)
}

// UpdateStatus updates all instance_observations columns with StatusVersion CAS,
// leaving instances and resource_version completely untouched.
func (s *DB) UpdateStatus(ctx context.Context, in *corev1.Instance) (*corev1.Instance, error) {
	return s.updateObservationColumns(ctx, in, true, nil)
}

func (s *DB) updateObservationColumns(ctx context.Context, in *corev1.Instance, includeSchedulesAndBackups bool, facts []ExecutionFactEntry) (*corev1.Instance, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	existing, err := s.getRaw(ctx, in.Metadata.Tenant, in.Metadata.Name)
	if err != nil {
		return nil, err
	}
	if (in.Metadata.UID != "" && in.Metadata.UID != existing.Metadata.UID) ||
		(in.Metadata.StatusVersion != "" && in.Metadata.StatusVersion != existing.Metadata.StatusVersion) {
		return nil, db.ErrConflict
	}
	if !includeSchedulesAndBackups && len(facts) == 0 && canServeUsageFromCacheOnly(existing, in) {
		syncUsageCache(ctx, existing.Metadata.UID, in)
		existing.Status.Usage = in.Status.Usage
		existing.Status.ObservedAt = in.Status.ObservedAt
		return existing, nil
	}
	oldSV, _ := pgutil.ParseVersion(existing.Metadata.StatusVersion)
	newSV := oldSV + 1
	cp := *in
	cp.Metadata.UID = existing.Metadata.UID
	cols := buildObservationCols(toObservationRecord(&cp, newSV), newSV, includeSchedulesAndBackups)
	if err := s.commitObservationTx(ctx, existing.Metadata.UID, in.Metadata.StatusVersion != "", oldSV, cols, facts); err != nil {
		return nil, err
	}
	syncUsageCache(ctx, existing.Metadata.UID, in)
	if hooks, ok := observationWriteHooksFrom(ctx); ok && hooks.AfterCommitBeforeGet != nil {
		if err := hooks.AfterCommitBeforeGet(ctx); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, existing.Metadata.Tenant, existing.Metadata.Name)
}

func (s *DB) commitObservationTx(ctx context.Context, instanceUID string, checkSV bool, oldSV int64, cols map[string]any, facts []ExecutionFactEntry) error {
	opID := executionFactOperationID(facts)
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return pgutil.Error(tx.Error, opID)
	}
	upd := tx.WithContext(ctx).Model(&corev1.InstanceObservationRecord{}).Where("instance_uid = ?", instanceUID)
	if checkSV {
		upd = upd.Where("version = ?", oldSV)
	}
	res := upd.Updates(cols)
	if res.Error != nil {
		_ = tx.Rollback()
		return pgutil.Error(res.Error, opID)
	}
	if res.RowsAffected == 0 {
		_ = tx.Rollback()
		return db.ErrConflict
	}
	if err := insertExecutionFactsTx(ctx, tx, facts); err != nil {
		_ = tx.Rollback()
		return err
	}
	if hooks, ok := observationWriteHooksFrom(ctx); ok {
		if hooks.BeforeCommit != nil {
			if err := hooks.BeforeCommit(ctx); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if hooks.CommitOverride != nil {
			return hooks.CommitOverride(ctx, tx, opID)
		}
	}
	return pgutil.CommitGORM(ctx, tx, opID)
}

func executionFactOperationID(facts []ExecutionFactEntry) string {
	if len(facts) == 0 {
		return ""
	}
	return facts[0].FactID
}

func syncUsageCache(ctx context.Context, uid string, in *corev1.Instance) {
	if uid == "" || cache.Default() == nil {
		return
	}
	if in.Status.Usage != nil && in.Status.Phase == corev1.InstancePhaseRunning && in.Metadata.DeletionTimestamp == nil {
		_ = cache.SetJSON(ctx, nil, usageCacheKey(uid), cachedUsageEntry{
			ObservedAt: in.Status.ObservedAt,
			Usage:      in.Status.Usage,
		}, usageCacheTTL)
		return
	}
	deleteCachedUsage(ctx, uid)
}

func canServeUsageFromCacheOnly(existing, incoming *corev1.Instance) bool {
	if cache.Default() == nil || incoming.Status.Usage == nil || incoming.Status.Phase != corev1.InstancePhaseRunning {
		return false
	}
	if existing.Status.Phase != incoming.Status.Phase ||
		existing.Status.ObservedGeneration != incoming.Status.ObservedGeneration ||
		existing.Status.RuntimeID != incoming.Status.RuntimeID ||
		existing.Status.TransitionID != incoming.Status.TransitionID ||
		!reflect.DeepEqual(existing.Status.Endpoint, incoming.Status.Endpoint) ||
		!sameTimePtr(existing.Status.StartedAt, incoming.Status.StartedAt) ||
		!sameObservedControl(existing.Status.ResourceControl, incoming.Status.ResourceControl) ||
		!sameNonControlConditions(existing.Status.Conditions, incoming.Status.Conditions) {
		return false
	}
	if existing.Status.ObservedAt == nil || incoming.Status.ObservedAt == nil {
		return false
	}
	age := incoming.Status.ObservedAt.Sub(*existing.Status.ObservedAt)
	return age >= 0 && age < usagePersistInterval
}

func sameTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func sameObservedControl(a, b *corev1.InstanceResourceControl) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ObservedRevision == b.ObservedRevision &&
		a.ObservedAttemptID == b.ObservedAttemptID &&
		a.ObservedEpoch == b.ObservedEpoch &&
		reflect.DeepEqual(a.ObservedResources, b.ObservedResources) &&
		reflect.DeepEqual(a.ObservedPorts, b.ObservedPorts)
}

func sameNonControlConditions(a, b []metav1.Condition) bool {
	ca, cb := filterConditions(a, false), filterConditions(b, false)
	if len(ca) != len(cb) {
		return false
	}
	for i := range ca {
		if ca[i].Type != cb[i].Type || ca[i].Status != cb[i].Status ||
			ca[i].Reason != cb[i].Reason || ca[i].Message != cb[i].Message ||
			ca[i].ObservedGeneration != cb[i].ObservedGeneration {
			return false
		}
	}
	return true
}

func buildObservationCols(obs corev1.InstanceObservationRecord, newSV int64, includeSchedulesAndBackups bool) map[string]any {
	epJSON, _ := pgutil.MarshalJSONCol(obs.Endpoint)
	obsResJSON, _ := pgutil.MarshalJSONCol(obs.ObservedResources)
	obsPortsJSON, _ := pgutil.MarshalJSONCol(obs.ObservedPorts)
	condsJSON, _ := pgutil.MarshalJSONCol(obs.Conditions)
	cols := map[string]any{
		"version":             newSV,
		"observed_generation": obs.ObservedGeneration,
		"phase":               obs.Phase,
		"runtime_id":          obs.RuntimeID,
		"endpoint":            epJSON,
		"started_at":          obs.StartedAt,
		"observed_at":         obs.ObservedAt,
		"transition_id":       obs.TransitionID,
		"observed_revision":   obs.ObservedRevision,
		"observed_attempt_id": obs.ObservedAttemptID,
		"observed_epoch":      obs.ObservedEpoch,
		"observed_resources":  obsResJSON,
		"observed_ports":      obsPortsJSON,
		"conditions":          condsJSON,
		"usage":               marshalOptionalJSON(obs.Usage, obs.Usage != nil),
		"updated_at":          time.Now().UTC(),
	}
	if includeSchedulesAndBackups {
		schedJSON, _ := pgutil.MarshalJSONCol(obs.ScheduleStatuses)
		bakJSON, _ := pgutil.MarshalJSONCol(obs.Backups)
		cols["schedule_statuses"] = schedJSON
		cols["backups"] = bakJSON
	}
	return cols
}

// UpdateScheduleStatuses atomically updates only schedule_statuses in instance_observations with StatusVersion CAS.
func (s *DB) UpdateScheduleStatuses(ctx context.Context, tenant, name, expectedUID, expectedSV string, statuses []corev1.InstanceScheduleStatus) (*corev1.Instance, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	existing, err := s.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if (expectedUID != "" && expectedUID != existing.Metadata.UID) ||
		(expectedSV != "" && expectedSV != existing.Metadata.StatusVersion) {
		return nil, db.ErrConflict
	}
	oldSV, _ := pgutil.ParseVersion(existing.Metadata.StatusVersion)
	schedJSON, _ := pgutil.MarshalJSONCol(nonNilSlice(statuses))
	res := s.orm.WithContext(ctx).Model(&corev1.InstanceObservationRecord{}).
		Where("instance_uid = ? AND version = ?", existing.Metadata.UID, oldSV).
		Updates(map[string]any{"version": oldSV + 1, "schedule_statuses": schedJSON, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, tenant, name)
}

// UpdateBackups atomically updates only backups in instance_observations with StatusVersion CAS.
func (s *DB) UpdateBackups(ctx context.Context, tenant, name, expectedUID, expectedSV string, backups []corev1.InstanceBackupRecord) (*corev1.Instance, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	existing, err := s.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if (expectedUID != "" && expectedUID != existing.Metadata.UID) ||
		(expectedSV != "" && expectedSV != existing.Metadata.StatusVersion) {
		return nil, db.ErrConflict
	}
	oldSV, _ := pgutil.ParseVersion(existing.Metadata.StatusVersion)
	bakJSON, _ := pgutil.MarshalJSONCol(nonNilSlice(backups))
	res := s.orm.WithContext(ctx).Model(&corev1.InstanceObservationRecord{}).
		Where("instance_uid = ? AND version = ?", existing.Metadata.UID, oldSV).
		Updates(map[string]any{"version": oldSV + 1, "backups": bakJSON, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, tenant, name)
}
