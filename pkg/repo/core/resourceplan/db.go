package resourceplan

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB provides PostgreSQL persistence for cluster-scoped ResourcePlans.
type DB struct {
	orm *gorm.DB
}

// NewDB constructs a ResourcePlan repository backed by PostgreSQL.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{orm: db.OpenGORM(pool)}
}

func toRecord(in *corev1.ResourcePlan, rv int64) corev1.ResourcePlanRecord {
	prices := append([]corev1.ResourcePlanPrice(nil), in.Prices...)
	if prices == nil {
		prices = []corev1.ResourcePlanPrice{}
	}
	return corev1.ResourcePlanRecord{
		Name:            in.Metadata.Name,
		UID:             in.Metadata.UID,
		DisplayName:     in.DisplayName,
		Description:     in.Description,
		Kind:            in.PlanKind,
		Quota:           nonNilQuota(in.Quota),
		SortOrder:       in.SortOrder,
		Published:       in.Published,
		Prices:          prices,
		CreatedAt:       in.Metadata.CreationTimestamp,
		UpdatedAt:       time.Now().UTC(),
		ResourceVersion: rv,
		Labels:          pgutil.NonNilMap(in.Metadata.Labels),
		Annotations:     pgutil.NonNilMap(in.Metadata.Annotations),
	}
}

func fromRecord(rec corev1.ResourcePlanRecord) *corev1.ResourcePlan {
	out := &corev1.ResourcePlan{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindResourcePlan},
		Metadata: metav1.ObjectMeta{
			Name:              rec.Name,
			UID:               rec.UID,
			ResourceVersion:   pgutil.FormatVersion(rec.ResourceVersion),
			Generation:        1,
			CreationTimestamp: rec.CreatedAt,
		},
		DisplayName: rec.DisplayName,
		Description: rec.Description,
		PlanKind:    rec.Kind,
		Quota:       nonNilQuota(rec.Quota),
		SortOrder:   rec.SortOrder,
		Published:   rec.Published,
		Prices:      append([]corev1.ResourcePlanPrice(nil), rec.Prices...),
	}
	if len(rec.Labels) > 0 {
		out.Metadata.Labels = pgutil.NonNilMap(rec.Labels)
	}
	if len(rec.Annotations) > 0 {
		out.Metadata.Annotations = pgutil.NonNilMap(rec.Annotations)
	}
	return out
}

func nonNilQuota(in corev1.ResourceList) corev1.ResourceList {
	out := make(corev1.ResourceList, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Create inserts a new ResourcePlan record.
func (s *DB) Create(ctx context.Context, in *corev1.ResourcePlan) (*corev1.ResourcePlan, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	cp := *in
	cp.DefaultMeta()
	pgutil.InitCreateMeta(&cp.Metadata)
	rec := toRecord(&cp, 1)
	if err := s.orm.WithContext(ctx).Create(&rec).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, db.ErrAlreadyExists
		}
		return nil, err
	}
	return fromRecord(rec), nil
}

// Get fetches one ResourcePlan by unique name.
func (s *DB) Get(ctx context.Context, name string) (*corev1.ResourcePlan, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec corev1.ResourcePlanRecord
	err := s.orm.WithContext(ctx).Where("name = ?", name).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return fromRecord(rec), nil
}

// List queries ResourcePlans matching selectors sorted by sort_order ASC, name ASC.
func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.ResourcePlan], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx, err := applyPlanFilter(s.orm.WithContext(ctx).Model(&corev1.ResourcePlanRecord{}), opts)
	if err != nil {
		return nil, err
	}
	rows, total, err := pgutil.Paginate[corev1.ResourcePlanRecord](tx, "sort_order ASC, name ASC", opts)
	if err != nil {
		return nil, err
	}
	items := make([]*corev1.ResourcePlan, 0, len(rows))
	for _, row := range rows {
		items = append(items, fromRecord(row))
	}
	return &metav1.List[*corev1.ResourcePlan]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindResourcePlanList},
		Metadata: metav1.ListMeta{
			Continue:   pgutil.NextContinue(opts.Offset, len(items), total),
			TotalCount: total,
		},
		Items: items,
	}, nil
}

func applyPlanFilter(tx *gorm.DB, opts db.ListOptions) (*gorm.DB, error) {
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "name", val)
		case "planKind", "kind":
			tx = tx.Where("kind = ?", val)
		case "published":
			b, err := strconv.ParseBool(strings.TrimSpace(val))
			if err != nil {
				return nil, fmt.Errorf("invalid published selector %q", val)
			}
			tx = tx.Where("published = ?", b)
		default:
			return nil, fmt.Errorf("unsupported resourceplan fieldSelector %q", field)
		}
	}
	return db.ApplyLabelSelectorCol(tx, "labels", opts.LabelSelector), nil
}

// Update persists modifications to an existing ResourcePlan with optional ResourceVersion CAS.
func (s *DB) Update(ctx context.Context, in *corev1.ResourcePlan) (*corev1.ResourcePlan, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	existing, err := s.Get(ctx, in.Metadata.Name)
	if err != nil {
		return nil, err
	}
	if in.Metadata.UID != "" && in.Metadata.UID != existing.Metadata.UID {
		return nil, db.ErrConflict
	}
	oldRV, checkRV, err := resolveUpdateRV(in.Metadata.ResourceVersion, existing.Metadata.ResourceVersion)
	if err != nil {
		return nil, err
	}
	return s.commitUpdate(ctx, in, existing.Metadata.UID, oldRV, checkRV)
}

func resolveUpdateRV(reqRV, curRV string) (int64, bool, error) {
	existingRV, err := pgutil.ParseVersion(curRV)
	if err != nil {
		existingRV = 1
	}
	if reqRV == "" {
		return existingRV, false, nil
	}
	parsed, err := pgutil.ParseVersion(reqRV)
	if err != nil || parsed != existingRV {
		return 0, false, db.ErrConflict
	}
	return parsed, true, nil
}

func (s *DB) commitUpdate(ctx context.Context, in *corev1.ResourcePlan, uid string, oldRV int64, checkRV bool) (*corev1.ResourcePlan, error) {
	cp := *in
	cp.DefaultMeta()
	cp.Metadata.UID = uid
	rec := toRecord(&cp, oldRV+1)
	quotaJSON, _ := pgutil.MarshalJSONCol(rec.Quota)
	pricesJSON, _ := pgutil.MarshalJSONCol(rec.Prices)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	tx := s.orm.WithContext(ctx).Model(&corev1.ResourcePlanRecord{}).Where("name = ? AND uid = ?", rec.Name, rec.UID)
	if checkRV {
		tx = tx.Where("resource_version = ?", oldRV)
	}
	res := tx.Updates(map[string]any{
		"display_name": rec.DisplayName, "description": rec.Description, "kind": rec.Kind,
		"quota": quotaJSON, "sort_order": rec.SortOrder, "published": rec.Published,
		"prices": pricesJSON, "updated_at": rec.UpdatedAt, "resource_version": rec.ResourceVersion,
		"labels": lblJSON, "annotations": annJSON,
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, rec.Name)
}

// UpdateObject applies a guarded CAS mutation to a ResourcePlan.
func (s *DB) UpdateObject(ctx context.Context, name string, mutate func(*corev1.ResourcePlan) error) (*corev1.ResourcePlan, error) {
	for range 8 {
		cur, err := s.Get(ctx, name)
		if err != nil {
			return nil, err
		}
		if err := mutate(cur); err != nil {
			return nil, err
		}
		updated, err := s.Update(ctx, cur)
		if errors.Is(err, db.ErrConflict) {
			continue
		}
		return updated, err
	}
	return nil, db.ErrConflict
}

// Delete deletes a ResourcePlan by name and optional expected UID.
func (s *DB) Delete(ctx context.Context, name, expectedUID string) error {
	if s == nil || s.orm == nil {
		return errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Where("name = ?", name)
	if expectedUID != "" {
		tx = tx.Where("uid = ?", expectedUID)
	}
	res := tx.Delete(&corev1.ResourcePlanRecord{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		if expectedUID != "" {
			var count int64
			if s.orm.WithContext(ctx).Model(&corev1.ResourcePlanRecord{}).Where("name = ?", name).Count(&count).Error == nil && count > 0 {
				return db.ErrConflict
			}
		}
		return db.ErrNotFound
	}
	return nil
}
