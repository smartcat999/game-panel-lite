package artifact

import (
	"context"
	"errors"
	"reflect"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

func (s *DB) Update(ctx context.Context, in *registryv1.Artifact) (*registryv1.Artifact, error) {
	return s.updateSpec(ctx, in, false)
}

func (s *DB) UpdateObject(ctx context.Context, in *registryv1.Artifact) (*registryv1.Artifact, error) {
	return s.updateSpec(ctx, in, true)
}

func (s *DB) updateSpec(ctx context.Context, in *registryv1.Artifact, requireCAS bool) (*registryv1.Artifact, error) {
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

func (s *DB) commitSpecUpdate(ctx context.Context, cp *registryv1.Artifact, oldRV, newRV int64) (*registryv1.Artifact, error) {
	rec, _ := toRecords(cp, newRV, 1)
	gvJSON, _ := pgutil.MarshalJSONCol(rec.GameVersions)
	ldrJSON, _ := pgutil.MarshalJSONCol(rec.Loaders)
	descJSON, _ := pgutil.MarshalJSONCol(rec.SpecDescriptor)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&registryv1.ArtifactRecord{}).
		Where("name = ? AND uid = ? AND resource_version = ?", rec.Name, rec.UID, oldRV)
	res := tx.Updates(map[string]any{
		"source_url": rec.SourceURL, "archive_format": rec.ArchiveFormat, "target_path": rec.TargetPath,
		"game_versions": gvJSON, "loaders": ldrJSON, "spec_descriptor": descJSON,
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

// UpdateStatus updates only registry_artifact_states without touching resource_version.
func (s *DB) UpdateStatus(ctx context.Context, in *registryv1.Artifact) (*registryv1.Artifact, error) {
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
	return s.commitStatusUpdate(ctx, existing.Metadata.Name, existing.Metadata.UID, in, oldSV, newSV)
}

func (s *DB) commitStatusUpdate(ctx context.Context, name, uid string, in *registryv1.Artifact, oldSV, newSV int64) (*registryv1.Artifact, error) {
	phase := in.Status.Phase
	if phase == "" {
		phase = registryv1.ArtifactPhasePending
	}
	digest := ""
	if in.Status.Descriptor != nil {
		digest = in.Status.Descriptor.Digest
	}
	descJSON, _ := pgutil.MarshalJSONCol(in.Status.Descriptor)
	modsJSON, _ := pgutil.MarshalJSONCol(pgutil.NonNilStrings(in.Status.ModIDs))
	mapsJSON, _ := pgutil.MarshalJSONCol(pgutil.NonNilStrings(in.Status.MapFolders))
	depsJSON, _ := pgutil.MarshalJSONCol(nonNilDeps(in.Status.Dependencies))
	schemaJSON, _ := pgutil.MarshalJSONCol(nonNilConfigSchema(in.Status.ConfigSchema))
	condsJSON, _ := pgutil.MarshalJSONCol(pgutil.NonNilConditions(in.Status.Conditions))
	tx := s.orm.WithContext(ctx).Model(&registryv1.ArtifactStateRecord{}).
		Where("artifact_uid = ? AND version = ?", uid, oldSV)
	res := tx.Updates(map[string]any{
		"version": newSV, "observed_generation": in.Status.ObservedGeneration,
		"phase": string(phase), "stage": string(in.Status.Stage), "progress": in.Status.Progress,
		"processed_bytes": in.Status.ProcessedBytes, "total_bytes": in.Status.TotalBytes,
		"speed_bytes_per_sec": in.Status.SpeedBytesPerSec, "started_at": in.Status.StartedAt,
		"build": in.Status.Build, "descriptor": descJSON, "blob_digest": digest, "mod_ids": modsJSON,
		"map_folders": mapsJSON, "dependencies": depsJSON, "config_schema": schemaJSON,
		"published_at": in.Status.PublishedAt, "changelog": in.Status.Changelog, "upstream_url": in.Status.UpstreamURL,
		"synced_at": in.Status.SyncedAt,
		"message":   in.Status.Message, "conditions": condsJSON, "updated_at": time.Now().UTC(),
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, name)
}

func (s *DB) SummaryByRepository(ctx context.Context, repository string) (int64, int64, error) {
	if s == nil || s.orm == nil {
		return 0, 0, errors.New("db is nil")
	}
	var out struct {
		Count int64 `gorm:"column:cnt"`
		Bytes int64 `gorm:"column:total_bytes"`
	}
	sql := `SELECT COUNT(a.uid) AS cnt,
	               COALESCE(SUM(COALESCE((st.descriptor->>'sizeBytes')::bigint, 0)), 0) AS total_bytes
	        FROM registry_artifacts a
	        LEFT JOIN registry_artifact_states st ON st.artifact_uid = a.uid
	        WHERE a.repository = ? AND a.deleted_at IS NULL`
	if err := s.orm.WithContext(ctx).Raw(sql, repository).Scan(&out).Error; err != nil {
		return 0, 0, err
	}
	return out.Count, out.Bytes, nil
}
