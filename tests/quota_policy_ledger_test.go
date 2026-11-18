package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	tenantctrl "github.com/smartcat999/game-panel-cloud/pkg/controller/core/tenant"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
	"github.com/smartcat999/game-panel-cloud/tests/fixtures"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

func testLimits(cpuMillis int64, memGiB int64) corev1.Limits {
	return corev1.Limits{
		CPU:           corev1.Limit{Value: cpuMillis},
		Memory:        corev1.Limit{Value: memGiB << 30},
		Storage:       corev1.Limit{Value: 50 << 30},
		Instances:     corev1.Limit{Value: 5},
		Backups:       corev1.Limit{Value: 5},
		BackupStorage: corev1.Limit{Value: 20 << 30},
	}
}

func TestPostgresQuotaPolicyChainBootstrapAndAdjust(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	quotas := quotadb.NewDB(reg.Pool())
	tenantUID := "tenant-policy-chain-01"

	l1 := testLimits(4000, 8)
	acct, chg1, err := quotas.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       tenantUID,
		UpstreamVersion: 1,
		Target:          quotadb.QuotaPolicy{Limits: l1},
		Kind:            corev1.QuotaPolicyChangeKindAdminAdjust,
	})
	if err != nil {
		t.Fatalf("SyncPolicy bootstrap: %v", err)
	}
	if chg1 == nil || chg1.Plan.Kind != corev1.QuotaPolicyChangeKindBootstrap || chg1.Plan.FromVersion != 0 || chg1.Plan.ToVersion != 1 {
		t.Fatalf("unexpected bootstrap change: %+v", chg1)
	}
	if acct.PolicyVersion != 1 || acct.UpstreamVersion != 1 {
		t.Fatalf("unexpected initial account state: %+v", acct)
	}

	// Non-quota metadata update bumps upstream_version without adding a redundant ledger entry.
	acct2, chgNone, err := quotas.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       tenantUID,
		UpstreamVersion: 2,
		Target:          quotadb.QuotaPolicy{Limits: l1},
		Kind:            corev1.QuotaPolicyChangeKindAdminAdjust,
	})
	if err != nil {
		t.Fatalf("SyncPolicy unchanged limits: %v", err)
	}
	if chgNone != nil || acct2.PolicyVersion != 1 || acct2.UpstreamVersion != 2 {
		t.Fatalf("expected silent upstream advance to 2 without ledger entry, got chg=%+v acct=%+v", chgNone, acct2)
	}

	// Real quota adjustment advances policy_version 1 -> 2 and links Old == l1.
	l2 := testLimits(8000, 16)
	acct3, chg2, err := quotas.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       tenantUID,
		UpstreamVersion: 3,
		Target:          quotadb.QuotaPolicy{Limits: l2},
		Kind:            corev1.QuotaPolicyChangeKindAdminAdjust,
	})
	if err != nil {
		t.Fatalf("SyncPolicy adjust: %v", err)
	}
	if chg2 == nil || chg2.Plan.Kind != corev1.QuotaPolicyChangeKindAdminAdjust || chg2.Plan.FromVersion != 1 || chg2.Plan.ToVersion != 2 {
		t.Fatalf("unexpected adjust change: %+v", chg2)
	}
	if !chg2.Plan.Old.Equal(quotadb.QuotaPolicy{Limits: l1}) || acct3.PolicyVersion != 2 || acct3.UpstreamVersion != 3 {
		t.Fatalf("unexpected adjust linkage or account state: chg=%+v acct=%+v", chg2, acct3)
	}

	report, err := quotas.VerifyPolicyChain(ctx, tenantUID)
	if err != nil || !report.Valid || report.CheckedChanges != 2 {
		t.Fatalf("expected valid chain with 2 changes, got report=%+v err=%v", report, err)
	}
}

func TestPostgresQuotaPolicyCrashRecoveryAndStaleRevision(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	quotas := quotadb.NewDB(reg.Pool())
	tenantUID := "tenant-policy-crash-01"

	l1 := testLimits(4000, 8)
	l2 := testLimits(6000, 12)
	l3 := testLimits(8000, 16)

	if _, _, err := quotas.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       tenantUID,
		UpstreamVersion: 1,
		Target:          quotadb.QuotaPolicy{Limits: l1},
	}); err != nil {
		t.Fatalf("initial SyncPolicy: %v", err)
	}

	// Simulate crash right after inserting source_version=1 in quota_policy_changes before posting to quota_accounts.
	unpostedPlan := quotadb.QuotaPolicyChangePlan{
		ChangeID:              "policy:" + tenantUID + ":1",
		TenantUID:             tenantUID,
		Kind:                  corev1.QuotaPolicyChangeKindAdminAdjust,
		FromVersion:           1,
		ToVersion:             2,
		TenantResourceVersion: 2,
		Old:                   quotadb.QuotaPolicy{Limits: l1},
		Target:                quotadb.QuotaPolicy{Limits: l2},
	}
	hash, err := unpostedPlan.Fingerprint()
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	raw, err := json.Marshal(unpostedPlan)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := db.OpenGORM(reg.Pool()).WithContext(ctx).Create(&corev1.QuotaPolicyChangeRecord{
		ChangeID:      unpostedPlan.ChangeID,
		TenantUID:     tenantUID,
		RequestHash:   hash,
		Plan:          raw,
		SourceVersion: 1,
	}).Error; err != nil {
		t.Fatalf("inject unposted record: %v", err)
	}

	// Unposted source_version=1 must stay invisible until quota_accounts.policy_version advances.
	visibleBefore, totalBefore, pvBefore, err := quotas.ListPolicyChanges(ctx, tenantUID, 50, 0)
	if err != nil || totalBefore != 1 || pvBefore != 1 || len(visibleBefore) != 1 {
		t.Fatalf("expected only committed v1 change before recovery, got len=%d total=%d pv=%d err=%v", len(visibleBefore), totalBefore, pvBefore, err)
	}

	// Recover with an even newer upstream revision (RV=3, L3): pass 1 posts 1->2 (L2), pass 2 records and posts 2->3 (L3).
	acct, chg3, err := quotas.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       tenantUID,
		UpstreamVersion: 3,
		Target:          quotadb.QuotaPolicy{Limits: l3},
		Kind:            corev1.QuotaPolicyChangeKindAdminAdjust,
	})
	if err != nil {
		t.Fatalf("SyncPolicy crash recovery: %v", err)
	}
	if chg3 == nil || acct.PolicyVersion != 3 || acct.UpstreamVersion != 3 || acct.Limits != l3 {
		t.Fatalf("expected recovery + catch-up to v3/L3, got chg=%+v acct=%+v", chg3, acct)
	}

	// Stale upstreamVersion must fail with RevisionChanged without rolling back account limits.
	if _, _, err := quotas.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       tenantUID,
		UpstreamVersion: 2,
		Target:          quotadb.QuotaPolicy{Limits: l1},
	}); !errors.Is(err, corev1.RevisionChanged) {
		t.Fatalf("expected RevisionChanged for stale upstreamVersion=2, got %v", err)
	}

	// Same upstreamVersion with conflicting target must fail with IdentityConflict.
	if _, _, err := quotas.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       tenantUID,
		UpstreamVersion: 3,
		Target:          quotadb.QuotaPolicy{Limits: l1},
	}); !errors.Is(err, corev1.IdentityConflict) {
		t.Fatalf("expected IdentityConflict for mismatched target at RV=3, got %v", err)
	}

	items, total, _, err := quotas.ListPolicyChanges(ctx, tenantUID, 50, 0)
	if err != nil || total != 3 || len(items) != 3 || items[0].Plan.ToVersion != 3 || items[2].Plan.ToVersion != 1 {
		t.Fatalf("unexpected ledger history after recovery: len=%d total=%d items=%+v err=%v", len(items), total, items, err)
	}
	report, err := quotas.VerifyPolicyChain(ctx, tenantUID)
	if err != nil || !report.Valid || report.CheckedChanges != 3 {
		t.Fatalf("expected valid chain after recovery, got report=%+v err=%v", report, err)
	}
}

func TestPostgresQuotaExpiresAtAtomicAdmissionGate(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	quotas := quotadb.NewDB(reg.Pool())
	owner := pgutil.Owner{TenantUID: "tenant-exp-uid", InstanceUID: "inst-exp-uid", InstanceName: "srv-exp"}
	qty := corev1.Resources{CPUMillicores: 1000, MemoryBytes: 2 << 30, StorageBytes: 5 << 30, InstanceCount: 1}
	limits := testLimits(4000, 8)
	past := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)

	if _, _, err := quotas.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       owner.TenantUID,
		UpstreamVersion: 1,
		Target:          quotadb.QuotaPolicy{Limits: limits, ExpiresAt: &past},
		Kind:            corev1.QuotaPolicyChangeKindSubscription,
	}); err != nil {
		t.Fatalf("SyncPolicy expired: %v", err)
	}
	if _, err := quotas.OpenAllocation(ctx, pgutil.AllocationIntent{
		AllocationID: "alloc-exp", Owner: owner, Quantity: qty, ResourceRevision: 1,
	}); err != nil {
		t.Fatalf("OpenAllocation: %v", err)
	}
	cmd := pgutil.Command{
		OperationID: "op-grant-exp-1", Domain: corev1.QuotaDomain, Kind: corev1.Grant, AccountID: owner.TenantUID,
		AllocationID: "alloc-exp", Owner: owner, ExpectedVersion: 1, ResourceRevision: 1, Quantity: qty,
	}
	if _, err := quotas.RegisterCommand(ctx, cmd); err != nil {
		t.Fatalf("RegisterCommand: %v", err)
	}
	if _, err := quotas.TryGrant(ctx, cmd.OperationID); !errors.Is(err, corev1.AdmissionDenied) {
		t.Fatalf("expected AdmissionDenied on expired quota account, got %v", err)
	}

	// Extending ExpiresAt with identical Limits must still produce a policy change and unlock admission.
	future := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	acct, chg, err := quotas.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       owner.TenantUID,
		UpstreamVersion: 2,
		Target:          quotadb.QuotaPolicy{Limits: limits, ExpiresAt: &future},
		Kind:            corev1.QuotaPolicyChangeKindSubscription,
	})
	if err != nil {
		t.Fatalf("SyncPolicy renew expiresAt: %v", err)
	}
	if chg == nil || acct.PolicyVersion != 2 {
		t.Fatalf("expected Expiry extension to advance policyVersion to 2, got chg=%+v acct=%+v", chg, acct)
	}
	cmd2 := cmd
	if _, err := quotas.RegisterCommand(ctx, cmd2); err != nil {
		t.Fatalf("RegisterCommand idempotent retry: %v", err)
	}
	if res, err := quotas.TryGrant(ctx, cmd2.OperationID); err != nil || res.Allocation.Current != qty {
		t.Fatalf("expected TryGrant to succeed after expiry extension, got res=%+v err=%v", res, err)
	}
	report, err := quotas.VerifyPolicyChain(ctx, owner.TenantUID)
	if err != nil || !report.Valid || report.CheckedChanges != 2 {
		t.Fatalf("expected valid chain with 2 changes, got report=%+v err=%v", report, err)
	}
}

func TestQuotaPolicyChangesAPIAndRBAC(t *testing.T) {
	ctx := context.Background()
	h := setupSecurityTestHarness(t)
	h.createUser(t, "alice")
	h.createUser(t, "bob")
	h.createUser(t, "eve")

	studio := h.createTenant(t, "studio", "alice")
	if err := (&tenantctrl.Controller{}).Reconcile(ctx, h.reg, nil, nil, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", "studio", 0, "")); err != nil {
		t.Fatalf("Reconcile studio bootstrap: %v", err)
	}
	if _, err := h.trb.Create(ctx, "studio", &iamv1.TenantRoleBinding{
		Metadata: metav1.ObjectMeta{Name: "bob-operator", Tenant: "studio"},
		RoleRef:  iamv1.RoleRef{Name: "instance-operator"},
		Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "bob"}},
	}); err != nil {
		t.Fatalf("bind bob instance-operator: %v", err)
	}

	adminTok := issueUserToken(t, h, "admin")
	aliceTok := issueUserToken(t, h, "alice")
	bobTok := issueUserToken(t, h, "bob")
	eveTok := issueUserToken(t, h, "eve")

	for _, tok := range []string{adminTok, aliceTok, bobTok} {
		code, body := h.doGET(t, tok, "/apis/core.gamepanel.io/v1/tenants/studio/quotachanges")
		if code != http.StatusOK {
			t.Fatalf("expected 200 for authorized quotachanges GET, got %d body=%s", code, string(body))
		}
	}
	if code, _ := h.doGET(t, eveTok, "/apis/core.gamepanel.io/v1/tenants/studio/quotachanges"); code != http.StatusForbidden {
		t.Fatalf("expected 403 for outsider eve on quotachanges, got %d", code)
	}

	// Update quota and verify pagination + invariants.
	loaded, err := h.tenants.Get(ctx, "studio")
	if err != nil {
		t.Fatalf("Get studio: %v", err)
	}
	loaded.Spec.Quota[corev1.ResourceCPU] = "64"
	if _, err := h.tenants.Update(ctx, "studio", loaded); err != nil {
		t.Fatalf("Update studio quota: %v", err)
	}
	if err := (&tenantctrl.Controller{}).Reconcile(ctx, h.reg, nil, nil, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", studio.Metadata.Name, 0, "")); err != nil {
		t.Fatalf("Reconcile studio update: %v", err)
	}

	code, body := h.doGET(t, bobTok, "/apis/core.gamepanel.io/v1/tenants/studio/quotachanges?limit=1&offset=0")
	if code != http.StatusOK {
		t.Fatalf("GET page 1 status=%d body=%s", code, string(body))
	}
	var page1 metav1.List[corev1.QuotaPolicyChange]
	if err := json.Unmarshal(body, &page1); err != nil || len(page1.Items) != 1 || page1.Metadata.TotalCount != 2 || page1.Metadata.Continue != "1" {
		t.Fatalf("unexpected page1 response: %+v err=%v", page1, err)
	}
	if page1.Items[0].ChangeKind != corev1.QuotaPolicyChangeKindAdminAdjust || page1.Items[0].FromVersion != 1 || page1.Items[0].ToVersion != 2 {
		t.Fatalf("unexpected newest ledger item: %+v", page1.Items[0])
	}

	code2, body2 := h.doGET(t, bobTok, "/apis/core.gamepanel.io/v1/tenants/studio/quotachanges?limit=1&continue="+page1.Metadata.Continue)
	if code2 != http.StatusOK {
		t.Fatalf("GET page 2 status=%d body=%s", code2, string(body2))
	}
	var page2 metav1.List[corev1.QuotaPolicyChange]
	if err := json.Unmarshal(body2, &page2); err != nil || len(page2.Items) != 1 || page2.Items[0].ChangeKind != corev1.QuotaPolicyChangeKindBootstrap {
		t.Fatalf("unexpected page2 response: %+v err=%v", page2, err)
	}

	// Verify invariants endpoint is restricted to platform admins.
	if code, _ := h.doGET(t, aliceTok, "/apis/core.gamepanel.io/v1/tenants/studio/quotachanges?verify=true"); code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin verify=true, got %d", code)
	}
	if code, _ := h.doGET(t, bobTok, "/apis/core.gamepanel.io/v1/tenants/studio/quotachanges?verify=true"); code != http.StatusForbidden {
		t.Fatalf("expected 403 for instance-operator verify=true, got %d", code)
	}
	vCode, vBody := h.doGET(t, adminTok, "/apis/core.gamepanel.io/v1/tenants/studio/quotachanges?verify=true")
	if vCode != http.StatusOK {
		t.Fatalf("expected 200 for admin verify=true, got %d body=%s", vCode, string(vBody))
	}
	var verification corev1.QuotaPolicyVerification
	if err := json.Unmarshal(vBody, &verification); err != nil || !verification.Valid || verification.CheckedChanges != 2 {
		t.Fatalf("unexpected verification response: %+v err=%v", verification, err)
	}

	// Instance admission must wait for Tenant controller single writer (RequireAccountAt -> ErrPolicySyncPending).
	h.createTenant(t, "unsynced", "alice")
	fixtures.ReadyNode(t, ctx, h.reg, "node-a")
	instOp := instancemodel.NewOperator(h.reg, h.drivers, nil)
	createdInst, err := instOp.Create(ctx, "unsynced", &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "wait-quota-01"},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: "docker",
			Image:            "alpine:3.20",
			State:            corev1.InstanceStateRunning,
			NodeSelector:     map[string]string{fixtures.NodeLabel: "node-a"},
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceCPU: "1", corev1.ResourceMemory: "1Gi", corev1.ResourceStorage: "5Gi"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Create unsynced instance: %v", err)
	}
	if _, err := instancemodel.NewStageReconciler(h.reg).ReconcileStep(ctx, "unsynced", createdInst.Metadata.Name); !errors.Is(err, quotadb.ErrPolicySyncPending) {
		t.Fatalf("expected ErrPolicySyncPending before Tenant controller sync, got %v", err)
	}
	fixtures.ReconcileActive(t, ctx, h.reg, "unsynced", createdInst.Metadata.Name)
}

func issueUserToken(t *testing.T, h *securityTestHarness, name string) string {
	t.Helper()
	sess, err := h.sessions.Issue(context.Background(), iamv1.SubjectKindUser, name, "")
	if err != nil {
		t.Fatalf("issue session for %s: %v", name, err)
	}
	return sess.Token
}
