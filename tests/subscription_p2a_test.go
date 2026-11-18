package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	tenantcontroller "github.com/smartcat999/game-panel-cloud/pkg/controller/core/tenant"
	usercontroller "github.com/smartcat999/game-panel-cloud/pkg/controller/iam/user"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	plandb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/resourceplan"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

func (h *securityTestHarness) doJSON(t *testing.T, method, token, path string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Marshal(%s %s): %v", method, path, err)
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, h.httpSrv.URL+path, r)
	if err != nil {
		t.Fatalf("NewRequest(%s %s): %v", method, path, err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll(%s %s): %v", method, path, err)
	}
	return resp.StatusCode, data
}

func (h *securityTestHarness) mustLogin(t *testing.T, identifier, password string) string {
	t.Helper()
	code, raw := h.doJSON(t, http.MethodPost, "", "/apis/iam.gamepanel.io/v1/sessions", map[string]any{
		"apiVersion": iamv1.GroupVersion,
		"kind":       iamv1.SubjectKindUser,
		"identifier": identifier,
		"password":   password,
	})
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("login %s failed: %d %s", identifier, code, raw)
	}
	var sess iamv1.Session
	if err := json.Unmarshal(raw, &sess); err != nil || sess.Token == "" {
		t.Fatalf("decode login %s session: err=%v body=%s", identifier, err, raw)
	}
	return sess.Token
}

func TestSubscriptionP2a(t *testing.T) {
	ctx := context.Background()
	h := setupSecurityTestHarness(t)
	userCtrl := &usercontroller.Controller{}
	tenantCtrl := &tenantcontroller.Controller{}
	adminTok := h.mustLogin(t, "admin", "P@88w0rd")

	// 1. Built-in 4 ResourcePlans seeded & readable; admin stays Manual
	var planList metav1.List[*corev1.ResourcePlan]
	if code, raw := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/resourceplans"); code != http.StatusOK {
		t.Fatalf("GET resourceplans failed: %d %s", code, raw)
	} else if err := json.Unmarshal(raw, &planList); err != nil || len(planList.Items) != 4 {
		t.Fatalf("expected 4 built-in plans, got len=%d err=%v body=%s", len(planList.Items), err, raw)
	}
	adminTenant, err := h.tenants.Get(ctx, "admin")
	if err != nil || adminTenant.Spec.BillingMode != corev1.TenantBillingModeManual {
		t.Fatalf("expected admin BillingMode=Manual, got %+v err=%v", adminTenant, err)
	}

	// 2. Non-admin user alice -> Subscription personal tenant + deterministic 7-day starter Trial
	if code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/iam.gamepanel.io/v1/users", &iamv1.User{
		Metadata: metav1.ObjectMeta{Name: "alice"},
		Spec:     iamv1.UserSpec{Email: "alice@example.com", DisplayName: "Alice", Password: "AlicePassword1!"},
	}); code != http.StatusCreated {
		t.Fatalf("create user alice failed: %d %s", code, raw)
	}
	if err := userCtrl.Reconcile(ctx, h.reg, h.bus, &ctrlruntime.Config{}, mq.Event{Name: "alice"}); err != nil {
		t.Fatalf("userCtrl.Reconcile(alice): %v", err)
	}
	if err := tenantCtrl.Reconcile(ctx, h.reg, h.bus, &ctrlruntime.Config{}, mq.Event{Name: "alice"}); err != nil {
		t.Fatalf("tenantCtrl.Reconcile(alice): %v", err)
	}
	// Re-reconcile & Resync must be idempotent
	if err := tenantCtrl.Reconcile(ctx, h.reg, h.bus, &ctrlruntime.Config{}, mq.Event{Name: "alice"}); err != nil {
		t.Fatalf("second tenantCtrl.Reconcile(alice): %v", err)
	}
	aliceTenant, err := tenantdb.NewDB(h.reg.Pool()).Get(ctx, "alice")
	if err != nil {
		t.Fatalf("get tenant alice: %v", err)
	}
	if aliceTenant.Spec.BillingMode != corev1.TenantBillingModeSubscription || aliceTenant.Spec.Quota["cpu"] != "2" ||
		aliceTenant.Status.Subscription == nil || aliceTenant.Status.Subscription.State != corev1.SubscriptionStateTrialing {
		t.Fatalf("unexpected alice personal subscription state: spec=%+v status=%+v", aliceTenant.Spec, aliceTenant.Status)
	}

	aliceTok := h.mustLogin(t, "alice", "AlicePassword1!")
	if code, _ := h.doGET(t, aliceTok, "/apis/core.gamepanel.io/v1/resourceplans"); code != http.StatusOK {
		t.Fatalf("expected authenticated non-admin user to list resourceplans, got %d", code)
	}
	if code, _ := h.doJSON(t, http.MethodPost, aliceTok, "/apis/core.gamepanel.io/v1/redeemcodes", map[string]any{
		"planName": "pro", "durationDays": 30, "count": 1,
	}); code != http.StatusForbidden {
		t.Fatalf("expected non-admin POST /redeemcodes to return 403, got %d", code)
	}

	// 3. Alice creates Org workspace -> Subscription + 0 finite quota
	if code, raw := h.doJSON(t, http.MethodPost, aliceTok, "/apis/core.gamepanel.io/v1/tenants", map[string]any{
		"apiVersion": corev1.GroupVersion,
		"kind":       "Tenant",
		"metadata":   map[string]any{"name": "alice-org"},
		"spec":       map[string]any{"type": "Organization", "displayName": "Alice Org", "owner": "alice"},
	}); code != http.StatusCreated {
		t.Fatalf("alice create org failed: %d %s", code, raw)
	}
	if err := tenantCtrl.Reconcile(ctx, h.reg, h.bus, &ctrlruntime.Config{}, mq.Event{Name: "alice-org"}); err != nil {
		t.Fatalf("reconcile alice-org: %v", err)
	}
	aliceOrg, err := h.tenants.Get(ctx, "alice-org")
	if err != nil {
		t.Fatalf("get alice-org: %v", err)
	}
	if aliceOrg.Spec.BillingMode != corev1.TenantBillingModeSubscription || aliceOrg.Spec.Quota["cpu"] != "0" ||
		aliceOrg.Status.Subscription == nil || aliceOrg.Status.Subscription.State != corev1.SubscriptionStateNone {
		t.Fatalf("expected Subscription + 0 quota on alice-org, got spec=%+v status=%+v", aliceOrg.Spec, aliceOrg.Status)
	}

	// 4. Create Addon plan + generate codes; addon before base must fail without burning code
	addonPlan := &corev1.ResourcePlan{
		Metadata:    metav1.ObjectMeta{Name: "extra-cpu"},
		DisplayName: "算力扩容包",
		PlanKind:    corev1.PlanKindAddon,
		Published:   true,
		Quota: corev1.ResourceList{
			corev1.ResourceCPU: "2", corev1.ResourceMemory: "4Gi", corev1.ResourceStorage: "20Gi",
			corev1.ResourceInstances: "1", corev1.ResourceBackups: "1", corev1.ResourceBackupStorage: "5Gi",
		},
	}
	if _, err := plandb.NewDB(h.reg.Pool()).Create(ctx, addonPlan); err != nil {
		t.Fatalf("create addon plan: %v", err)
	}
	var addonBatch, raceBatch, proBatch corev1.RedeemCodeBatch
	if code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/core.gamepanel.io/v1/redeemcodes", corev1.CreateRedeemCodeBatchRequest{
		BatchName: "addon-b1", PlanName: "extra-cpu", DurationDays: 15, Count: 1,
	}); code != http.StatusCreated {
		t.Fatalf("create addon batch: %d %s", code, raw)
	} else {
		_ = json.Unmarshal(raw, &addonBatch)
	}
	addonPlain := addonBatch.Codes[0].Code

	// Verify GET /redeemcodes masks plaintext
	if code, raw := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/redeemcodes?batchName=addon-b1"); code != http.StatusOK {
		t.Fatalf("GET redeemcodes: %d %s", code, raw)
	} else if strings.Contains(string(raw), addonPlain) {
		t.Fatalf("plaintext redeem code leaked in GET /redeemcodes")
	}

	// Addon before base -> 409 and code remains unconsumed
	if code, raw := h.doJSON(t, http.MethodPost, aliceTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/redeem", corev1.RedeemRequest{
		Code: addonPlain,
	}); code != http.StatusConflict {
		t.Fatalf("expected 409 redeeming addon before base, got %d %s", code, raw)
	}
	if rcMeta, err := quotadb.NewDB(h.reg.Pool()).GetRedeemCodeByHash(ctx, quotadb.HashRedeemCode(addonPlain)); err != nil || rcMeta.RedeemedCount != 0 {
		t.Fatalf("expected failed pre-validation redeem NOT to burn code, got %+v err=%v", rcMeta, err)
	}

	// 5. Concurrent redemption race across 8 distinct org tenants on 1 single-use code
	if code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/core.gamepanel.io/v1/redeemcodes", corev1.CreateRedeemCodeBatchRequest{
		BatchName: "race-b1", PlanName: "pro", DurationDays: 30, Count: 1,
	}); code != http.StatusCreated {
		t.Fatalf("create race batch: %d %s", code, raw)
	} else {
		_ = json.Unmarshal(raw, &raceBatch)
	}
	for i := 0; i < 8; i++ {
		orgName := fmt.Sprintf("race-org-%d", i)
		if code, raw := h.doJSON(t, http.MethodPost, aliceTok, "/apis/core.gamepanel.io/v1/tenants", map[string]any{
			"apiVersion": corev1.GroupVersion,
			"kind":       "Tenant",
			"metadata":   map[string]any{"name": orgName},
			"spec":       map[string]any{"type": "Organization", "displayName": orgName, "owner": "alice"},
		}); code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", orgName, code, raw)
		}
		if err := tenantCtrl.Reconcile(ctx, h.reg, h.bus, &ctrlruntime.Config{}, mq.Event{Name: orgName}); err != nil {
			t.Fatalf("reconcile %s: %v", orgName, err)
		}
	}
	statuses := make([]int, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			code, _ := h.doJSON(t, http.MethodPost, aliceTok, fmt.Sprintf("/apis/core.gamepanel.io/v1/tenants/race-org-%d/redeem", idx), corev1.RedeemRequest{
				Code: raceBatch.Codes[0].Code,
			})
			statuses[idx] = code
		}(i)
	}
	wg.Wait()
	okCount, conflictCount := 0, 0
	for _, st := range statuses {
		switch st {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
			conflictCount++
		}
	}
	if okCount != 1 || conflictCount != 7 {
		t.Fatalf("expected 1x200 and 7x409 in concurrent redeem race, got statuses=%v", statuses)
	}

	// Redeem pro + addon into alice-org cleanly
	if code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/core.gamepanel.io/v1/redeemcodes", corev1.CreateRedeemCodeBatchRequest{
		BatchName: "pro-b1", PlanName: "pro", DurationDays: 30, Count: 1,
	}); code != http.StatusCreated {
		t.Fatalf("create pro batch: %d %s", code, raw)
	} else {
		_ = json.Unmarshal(raw, &proBatch)
	}
	if code, raw := h.doJSON(t, http.MethodPost, aliceTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/redeem", corev1.RedeemRequest{
		Code: proBatch.Codes[0].Code,
	}); code != http.StatusOK {
		t.Fatalf("redeem pro into alice-org: %d %s", code, raw)
	}
	var addResp corev1.RedeemResponse
	if code, raw := h.doJSON(t, http.MethodPost, aliceTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/redeem", corev1.RedeemRequest{
		Code: addonPlain,
	}); code != http.StatusOK {
		t.Fatalf("redeem previously unburned addon code into alice-org: %d %s", code, raw)
	} else {
		_ = json.Unmarshal(raw, &addResp)
	}
	if addResp.Tenant == nil || addResp.Tenant.Spec.Quota["cpu"] != "10" {
		t.Fatalf("expected stacked pro+addon cpu=10, got %+v", addResp.Tenant)
	}

	// 6. D2 lock blocks admin direct quota edit; displayName edit passes
	curOrg, _ := h.tenants.Get(ctx, "alice-org")
	curOrgEdit := *curOrg
	curOrgEdit.Spec.Quota = corev1.ResourceList{corev1.ResourceCPU: "32"}
	if code, _ := h.doJSON(t, http.MethodPut, adminTok, "/apis/core.gamepanel.io/v1/tenants/alice-org", &curOrgEdit); code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on admin direct quota edit in Subscription mode, got %d", code)
	}
	curOrgNameEdit := *curOrg
	curOrgNameEdit.Spec.DisplayName = "Alice Renamed"
	if code, raw := h.doJSON(t, http.MethodPut, adminTok, "/apis/core.gamepanel.io/v1/tenants/alice-org", &curOrgNameEdit); code != http.StatusOK {
		t.Fatalf("expected 200 OK on non-quota metadata edit, got %d %s", code, raw)
	}

	// 7. AdminGrant validation + Cancel + I1-I4 verification
	if code, _ := h.doJSON(t, http.MethodPost, adminTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/orders", corev1.CreateQuotaOrderRequest{
		PlanName: "team", DurationDays: 30,
	}); code != http.StatusBadRequest {
		t.Fatalf("expected 400 BadRequest for AdminGrant without reason, got %d", code)
	}
	if code, _ := h.doJSON(t, http.MethodPost, aliceTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/orders", corev1.CreateQuotaOrderRequest{
		PlanName: "team", DurationDays: 30, Reason: "try bypass",
	}); code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-admin AdminGrant, got %d", code)
	}
	if code, raw := h.doJSON(t, http.MethodPost, adminTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/orders", corev1.CreateQuotaOrderRequest{
		PlanName: "team", DurationDays: 30, Reason: "VIP upgrade",
	}); code != http.StatusCreated {
		t.Fatalf("admin grant team failed: %d %s", code, raw)
	}
	if code, raw := h.doJSON(t, http.MethodPatch, adminTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/orders/"+addResp.Order.ID, corev1.PatchQuotaOrderRequest{
		State: corev1.QuotaOrderStateCancelled, Reason: "cleanup addon",
	}); code != http.StatusOK {
		t.Fatalf("cancel addon order failed: %d %s", code, raw)
	}
	updatedOrg, _ := h.tenants.Get(ctx, "alice-org")
	if updatedOrg.Spec.Quota["cpu"] != "16" {
		t.Fatalf("expected team cpu=16 after addon cancellation, got %s", updatedOrg.Spec.Quota["cpu"])
	}

	for _, target := range []string{"alice", "alice-org"} {
		code, raw := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/tenants/"+target+"/quotachanges?verify=true")
		if code != http.StatusOK {
			t.Fatalf("verify %s failed: %d %s", target, code, raw)
		}
		var ver corev1.QuotaPolicyVerification
		if err := json.Unmarshal(raw, &ver); err != nil {
			t.Fatalf("decode verification %s: %v", target, err)
		}
		if !ver.Valid || len(ver.Checks) != 4 {
			t.Fatalf("expected 4 valid checks (I1-I4) for %s, got %+v", target, ver)
		}
		for _, chk := range ver.Checks {
			if !chk.Passed {
				t.Fatalf("check %s failed for %s: %+v", chk.ID, target, chk)
			}
		}
	}
	verifySubscriptionListOptions(t, h, adminTok)
}

func verifySubscriptionListOptions(t *testing.T, h *securityTestHarness, adminTok string) {
	t.Helper()
	var rcPage1, rcPage2, rcFiltered metav1.List[corev1.RedeemCode]
	if code, raw := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/redeemcodes?limit=1"); code != http.StatusOK {
		t.Fatalf("GET redeemcodes limit=1: %d %s", code, raw)
	} else if err := json.Unmarshal(raw, &rcPage1); err != nil || len(rcPage1.Items) != 1 || rcPage1.Metadata.Continue != "1" {
		t.Fatalf("expected redeemcodes page1 limit=1 continue=1, got meta=%+v items=%d err=%v", rcPage1.Metadata, len(rcPage1.Items), err)
	}
	if code, raw := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/redeemcodes?limit=1&continue=1"); code != http.StatusOK {
		t.Fatalf("GET redeemcodes continue=1: %d %s", code, raw)
	} else if err := json.Unmarshal(raw, &rcPage2); err != nil || len(rcPage2.Items) != 1 || rcPage2.Items[0].ID == rcPage1.Items[0].ID {
		t.Fatalf("expected distinct redeemcodes page2, p1=%+v p2=%+v err=%v", rcPage1.Items, rcPage2.Items, err)
	}
	if code, raw := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/redeemcodes?fieldSelector=planName=extra-cpu"); code != http.StatusOK {
		t.Fatalf("GET redeemcodes fieldSelector=planName: %d %s", code, raw)
	} else if err := json.Unmarshal(raw, &rcFiltered); err != nil || len(rcFiltered.Items) != 1 || rcFiltered.Items[0].PlanName != "extra-cpu" {
		t.Fatalf("expected 1 extra-cpu redeemcode, got %+v err=%v", rcFiltered.Items, err)
	}
	if code, _ := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/redeemcodes?fieldSelector=foo=bar"); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown redeemcode fieldSelector, got %d", code)
	}

	var ordPage1, ordPage2, ordFiltered metav1.List[corev1.QuotaOrder]
	if code, raw := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/orders?limit=1"); code != http.StatusOK {
		t.Fatalf("GET orders limit=1: %d %s", code, raw)
	} else if err := json.Unmarshal(raw, &ordPage1); err != nil || len(ordPage1.Items) != 1 || ordPage1.Metadata.Continue != "1" {
		t.Fatalf("expected orders page1 limit=1 continue=1, got meta=%+v items=%d err=%v", ordPage1.Metadata, len(ordPage1.Items), err)
	}
	if code, raw := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/orders?limit=1&continue=1"); code != http.StatusOK {
		t.Fatalf("GET orders continue=1: %d %s", code, raw)
	} else if err := json.Unmarshal(raw, &ordPage2); err != nil || len(ordPage2.Items) != 1 || ordPage2.Items[0].ID == ordPage1.Items[0].ID {
		t.Fatalf("expected distinct orders page2, p1=%+v p2=%+v err=%v", ordPage1.Items, ordPage2.Items, err)
	}
	if code, raw := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/orders?fieldSelector=planName=extra-cpu"); code != http.StatusOK {
		t.Fatalf("GET orders fieldSelector=planName: %d %s", code, raw)
	} else if err := json.Unmarshal(raw, &ordFiltered); err != nil || len(ordFiltered.Items) != 1 || ordFiltered.Items[0].PlanName != "extra-cpu" {
		t.Fatalf("expected 1 extra-cpu order, got %+v err=%v", ordFiltered.Items, err)
	}
	if code, _ := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/tenants/alice-org/orders?fieldSelector=foo=bar"); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown order fieldSelector, got %d", code)
	}
}
