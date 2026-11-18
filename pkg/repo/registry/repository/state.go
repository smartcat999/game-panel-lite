package repository

import (
	"context"
	"errors"
	"reflect"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

func (s *DB) Update(ctx context.Context, in *registryv1.Repository) (*registryv1.Repository, error) {
	return s.updateSpec(ctx, in, false)
}

func (s *DB) UpdateObject(ctx context.Context, in *registryv1.Repository) (*registryv1.Repository, error) {
	return s.updateSpec(ctx, in, true)
}

func (s *DB) updateSpec(ctx context.Context, in *registryv1.Repository, requireCAS bool) (*registryv1.Repository, error) {
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
	return s.commitSpecUpdate(ctx, &cp, oldRV, newRV)
}

func (s *DB) commitSpecUpdate(ctx context.Context, cp *registryv1.Repository, oldRV, newRV int64) (*registryv1.Repository, error) {
	rec, _ := toRecords(cp, newRV, 1)
	membersJSON, _ := pgutil.MarshalJSONCol(rec.MemberRepos)
	retJSON, _ := pgutil.MarshalJSONCol(rec.RetentionPolicy)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&registryv1.RepositoryRecord{}).
		Where("name = ? AND uid = ? AND resource_version = ?", rec.Name, rec.UID, oldRV)
	res := tx.Updates(map[string]any{
		"display_name": rec.DisplayName, "description": rec.Description,
		"mode": rec.Mode, "format": rec.Format, "upstream_name": rec.UpstreamName,
		"member_repos": membersJSON, "public_read": rec.PublicRead, "retention_policy": retJSON,
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

// UpdateStatus updates only registry_repository_states without touching resource_version.
func (s *DB) UpdateStatus(ctx context.Context, in *registryv1.Repository) (*registryv1.Repository, error) {
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
		phase = registryv1.RepositoryPhaseActive
	}
	condsJSON, _ := pgutil.MarshalJSONCol(pgutil.NonNilConditions(in.Status.Conditions))
	tx := s.orm.WithContext(ctx).Model(&registryv1.RepositoryStateRecord{}).
		Where("repository_uid = ? AND version = ?", existing.Metadata.UID, oldSV)
	res := tx.Updates(map[string]any{
		"version":             newSV,
		"observed_generation": in.Status.ObservedGeneration,
		"phase":               string(phase),
		"package_count":       in.Status.PackageCount,
		"artifact_count":      in.Status.ArtifactCount,
		"storage_bytes":       in.Status.StorageBytes,
		"last_synced_at":      in.Status.LastSyncedAt,
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
