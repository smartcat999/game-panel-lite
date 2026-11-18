package artifact

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func nonNilDeps(in []registryv1.PackageDependency) []registryv1.PackageDependency {
	if len(in) == 0 {
		return []registryv1.PackageDependency{}
	}
	return append([]registryv1.PackageDependency(nil), in...)
}

func nonNilConfigSchema(in []registryv1.AddonConfigField) []registryv1.AddonConfigField {
	if len(in) == 0 {
		return []registryv1.AddonConfigField{}
	}
	return append([]registryv1.AddonConfigField(nil), in...)
}

func toRecords(in *registryv1.Artifact, rv, sv int64) (registryv1.ArtifactRecord, registryv1.ArtifactStateRecord) {
	phase := in.Status.Phase
	if phase == "" {
		if in.Spec.Descriptor != nil && in.Spec.Descriptor.Digest != "" {
			phase = registryv1.ArtifactPhaseReady
		} else {
			phase = registryv1.ArtifactPhasePending
		}
	}
	desc := in.Status.Descriptor
	if desc == nil && in.Spec.Descriptor != nil {
		cp := *in.Spec.Descriptor
		desc = &cp
	}
	digest := ""
	if desc != nil {
		digest = desc.Digest
	}
	rec := registryv1.ArtifactRecord{
		Name: in.Metadata.Name, UID: in.Metadata.UID,
		Repository: in.Spec.Repository, Package: in.Spec.Package, Version: in.Spec.Version,
		SourceURL: in.Spec.SourceURL, ArchiveFormat: in.Spec.ArchiveFormat, TargetPath: in.Spec.TargetPath,
		GameVersions: pgutil.NonNilStrings(in.Spec.GameVersions), Loaders: pgutil.NonNilStrings(in.Spec.Loaders),
		SpecDescriptor:  in.Spec.Descriptor,
		ResourceVersion: rv, Generation: in.Metadata.Generation,
		CreatedAt: in.Metadata.CreationTimestamp, DeletedAt: in.Metadata.DeletionTimestamp,
		Finalizers: pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:     pgutil.NonNilMap(in.Metadata.Labels), Annotations: pgutil.NonNilMap(in.Metadata.Annotations),
	}
	st := registryv1.ArtifactStateRecord{
		ArtifactUID: in.Metadata.UID, Version: sv, ObservedGeneration: in.Status.ObservedGeneration,
		Phase: string(phase), Stage: string(in.Status.Stage), Progress: in.Status.Progress,
		ProcessedBytes: in.Status.ProcessedBytes, TotalBytes: in.Status.TotalBytes,
		SpeedBytesPerSec: in.Status.SpeedBytesPerSec, StartedAt: in.Status.StartedAt, Build: in.Status.Build,
		Descriptor: desc, BlobDigest: digest,
		ModIDs: pgutil.NonNilStrings(in.Status.ModIDs), MapFolders: pgutil.NonNilStrings(in.Status.MapFolders),
		Dependencies: nonNilDeps(in.Status.Dependencies),
		ConfigSchema: nonNilConfigSchema(in.Status.ConfigSchema),
		PublishedAt:  in.Status.PublishedAt, Changelog: in.Status.Changelog, UpstreamURL: in.Status.UpstreamURL,
		SyncedAt: in.Status.SyncedAt,
		Message:  in.Status.Message, Conditions: pgutil.NonNilConditions(in.Status.Conditions),
		UpdatedAt: time.Now().UTC(),
	}
	return rec, st
}

func fromRecords(rec registryv1.ArtifactRecord, st registryv1.ArtifactStateRecord, hasState bool) *registryv1.Artifact {
	out := &registryv1.Artifact{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: registryv1.KindArtifact},
		Metadata: metav1.ObjectMeta{
			Name: rec.Name, UID: rec.UID, ResourceVersion: pgutil.FormatVersion(rec.ResourceVersion),
			Generation: rec.Generation, CreationTimestamp: rec.CreatedAt, DeletionTimestamp: rec.DeletedAt,
		},
		Spec: registryv1.ArtifactSpec{
			Repository: rec.Repository, Package: rec.Package, Version: rec.Version,
			SourceURL: rec.SourceURL, ArchiveFormat: rec.ArchiveFormat, TargetPath: rec.TargetPath,
			Descriptor: rec.SpecDescriptor,
		},
		Status: registryv1.ArtifactStatus{Phase: registryv1.ArtifactPhasePending},
	}
	hydrateArtifactSlices(out, rec, st, hasState)
	return out
}

func hydrateArtifactSlices(out *registryv1.Artifact, rec registryv1.ArtifactRecord, st registryv1.ArtifactStateRecord, hasState bool) {
	if len(rec.GameVersions) > 0 {
		out.Spec.GameVersions = append([]string(nil), rec.GameVersions...)
	}
	if len(rec.Loaders) > 0 {
		out.Spec.Loaders = append([]string(nil), rec.Loaders...)
	}
	if hasState {
		out.Metadata.StatusVersion = pgutil.FormatVersion(st.Version)
		out.Status = registryv1.ArtifactStatus{
			ObservedGeneration: st.ObservedGeneration,
			Phase:              registryv1.ArtifactPhase(st.Phase),
			Stage:              registryv1.ArtifactSyncStage(st.Stage),
			Progress:           st.Progress,
			ProcessedBytes:     st.ProcessedBytes,
			TotalBytes:         st.TotalBytes,
			SpeedBytesPerSec:   st.SpeedBytesPerSec,
			StartedAt:          st.StartedAt,
			Build:              st.Build,
			Descriptor:         st.Descriptor,
			PublishedAt:        st.PublishedAt,
			Changelog:          st.Changelog,
			UpstreamURL:        st.UpstreamURL,
			SyncedAt:           st.SyncedAt,
			Message:            st.Message,
			Conditions:         append([]metav1.Condition(nil), st.Conditions...),
		}
		if len(st.ModIDs) > 0 {
			out.Status.ModIDs = append([]string(nil), st.ModIDs...)
		}
		if len(st.MapFolders) > 0 {
			out.Status.MapFolders = append([]string(nil), st.MapFolders...)
		}
		if len(st.Dependencies) > 0 {
			out.Status.Dependencies = append([]registryv1.PackageDependency(nil), st.Dependencies...)
		}
		if len(st.ConfigSchema) > 0 {
			out.Status.ConfigSchema = append([]registryv1.AddonConfigField(nil), st.ConfigSchema...)
		}
	}
	if len(rec.Finalizers) > 0 {
		out.Metadata.Finalizers = append([]string(nil), rec.Finalizers...)
	}
	if len(rec.Labels) > 0 {
		out.Metadata.Labels = pgutil.NonNilMap(rec.Labels)
	}
	if len(rec.Annotations) > 0 {
		out.Metadata.Annotations = pgutil.NonNilMap(rec.Annotations)
	}
}

func (s *DB) Create(ctx context.Context, in *registryv1.Artifact) (*registryv1.Artifact, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	pgutil.InitCreateMeta(&cp.Metadata)
	cp.Metadata.StatusVersion = "1"
	rec, st := toRecords(&cp, 1, 1)
	tx := s.orm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer tx.Rollback()
	res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoNothing: true}).Create(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrAlreadyExists
	}
	if err := tx.Create(&st).Error; err != nil {
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return fromRecords(rec, st, true), nil
}

func (s *DB) Get(ctx context.Context, name string) (*registryv1.Artifact, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec registryv1.ArtifactRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	var st registryv1.ArtifactStateRecord
	err := s.orm.WithContext(ctx).Where("artifact_uid = ?", rec.UID).Take(&st).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return fromRecords(rec, st, err == nil), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Artifact], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&registryv1.ArtifactRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("registry_artifacts.name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "registry_artifacts.name", val)
		case "spec.repository", "repository":
			tx = tx.Where("registry_artifacts.repository = ?", val)
		case "spec.package", "package":
			tx = tx.Where("registry_artifacts.package = ?", val)
		case "spec.version", "version":
			tx = tx.Where("registry_artifacts.version = ?", val)
		case "status.phase":
			tx = tx.Joins("JOIN registry_artifact_states ON registry_artifact_states.artifact_uid = registry_artifacts.uid").
				Where("registry_artifact_states.phase = ?", val)
		default:
			return nil, fmt.Errorf("unsupported artifact fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelectorCol(tx, "registry_artifacts.labels", opts.LabelSelector)
	rows, total, err := pgutil.Paginate[registryv1.ArtifactRecord](tx, "registry_artifacts.created_at DESC, registry_artifacts.name ASC", opts)
	if err != nil {
		return nil, err
	}
	return s.hydrateList(ctx, rows, total)
}

func (s *DB) hydrateList(ctx context.Context, rows []registryv1.ArtifactRecord, total int) (*metav1.List[*registryv1.Artifact], error) {
	stateMap := make(map[string]registryv1.ArtifactStateRecord, len(rows))
	if len(rows) > 0 {
		uids := make([]string, len(rows))
		for i, r := range rows {
			uids[i] = r.UID
		}
		var states []registryv1.ArtifactStateRecord
		if err := s.orm.WithContext(ctx).Where("artifact_uid IN ?", uids).Find(&states).Error; err != nil {
			return nil, err
		}
		for _, st := range states {
			stateMap[st.ArtifactUID] = st
		}
	}
	items := make([]*registryv1.Artifact, 0, len(rows))
	for _, row := range rows {
		st, ok := stateMap[row.UID]
		items = append(items, fromRecords(row, st, ok))
	}
	return &metav1.List[*registryv1.Artifact]{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: "ArtifactList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Delete(ctx context.Context, name string) error {
	return s.DeleteIfUID(ctx, name, "")
}

func (s *DB) DeleteIfUID(ctx context.Context, name, expectedUID string) error {
	if s == nil {
		return errors.New("db is nil")
	}
	return pgutil.DeleteByCondition(ctx, s.orm, "registry_artifacts", "name = ?", []any{name}, expectedUID)
}
