package subscription

import (
	"errors"
	"fmt"
	"sort"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
)

var (
	ErrAddonRequiresActiveBase = fmt.Errorf("%w: addon pack requires an active base subscription plan", models.ErrConflict)
	ErrIncompatiblePlanChange  = fmt.Errorf("%w: cannot switch to a non-upgrading plan while current plan is still active", models.ErrConflict)
)

// FoldResult contains the converged Tenant subscription view, effective quota limits, and contributing order IDs.
type FoldResult struct {
	Subscription    corev1.TenantSubscriptionStatus
	EffectiveLimits corev1.Limits
	EffectiveQuota  corev1.ResourceList
	PolicyExpiresAt *time.Time
	OrderIDs        []string
	Phase           corev1.TenantPhase
}

type addonSlot struct {
	id        string
	res       corev1.Resources
	startsAt  time.Time
	expiresAt time.Time
}

type foldState struct {
	hasBase       bool
	basePlan      string
	baseSource    string
	baseRes       corev1.Resources
	baseExpiresAt time.Time
	addons        []addonSlot
	baseOrderIDs  []string
	cycleOrderIDs []string
}

// Fold deterministically reduces Paid QuotaOrders chronologically into the effective quota state at now.
func Fold(orders []corev1.QuotaOrder, plans map[string]*corev1.ResourcePlan, now time.Time) (FoldResult, error) {
	gracePeriod := config.Get().Controller.SubscriptionGracePeriod
	if gracePeriod <= 0 {
		gracePeriod = corev1.DefaultSubscriptionGraceDays * 24 * time.Hour
	}
	return FoldWithGrace(orders, plans, now, gracePeriod)
}

// FoldWithGrace deterministically reduces Paid QuotaOrders using an explicit grace duration after expiresAt.
func FoldWithGrace(orders []corev1.QuotaOrder, plans map[string]*corev1.ResourcePlan, now time.Time, gracePeriod time.Duration) (FoldResult, error) {
	now = now.UTC().Truncate(time.Microsecond)
	if gracePeriod <= 0 {
		gracePeriod = corev1.DefaultSubscriptionGraceDays * 24 * time.Hour
	}
	paid := sortedPaidOrders(orders)
	var st foldState
	for _, ord := range paid {
		if err := st.applyOrder(ord); err != nil {
			return FoldResult{}, err
		}
	}
	return buildFoldResult(st, plans, now, gracePeriod)
}

func sortedPaidOrders(orders []corev1.QuotaOrder) []corev1.QuotaOrder {
	paid := make([]corev1.QuotaOrder, 0, len(orders))
	for _, o := range orders {
		if o.State == corev1.QuotaOrderStatePaid {
			paid = append(paid, o)
		}
	}
	sort.Slice(paid, func(i, j int) bool {
		pi := orderSortTime(paid[i])
		pj := orderSortTime(paid[j])
		if !pi.Equal(pj) {
			return pi.Before(pj)
		}
		if !paid[i].CreatedAt.Equal(paid[j].CreatedAt) {
			return paid[i].CreatedAt.Before(paid[j].CreatedAt)
		}
		return paid[i].ID < paid[j].ID
	})
	return paid
}

func orderSortTime(o corev1.QuotaOrder) time.Time {
	if o.PaidAt != nil {
		return o.PaidAt.UTC().Truncate(time.Microsecond)
	}
	return o.CreatedAt.UTC().Truncate(time.Microsecond)
}

func (s *foldState) applyOrder(o corev1.QuotaOrder) error {
	if o.PaidAt == nil {
		return fmt.Errorf("%w: paid order %s is missing paidAt", models.ErrBadRequest, o.ID)
	}
	if o.DurationDays <= 0 {
		return fmt.Errorf("%w: order %s durationDays must be positive", models.ErrBadRequest, o.ID)
	}
	paidAt := o.PaidAt.UTC().Truncate(time.Microsecond)
	res, err := corev1.ParseResources(o.QuotaDelta)
	if err != nil {
		return fmt.Errorf("%w: invalid order %s quotaDelta: %v", models.ErrBadRequest, o.ID, err)
	}
	dur := time.Duration(o.DurationDays) * 24 * time.Hour
	switch o.OrderKind {
	case corev1.QuotaOrderKindBasePlan:
		return s.applyBaseOrder(o, res, paidAt, dur)
	case corev1.QuotaOrderKindAddonPack:
		return s.applyAddonOrder(o, res, paidAt, dur)
	default:
		return fmt.Errorf("%w: unsupported orderKind %q", models.ErrBadRequest, o.OrderKind)
	}
}

func (s *foldState) applyBaseOrder(o corev1.QuotaOrder, res corev1.Resources, paidAt time.Time, dur time.Duration) error {
	if !s.hasBase || !paidAt.Before(s.baseExpiresAt) {
		s.hasBase = true
		s.basePlan = o.PlanName
		s.baseSource = o.Source
		s.baseRes = res
		s.baseExpiresAt = paidAt.Add(dur)
		s.pruneExpiredAddonsAt(paidAt)
		s.baseOrderIDs = []string{o.ID}
		s.cycleOrderIDs = append(activeSlotIDs(s.addons), o.ID)
		return nil
	}
	if o.PlanName == s.basePlan {
		s.baseExpiresAt = s.baseExpiresAt.Add(dur)
		s.baseRes = res
		if o.Source != corev1.QuotaOrderSourceTrial {
			s.baseSource = o.Source
		}
		s.baseOrderIDs = append(s.baseOrderIDs, o.ID)
		s.cycleOrderIDs = append(s.cycleOrderIDs, o.ID)
		return nil
	}
	if coversResources(res, s.baseRes) {
		s.basePlan = o.PlanName
		s.baseSource = o.Source
		s.baseRes = res
		s.baseExpiresAt = paidAt.Add(dur)
		s.baseOrderIDs = append(s.baseOrderIDs, o.ID)
		s.cycleOrderIDs = append(s.cycleOrderIDs, o.ID)
		return nil
	}
	return ErrIncompatiblePlanChange
}

func (s *foldState) applyAddonOrder(o corev1.QuotaOrder, res corev1.Resources, paidAt time.Time, dur time.Duration) error {
	if !s.hasBase || !paidAt.Before(s.baseExpiresAt) {
		return ErrAddonRequiresActiveBase
	}
	s.addons = append(s.addons, addonSlot{
		id:        o.ID,
		res:       res,
		startsAt:  paidAt,
		expiresAt: paidAt.Add(dur),
	})
	s.cycleOrderIDs = append(s.cycleOrderIDs, o.ID)
	return nil
}

func (s *foldState) pruneExpiredAddonsAt(t time.Time) {
	kept := s.addons[:0]
	for _, a := range s.addons {
		if a.expiresAt.After(t) {
			kept = append(kept, a)
		}
	}
	s.addons = kept
}

func activeSlotIDs(slots []addonSlot) []string {
	out := make([]string, 0, len(slots))
	for _, a := range slots {
		out = append(out, a.id)
	}
	return out
}

func coversResources(next, prev corev1.Resources) bool {
	return next.CPUMillicores >= prev.CPUMillicores &&
		next.MemoryBytes >= prev.MemoryBytes &&
		next.StorageBytes >= prev.StorageBytes &&
		next.InstanceCount >= prev.InstanceCount &&
		next.BackupsCount >= prev.BackupsCount &&
		next.BackupStorageBytes >= prev.BackupStorageBytes
}

func buildFoldResult(st foldState, plans map[string]*corev1.ResourcePlan, now time.Time, gracePeriod time.Duration) (FoldResult, error) {
	zeroQuota := corev1.ZeroQuotaResourceList()
	zeroLimits, _ := corev1.NormalizeLimits(zeroQuota, nil)
	if !st.hasBase {
		return FoldResult{
			Subscription: corev1.TenantSubscriptionStatus{
				State:          corev1.SubscriptionStateNone,
				EffectiveQuota: zeroQuota,
			},
			EffectiveLimits: zeroLimits,
			EffectiveQuota:  zeroQuota,
			Phase:           corev1.TenantPhaseActive,
		}, nil
	}
	return evaluateCycleAt(st, plans, now, gracePeriod, zeroQuota, zeroLimits)
}

func evaluateCycleAt(
	st foldState,
	plans map[string]*corev1.ResourcePlan,
	now time.Time,
	gracePeriod time.Duration,
	zeroQuota corev1.ResourceList,
	zeroLimits corev1.Limits,
) (FoldResult, error) {
	exp := st.baseExpiresAt
	grace := exp.Add(gracePeriod)
	display := st.basePlan
	if p := plans[st.basePlan]; p != nil && p.DisplayName != "" {
		display = p.DisplayName
	}
	if !now.Before(grace) {
		baseQuota, _, _ := formatResources(st.baseRes)
		return FoldResult{
			Subscription: corev1.TenantSubscriptionStatus{
				PlanName:        st.basePlan,
				PlanDisplayName: display,
				State:           corev1.SubscriptionStateSuspended,
				ExpiresAt:       &exp,
				GraceUntil:      &grace,
				BaseQuota:       baseQuota,
				EffectiveQuota:  zeroQuota,
			},
			EffectiveLimits: zeroLimits,
			EffectiveQuota:  zeroQuota,
			OrderIDs:        append([]string(nil), st.cycleOrderIDs...),
			Phase:           corev1.TenantPhaseSuspended,
		}, nil
	}
	return buildNonSuspendedResult(st, display, exp, grace, now)
}

func formatResources(r corev1.Resources) (corev1.ResourceList, corev1.Limits, error) {
	limits, err := corev1.NormalizeLimits(corev1.CanonicalResources(r), nil)
	if err != nil {
		return nil, corev1.Limits{}, err
	}
	rl, _ := corev1.FormatLimits(limits)
	return rl, limits, nil
}

func buildNonSuspendedResult(st foldState, display string, exp, grace, now time.Time) (FoldResult, error) {
	addonRes, activeAddonIDs, err := sumActiveAddons(st.addons, now)
	if err != nil {
		return FoldResult{}, err
	}
	totalRes, err := st.baseRes.Add(addonRes)
	if err != nil {
		return FoldResult{}, fmt.Errorf("%w: quota overflow: %v", models.ErrConflict, err)
	}
	effectiveQuota, effectiveLimits, err := formatResources(totalRes)
	if err != nil {
		return FoldResult{}, err
	}
	baseQuota, _, _ := formatResources(st.baseRes)
	state := corev1.SubscriptionStateActive
	phase := corev1.TenantPhaseActive
	if !now.Before(exp) {
		state = corev1.SubscriptionStateGrace
		phase = corev1.TenantPhaseSuspended
	} else if st.baseSource == corev1.QuotaOrderSourceTrial {
		state = corev1.SubscriptionStateTrialing
	}
	var addonQuota corev1.ResourceList
	if !addonRes.IsZero() {
		addonQuota, _, _ = formatResources(addonRes)
	}
	return FoldResult{
		Subscription: corev1.TenantSubscriptionStatus{
			PlanName:        st.basePlan,
			PlanDisplayName: display,
			State:           state,
			ExpiresAt:       &exp,
			GraceUntil:      &grace,
			BaseQuota:       baseQuota,
			AddonQuota:      addonQuota,
			EffectiveQuota:  effectiveQuota,
			ActiveOrderIDs:  append(append([]string(nil), st.baseOrderIDs...), activeAddonIDs...),
		},
		EffectiveLimits: effectiveLimits,
		EffectiveQuota:  effectiveQuota,
		PolicyExpiresAt: &exp,
		OrderIDs:        append([]string(nil), st.cycleOrderIDs...),
		Phase:           phase,
	}, nil
}

func sumActiveAddons(addons []addonSlot, now time.Time) (corev1.Resources, []string, error) {
	var sum corev1.Resources
	var ids []string
	var err error
	for _, a := range addons {
		if now.Before(a.startsAt) || !now.Before(a.expiresAt) {
			continue
		}
		sum, err = sum.Add(a.res)
		if err != nil {
			return corev1.Resources{}, nil, fmt.Errorf("%w: addon quota overflow: %v", models.ErrConflict, err)
		}
		ids = append(ids, a.id)
	}
	return sum, ids, nil
}

var _ = errors.Is
