package repository

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

func toRecords(in *registryv1.Repository, rv, sv int64) (registryv1.RepositoryRecord, registryv1.RepositoryStateRecord) {
	phase := in.Status.Phase
	if phase == "" {
		phase = registryv1.RepositoryPhaseActive
	}
	rec := registryv1.RepositoryRecord{
		Name: in.Metadata.Name, UID: in.Metadata.UID,
		DisplayName: in.Spec.DisplayName, Description: in.Spec.Description,
		Mode: in.Spec.Mode, Format: in.Spec.Format, UpstreamName: in.Spec.UpstreamName,
		MemberRepos: pgutil.NonNilStrings(in.Spec.MemberRepos), PublicRead: in.Spec.PublicRead,
		RetentionPolicy: in.Spec.RetentionPolicy,
		ResourceVersion: rv, Generation: in.Metadata.Generation,
		CreatedAt: in.Metadata.CreationTimestamp, DeletedAt: in.Metadata.DeletionTimestamp,
		Finalizers: pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:     pgutil.NonNilMap(in.Metadata.Labels), Annotations: pgutil.NonNilMap(in.Metadata.Annotations),
	}
	st := registryv1.RepositoryStateRecord{
		RepositoryUID: in.Metadata.UID, Version: sv, ObservedGeneration: in.Status.ObservedGeneration,
		Phase: string(phase), PackageCount: in.Status.PackageCount,
		ArtifactCount: in.Status.ArtifactCount, StorageBytes: in.Status.StorageBytes,
		LastSyncedAt: in.Status.LastSyncedAt,
		Conditions:   pgutil.NonNilConditions(in.Status.Conditions), UpdatedAt: time.Now().UTC(),
	}
	return rec, st
}

func fromRecords(rec registryv1.RepositoryRecord, st registryv1.RepositoryStateRecord, hasState bool) *registryv1.Repository {
	out := &registryv1.Repository{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: registryv1.KindRepository},
		Metadata: metav1.ObjectMeta{
			Name: rec.Name, UID: rec.UID, ResourceVersion: pgutil.FormatVersion(rec.ResourceVersion),
			Generation: rec.Generation, CreationTimestamp: rec.CreatedAt, DeletionTimestamp: rec.DeletedAt,
		},
		Spec: registryv1.RepositorySpec{
			DisplayName: rec.DisplayName, Description: rec.Description,
			Mode: rec.Mode, Format: rec.Format, UpstreamName: rec.UpstreamName,
			PublicRead: rec.PublicRead, RetentionPolicy: rec.RetentionPolicy,
		},
		Status: registryv1.RepositoryStatus{Phase: registryv1.RepositoryPhaseActive},
	}
	if len(rec.MemberRepos) > 0 {
		out.Spec.MemberRepos = append([]string(nil), rec.MemberRepos...)
	}
	if hasState {
		out.Metadata.StatusVersion = pgutil.FormatVersion(st.Version)
		out.Status = registryv1.RepositoryStatus{
			ObservedGeneration: st.ObservedGeneration,
			Phase:              registryv1.RepositoryPhase(st.Phase),
			PackageCount:       st.PackageCount,
			ArtifactCount:      st.ArtifactCount,
			StorageBytes:       st.StorageBytes,
			LastSyncedAt:       st.LastSyncedAt,
			Conditions:         append([]metav1.Condition(nil), st.Conditions...),
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
	return out
}

func (s *DB) Create(ctx context.Context, in *registryv1.Repository) (*registryv1.Repository, error) {
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

func (s *DB) Get(ctx context.Context, name string) (*registryv1.Repository, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec registryv1.RepositoryRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	var st registryv1.RepositoryStateRecord
	err := s.orm.WithContext(ctx).Where("repository_uid = ?", rec.UID).Take(&st).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return fromRecords(rec, st, err == nil), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Repository], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&registryv1.RepositoryRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("registry_repositories.name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "registry_repositories.name", val)
		case "spec.mode", "mode":
			tx = tx.Where("registry_repositories.mode = ?", val)
		case "spec.format", "format":
			tx = tx.Where("registry_repositories.format = ?", val)
		case "spec.upstreamName", "upstreamName":
			tx = tx.Where("registry_repositories.upstream_name = ?", val)
		case "status.phase":
			tx = tx.Joins("JOIN registry_repository_states ON registry_repository_states.repository_uid = registry_repositories.uid").
				Where("registry_repository_states.phase = ?", val)
		default:
			return nil, fmt.Errorf("unsupported repository fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelectorCol(tx, "registry_repositories.labels", opts.LabelSelector)
	rows, total, err := pgutil.Paginate[registryv1.RepositoryRecord](tx, "registry_repositories.created_at DESC, registry_repositories.name ASC", opts)
	if err != nil {
		return nil, err
	}
	return s.hydrateList(ctx, rows, total)
}

func (s *DB) hydrateList(ctx context.Context, rows []registryv1.RepositoryRecord, total int) (*metav1.List[*registryv1.Repository], error) {
	stateMap := make(map[string]registryv1.RepositoryStateRecord, len(rows))
	if len(rows) > 0 {
		uids := make([]string, len(rows))
		for i, r := range rows {
			uids[i] = r.UID
		}
		var states []registryv1.RepositoryStateRecord
		if err := s.orm.WithContext(ctx).Where("repository_uid IN ?", uids).Find(&states).Error; err != nil {
			return nil, err
		}
		for _, st := range states {
			stateMap[st.RepositoryUID] = st
		}
	}
	items := make([]*registryv1.Repository, 0, len(rows))
	for _, row := range rows {
		st, ok := stateMap[row.UID]
		items = append(items, fromRecords(row, st, ok))
	}
	return &metav1.List[*registryv1.Repository]{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: "RepositoryList"},
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
	return pgutil.DeleteByCondition(ctx, s.orm, "registry_repositories", "name = ?", []any{name}, expectedUID)
}
