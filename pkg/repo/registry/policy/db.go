package policy

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
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

func toRecord(in *registryv1.ReplicationPolicy, rv int64) registryv1.ReplicationPolicyRecord {
	return registryv1.ReplicationPolicyRecord{
		Name: in.Metadata.Name, UID: in.Metadata.UID,
		Repository: in.Repository, Enabled: in.Enabled, Mode: in.Mode,
		IntervalMinutes: in.IntervalMinutes, TopN: in.TopN,
		Packages: pgutil.NonNilStrings(in.Packages), GameVersion: in.GameVersion,
		LastRunAt: in.LastRunAt, LastMessage: in.LastMessage,
		ResourceVersion: rv, Generation: in.Metadata.Generation,
		CreatedAt: in.Metadata.CreationTimestamp, DeletedAt: in.Metadata.DeletionTimestamp,
		Finalizers: pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:     pgutil.NonNilMap(in.Metadata.Labels), Annotations: pgutil.NonNilMap(in.Metadata.Annotations),
	}
}

func fromRecord(rec registryv1.ReplicationPolicyRecord) *registryv1.ReplicationPolicy {
	out := &registryv1.ReplicationPolicy{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: registryv1.KindReplicationPolicy},
		Metadata: metav1.ObjectMeta{
			Name: rec.Name, UID: rec.UID, ResourceVersion: pgutil.FormatVersion(rec.ResourceVersion),
			Generation: rec.Generation, CreationTimestamp: rec.CreatedAt, DeletionTimestamp: rec.DeletedAt,
		},
		Repository: rec.Repository, Enabled: rec.Enabled, Mode: rec.Mode,
		IntervalMinutes: rec.IntervalMinutes, TopN: rec.TopN,
		GameVersion: rec.GameVersion, LastRunAt: rec.LastRunAt, LastMessage: rec.LastMessage,
	}
	if len(rec.Packages) > 0 {
		out.Packages = append([]string(nil), rec.Packages...)
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

func (s *DB) Create(ctx context.Context, in *registryv1.ReplicationPolicy) (*registryv1.ReplicationPolicy, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	pgutil.InitCreateMeta(&cp.Metadata)
	rec := toRecord(&cp, 1)
	res := s.orm.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoNothing: true}).Create(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrAlreadyExists
	}
	return fromRecord(rec), nil
}

func (s *DB) Get(ctx context.Context, name string) (*registryv1.ReplicationPolicy, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec registryv1.ReplicationPolicyRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return fromRecord(rec), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.ReplicationPolicy], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&registryv1.ReplicationPolicyRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("name = ?", val)
		case "repository":
			tx = tx.Where("repository = ?", val)
		case "enabled":
			b, err := strconv.ParseBool(val)
			if err != nil {
				return nil, fmt.Errorf("invalid boolean fieldSelector enabled=%q", val)
			}
			tx = tx.Where("enabled = ?", b)
		default:
			return nil, fmt.Errorf("unsupported replicationPolicy fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelector(tx, opts.LabelSelector)
	rows, total, err := pgutil.Paginate[registryv1.ReplicationPolicyRecord](tx, "created_at DESC, name ASC", opts)
	if err != nil {
		return nil, err
	}
	items := make([]*registryv1.ReplicationPolicy, 0, len(rows))
	for _, row := range rows {
		items = append(items, fromRecord(row))
	}
	return &metav1.List[*registryv1.ReplicationPolicy]{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: "ReplicationPolicyList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Update(ctx context.Context, in *registryv1.ReplicationPolicy) (*registryv1.ReplicationPolicy, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
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
	if cp.Metadata.ResourceVersion != "" && cp.Metadata.ResourceVersion != existing.Metadata.ResourceVersion {
		return nil, db.ErrConflict
	}
	oldRV, _ := pgutil.ParseVersion(existing.Metadata.ResourceVersion)
	newRV := oldRV + 1
	cp.Metadata.UID = existing.Metadata.UID
	cp.Metadata.CreationTimestamp = existing.Metadata.CreationTimestamp
	cp.Metadata.Generation = existing.Metadata.Generation
	if policySpecChanged(existing, &cp) {
		cp.Metadata.Generation++
	}
	return s.commitUpdate(ctx, &cp, oldRV, newRV)
}

func policySpecChanged(a, b *registryv1.ReplicationPolicy) bool {
	if a == nil || b == nil {
		return a != b
	}
	return a.Repository != b.Repository ||
		a.Enabled != b.Enabled ||
		a.Mode != b.Mode ||
		a.IntervalMinutes != b.IntervalMinutes ||
		a.TopN != b.TopN ||
		a.GameVersion != b.GameVersion ||
		!slices.Equal(a.Packages, b.Packages)
}

// RecordRunOutcome updates execution telemetry without modifying spec or bumping generation.
func (s *DB) RecordRunOutcome(ctx context.Context, name, uid string, runAt time.Time, msg string) (*registryv1.ReplicationPolicy, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	res := s.orm.WithContext(ctx).Exec(
		`UPDATE registry_replication_policies SET last_run_at = ?, last_message = ?, resource_version = resource_version + 1 WHERE name = ? AND uid = ?`,
		runAt, msg, name, uid,
	)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		if _, gErr := s.Get(ctx, name); gErr != nil {
			return nil, gErr
		}
		return nil, db.ErrConflict
	}
	return s.Get(ctx, name)
}

func (s *DB) commitUpdate(ctx context.Context, cp *registryv1.ReplicationPolicy, oldRV, newRV int64) (*registryv1.ReplicationPolicy, error) {
	rec := toRecord(cp, newRV)
	pkgsJSON, _ := pgutil.MarshalJSONCol(rec.Packages)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&registryv1.ReplicationPolicyRecord{}).
		Where("name = ? AND uid = ? AND resource_version = ?", rec.Name, rec.UID, oldRV)
	res := tx.Updates(map[string]any{
		"repository": rec.Repository, "enabled": rec.Enabled, "mode": rec.Mode,
		"interval_minutes": rec.IntervalMinutes, "top_n": rec.TopN,
		"packages": pkgsJSON, "game_version": rec.GameVersion,
		"last_run_at": rec.LastRunAt, "last_message": rec.LastMessage,
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

func (s *DB) Delete(ctx context.Context, name string) error {
	if s == nil {
		return errors.New("db is nil")
	}
	return pgutil.DeleteByCondition(ctx, s.orm, "registry_replication_policies", "name = ?", []any{name}, "")
}
