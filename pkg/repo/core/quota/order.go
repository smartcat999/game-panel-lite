package quota

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

func normalizeTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC().Truncate(time.Microsecond)
	return &v
}

func orderToRecord(in corev1.QuotaOrder) corev1.QuotaOrderRecord {
	delta := corev1.ResourceList{}
	for k, v := range in.QuotaDelta {
		delta[k] = v
	}
	state := in.State
	if state == "" {
		state = corev1.QuotaOrderStatePending
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	createdAt := in.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	return corev1.QuotaOrderRecord{
		ID:            in.ID,
		TenantUID:     in.TenantUID,
		OrderKind:     in.OrderKind,
		PlanName:      in.PlanName,
		Source:        in.Source,
		SourceRef:     in.SourceRef,
		Reason:        in.Reason,
		QuotaDelta:    delta,
		DurationDays:  in.DurationDays,
		AmountCents:   in.AmountCents,
		Currency:      in.Currency,
		State:         state,
		PaidAt:        normalizeTimePtr(in.PaidAt),
		ExpiresAt:     normalizeTimePtr(in.ExpiresAt),
		AppliedAt:     normalizeTimePtr(in.AppliedAt),
		ExportedAt:    normalizeTimePtr(in.ExportedAt),
		PolicyVersion: in.PolicyVersion,
		CreatedBy:     in.CreatedBy,
		CreatedAt:     createdAt,
		UpdatedAt:     now,
	}
}

func orderFromRecord(rec corev1.QuotaOrderRecord) corev1.QuotaOrder {
	delta := corev1.ResourceList{}
	for k, v := range rec.QuotaDelta {
		delta[k] = v
	}
	return corev1.QuotaOrder{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindQuotaOrder},
		Metadata: metav1.ObjectMeta{
			Name:              rec.ID,
			UID:               rec.ID,
			CreationTimestamp: rec.CreatedAt.UTC(),
		},
		ID:            rec.ID,
		TenantUID:     rec.TenantUID,
		OrderKind:     rec.OrderKind,
		PlanName:      rec.PlanName,
		Source:        rec.Source,
		SourceRef:     rec.SourceRef,
		Reason:        rec.Reason,
		QuotaDelta:    delta,
		DurationDays:  rec.DurationDays,
		AmountCents:   rec.AmountCents,
		Currency:      rec.Currency,
		State:         rec.State,
		PaidAt:        normalizeTimePtr(rec.PaidAt),
		ExpiresAt:     normalizeTimePtr(rec.ExpiresAt),
		AppliedAt:     normalizeTimePtr(rec.AppliedAt),
		ExportedAt:    normalizeTimePtr(rec.ExportedAt),
		PolicyVersion: rec.PolicyVersion,
		CreatedBy:     rec.CreatedBy,
		CreatedAt:     rec.CreatedAt.UTC(),
		UpdatedAt:     rec.UpdatedAt.UTC(),
	}
}

func validateOrderInput(in corev1.QuotaOrder) error {
	if err := corev1.ValidateIdentifier("order.id", in.ID); err != nil {
		return err
	}
	if err := corev1.ValidateIdentifier("order.tenantUID", in.TenantUID); err != nil {
		return err
	}
	if in.OrderKind != corev1.QuotaOrderKindBasePlan && in.OrderKind != corev1.QuotaOrderKindAddonPack {
		return fmt.Errorf("invalid orderKind %q", in.OrderKind)
	}
	if in.PlanName == "" || in.DurationDays <= 0 {
		return errors.New("order planName and positive durationDays are required")
	}
	return in.QuotaDelta.Validate()
}

func sameOrderIdentity(existing, incoming corev1.QuotaOrderRecord) bool {
	return existing.TenantUID == incoming.TenantUID &&
		existing.OrderKind == incoming.OrderKind &&
		existing.PlanName == incoming.PlanName &&
		existing.Source == incoming.Source &&
		existing.SourceRef == incoming.SourceRef &&
		existing.DurationDays == incoming.DurationDays
}

// InsertOrder persists a QuotaOrder idempotently when an identical immutable ID already exists.
func (s *DB) InsertOrder(ctx context.Context, in corev1.QuotaOrder) (corev1.QuotaOrder, error) {
	if err := validateOrderInput(in); err != nil {
		return corev1.QuotaOrder{}, err
	}
	rec := orderToRecord(in)
	var out corev1.QuotaOrderRecord
	err := s.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing corev1.QuotaOrderRecord
		findErr := tx.Where("id = ?", rec.ID).First(&existing).Error
		if findErr == nil {
			if !sameOrderIdentity(existing, rec) {
				return db.ErrAlreadyExists
			}
			out = existing
			return nil
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return pgutil.Error(findErr, rec.ID)
		}
		if createErr := tx.Create(&rec).Error; createErr != nil {
			var pgErr *pgconn.PgError
			if errors.As(createErr, &pgErr) && pgErr.Code == "23505" {
				return db.ErrAlreadyExists
			}
			return pgutil.WriteError(createErr, rec.ID)
		}
		out = rec
		return nil
	})
	if err != nil {
		return corev1.QuotaOrder{}, err
	}
	return orderFromRecord(out), nil
}

// GetOrder loads one QuotaOrder belonging to tenantUID.
func (s *DB) GetOrder(ctx context.Context, tenantUID, orderID string) (corev1.QuotaOrder, error) {
	var rec corev1.QuotaOrderRecord
	err := s.orm.WithContext(ctx).Where("tenant_uid = ? AND id = ?", tenantUID, orderID).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return corev1.QuotaOrder{}, errors.Join(ErrNotFound, db.ErrNotFound)
		}
		return corev1.QuotaOrder{}, pgutil.Error(err, orderID)
	}
	return orderFromRecord(rec), nil
}

// ListOrders returns paginated QuotaOrders for a Tenant newest first, filtered by
// opts.FieldSelector (metadata.name, metadata.name.in, state, orderKind, source, planName).
func (s *DB) ListOrders(ctx context.Context, tenantUID string, opts db.ListOptions) ([]corev1.QuotaOrder, int, error) {
	base, err := applyOrderFieldSelector(s.orm.WithContext(ctx).Model(&corev1.QuotaOrderRecord{}).Where("tenant_uid = ?", tenantUID), opts.FieldSelector)
	if err != nil {
		return nil, 0, err
	}
	base = db.ApplyLabelSelectorCol(base, "'{}'::jsonb", opts.LabelSelector)
	recs, total, err := pgutil.Paginate[corev1.QuotaOrderRecord](base, "COALESCE(paid_at, created_at) DESC, created_at DESC, id DESC", opts)
	if err != nil {
		return nil, 0, pgutil.Error(err, "")
	}
	out := make([]corev1.QuotaOrder, 0, len(recs))
	for _, r := range recs {
		out = append(out, orderFromRecord(r))
	}
	return out, total, nil
}

func applyOrderFieldSelector(tx *gorm.DB, selector map[string]string) (*gorm.DB, error) {
	for _, field := range pgutil.SortedFieldKeys(selector) {
		val := selector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("id = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "id", val)
		case "state":
			tx = tx.Where("state = ?", val)
		case "orderKind":
			tx = tx.Where("order_kind = ?", val)
		case "source":
			tx = tx.Where("source = ?", val)
		case "planName":
			tx = tx.Where("plan_name = ?", val)
		default:
			return nil, fmt.Errorf("unsupported order fieldSelector %q", field)
		}
	}
	return tx, nil
}

// ListFoldableOrders returns all Paid orders for tenantUID in deterministic replay order.
func (s *DB) ListFoldableOrders(ctx context.Context, tenantUID string) ([]corev1.QuotaOrder, error) {
	var recs []corev1.QuotaOrderRecord
	err := s.orm.WithContext(ctx).
		Where("tenant_uid = ? AND state = ?", tenantUID, corev1.QuotaOrderStatePaid).
		Order("paid_at ASC, created_at ASC, id ASC").
		Find(&recs).Error
	if err != nil {
		return nil, pgutil.Error(err, "")
	}
	out := make([]corev1.QuotaOrder, 0, len(recs))
	for _, r := range recs {
		out = append(out, orderFromRecord(r))
	}
	return out, nil
}

// HasAnyOrders reports whether a tenant already has at least one order in quota_orders.
func (s *DB) HasAnyOrders(ctx context.Context, tenantUID string) (bool, error) {
	var count int64
	err := s.orm.WithContext(ctx).Model(&corev1.QuotaOrderRecord{}).Where("tenant_uid = ?", tenantUID).Limit(1).Count(&count).Error
	if err != nil {
		return false, pgutil.Error(err, "")
	}
	return count > 0, nil
}

// MarkOrdersApplied records the ledger policyVersion at which Paid orders first took effect.
func (s *DB) MarkOrdersApplied(ctx context.Context, tenantUID string, orderIDs []string, policyVersion int64, appliedAt time.Time) error {
	if len(orderIDs) == 0 {
		return nil
	}
	ts := appliedAt.UTC().Truncate(time.Microsecond)
	res := s.orm.WithContext(ctx).Model(&corev1.QuotaOrderRecord{}).
		Where("tenant_uid = ? AND id IN ? AND state = ? AND applied_at IS NULL",
			tenantUID, orderIDs, corev1.QuotaOrderStatePaid).
		Updates(map[string]any{"applied_at": ts, "policy_version": policyVersion, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return pgutil.WriteError(res.Error, "")
	}
	return nil
}

// CancelOrder transitions an order to Cancelled idempotently.
func (s *DB) CancelOrder(ctx context.Context, tenantUID, orderID, reason string) (corev1.QuotaOrder, error) {
	var rec corev1.QuotaOrderRecord
	err := s.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uid = ? AND id = ?", tenantUID, orderID).First(&rec).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.Join(ErrNotFound, db.ErrNotFound)
			}
			return pgutil.Error(err, orderID)
		}
		if rec.State == corev1.QuotaOrderStateCancelled {
			return nil
		}
		updates := map[string]any{"state": corev1.QuotaOrderStateCancelled, "updated_at": time.Now().UTC()}
		if reason != "" {
			updates["reason"] = reason
		}
		if err := tx.Model(&corev1.QuotaOrderRecord{}).Where("tenant_uid = ? AND id = ?", tenantUID, orderID).Updates(updates).Error; err != nil {
			return pgutil.WriteError(err, orderID)
		}
		return tx.Where("tenant_uid = ? AND id = ?", tenantUID, orderID).First(&rec).Error
	})
	if err != nil {
		return corev1.QuotaOrder{}, err
	}
	return orderFromRecord(rec), nil
}

// ListTenantUIDsWithUnappliedOrders returns tenant UIDs with Paid orders not yet applied.
func (s *DB) ListTenantUIDsWithUnappliedOrders(ctx context.Context) ([]string, error) {
	var uids []string
	err := s.orm.WithContext(ctx).Model(&corev1.QuotaOrderRecord{}).
		Where("state = ? AND applied_at IS NULL", corev1.QuotaOrderStatePaid).
		Distinct("tenant_uid").Pluck("tenant_uid", &uids).Error
	if err != nil {
		return nil, pgutil.Error(err, "")
	}
	return uids, nil
}

// ListTenantUIDsApproachingExpiry returns tenant UIDs whose quota expires_at <= horizon.
func (s *DB) ListTenantUIDsApproachingExpiry(ctx context.Context, horizon time.Time) ([]string, error) {
	var uids []string
	err := s.orm.WithContext(ctx).Model(&corev1.QuotaAccountRecord{}).
		Where("expires_at IS NOT NULL AND expires_at <= ?", horizon.UTC()).
		Pluck("tenant_uid", &uids).Error
	if err != nil {
		return nil, pgutil.Error(err, "")
	}
	return uids, nil
}

// ListTenantUIDsWithExpiringAccounts returns tenant UIDs whose quota policy expires_at <= now.
func (s *DB) ListTenantUIDsWithExpiringAccounts(ctx context.Context, now time.Time) ([]string, error) {
	return s.ListTenantUIDsApproachingExpiry(ctx, now)
}

// ListUnexportedAppliedOrders returns Applied Paid orders not yet published to the billing outbox.
func (s *DB) ListUnexportedAppliedOrders(ctx context.Context, tenantUID string, limit int) ([]corev1.QuotaOrder, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var recs []corev1.QuotaOrderRecord
	err := s.orm.WithContext(ctx).
		Where("tenant_uid = ? AND state = ? AND applied_at IS NOT NULL AND exported_at IS NULL", tenantUID, corev1.QuotaOrderStatePaid).
		Order("applied_at ASC, id ASC").
		Limit(limit).
		Find(&recs).Error
	if err != nil {
		return nil, pgutil.Error(err, "")
	}
	out := make([]corev1.QuotaOrder, 0, len(recs))
	for _, r := range recs {
		out = append(out, orderFromRecord(r))
	}
	return out, nil
}

// MarkOrderExported records that an applied order CloudEvent has been delivered to MQ.
func (s *DB) MarkOrderExported(ctx context.Context, orderID string, exportedAt time.Time) error {
	ts := exportedAt.UTC().Truncate(time.Microsecond)
	res := s.orm.WithContext(ctx).Model(&corev1.QuotaOrderRecord{}).
		Where("id = ? AND exported_at IS NULL", orderID).
		Updates(map[string]any{"exported_at": ts, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return pgutil.WriteError(res.Error, orderID)
	}
	return nil
}

// ListTenantUIDsWithUnexportedBilling returns distinct tenant UIDs with unexported orders or policy changes.
func (s *DB) ListTenantUIDsWithUnexportedBilling(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	var orderUIDs []string
	if err := s.orm.WithContext(ctx).Model(&corev1.QuotaOrderRecord{}).
		Where("state = ? AND applied_at IS NOT NULL AND exported_at IS NULL", corev1.QuotaOrderStatePaid).
		Distinct("tenant_uid").Pluck("tenant_uid", &orderUIDs).Error; err != nil {
		return nil, pgutil.Error(err, "")
	}
	for _, id := range orderUIDs {
		seen[id] = true
	}
	var policyUIDs []string
	if err := s.orm.WithContext(ctx).Model(&QuotaPolicyChangeRecord{}).
		Where("exported_at IS NULL").
		Distinct("tenant_uid").Pluck("tenant_uid", &policyUIDs).Error; err != nil {
		return nil, pgutil.Error(err, "")
	}
	for _, id := range policyUIDs {
		seen[id] = true
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out, nil
}

// ListTenantUIDsWithExpiringOrders returns tenant UIDs having Paid orders with expires_at <= now.
func (s *DB) ListTenantUIDsWithExpiringOrders(ctx context.Context, now time.Time) ([]string, error) {
	var uids []string
	err := s.orm.WithContext(ctx).Model(&corev1.QuotaOrderRecord{}).
		Where("state = ? AND expires_at IS NOT NULL AND expires_at <= ?", corev1.QuotaOrderStatePaid, now.UTC()).
		Distinct("tenant_uid").Pluck("tenant_uid", &uids).Error
	if err != nil {
		return nil, pgutil.Error(err, "")
	}
	return uids, nil
}

// ReconcileAppliedOrders stamps applied_at and policy_version for unapplied Paid orders already committed to quota_policy_changes.
func (s *DB) ReconcileAppliedOrders(ctx context.Context, tenantUID string) error {
	acct, err := s.GetAccount(ctx, tenantUID)
	if errors.Is(err, ErrNotFound) || (err == nil && acct.PolicyVersion == 0) {
		return nil
	}
	if err != nil {
		return err
	}
	var unapplied []corev1.QuotaOrderRecord
	if err := s.orm.WithContext(ctx).
		Where("tenant_uid = ? AND state = ? AND applied_at IS NULL", tenantUID, corev1.QuotaOrderStatePaid).
		Find(&unapplied).Error; err != nil {
		return pgutil.Error(err, "")
	}
	if len(unapplied) == 0 {
		return nil
	}
	var recs []QuotaPolicyChangeRecord
	if err := s.orm.WithContext(ctx).
		Where("tenant_uid = ? AND source_version < ?", tenantUID, acct.PolicyVersion).
		Order("source_version ASC").Find(&recs).Error; err != nil {
		return pgutil.Error(err, "")
	}
	applied := collectEarliestOrderVersions(recs)
	for _, ord := range unapplied {
		if info, ok := applied[ord.ID]; ok {
			_ = s.MarkOrdersApplied(ctx, tenantUID, []string{ord.ID}, info.version, info.at)
		}
	}
	return nil
}

type orderAppliedRef struct {
	version int64
	at      time.Time
}

func collectEarliestOrderVersions(recs []QuotaPolicyChangeRecord) map[string]orderAppliedRef {
	out := map[string]orderAppliedRef{}
	for _, rec := range recs {
		chg, err := toPolicyChange(rec)
		if err != nil {
			continue
		}
		for _, id := range chg.Plan.OrderIDs {
			if _, exists := out[id]; !exists {
				out[id] = orderAppliedRef{version: chg.Plan.ToVersion, at: chg.CreatedAt}
			}
		}
	}
	return out
}
