package user

import (
	"context"
	"errors"
	"time"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

func (s *DB) Update(ctx context.Context, in *iamv1.User) (*iamv1.User, error) {
	return s.updateSpec(ctx, in, false)
}

func (s *DB) UpdateObject(ctx context.Context, in *iamv1.User) (*iamv1.User, error) {
	return s.updateSpec(ctx, in, true)
}

func (s *DB) updateSpec(ctx context.Context, in *iamv1.User, requireCAS bool) (*iamv1.User, error) {
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
	if existing.Spec != cp.Spec {
		cp.Metadata.Generation++
	}
	return s.commitSpecUpdate(ctx, &cp, oldRV, newRV, requireCAS || in.Metadata.ResourceVersion != "")
}

func (s *DB) commitSpecUpdate(ctx context.Context, cp *iamv1.User, oldRV, newRV int64, checkRV bool) (*iamv1.User, error) {
	rec, _ := toRecords(cp, newRV, 1)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&iamv1.UserRecord{}).Where("name = ? AND uid = ?", rec.Name, rec.UID)
	if checkRV {
		tx = tx.Where("resource_version = ?", oldRV)
	}
	res := tx.Updates(map[string]any{
		"email": rec.Email, "display_name": rec.DisplayName, "password_hash": rec.PasswordHash,
		"state": rec.State, "resource_version": newRV, "generation": rec.Generation,
		"deleted_at": rec.DeletedAt, "finalizers": finJSON, "labels": lblJSON, "annotations": annJSON,
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, rec.Name)
}

// UpdateStatus updates only user_states without touching users or resource_version.
func (s *DB) UpdateStatus(ctx context.Context, in *iamv1.User) (*iamv1.User, error) {
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
		phase = existing.Spec.State
	}
	condsJSON, _ := pgutil.MarshalJSONCol(pgutil.NonNilConditions(in.Status.Conditions))
	tx := s.orm.WithContext(ctx).Model(&iamv1.UserStateRecord{}).Where("user_uid = ?", existing.Metadata.UID)
	if in.Metadata.StatusVersion != "" {
		tx = tx.Where("version = ?", oldSV)
	}
	res := tx.Updates(map[string]any{
		"version":             newSV,
		"observed_generation": in.Status.ObservedGeneration,
		"phase":               string(phase),
		"default_tenant":      in.Status.DefaultTenant,
		"last_login_time":     in.Status.LastLoginTime,
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
