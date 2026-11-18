package subscription

import (
	"errors"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func testQuota(cpu, mem, storage, inst, backups, bstor string) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:           corev1.Quantity(cpu),
		corev1.ResourceMemory:        corev1.Quantity(mem),
		corev1.ResourceStorage:       corev1.Quantity(storage),
		corev1.ResourceInstances:     corev1.Quantity(inst),
		corev1.ResourceBackups:       corev1.Quantity(backups),
		corev1.ResourceBackupStorage: corev1.Quantity(bstor),
	}
}

func testPlansMap() map[string]*corev1.ResourcePlan {
	return map[string]*corev1.ResourcePlan{
		"starter":  {Metadata: metav1.ObjectMeta{Name: "starter"}, DisplayName: "Starter"},
		"standard": {Metadata: metav1.ObjectMeta{Name: "standard"}, DisplayName: "Standard"},
		"pro":      {Metadata: metav1.ObjectMeta{Name: "pro"}, DisplayName: "Pro"},
	}
}

func makeOrder(id, kind, plan, src string, days int, delta corev1.ResourceList, paidAt time.Time) corev1.QuotaOrder {
	ts := paidAt.UTC()
	return corev1.QuotaOrder{
		ID: id, TenantUID: "t-1", OrderKind: kind, PlanName: plan, Source: src,
		QuotaDelta: delta, DurationDays: days, State: corev1.QuotaOrderStatePaid,
		PaidAt: &ts, CreatedAt: ts,
	}
}

func TestFold_BasicAndUpgrade(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	starterQ := testQuota("2", "4Gi", "20Gi", "1", "2", "5Gi")
	proQ := testQuota("8", "16Gi", "100Gi", "6", "10", "30Gi")
	plans := testPlansMap()

	t.Run("empty_orders", func(t *testing.T) {
		res, err := Fold(nil, plans, t0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Subscription.State != corev1.SubscriptionStateNone || res.Phase != corev1.TenantPhaseActive || res.EffectiveQuota["cpu"] != "0" {
			t.Fatalf("unexpected empty fold result: %+v", res)
		}
	})

	t.Run("trial_then_same_plan_renewal", func(t *testing.T) {
		orders := []corev1.QuotaOrder{
			makeOrder("trial:u1", corev1.QuotaOrderKindBasePlan, "starter", corev1.QuotaOrderSourceTrial, 7, starterQ, t0),
			makeOrder("rc-1", corev1.QuotaOrderKindBasePlan, "starter", corev1.QuotaOrderSourceRedeem, 30, starterQ, t0.AddDate(0, 0, 2)),
		}
		res, err := Fold(orders, plans, t0.AddDate(0, 0, 3))
		if err != nil {
			t.Fatalf("Fold failed: %v", err)
		}
		wantExp := t0.AddDate(0, 0, 37)
		if res.Subscription.State != corev1.SubscriptionStateActive || res.Subscription.ExpiresAt == nil || !res.Subscription.ExpiresAt.Equal(wantExp) {
			t.Fatalf("expected Active expiring at %v, got %+v", wantExp, res.Subscription)
		}
	})

	t.Run("upgrade_starter_to_pro_immediately", func(t *testing.T) {
		tUpgrade := t0.AddDate(0, 0, 2)
		orders := []corev1.QuotaOrder{
			makeOrder("trial:u1", corev1.QuotaOrderKindBasePlan, "starter", corev1.QuotaOrderSourceTrial, 7, starterQ, t0),
			makeOrder("rc-pro", corev1.QuotaOrderKindBasePlan, "pro", corev1.QuotaOrderSourceRedeem, 30, proQ, tUpgrade),
		}
		res, err := Fold(orders, plans, tUpgrade.Add(time.Hour))
		if err != nil {
			t.Fatalf("Fold upgrade failed: %v", err)
		}
		if res.Subscription.PlanName != "pro" || res.EffectiveQuota["cpu"] != "8" || !res.Subscription.ExpiresAt.Equal(tUpgrade.AddDate(0, 0, 30)) {
			t.Fatalf("unexpected upgrade fold: %+v", res)
		}
	})
}

func TestFold_DowngradeAndAddons(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	starterQ := testQuota("2", "4Gi", "20Gi", "1", "2", "5Gi")
	proQ := testQuota("8", "16Gi", "100Gi", "6", "10", "30Gi")
	addonQ := testQuota("2", "2Gi", "10Gi", "1", "0", "0")
	plans := testPlansMap()

	t.Run("downgrade_while_active_rejected", func(t *testing.T) {
		orders := []corev1.QuotaOrder{
			makeOrder("o-pro", corev1.QuotaOrderKindBasePlan, "pro", corev1.QuotaOrderSourceRedeem, 30, proQ, t0),
			makeOrder("o-start", corev1.QuotaOrderKindBasePlan, "starter", corev1.QuotaOrderSourceRedeem, 30, starterQ, t0.AddDate(0, 0, 5)),
		}
		if _, err := Fold(orders, plans, t0.AddDate(0, 0, 6)); !errors.Is(err, ErrIncompatiblePlanChange) {
			t.Fatalf("expected ErrIncompatiblePlanChange, got %v", err)
		}
	})

	t.Run("addon_without_active_base_rejected", func(t *testing.T) {
		orders := []corev1.QuotaOrder{
			makeOrder("add-1", corev1.QuotaOrderKindAddonPack, "extra", corev1.QuotaOrderSourceRedeem, 15, addonQ, t0),
		}
		if _, err := Fold(orders, plans, t0.Add(time.Hour)); !errors.Is(err, ErrAddonRequiresActiveBase) {
			t.Fatalf("expected ErrAddonRequiresActiveBase, got %v", err)
		}
	})

	t.Run("addon_stacking_and_deduplicated_active_ids", func(t *testing.T) {
		orders := []corev1.QuotaOrder{
			makeOrder("base-1", corev1.QuotaOrderKindBasePlan, "starter", corev1.QuotaOrderSourceRedeem, 30, starterQ, t0),
			makeOrder("add-expired", corev1.QuotaOrderKindAddonPack, "extra", corev1.QuotaOrderSourceRedeem, 5, addonQ, t0.Add(time.Hour)),
			makeOrder("add-live", corev1.QuotaOrderKindAddonPack, "extra", corev1.QuotaOrderSourceRedeem, 20, addonQ, t0.Add(2*time.Hour)),
		}
		res, err := Fold(orders, plans, t0.AddDate(0, 0, 10))
		if err != nil {
			t.Fatalf("Fold addon failed: %v", err)
		}
		if res.EffectiveQuota["cpu"] != "4" || len(res.Subscription.ActiveOrderIDs) != 2 || res.Subscription.ActiveOrderIDs[1] != "add-live" {
			t.Fatalf("unexpected stacked addon result: quota=%v ids=%v", res.EffectiveQuota, res.Subscription.ActiveOrderIDs)
		}
	})
}

func TestFold_GraceSuspendedAndRecovery(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	starterQ := testQuota("2", "4Gi", "20Gi", "1", "2", "5Gi")
	proQ := testQuota("8", "16Gi", "100Gi", "6", "10", "30Gi")
	plans := testPlansMap()
	baseOrders := []corev1.QuotaOrder{
		makeOrder("trial:u1", corev1.QuotaOrderKindBasePlan, "starter", corev1.QuotaOrderSourceTrial, 7, starterQ, t0),
	}

	graceRes, err := Fold(baseOrders, plans, t0.AddDate(0, 0, 8))
	if err != nil {
		t.Fatalf("grace fold failed: %v", err)
	}
	if graceRes.Subscription.State != corev1.SubscriptionStateGrace || graceRes.Phase != corev1.TenantPhaseSuspended || graceRes.EffectiveQuota["cpu"] != "2" {
		t.Fatalf("unexpected grace state: %+v", graceRes)
	}

	suspRes, err := Fold(baseOrders, plans, t0.AddDate(0, 0, 14))
	if err != nil {
		t.Fatalf("suspended fold failed: %v", err)
	}
	if suspRes.Subscription.State != corev1.SubscriptionStateSuspended || suspRes.Phase != corev1.TenantPhaseSuspended || suspRes.EffectiveQuota["cpu"] != "0" || suspRes.PolicyExpiresAt != nil {
		t.Fatalf("unexpected suspended state: %+v", suspRes)
	}

	recoverOrders := append(baseOrders, makeOrder("rc-pro", corev1.QuotaOrderKindBasePlan, "pro", corev1.QuotaOrderSourceRedeem, 30, proQ, t0.AddDate(0, 0, 15)))
	recRes, err := Fold(recoverOrders, plans, t0.AddDate(0, 0, 15).Add(time.Minute))
	if err != nil {
		t.Fatalf("recovery fold failed: %v", err)
	}
	if recRes.Subscription.State != corev1.SubscriptionStateActive || recRes.Phase != corev1.TenantPhaseActive || recRes.EffectiveQuota["cpu"] != "8" {
		t.Fatalf("unexpected recovery state: %+v", recRes)
	}
}
