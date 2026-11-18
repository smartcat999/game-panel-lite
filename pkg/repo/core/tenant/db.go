package tenant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// DB manages persistence for Tenant across tenants (spec/metadata) and tenant_states (runtime status).
type DB struct{ orm *gorm.DB }

func NewDB(pool *pgxpool.Pool) *DB {
	if pool == nil {
		return nil
	}
	return &DB{orm: db.OpenGORM(pool)}
}

func nonNilResourceList(in corev1.ResourceList) corev1.ResourceList {
	if in == nil {
		return corev1.ResourceList{}
	}
	cp := make(corev1.ResourceList, len(in))
	for k, v := range in {
		cp[k] = v
	}
	return cp
}

func nonNilSlice[T any](in []T) []T {
	if len(in) == 0 {
		return []T{}
	}
	return append([]T(nil), in...)
}

func toRecords(in *corev1.Tenant, rv, sv int64) (corev1.TenantRecord, corev1.TenantStateRecord) {
	phase := in.Status.Phase
	if phase == "" {
		phase = corev1.TenantPhaseActive
	}
	rec := corev1.TenantRecord{
		Name:                  in.Metadata.Name,
		UID:                   in.Metadata.UID,
		DisplayName:           in.Spec.DisplayName,
		Type:                  string(in.Spec.Type),
		Owner:                 in.Spec.Owner,
		BillingMode:           string(billingModeOrDefault(in.Spec.BillingMode)),
		Quota:                 nonNilResourceList(in.Spec.Quota),
		UnlimitedQuota:        nonNilSlice(in.Spec.UnlimitedQuota),
		AlertRules:            nonNilSlice(in.Spec.AlertRules),
		AlertChannels:         nonNilSlice(in.Spec.AlertChannels),
		PlatformAlertChannels: nonNilSlice(in.Spec.PlatformAlertChannels),
		AlertSilences:         nonNilSlice(in.Spec.AlertSilences),
		PlatformAlertSilences: nonNilSlice(in.Spec.PlatformAlertSilences),
		ResourceVersion:       rv,
		Generation:            in.Metadata.Generation,
		CreatedAt:             in.Metadata.CreationTimestamp,
		DeletedAt:             in.Metadata.DeletionTimestamp,
		Finalizers:            pgutil.NonNilStrings(in.Metadata.Finalizers),
		Labels:                pgutil.NonNilMap(in.Metadata.Labels),
		Annotations:           pgutil.NonNilMap(in.Metadata.Annotations),
	}
	st := corev1.TenantStateRecord{
		TenantUID:          in.Metadata.UID,
		Version:            sv,
		ObservedGeneration: in.Status.ObservedGeneration,
		Phase:              string(phase),
		Usage:              nonNilResourceList(in.Status.Usage),
		Subscription:       cloneSubscription(in.Status.Subscription),
		Conditions:         pgutil.NonNilConditions(in.Status.Conditions),
		UpdatedAt:          time.Now().UTC(),
	}
	return rec, st
}

func billingModeOrDefault(mode corev1.TenantBillingMode) corev1.TenantBillingMode {
	if mode == "" {
		return corev1.TenantBillingModeManual
	}
	return mode
}

func cloneSubscription(in *corev1.TenantSubscriptionStatus) *corev1.TenantSubscriptionStatus {
	if in == nil {
		return nil
	}
	cp := *in
	if in.ExpiresAt != nil {
		t := in.ExpiresAt.UTC()
		cp.ExpiresAt = &t
	}
	if in.GraceUntil != nil {
		t := in.GraceUntil.UTC()
		cp.GraceUntil = &t
	}
	if in.BaseQuota != nil {
		cp.BaseQuota = nonNilResourceList(in.BaseQuota)
	}
	if in.AddonQuota != nil {
		cp.AddonQuota = nonNilResourceList(in.AddonQuota)
	}
	if in.EffectiveQuota != nil {
		cp.EffectiveQuota = nonNilResourceList(in.EffectiveQuota)
	}
	if len(in.ActiveOrderIDs) > 0 {
		cp.ActiveOrderIDs = append([]string(nil), in.ActiveOrderIDs...)
	}
	return &cp
}

func fromRecords(rec corev1.TenantRecord, st corev1.TenantStateRecord, hasState bool) *corev1.Tenant {
	out := &corev1.Tenant{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "Tenant"},
		Metadata: metav1.ObjectMeta{
			Name:              rec.Name,
			UID:               rec.UID,
			ResourceVersion:   pgutil.FormatVersion(rec.ResourceVersion),
			Generation:        rec.Generation,
			CreationTimestamp: rec.CreatedAt,
			DeletionTimestamp: rec.DeletedAt,
		},
		Spec: corev1.TenantSpec{
			DisplayName:           rec.DisplayName,
			Type:                  corev1.TenantType(rec.Type),
			Owner:                 rec.Owner,
			BillingMode:           billingModeOrDefault(corev1.TenantBillingMode(rec.BillingMode)),
			Quota:                 nonNilResourceList(rec.Quota),
			UnlimitedQuota:        append([]corev1.ResourceName(nil), rec.UnlimitedQuota...),
			AlertRules:            append([]corev1.AlertRule(nil), rec.AlertRules...),
			AlertChannels:         append([]corev1.AlertChannel(nil), rec.AlertChannels...),
			PlatformAlertChannels: append([]corev1.AlertChannel(nil), rec.PlatformAlertChannels...),
			AlertSilences:         append([]corev1.AlertSilence(nil), rec.AlertSilences...),
			PlatformAlertSilences: append([]corev1.AlertSilence(nil), rec.PlatformAlertSilences...),
		},
		Status: corev1.TenantStatus{Phase: corev1.TenantPhaseActive},
	}
	if hasState {
		out.Metadata.StatusVersion = pgutil.FormatVersion(st.Version)
		out.Status = corev1.TenantStatus{
			ObservedGeneration: st.ObservedGeneration,
			Phase:              corev1.TenantPhase(st.Phase),
			Conditions:         append([]metav1.Condition(nil), st.Conditions...),
		}
		if len(st.Usage) > 0 {
			out.Status.Usage = nonNilResourceList(st.Usage)
		}
		out.Status.Subscription = cloneSubscription(st.Subscription)
	}
	if rec.DeletedAt != nil {
		out.Status.Phase = corev1.TenantPhaseTerminating
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

func (s *DB) Create(ctx context.Context, in *corev1.Tenant) (*corev1.Tenant, error) {
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

func (s *DB) Get(ctx context.Context, name string) (*corev1.Tenant, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	var rec corev1.TenantRecord
	if err := s.orm.WithContext(ctx).Where("name = ?", name).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	var st corev1.TenantStateRecord
	err := s.orm.WithContext(ctx).Where("tenant_uid = ?", rec.UID).Take(&st).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return fromRecords(rec, st, err == nil), nil
}

func (s *DB) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Tenant], error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	tx := s.orm.WithContext(ctx).Model(&corev1.TenantRecord{})
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("tenants.name = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "tenants.name", val)
		case "spec.owner":
			tx = tx.Where("tenants.owner = ?", val)
		case "spec.type":
			tx = tx.Where("tenants.type = ?", val)
		case "spec.billingMode":
			tx = tx.Where("tenants.billing_mode = ?", val)
		case "status.phase":
			tx = tx.Joins("JOIN tenant_states ON tenant_states.tenant_uid = tenants.uid").
				Where("CASE WHEN tenants.deleted_at IS NOT NULL THEN ? ELSE tenant_states.phase END = ?", string(corev1.TenantPhaseTerminating), val)
		default:
			return nil, fmt.Errorf("unsupported tenant fieldSelector %q", field)
		}
	}
	tx = db.ApplyLabelSelectorCol(tx, "tenants.labels", opts.LabelSelector)
	rows, total, err := pgutil.Paginate[corev1.TenantRecord](tx, "tenants.created_at DESC, tenants.name ASC", opts)
	if err != nil {
		return nil, err
	}
	return s.hydrateList(ctx, rows, total, opts.LabelSelector)
}

func (s *DB) hydrateList(ctx context.Context, rows []corev1.TenantRecord, total int, sel *metav1.LabelSelector) (*metav1.List[*corev1.Tenant], error) {
	stateMap := make(map[string]corev1.TenantStateRecord, len(rows))
	if len(rows) > 0 {
		uids := make([]string, len(rows))
		for i, r := range rows {
			uids[i] = r.UID
		}
		var states []corev1.TenantStateRecord
		if err := s.orm.WithContext(ctx).Where("tenant_uid IN ?", uids).Find(&states).Error; err != nil {
			return nil, err
		}
		for _, st := range states {
			stateMap[st.TenantUID] = st
		}
	}
	items := make([]*corev1.Tenant, 0, len(rows))
	for _, row := range rows {
		st, ok := stateMap[row.UID]
		item := fromRecords(row, st, ok)
		if sel != nil && !sel.Matches(item.Metadata.Labels) {
			continue
		}
		items = append(items, item)
	}
	return &metav1.List[*corev1.Tenant]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "TenantList"},
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
	return pgutil.DeleteByCondition(ctx, s.orm, "tenants", "name = ?", []any{name}, expectedUID)
}
