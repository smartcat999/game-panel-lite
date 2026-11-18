package runtimeclass

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB manages persistence for cluster-scoped RuntimeClass objects.
type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func toRecord(in *corev1.RuntimeClass, rv int64) corev1.RuntimeClassRecord {
	var sched *corev1.RuntimeClassScheduling
	if in.Scheduling != nil {
		cp := *in.Scheduling
		if in.Scheduling.NodeSelector != nil {
			cp.NodeSelector = pgutil.NonNilMap(in.Scheduling.NodeSelector)
		}
		sched = &cp
	}
	return corev1.RuntimeClassRecord{
		Name:            in.Metadata.Name,
		UID:             in.Metadata.UID,
		Handler:         in.Handler,
		IsDefault:       in.IsDefault,
		Parameters:      pgutil.NonNilMap(in.Parameters),
		Scheduling:      sched,
		ResourceVersion: rv,
		Generation:      in.Metadata.Generation,
		CreatedAt:       in.Metadata.CreationTimestamp,
		DeletedAt:       in.Metadata.DeletionTimestamp,
		Finalizers:      pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:          pgutil.NonNilMap(in.Metadata.Labels),
		Annotations:     pgutil.NonNilMap(in.Metadata.Annotations),
	}
}

func fromRecord(rec corev1.RuntimeClassRecord) *corev1.RuntimeClass {
	out := &corev1.RuntimeClass{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "RuntimeClass"},
		Metadata: metav1.ObjectMeta{
			Name:              rec.Name,
			UID:               rec.UID,
			ResourceVersion:   pgutil.FormatVersion(rec.ResourceVersion),
			Generation:        rec.Generation,
			CreationTimestamp: rec.CreatedAt,
			DeletionTimestamp: rec.DeletedAt,
		},
		Handler:    rec.Handler,
		IsDefault:  rec.IsDefault,
		Scheduling: rec.Scheduling,
	}
	if len(rec.Parameters) > 0 {
		out.Parameters = pgutil.NonNilMap(rec.Parameters)
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

func (s *DB) Create(ctx context.Context, in *corev1.RuntimeClass) (*corev1.RuntimeClass, error) {
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

func (s *DB) Get(ctx context.Context, name string) (*corev1.RuntimeClass, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec corev1.RuntimeClassRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return fromRecord(rec), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.RuntimeClass], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&corev1.RuntimeClassRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "name", val)
		case "handler", "spec.handler":
			tx = tx.Where("handler = ?", val)
		case "isDefault", "spec.isDefault":
			b, err := strconv.ParseBool(val)
			if err != nil {
				return nil, fmt.Errorf("invalid boolean fieldSelector %s=%q", field, val)
			}
			tx = tx.Where("is_default = ?", b)
		default:
			return nil, fmt.Errorf("unsupported runtimeclass fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelectorCol(tx, "labels", opts.LabelSelector)
	rows, total, err := pgutil.Paginate[corev1.RuntimeClassRecord](tx, "created_at DESC, name ASC", opts)
	if err != nil {
		return nil, err
	}
	items := make([]*corev1.RuntimeClass, 0, len(rows))
	for _, row := range rows {
		item := fromRecord(row)
		if opts.LabelSelector != nil && !opts.LabelSelector.Matches(item.Metadata.Labels) {
			continue
		}
		items = append(items, item)
	}
	return &metav1.List[*corev1.RuntimeClass]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "RuntimeClassList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Update(ctx context.Context, in *corev1.RuntimeClass) (*corev1.RuntimeClass, error) {
	return s.update(ctx, in, false)
}

func (s *DB) UpdateObject(ctx context.Context, in *corev1.RuntimeClass) (*corev1.RuntimeClass, error) {
	return s.update(ctx, in, true)
}

func (s *DB) update(ctx context.Context, in *corev1.RuntimeClass, requireCAS bool) (*corev1.RuntimeClass, error) {
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
	if existing.Handler != cp.Handler || existing.IsDefault != cp.IsDefault ||
		!reflect.DeepEqual(existing.Parameters, cp.Parameters) || !reflect.DeepEqual(existing.Scheduling, cp.Scheduling) {
		cp.Metadata.Generation++
	}
	return s.commitUpdate(ctx, &cp, oldRV, newRV, requireCAS || in.Metadata.ResourceVersion != "")
}

func (s *DB) commitUpdate(ctx context.Context, cp *corev1.RuntimeClass, oldRV, newRV int64, checkRV bool) (*corev1.RuntimeClass, error) {
	rec := toRecord(cp, newRV)
	paramJSON, _ := pgutil.MarshalJSONCol(rec.Parameters)
	var schedCol any
	if rec.Scheduling != nil {
		schedJSON, _ := pgutil.MarshalJSONCol(rec.Scheduling)
		schedCol = schedJSON
	}
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&corev1.RuntimeClassRecord{}).Where("name = ? AND uid = ?", rec.Name, rec.UID)
	if checkRV {
		tx = tx.Where("resource_version = ?", oldRV)
	}
	res := tx.Updates(map[string]any{
		"handler": rec.Handler, "is_default": rec.IsDefault, "parameters": paramJSON, "scheduling": schedCol,
		"resource_version": newRV, "generation": rec.Generation, "deleted_at": rec.DeletedAt,
		"finalizers": finJSON, "labels": lblJSON, "annotations": annJSON,
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return fromRecord(rec), nil
}

func (s *DB) Delete(ctx context.Context, name string) error {
	return s.DeleteIfUID(ctx, name, "")
}

func (s *DB) DeleteIfUID(ctx context.Context, name, expectedUID string) error {
	if s == nil {
		return errors.New("db is nil")
	}
	return pgutil.DeleteByCondition(ctx, s.orm, "runtime_classes", "name = ?", []any{name}, expectedUID)
}
