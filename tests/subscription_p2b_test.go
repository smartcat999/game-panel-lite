package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	tenantcontroller "github.com/smartcat999/game-panel-cloud/pkg/controller/core/tenant"
	usercontroller "github.com/smartcat999/game-panel-cloud/pkg/controller/iam/user"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/subscription"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	"github.com/smartcat999/game-panel-cloud/tests/fixtures"
)

type outboxSpyMQ struct {
	mu            sync.Mutex
	failRemaining int
	events        []mq.Event
}

func (s *outboxSpyMQ) Publish(_ context.Context, topic string, event mq.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if topic == mq.TopicMetering && s.failRemaining > 0 {
		s.failRemaining--
		return errors.New("injected mq transient error")
	}
	event.Topic = topic
	s.events = append(s.events, event)
	return nil
}

type noopMQSubscription struct{}

func (noopMQSubscription) Unsubscribe() error { return nil }

func (s *outboxSpyMQ) Subscribe(context.Context, string, string, mq.HandlerFunc) (mq.Subscription, error) {
	return noopMQSubscription{}, nil
}

func (s *outboxSpyMQ) Ping(context.Context) error { return nil }
func (s *outboxSpyMQ) Close() error               { return nil }

func TestSubscriptionP2b(t *testing.T) {
	ctx := context.Background()
	h := setupSecurityTestHarness(t)
	userCtrl := &usercontroller.Controller{}
	tenantCtrl := &tenantcontroller.Controller{}
	adminTok := h.mustLogin(t, "admin", "P@88w0rd")

	// Create user bob + personal tenant reconciled
	if code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/iam.gamepanel.io/v1/users", &iamv1.User{
		Metadata: metav1.ObjectMeta{Name: "bob"},
		Spec:     iamv1.UserSpec{Email: "bob@example.com", DisplayName: "Bob", Password: "BobPassword1!"},
	}); code != http.StatusCreated {
		t.Fatalf("create user bob failed: %d %s", code, raw)
	}
	if err := userCtrl.Reconcile(ctx, h.reg, h.bus, &ctrlruntime.Config{}, mq.Event{Name: "bob"}); err != nil {
		t.Fatalf("userCtrl.Reconcile(bob): %v", err)
	}
	if err := tenantCtrl.Reconcile(ctx, h.reg, h.bus, &ctrlruntime.Config{}, mq.Event{Name: "bob"}); err != nil {
		t.Fatalf("tenantCtrl.Reconcile(bob): %v", err)
	}
	bobTok := h.mustLogin(t, "bob", "BobPassword1!")

	// Bob creates organization workspace bob-ws + admin issues 30d starter order
	if code, raw := h.doJSON(t, http.MethodPost, bobTok, "/apis/core.gamepanel.io/v1/tenants", map[string]any{
		"apiVersion": corev1.GroupVersion,
		"kind":       "Tenant",
		"metadata":   map[string]any{"name": "bob-ws"},
		"spec":       map[string]any{"type": "Organization", "displayName": "Bob Workspace", "owner": "bob"},
	}); code != http.StatusCreated {
		t.Fatalf("bob create org failed: %d %s", code, raw)
	}
	var appliedOrd corev1.QuotaOrder
	if code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/core.gamepanel.io/v1/tenants/bob-ws/orders", corev1.CreateQuotaOrderRequest{
		PlanName: "starter", DurationDays: 30, OrderKind: corev1.QuotaOrderKindBasePlan, Reason: "p2b initial grant",
	}); code != http.StatusCreated {
		t.Fatalf("create admin order for bob-ws failed: %d %s", code, raw)
	} else {
		_ = json.Unmarshal(raw, &appliedOrd)
	}
	if err := tenantCtrl.Reconcile(ctx, h.reg, h.bus, &ctrlruntime.Config{}, mq.Event{Name: "bob-ws"}); err != nil {
		t.Fatalf("tenantCtrl.Reconcile(bob-ws): %v", err)
	}
	wsTenant, err := h.tenants.Get(ctx, "bob-ws")
	if err != nil || wsTenant.Status.Subscription == nil || wsTenant.Status.Subscription.ExpiresAt == nil {
		t.Fatalf("expected active subscription on bob-ws, got %+v err=%v", wsTenant, err)
	}
	initialExpiresAt := *wsTenant.Status.Subscription.ExpiresAt

	// Create Active Running instance mc-1
	inst := fixtures.CreateActive(t, ctx, h.reg, h.drivers, "bob-ws", "node-p2b", &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "mc-1"},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: "docker",
			State:            corev1.InstanceStateRunning,
			Image:            "alpine:3.20",
			Ports:            []corev1.InstancePort{{TargetPort: 25565, Protocol: corev1.ProtocolTCP}},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: "1", corev1.ResourceMemory: "1Gi", corev1.ResourceStorage: "5Gi"},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: "1", corev1.ResourceMemory: "1Gi", corev1.ResourceStorage: "5Gi"},
			},
		},
	})
	if inst.Spec.State != corev1.InstanceStateRunning || (inst.Status.ResourceControl != nil && inst.Status.ResourceControl.Suspended) {
		t.Fatalf("expected active unsuspended Running instance, got spec=%s rc=%+v", inst.Spec.State, inst.Status.ResourceControl)
	}

	t.Run("CloudEventsOutboxAndTransientRetry", func(t *testing.T) {
		if code, raw := h.doJSON(t, http.MethodPost, bobTok, "/apis/core.gamepanel.io/v1/tenants", map[string]any{
			"apiVersion": corev1.GroupVersion,
			"kind":       "Tenant",
			"metadata":   map[string]any{"name": "bob-outbox"},
			"spec":       map[string]any{"type": "Organization", "displayName": "Bob Outbox", "owner": "bob"},
		}); code != http.StatusCreated {
			t.Fatalf("bob create outbox org failed: %d %s", code, raw)
		}
		if code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/core.gamepanel.io/v1/tenants/bob-outbox/orders", corev1.CreateQuotaOrderRequest{
			PlanName: "starter", DurationDays: 30, OrderKind: corev1.QuotaOrderKindBasePlan, Reason: "p2b outbox test",
		}); code != http.StatusCreated {
			t.Fatalf("create admin order for bob-outbox failed: %d %s", code, raw)
		}
		outboxTenant, err := h.tenants.Get(ctx, "bob-outbox")
		if err != nil {
			t.Fatalf("get bob-outbox: %v", err)
		}
		spy := &outboxSpyMQ{failRemaining: 1}
		if err := subscription.FlushTenantBillingOutbox(ctx, h.reg, spy, outboxTenant); err == nil {
			t.Fatalf("expected first FlushTenantBillingOutbox to propagate transient MQ failure")
		}
		quotaDB := quotadb.NewDB(h.reg.Pool())
		unexported, err := quotaDB.ListTenantUIDsWithUnexportedBilling(ctx)
		if err != nil || len(unexported) == 0 {
			t.Fatalf("expected unexported billing pending after failed publish, got %v err=%v", unexported, err)
		}

		// Second flush succeeds and emits both CloudEvents 1.0 records
		if err := subscription.FlushTenantBillingOutbox(ctx, h.reg, spy, outboxTenant); err != nil {
			t.Fatalf("second FlushTenantBillingOutbox failed: %v", err)
		}
		var policyCount, orderCount int
		for _, ev := range spy.events {
			if ev.Topic != mq.TopicMetering {
				continue
			}
			switch ev.Type {
			case mq.EventType(corev1.CloudEventQuotaPolicyChanged):
				policyCount++
			case mq.EventType(corev1.CloudEventOrderApplied):
				orderCount++
			}
			ce, err := mq.ParseMeteringCloudEvent(ev)
			if err != nil {
				t.Fatalf("ParseMeteringCloudEvent(%s): %v (raw=%q)", ev.Type, err, ev.Attributes["cloudevent_json"])
			}
			if ce.SpecVersion() != "1.0" {
				t.Fatalf("expected CloudEvents specversion 1.0, got %s", ce.SpecVersion())
			}
		}
		if policyCount == 0 || orderCount == 0 {
			t.Fatalf("expected both policy.changed and order.applied CloudEvents, got policy=%d order=%d", policyCount, orderCount)
		}

		// Third flush is a no-op (zero duplicates)
		prevLen := len(spy.events)
		if err := subscription.FlushTenantBillingOutbox(ctx, h.reg, spy, outboxTenant); err != nil {
			t.Fatalf("third FlushTenantBillingOutbox failed: %v", err)
		}
		if len(spy.events) != prevLen {
			t.Fatalf("expected idempotent 0 additional events on re-flush, got +%d", len(spy.events)-prevLen)
		}
	})

	t.Run("FourStageNotificationsAndInboxIdempotency", func(t *testing.T) {
		cycleExp := time.Now().UTC()
		cycleSub := corev1.TenantSubscriptionStatus{
			State:      corev1.SubscriptionStateActive,
			ExpiresAt:  &cycleExp,
			GraceUntil: func() *time.Time { v := cycleExp.Add(7 * 24 * time.Hour); return &v }(),
		}
		stageTimes := []time.Time{
			cycleExp.Add(-48 * time.Hour),            // D-3
			cycleExp.Add(-12 * time.Hour),            // D-1
			cycleExp.Add(2 * time.Hour),              // D0
			cycleSub.GraceUntil.Add(-12 * time.Hour), // Grace-1d
		}
		for i, ts := range stageTimes {
			inserted, err := subscription.EvaluateSubscriptionNotifications(ctx, h.reg, h.bus, wsTenant, cycleSub, ts)
			if err != nil || !inserted {
				t.Fatalf("stage %d first eval expected inserted=true, got %v err=%v", i, inserted, err)
			}
			insertedAgain, err := subscription.EvaluateSubscriptionNotifications(ctx, h.reg, h.bus, wsTenant, cycleSub, ts)
			if err != nil || insertedAgain {
				t.Fatalf("stage %d second eval expected inserted=false, got %v err=%v", i, insertedAgain, err)
			}
		}
		code, raw := h.doGET(t, bobTok, "/apis/iam.gamepanel.io/v1/users/bob/messages")
		if code != http.StatusOK {
			t.Fatalf("GET bob messages failed: %d %s", code, raw)
		}
		var msgs metav1.List[*iamv1.UserMessage]
		if err := json.Unmarshal(raw, &msgs); err != nil {
			t.Fatalf("decode messages: %v", err)
		}
		subMsgCount := 0
		for _, m := range msgs.Items {
			if m != nil && m.Workspace == "bob-ws" && m.Category == iamv1.MessageCategoryQuota && strings.Contains(m.Title, "[订阅提醒]") {
				subMsgCount++
				if !strings.Contains(m.ActionURL, "/console/usage?tenant=bob-ws&tab=subscription") {
					t.Fatalf("unexpected subscription message ActionURL: %s", m.ActionURL)
				}
			}
		}
		if subMsgCount != 4 {
			t.Fatalf("expected 4 distinct subscription inbox messages for bob-ws, got %d", subMsgCount)
		}
	})

	t.Run("ExpiryFreezeGateAndAutoResume", func(t *testing.T) {
		// Reconcile bob-ws at initialExpiresAt + 1h -> TenantPhaseSuspended + Grace
		frozenTenant, err := subscription.ReconcileSubscriptionTenant(ctx, h.reg, "bob-ws", initialExpiresAt.Add(time.Hour))
		if err != nil || frozenTenant.Status.Phase != corev1.TenantPhaseSuspended ||
			frozenTenant.Status.Subscription == nil || frozenTenant.Status.Subscription.State != corev1.SubscriptionStateGrace {
			t.Fatalf("expected Suspended/Grace tenant phase, got phase=%s sub=%+v err=%v", frozenTenant.Status.Phase, frozenTenant.Status.Subscription, err)
		}
		instRec := instancemodel.NewStageReconciler(h.reg)
		stepInst, err := instRec.ReconcileStep(ctx, "bob-ws", "mc-1")
		if err != nil || stepInst.Status.ResourceControl == nil || !stepInst.Status.ResourceControl.Suspended {
			t.Fatalf("expected ResourceControl.Suspended=true, got rc=%+v err=%v", stepInst.Status.ResourceControl, err)
		}
		if stepInst.Spec.State != corev1.InstanceStateRunning {
			t.Fatalf("expected Spec.State preserved as Running while frozen, got %s", stepInst.Spec.State)
		}
		if corev1.EffectiveInstanceState(stepInst) != corev1.InstanceStateStopped {
			t.Fatalf("expected EffectiveInstanceState=Stopped while frozen")
		}

		// User Start/Restart rejected with 409 while frozen
		if code, raw := h.doJSON(t, http.MethodPut, bobTok, "/apis/core.gamepanel.io/v1/tenants/bob-ws/instances/mc-1/state", map[string]any{
			"state": "Running",
		}); code != http.StatusConflict {
			t.Fatalf("expected 409 on UpdateState(Running) while suspended, got %d %s", code, raw)
		}

		// Renew bob-ws -> Phase=Active -> Instance unfreezes automatically
		if code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/core.gamepanel.io/v1/tenants/bob-ws/orders", corev1.CreateQuotaOrderRequest{
			PlanName: "starter", DurationDays: 30, OrderKind: corev1.QuotaOrderKindBasePlan, Reason: "p2b renewal",
		}); code != http.StatusCreated {
			t.Fatalf("renewal order failed: %d %s", code, raw)
		}
		if _, err := subscription.ReconcileSubscriptionTenant(ctx, h.reg, "bob-ws", time.Now().UTC()); err != nil {
			t.Fatalf("reconcile after renewal: %v", err)
		}
		resumedInst, err := instRec.ReconcileStep(ctx, "bob-ws", "mc-1")
		if err != nil || resumedInst.Status.ResourceControl.Suspended {
			t.Fatalf("expected ResourceControl.Suspended=false after renewal, got rc=%+v err=%v", resumedInst.Status.ResourceControl, err)
		}
		if resumedInst.Spec.State != corev1.InstanceStateRunning || corev1.EffectiveInstanceState(resumedInst) != corev1.InstanceStateRunning {
			t.Fatalf("expected instance to auto-resume to EffectiveInstanceState=Running, got spec=%s", resumedInst.Spec.State)
		}
	})

	t.Run("QuotaChangesResolveOrdersViaBatchFieldSelector", func(t *testing.T) {
		verifyQuotaChangeOrderBatchLookup(t, h, bobTok, "bob-ws")
	})

	t.Run("ReclaimGuardrailsAndPastGraceCleanup", func(t *testing.T) {
		now := time.Now().UTC()
		// Two stacked 30d orders (= 60d total): backdate paid_at to now-60d-1h -> baseExpiresAt=now-1h (Grace)
		if _, err := h.reg.Pool().Exec(ctx,
			"UPDATE quota_orders SET paid_at = $1, expires_at = $2 WHERE tenant_uid = $3",
			now.Add(-60*24*time.Hour-time.Hour), now.Add(-time.Hour), wsTenant.Metadata.UID,
		); err != nil {
			t.Fatalf("backdate orders to grace: %v", err)
		}
		if _, err := subscription.ReconcileSubscriptionTenant(ctx, h.reg, "bob-ws", now); err != nil {
			t.Fatalf("reconcile grace: %v", err)
		}
		if code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/core.gamepanel.io/v1/tenants/bob-ws/reclaim", nil); code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict reclaiming inside grace period, got %d %s", code, raw)
		}

		// Two stacked 30d orders (= 60d total): backdate paid_at to now-70d -> baseExpiresAt=now-10d, graceUntil=now-3d
		if _, err := h.reg.Pool().Exec(ctx,
			"UPDATE quota_orders SET paid_at = $1, expires_at = $2 WHERE tenant_uid = $3",
			now.Add(-70*24*time.Hour), now.Add(-10*24*time.Hour), wsTenant.Metadata.UID,
		); err != nil {
			t.Fatalf("backdate orders past grace: %v", err)
		}
		if _, err := subscription.ReconcileSubscriptionTenant(ctx, h.reg, "bob-ws", now); err != nil {
			t.Fatalf("reconcile past grace: %v", err)
		}

		// Non-admin (tenant owner) forbidden from calling reclaim
		if code, raw := h.doJSON(t, http.MethodPost, bobTok, "/apis/core.gamepanel.io/v1/tenants/bob-ws/reclaim", nil); code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for non-admin reclaim, got %d %s", code, raw)
		}

		// Platform admin reclaims past-grace workspace
		code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/core.gamepanel.io/v1/tenants/bob-ws/reclaim", nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200 OK for admin reclaim past grace, got %d %s", code, raw)
		}
		var reclaimResp corev1.ReclaimTenantResponse
		if err := json.Unmarshal(raw, &reclaimResp); err != nil || reclaimResp.ReclaimedCount != 1 || reclaimResp.Kind != corev1.KindReclaimTenantResponse {
			t.Fatalf("expected ReclaimedCount=1 and Kind=%s, got %+v err=%v raw=%s", corev1.KindReclaimTenantResponse, reclaimResp, err, raw)
		}

		code, raw = h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/tenants/bob-ws/instances/mc-1")
		if code != http.StatusOK {
			t.Fatalf("GET reclaimed instance failed: %d %s", code, raw)
		}
		var deletedInst corev1.Instance
		_ = json.Unmarshal(raw, &deletedInst)
		if deletedInst.Metadata.DeletionTimestamp == nil {
			t.Fatalf("expected DeletionTimestamp set after reclaim, got nil")
		}
	})
}
