package quota

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

var (
	ErrRedeemCodeDisabled = errors.New("redeem code is disabled")
	ErrRedeemCodeExpired  = errors.New("redeem code has expired")
	ErrRedeemCodeUsed     = errors.New("redeem code has already been redeemed")
)

// HashRedeemCode canonicalizes a raw code string and returns its SHA-256 hex digest.
func HashRedeemCode(raw string) string {
	clean := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(raw), "-", ""), " ", ""))
	sum := sha256.Sum256([]byte(clean))
	return hex.EncodeToString(sum[:])
}

func redeemFromRecord(rec corev1.QuotaRedeemCodeRecord) corev1.RedeemCode {
	return corev1.RedeemCode{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindRedeemCode},
		Metadata: metav1.ObjectMeta{
			Name:              rec.ID,
			UID:               rec.ID,
			CreationTimestamp: rec.CreatedAt.UTC(),
		},
		ID:                  rec.ID,
		CodeHint:            rec.CodeHint,
		BatchName:           rec.BatchName,
		OrderKind:           rec.OrderKind,
		PlanName:            rec.PlanName,
		DurationDays:        rec.DurationDays,
		MaxRedemptions:      rec.MaxRedemptions,
		RedeemedCount:       rec.RedeemedCount,
		RedeemedByTenantUID: rec.RedeemedByTenantUID,
		RedeemedOrderID:     rec.RedeemedOrderID,
		RedeemedAt:          normalizeTimePtr(rec.RedeemedAt),
		Disabled:            rec.Disabled,
		ExpiresAt:           normalizeTimePtr(rec.ExpiresAt),
		CreatedBy:           rec.CreatedBy,
		CreatedAt:           rec.CreatedAt.UTC(),
	}
}

// CreateRedeemCodes atomically inserts a batch of hashed redeem codes.
func (s *DB) CreateRedeemCodes(ctx context.Context, recs []corev1.QuotaRedeemCodeRecord) error {
	if len(recs) == 0 {
		return nil
	}
	if err := s.orm.WithContext(ctx).Create(&recs).Error; err != nil {
		return pgutil.WriteError(err, "")
	}
	return nil
}

// GetRedeemCodeByHash fetches the masked metadata for a hashed redeem code.
func (s *DB) GetRedeemCodeByHash(ctx context.Context, codeHash string) (corev1.RedeemCode, error) {
	var rec corev1.QuotaRedeemCodeRecord
	err := s.orm.WithContext(ctx).Where("code_hash = ?", codeHash).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return corev1.RedeemCode{}, errors.Join(ErrNotFound, db.ErrNotFound)
		}
		return corev1.RedeemCode{}, pgutil.Error(err, "")
	}
	return redeemFromRecord(rec), nil
}

// ClaimRedeemCode atomically claims a redeem code and creates its Paid QuotaOrder in one transaction.
func (s *DB) ClaimRedeemCode(
	ctx context.Context,
	codeHash, tenantUID string,
	buildOrder func(code corev1.RedeemCode, claimedAt time.Time) (corev1.QuotaOrder, error),
) (corev1.RedeemCode, corev1.QuotaOrder, error) {
	var outCode corev1.RedeemCode
	var outOrder corev1.QuotaOrder
	err := s.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return claimCodeTx(tx, codeHash, tenantUID, buildOrder, &outCode, &outOrder)
	})
	return outCode, outOrder, err
}

func claimCodeTx(
	tx *gorm.DB,
	codeHash, tenantUID string,
	buildOrder func(code corev1.RedeemCode, claimedAt time.Time) (corev1.QuotaOrder, error),
	outCode *corev1.RedeemCode,
	outOrder *corev1.QuotaOrder,
) error {
	var rec corev1.QuotaRedeemCodeRecord
	if err := tx.Where("code_hash = ?", codeHash).First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.Join(ErrNotFound, db.ErrNotFound)
		}
		return pgutil.Error(err, "")
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if rec.RedeemedByTenantUID == tenantUID && rec.RedeemedOrderID != "" {
		return loadAlreadyClaimedOrder(tx, rec, outCode, outOrder)
	}
	if err := checkClaimable(rec, now); err != nil {
		return err
	}
	order, err := buildOrder(redeemFromRecord(rec), now)
	if err != nil {
		return err
	}
	return commitCodeClaimAndOrder(tx, rec.ID, tenantUID, order, now, outCode, outOrder)
}

func checkClaimable(rec corev1.QuotaRedeemCodeRecord, now time.Time) error {
	if rec.Disabled {
		return ErrRedeemCodeDisabled
	}
	if rec.ExpiresAt != nil && !rec.ExpiresAt.After(now) {
		return ErrRedeemCodeExpired
	}
	if rec.RedeemedCount >= rec.MaxRedemptions {
		return ErrRedeemCodeUsed
	}
	return nil
}

func loadAlreadyClaimedOrder(tx *gorm.DB, rec corev1.QuotaRedeemCodeRecord, outCode *corev1.RedeemCode, outOrder *corev1.QuotaOrder) error {
	var ord corev1.QuotaOrderRecord
	if err := tx.Where("tenant_uid = ? AND id = ?", rec.RedeemedByTenantUID, rec.RedeemedOrderID).First(&ord).Error; err != nil {
		return pgutil.Error(err, rec.RedeemedOrderID)
	}
	*outCode = redeemFromRecord(rec)
	*outOrder = orderFromRecord(ord)
	return nil
}

func commitCodeClaimAndOrder(
	tx *gorm.DB,
	codeID, tenantUID string,
	order corev1.QuotaOrder,
	now time.Time,
	outCode *corev1.RedeemCode,
	outOrder *corev1.QuotaOrder,
) error {
	res := tx.Model(&corev1.QuotaRedeemCodeRecord{}).
		Where("id = ? AND disabled = FALSE AND redeemed_count < max_redemptions AND (expires_at IS NULL OR expires_at > ?)", codeID, now).
		Updates(map[string]any{
			"redeemed_count":         gorm.Expr("redeemed_count + 1"),
			"redeemed_by_tenant_uid": tenantUID,
			"redeemed_order_id":      order.ID,
			"redeemed_at":            now,
		})
	if res.Error != nil {
		return pgutil.WriteError(res.Error, codeID)
	}
	if res.RowsAffected == 0 {
		return ErrRedeemCodeUsed
	}
	ordRec := orderToRecord(order)
	if err := tx.Create(&ordRec).Error; err != nil {
		return pgutil.WriteError(err, order.ID)
	}
	var updatedCode corev1.QuotaRedeemCodeRecord
	if err := tx.Where("id = ?", codeID).First(&updatedCode).Error; err != nil {
		return pgutil.Error(err, codeID)
	}
	*outCode = redeemFromRecord(updatedCode)
	*outOrder = orderFromRecord(ordRec)
	return nil
}

// ListRedeemCodes returns masked redeem codes for admin management newest first.
func (s *DB) ListRedeemCodes(ctx context.Context, opts db.ListOptions) ([]corev1.RedeemCode, int, error) {
	tx, err := applyRedeemCodeFilter(s.orm.WithContext(ctx).Model(&corev1.QuotaRedeemCodeRecord{}), opts)
	if err != nil {
		return nil, 0, err
	}
	recs, total, err := pgutil.Paginate[corev1.QuotaRedeemCodeRecord](tx, "created_at DESC, id DESC", opts)
	if err != nil {
		return nil, 0, pgutil.Error(err, "")
	}
	out := make([]corev1.RedeemCode, 0, len(recs))
	for _, r := range recs {
		out = append(out, redeemFromRecord(r))
	}
	return out, total, nil
}

func applyRedeemCodeFilter(tx *gorm.DB, opts db.ListOptions) (*gorm.DB, error) {
	for _, field := range pgutil.SortedFieldKeys(opts.FieldSelector) {
		val := opts.FieldSelector[field]
		switch field {
		case "metadata.name":
			tx = tx.Where("id = ?", val)
		case "metadata.name.in":
			tx = db.ApplyNameInWhere(tx, "id", val)
		case "batchName":
			tx = tx.Where("batch_name = ?", val)
		case "planName":
			tx = tx.Where("plan_name = ?", val)
		case "orderKind":
			tx = tx.Where("order_kind = ?", val)
		case "disabled":
			b, err := strconv.ParseBool(strings.TrimSpace(val))
			if err != nil {
				return nil, fmt.Errorf("invalid disabled selector %q", val)
			}
			tx = tx.Where("disabled = ?", b)
		default:
			return nil, fmt.Errorf("unsupported redeemcode fieldSelector %q", field)
		}
	}
	return db.ApplyLabelSelectorCol(tx, "'{}'::jsonb", opts.LabelSelector), nil
}

// ListTenantRedeemedCodes returns redeem codes already claimed by tenantUID.
func (s *DB) ListTenantRedeemedCodes(ctx context.Context, tenantUID string) ([]corev1.RedeemCode, error) {
	var recs []corev1.QuotaRedeemCodeRecord
	err := s.orm.WithContext(ctx).
		Where("redeemed_by_tenant_uid = ?", tenantUID).
		Order("redeemed_at ASC, id ASC").
		Find(&recs).Error
	if err != nil {
		return nil, pgutil.Error(err, "")
	}
	out := make([]corev1.RedeemCode, 0, len(recs))
	for _, r := range recs {
		out = append(out, redeemFromRecord(r))
	}
	return out, nil
}
