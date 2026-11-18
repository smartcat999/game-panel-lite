package globalrole

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB manages persistence for GlobalRole in global_roles.
type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func toRecord(in *iamv1.GlobalRole, rv int64) iamv1.GlobalRoleRecord {
	rules := in.Rules
	if rules == nil {
		rules = []iamv1.PolicyRule{}
	}
	return iamv1.GlobalRoleRecord{
		Name: in.Metadata.Name, UID: in.Metadata.UID, Description: in.Description,
		Rules: rules, ResourceVersion: rv, Generation: in.Metadata.Generation,
		CreatedAt: in.Metadata.CreationTimestamp, DeletedAt: in.Metadata.DeletionTimestamp,
		Finalizers: pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:     pgutil.NonNilMap(in.Metadata.Labels), Annotations: pgutil.NonNilMap(in.Metadata.Annotations),
	}
}

func fromRecord(rec iamv1.GlobalRoleRecord) *iamv1.GlobalRole {
	out := &iamv1.GlobalRole{
		TypeMeta: metav1.TypeMeta{APIVersion: iamv1.GroupVersion, Kind: "GlobalRole"},
		Metadata: metav1.ObjectMeta{
			Name: rec.Name, UID: rec.UID, ResourceVersion: pgutil.FormatVersion(rec.ResourceVersion),
			Generation: rec.Generation, CreationTimestamp: rec.CreatedAt, DeletionTimestamp: rec.DeletedAt,
		},
		Description: rec.Description,
		Rules:       append([]iamv1.PolicyRule(nil), rec.Rules...),
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

func (s *DB) Create(ctx context.Context, in *iamv1.GlobalRole) (*iamv1.GlobalRole, error) {
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
	res := s.orm.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "name"}}, DoNothing: true,
	}).Create(&rec)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrAlreadyExists
	}
	return fromRecord(rec), nil
}

func (s *DB) Get(ctx context.Context, name string) (*iamv1.GlobalRole, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec iamv1.GlobalRoleRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return fromRecord(rec), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*iamv1.GlobalRole], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&iamv1.GlobalRoleRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "name", val)
		default:
			return nil, fmt.Errorf("unsupported globalrole fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelector(tx, opts.LabelSelector)
	rows, total, err := pgutil.Paginate[iamv1.GlobalRoleRecord](tx, "created_at DESC, name ASC", opts)
	if err != nil {
		return nil, err
	}
	items := make([]*iamv1.GlobalRole, 0, len(rows))
	for _, row := range rows {
		item := fromRecord(row)
		if opts.LabelSelector != nil && !opts.LabelSelector.Matches(item.Metadata.Labels) {
			continue
		}
		items = append(items, item)
	}
	return &metav1.List[*iamv1.GlobalRole]{
		TypeMeta: metav1.TypeMeta{APIVersion: iamv1.GroupVersion, Kind: "GlobalRoleList"},
		Metadata: metav1.ListMeta{TotalCount: total},
		Items:    items,
	}, nil
}

func (s *DB) Update(ctx context.Context, in *iamv1.GlobalRole) (*iamv1.GlobalRole, error) {
	return s.update(ctx, in, false)
}

func (s *DB) UpdateObject(ctx context.Context, in *iamv1.GlobalRole) (*iamv1.GlobalRole, error) {
	return s.update(ctx, in, true)
}

func (s *DB) update(ctx context.Context, in *iamv1.GlobalRole, requireCAS bool) (*iamv1.GlobalRole, error) {
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
	if existing.Description != cp.Description || !reflect.DeepEqual(existing.Rules, cp.Rules) {
		cp.Metadata.Generation++
	}
	return s.commitUpdate(ctx, &cp, oldRV, newRV, requireCAS || in.Metadata.ResourceVersion != "")
}

func (s *DB) commitUpdate(ctx context.Context, cp *iamv1.GlobalRole, oldRV, newRV int64, checkRV bool) (*iamv1.GlobalRole, error) {
	rec := toRecord(cp, newRV)
	rulesJSON, _ := pgutil.MarshalJSONCol(rec.Rules)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&iamv1.GlobalRoleRecord{}).Where("name = ? AND uid = ?", rec.Name, rec.UID)
	if checkRV {
		tx = tx.Where("resource_version = ?", oldRV)
	}
	res := tx.Updates(map[string]any{
		"description": rec.Description, "rules": rulesJSON, "resource_version": newRV,
		"generation": rec.Generation, "deleted_at": rec.DeletedAt,
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
	return pgutil.DeleteByCondition(ctx, s.orm, "global_roles", "name = ?", []any{name}, expectedUID)
}
