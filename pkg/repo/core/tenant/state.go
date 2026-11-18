package tenant

import (
	"context"
	"errors"
	"reflect"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

func (s *DB) Update(ctx context.Context, in *corev1.Tenant) (*corev1.Tenant, error) {
	return s.updateSpec(ctx, in, false)
}

func (s *DB) UpdateObject(ctx context.Context, in *corev1.Tenant) (*corev1.Tenant, error) {
	return s.updateSpec(ctx, in, true)
}

func (s *DB) updateSpec(ctx context.Context, in *corev1.Tenant, requireCAS bool) (*corev1.Tenant, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	if requireCAS && (in.Metadata.UID == "" || in.Metadata.ResourceVersion == "") {
		return nil, db.ErrConflict
	}
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	existing, err := s.Get(ctx, cp.Metadata.Name)
	if err != nil {
		return nil, err
	}
	if (cp.Metadata.UID != "" && cp.Metadata.UID != existing.Metadata.UID) ||
		(cp.Metadata.ResourceVersion != "" && cp.Metadata.ResourceVersion != existing.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	oldRV, _ := pgutil.ParseVersion(existing.Metadata.ResourceVersion)
	newRV := oldRV + 1
	cp.Metadata.UID = existing.Metadata.UID
	cp.Metadata.CreationTimestamp = existing.Metadata.CreationTimestamp
	cp.Metadata.Generation = existing.Metadata.Generation
	if !reflect.DeepEqual(existing.Spec, cp.Spec) {
		cp.Metadata.Generation++
	}
	return s.commitSpecUpdate(ctx, &cp, oldRV, newRV, requireCAS || in.Metadata.ResourceVersion != "")
}

func (s *DB) commitSpecUpdate(ctx context.Context, cp *corev1.Tenant, oldRV, newRV int64, checkRV bool) (*corev1.Tenant, error) {
	rec, _ := toRecords(cp, newRV, 1)
	quotaJSON, _ := pgutil.MarshalJSONCol(rec.Quota)
	unlimJSON, _ := pgutil.MarshalJSONCol(rec.UnlimitedQuota)
	rulesJSON, _ := pgutil.MarshalJSONCol(rec.AlertRules)
	chJSON, _ := pgutil.MarshalJSONCol(rec.AlertChannels)
	platChJSON, _ := pgutil.MarshalJSONCol(rec.PlatformAlertChannels)
	silJSON, _ := pgutil.MarshalJSONCol(rec.AlertSilences)
	platSilJSON, _ := pgutil.MarshalJSONCol(rec.PlatformAlertSilences)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&corev1.TenantRecord{}).Where("name = ? AND uid = ?", rec.Name, rec.UID)
	if checkRV {
		tx = tx.Where("resource_version = ?", oldRV)
	}
	res := tx.Updates(map[string]any{
		"display_name": rec.DisplayName, "type": rec.Type, "owner": rec.Owner, "billing_mode": rec.BillingMode,
		"quota": quotaJSON, "unlimited_quota": unlimJSON, "alert_rules": rulesJSON,
		"alert_channels": chJSON, "platform_alert_channels": platChJSON,
		"alert_silences": silJSON, "platform_alert_silences": platSilJSON,
		"resource_version": newRV, "generation": rec.Generation, "deleted_at": rec.DeletedAt,
		"finalizers": finJSON, "labels": lblJSON, "annotations": annJSON,
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, rec.Name)
}

// UpdateStatus updates only tenant_states without touching tenants or resource_version.
func (s *DB) UpdateStatus(ctx context.Context, in *corev1.Tenant) (*corev1.Tenant, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	existing, err := s.Get(ctx, in.Metadata.Name)
	if err != nil {
		return nil, err
	}
	if (in.Metadata.UID != "" && in.Metadata.UID != existing.Metadata.UID) ||
		(in.Metadata.StatusVersion != "" && in.Metadata.StatusVersion != existing.Metadata.StatusVersion) {
		return nil, db.ErrConflict
	}
	oldSV, _ := pgutil.ParseVersion(existing.Metadata.StatusVersion)
	newSV := oldSV + 1
	phase := in.Status.Phase
	if phase == "" {
		phase = corev1.TenantPhaseActive
	}
	usageJSON, _ := pgutil.MarshalJSONCol(nonNilResourceList(in.Status.Usage))
	condsJSON, _ := pgutil.MarshalJSONCol(pgutil.NonNilConditions(in.Status.Conditions))
	var subVal any
	if in.Status.Subscription != nil {
		subVal, _ = pgutil.MarshalJSONCol(in.Status.Subscription)
	}
	tx := s.orm.WithContext(ctx).Model(&corev1.TenantStateRecord{}).Where("tenant_uid = ?", existing.Metadata.UID)
	if in.Metadata.StatusVersion != "" {
		tx = tx.Where("version = ?", oldSV)
	}
	res := tx.Updates(map[string]any{
		"version":             newSV,
		"observed_generation": in.Status.ObservedGeneration,
		"phase":               string(phase),
		"usage":               usageJSON,
		"subscription":        subVal,
		"conditions":          condsJSON,
		"updated_at":          time.Now().UTC(),
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, existing.Metadata.Name)
}
