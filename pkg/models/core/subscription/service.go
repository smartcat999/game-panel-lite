package subscription

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	plandb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/resourceplan"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

const redeemAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// Service coordinates redeem codes, entitlement orders, and subscription convergence.
type Service struct {
	reg      *repo.Registry
	bus      mq.Interface
	tenantDB *tenantdb.DB
	planDB   *plandb.DB
	quotaDB  *quotadb.DB
}

// NewService constructs a subscription Service.
func NewService(reg *repo.Registry, bus mq.Interface) *Service {
	svc := &Service{reg: reg, bus: bus}
	if reg != nil && reg.Pool() != nil {
		pool := reg.Pool()
		svc.tenantDB = tenantdb.NewDB(pool)
		svc.planDB = plandb.NewDB(pool)
		svc.quotaDB = quotadb.NewDB(pool)
	}
	return svc
}

// CreateRedeemBatch generates a batch of CSPRNG activation codes storing only SHA-256 hashes.
func (s *Service) CreateRedeemBatch(ctx context.Context, actor string, req corev1.CreateRedeemCodeBatchRequest) (*corev1.RedeemCodeBatch, error) {
	orderKind, planName, err := s.validateRedeemBatchRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	recs, issued, err := buildRedeemCodeRecords(actor, orderKind, planName, req)
	if err != nil {
		return nil, err
	}
	if err := s.quotaDB.CreateRedeemCodes(ctx, recs); err != nil {
		return nil, err
	}
	return &corev1.RedeemCodeBatch{
		TypeMeta:     metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindRedeemCodeBatch},
		Metadata:     metav1.ObjectMeta{Name: req.BatchName},
		BatchName:    req.BatchName,
		OrderKind:    orderKind,
		PlanName:     planName,
		DurationDays: req.DurationDays,
		Count:        len(issued),
		Codes:        issued,
	}, nil
}

func (s *Service) validateRedeemBatchRequest(ctx context.Context, req corev1.CreateRedeemCodeBatchRequest) (string, string, error) {
	if req.Count < 1 || req.Count > 100 {
		return "", "", fmt.Errorf("%w: count must be between 1 and 100", models.ErrBadRequest)
	}
	if req.DurationDays < 1 || req.DurationDays > 3650 {
		return "", "", fmt.Errorf("%w: durationDays must be between 1 and 3650", models.ErrBadRequest)
	}
	if req.MaxRedemptions != 0 && req.MaxRedemptions != 1 {
		return "", "", fmt.Errorf("%w: maxRedemptions must be 1", models.ErrBadRequest)
	}
	plan, err := s.planDB.Get(ctx, strings.TrimSpace(req.PlanName))
	if err != nil {
		return "", "", err
	}
	return orderKindFromPlan(plan.PlanKind), plan.Metadata.Name, nil
}

func orderKindFromPlan(planKind string) string {
	if planKind == corev1.PlanKindAddon {
		return corev1.QuotaOrderKindAddonPack
	}
	return corev1.QuotaOrderKindBasePlan
}

func buildRedeemCodeRecords(actor, orderKind, planName string, req corev1.CreateRedeemCodeBatchRequest) ([]corev1.QuotaRedeemCodeRecord, []corev1.RedeemCodeIssuedItem, error) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	recs := make([]corev1.QuotaRedeemCodeRecord, 0, req.Count)
	issued := make([]corev1.RedeemCodeIssuedItem, 0, req.Count)
	for range req.Count {
		raw, hint, err := generateRedeemCodePair()
		if err != nil {
			return nil, nil, err
		}
		id := "rc-" + db.NewUID()
		recs = append(recs, corev1.QuotaRedeemCodeRecord{
			ID: id, CodeHash: quotadb.HashRedeemCode(raw), CodeHint: hint,
			BatchName: strings.TrimSpace(req.BatchName), OrderKind: orderKind, PlanName: planName,
			DurationDays: req.DurationDays, MaxRedemptions: 1, ExpiresAt: req.ExpiresAt,
			CreatedBy: actor, CreatedAt: now,
		})
		issued = append(issued, corev1.RedeemCodeIssuedItem{ID: id, Code: raw, CodeHint: hint})
	}
	return recs, issued, nil
}

func generateRedeemCodePair() (string, string, error) {
	groups := make([]string, 3)
	for g := range 3 {
		buf := make([]byte, 4)
		for i := range 4 {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(redeemAlphabet))))
			if err != nil {
				return "", "", err
			}
			buf[i] = redeemAlphabet[n.Int64()]
		}
		groups[g] = string(buf)
	}
	raw := fmt.Sprintf("GP-%s-%s-%s", groups[0], groups[1], groups[2])
	hint := fmt.Sprintf("GP-%s-****-%s", groups[0], groups[2])
	return raw, hint, nil
}

// ListRedeemCodes returns masked redeem code metadata paginated newest first.
func (s *Service) ListRedeemCodes(ctx context.Context, opts db.ListOptions) (*metav1.List[corev1.RedeemCode], error) {
	items, total, err := s.quotaDB.ListRedeemCodes(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &metav1.List[corev1.RedeemCode]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindRedeemCodeList},
		Metadata: metav1.ListMeta{
			Continue:   pgutil.NextContinue(opts.Offset, len(items), total),
			TotalCount: total,
		},
		Items: items,
	}, nil
}

// Redeem validates Fold compatibility BEFORE claiming a single-use code, creates a Paid order, and converges quota.
func (s *Service) Redeem(ctx context.Context, actor, tenantName, rawCode string) (*corev1.RedeemResponse, error) {
	if strings.TrimSpace(rawCode) == "" {
		return nil, fmt.Errorf("%w: code is required", models.ErrBadRequest)
	}
	tenant, code, plan, err := s.prepareRedeem(ctx, tenantName, rawCode)
	if err != nil {
		return nil, err
	}
	if code.RedeemedByTenantUID == tenant.Metadata.UID && code.RedeemedOrderID != "" {
		ord, getErr := s.quotaDB.GetOrder(ctx, tenant.Metadata.UID, code.RedeemedOrderID)
		if getErr != nil {
			return nil, getErr
		}
		return newRedeemResponse(tenant, ord), nil
	}
	if err := s.dryRunOrderFold(ctx, tenant.Metadata.UID, candidateRedeemOrder(tenant.Metadata.UID, code, plan, actor, time.Now().UTC())); err != nil {
		return nil, err
	}
	_, order, err := s.quotaDB.ClaimRedeemCode(ctx, quotadb.HashRedeemCode(rawCode), tenant.Metadata.UID, func(claimed corev1.RedeemCode, claimedAt time.Time) (corev1.QuotaOrder, error) {
		return candidateRedeemOrder(tenant.Metadata.UID, claimed, plan, actor, claimedAt), nil
	})
	if err != nil {
		return nil, mapQuotaRepoError(err)
	}
	return s.finishOrderMutation(ctx, tenant, order), nil
}

func (s *Service) prepareRedeem(ctx context.Context, tenantName, rawCode string) (*corev1.Tenant, corev1.RedeemCode, *corev1.ResourcePlan, error) {
	tenant, err := s.tenantDB.Get(ctx, tenantName)
	if err != nil {
		return nil, corev1.RedeemCode{}, nil, err
	}
	if tenant.Spec.BillingMode != corev1.TenantBillingModeSubscription {
		return nil, corev1.RedeemCode{}, nil, fmt.Errorf("%w: redeem codes require BillingMode=Subscription", models.ErrConflict)
	}
	code, err := s.quotaDB.GetRedeemCodeByHash(ctx, quotadb.HashRedeemCode(rawCode))
	if err != nil {
		return nil, corev1.RedeemCode{}, nil, mapQuotaRepoError(err)
	}
	if code.RedeemedByTenantUID != tenant.Metadata.UID || code.RedeemedOrderID == "" {
		if err := ensureRedeemCodeReady(code); err != nil {
			return nil, corev1.RedeemCode{}, nil, err
		}
	}
	plan, err := s.planDB.Get(ctx, code.PlanName)
	if err != nil {
		return nil, corev1.RedeemCode{}, nil, err
	}
	return tenant, code, plan, nil
}

func ensureRedeemCodeReady(code corev1.RedeemCode) error {
	if code.Disabled {
		return fmt.Errorf("%w: %v", models.ErrConflict, quotadb.ErrRedeemCodeDisabled)
	}
	if code.ExpiresAt != nil && !code.ExpiresAt.After(time.Now().UTC()) {
		return fmt.Errorf("%w: %v", models.ErrConflict, quotadb.ErrRedeemCodeExpired)
	}
	if code.RedeemedCount >= code.MaxRedemptions {
		return fmt.Errorf("%w: %v", models.ErrConflict, quotadb.ErrRedeemCodeUsed)
	}
	return nil
}

func candidateRedeemOrder(tenantUID string, code corev1.RedeemCode, plan *corev1.ResourcePlan, actor string, paidAt time.Time) corev1.QuotaOrder {
	ts := paidAt.UTC().Truncate(time.Microsecond)
	exp := ts.Add(time.Duration(code.DurationDays) * 24 * time.Hour)
	return corev1.QuotaOrder{
		ID: "redeem:" + code.ID, TenantUID: tenantUID, OrderKind: code.OrderKind,
		PlanName: code.PlanName, Source: corev1.QuotaOrderSourceRedeem, SourceRef: code.ID,
		Reason: "Redeem code " + code.CodeHint, QuotaDelta: plan.Quota, DurationDays: code.DurationDays,
		State: corev1.QuotaOrderStatePaid, PaidAt: &ts, ExpiresAt: &exp, CreatedBy: actor,
	}
}

func (s *Service) dryRunOrderFold(ctx context.Context, tenantUID string, candidate corev1.QuotaOrder) error {
	existing, err := s.quotaDB.ListFoldableOrders(ctx, tenantUID)
	if err != nil {
		return err
	}
	plansMap, err := loadPlansMap(ctx, s.planDB)
	if err != nil {
		return err
	}
	_, err = Fold(append(existing, candidate), plansMap, time.Now().UTC())
	return err
}

func newRedeemResponse(tenant *corev1.Tenant, order corev1.QuotaOrder) *corev1.RedeemResponse {
	resp := &corev1.RedeemResponse{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindRedeemResponse},
		Metadata: metav1.ObjectMeta{
			Name:              order.ID,
			Tenant:            tenant.Metadata.Name,
			UID:               order.ID,
			CreationTimestamp: order.CreatedAt,
		},
		Order:  order,
		Tenant: tenant,
	}
	resp.Order.TenantName = tenant.Metadata.Name
	return resp
}

func (s *Service) finishOrderMutation(ctx context.Context, tenant *corev1.Tenant, order corev1.QuotaOrder) *corev1.RedeemResponse {
	reconciled, err := ReconcileSubscriptionTenant(ctx, s.reg, tenant.Metadata.Name, time.Now().UTC())
	if err != nil {
		log.S().Warnw("subscription policy convergence deferred to tenant controller",
			"tenant", tenant.Metadata.Name, "orderID", order.ID, "err", err)
	} else if reconciled != nil {
		tenant = reconciled
	}
	publishTenantReconcile(ctx, s.bus, tenant)
	if freshOrd, getErr := s.quotaDB.GetOrder(ctx, tenant.Metadata.UID, order.ID); getErr == nil {
		order = freshOrd
	}
	return newRedeemResponse(tenant, order)
}

// CreateAdminOrder grants a Paid AdminGrant order (requires non-empty Reason) and converges quota.
func (s *Service) CreateAdminOrder(ctx context.Context, actor, tenantName string, req corev1.CreateQuotaOrderRequest) (*corev1.QuotaOrder, error) {
	if strings.TrimSpace(req.Reason) == "" {
		return nil, fmt.Errorf("%w: reason is required for AdminGrant order", models.ErrBadRequest)
	}
	tenant, err := s.tenantDB.Get(ctx, tenantName)
	if err != nil {
		return nil, err
	}
	if tenant.Spec.BillingMode != corev1.TenantBillingModeSubscription {
		return nil, fmt.Errorf("%w: admin entitlement orders require BillingMode=Subscription", models.ErrConflict)
	}
	order, err := s.buildAdminOrder(ctx, actor, tenant.Metadata.UID, req)
	if err != nil {
		return nil, err
	}
	if err := s.dryRunOrderFold(ctx, tenant.Metadata.UID, order); err != nil {
		return nil, err
	}
	saved, err := s.quotaDB.InsertOrder(ctx, order)
	if err != nil {
		return nil, err
	}
	resp := s.finishOrderMutation(ctx, tenant, saved)
	return &resp.Order, nil
}

func (s *Service) buildAdminOrder(ctx context.Context, actor, tenantUID string, req corev1.CreateQuotaOrderRequest) (corev1.QuotaOrder, error) {
	plan, err := s.planDB.Get(ctx, strings.TrimSpace(req.PlanName))
	if err != nil {
		return corev1.QuotaOrder{}, err
	}
	kind := orderKindFromPlan(plan.PlanKind)
	if req.OrderKind != "" && req.OrderKind != kind {
		return corev1.QuotaOrder{}, fmt.Errorf("%w: orderKind %q does not match plan kind %q", models.ErrBadRequest, req.OrderKind, plan.PlanKind)
	}
	days := req.DurationDays
	if days == 0 && len(plan.Prices) > 0 {
		days = plan.Prices[0].DurationDays
	}
	if days <= 0 {
		days = 30
	}
	paidAt := time.Now().UTC().Truncate(time.Microsecond)
	if req.PaidAt != nil {
		paidAt = req.PaidAt.UTC().Truncate(time.Microsecond)
	}
	exp := paidAt.Add(time.Duration(days) * 24 * time.Hour)
	return corev1.QuotaOrder{
		ID: "grant-" + db.NewUID(), TenantUID: tenantUID, OrderKind: kind,
		PlanName: plan.Metadata.Name, Source: corev1.QuotaOrderSourceAdminGrant,
		Reason: strings.TrimSpace(req.Reason), QuotaDelta: plan.Quota, DurationDays: days,
		State: corev1.QuotaOrderStatePaid, PaidAt: &paidAt, ExpiresAt: &exp, CreatedBy: actor,
	}, nil
}

// CancelOrder cancels an order and re-converges Tenant quota.
func (s *Service) CancelOrder(ctx context.Context, _, tenantName, orderID, reason string) (*corev1.QuotaOrder, error) {
	tenant, err := s.tenantDB.Get(ctx, tenantName)
	if err != nil {
		return nil, err
	}
	cancelled, err := s.quotaDB.CancelOrder(ctx, tenant.Metadata.UID, orderID, strings.TrimSpace(reason))
	if err != nil {
		return nil, mapQuotaRepoError(err)
	}
	resp := s.finishOrderMutation(ctx, tenant, cancelled)
	return &resp.Order, nil
}

// ListOrders returns paginated QuotaOrders for a Tenant filtered by opts.FieldSelector.
func (s *Service) ListOrders(ctx context.Context, tenantName string, opts db.ListOptions) (*metav1.List[corev1.QuotaOrder], error) {
	tenant, err := s.tenantDB.Get(ctx, tenantName)
	if err != nil {
		return nil, err
	}
	items, total, err := s.quotaDB.ListOrders(ctx, tenant.Metadata.UID, opts)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].TenantName = tenant.Metadata.Name
	}
	return &metav1.List[corev1.QuotaOrder]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: corev1.KindQuotaOrderList},
		Metadata: metav1.ListMeta{
			Continue:   pgutil.NextContinue(opts.Offset, len(items), total),
			TotalCount: total,
		},
		Items: items,
	}, nil
}

func mapQuotaRepoError(err error) error {
	switch {
	case errors.Is(err, db.ErrNotFound), errors.Is(err, quotadb.ErrNotFound):
		return db.ErrNotFound
	case errors.Is(err, quotadb.ErrRedeemCodeDisabled),
		errors.Is(err, quotadb.ErrRedeemCodeExpired),
		errors.Is(err, quotadb.ErrRedeemCodeUsed):
		return fmt.Errorf("%w: %v", models.ErrConflict, err)
	default:
		return err
	}
}
