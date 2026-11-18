package tests

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	alertmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	"github.com/smartcat999/game-panel-cloud/pkg/models/iam/access"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	auditdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/audit"
	backupdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/backup"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
	"github.com/smartcat999/game-panel-cloud/tests/fixtures"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testpg.Stop()
	os.Exit(code)
}

func TestPostgresDeclarativeStoreCASAndVersionedDelete(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	nodeDB := nodedb.NewDB(reg.Pool())

	node, err := nodeDB.Create(ctx, &corev1.Node{
		Metadata: metav1.ObjectMeta{
			Name:       "node-cas-01",
			Finalizers: []string{"core.gamepanel.io/node-protection"},
		},
		Spec: corev1.NodeSpec{Unschedulable: false},
	})
	if err != nil {
		t.Fatalf("Create Node: %v", err)
	}

	// DeleteIfUID must reject while finalizer is present
	if err := nodeDB.DeleteIfUID(ctx, "node-cas-01", node.Metadata.UID); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("expected ErrConflict when deleting object with finalizers, got %v", err)
	}

	// Remove finalizer via atomic UpdateObject CAS
	node.Metadata.Finalizers = nil
	updated, err := nodeDB.UpdateObject(ctx, node)
	if err != nil {
		t.Fatalf("UpdateObject: %v", err)
	}

	// DeleteIfUID succeeds with matching UID and zero finalizers
	if err := nodeDB.DeleteIfUID(ctx, "node-cas-01", updated.Metadata.UID); err != nil {
		t.Fatalf("DeleteIfUID: %v", err)
	}
}

func TestPostgresQuotaAndNodeAtomicAdmission(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	owner := pgutil.Owner{TenantUID: "tenant-uid-1", InstanceUID: "inst-uid-1", InstanceName: "srv-1"}
	qty := corev1.Resources{CPUMillicores: 2000, MemoryBytes: 4 << 30, StorageBytes: 10 << 30, InstanceCount: 1}

	verifyQuotaAdmissionAndReplay(t, ctx, reg, owner, qty)
	verifyNodeCapacityAndPortAdmission(t, ctx, reg, owner, qty)
}

func verifyQuotaAdmissionAndReplay(t *testing.T, ctx context.Context, reg *repo.Registry, owner pgutil.Owner, qty corev1.Resources) {
	t.Helper()
	quotas := quotadb.NewDB(reg.Pool())
	_, chg, err := quotas.SyncPolicy(ctx, quotadb.PolicySync{
		TenantUID:       owner.TenantUID,
		UpstreamVersion: 1,
		Target: quotadb.QuotaPolicy{Limits: corev1.Limits{
			CPU: corev1.Limit{Value: 4000}, Memory: corev1.Limit{Value: 8 << 30}, Storage: corev1.Limit{Value: 50 << 30},
			Instances: corev1.Limit{Value: 5}, Backups: corev1.Limit{Value: 5}, BackupStorage: corev1.Limit{Value: 20 << 30},
		}},
		Kind: corev1.QuotaPolicyChangeKindAdminAdjust,
	})
	if err != nil {
		t.Fatalf("Quotas.SyncPolicy: %v", err)
	}
	if chg == nil || chg.Plan.Kind != corev1.QuotaPolicyChangeKindBootstrap {
		t.Fatalf("expected initial SyncPolicy to record Bootstrap ledger entry, got %+v", chg)
	}
	if _, err := quotas.OpenAllocation(ctx, pgutil.AllocationIntent{
		AllocationID: "alloc-1", Owner: owner, Quantity: qty, ResourceRevision: 1,
	}); err != nil {
		t.Fatalf("Quotas.OpenAllocation: %v", err)
	}
	grantCmd := pgutil.Command{
		OperationID: "op-grant-1", Domain: corev1.QuotaDomain, Kind: corev1.Grant, AccountID: owner.TenantUID,
		AllocationID: "alloc-1", Owner: owner, ExpectedVersion: 1, ResourceRevision: 1, Quantity: qty,
	}
	if _, err := quotas.RegisterCommand(ctx, grantCmd); err != nil {
		t.Fatalf("Quotas.RegisterCommand: %v", err)
	}
	res1, err := quotas.TryGrant(ctx, grantCmd.OperationID)
	if err != nil || res1.Allocation.Current != qty {
		t.Fatalf("Quotas.TryGrant: res=%+v err=%v", res1, err)
	}
	res2, err := quotas.TryGrant(ctx, grantCmd.OperationID)
	if err != nil || !res2.Replayed {
		t.Fatalf("expected idempotent replay, got res=%+v err=%v", res2, err)
	}
	acct, err := quotas.GetAccount(ctx, owner.TenantUID)
	if err != nil || acct.Allocated != qty {
		t.Fatalf("unexpected account allocated after replay: %+v err=%v", acct, err)
	}
}

func verifyNodeCapacityAndPortAdmission(t *testing.T, ctx context.Context, reg *repo.Registry, owner pgutil.Owner, qty corev1.Resources) {
	t.Helper()
	nodes := nodedb.NewDB(reg.Pool())
	nodeUID := "node-uid-1"
	if _, err := nodes.SyncAccount(ctx, nodeUID, 1, corev1.Limits{
		CPU: corev1.Limit{Value: 8000}, Memory: corev1.Limit{Value: 16 << 30},
		Storage: corev1.Limit{Value: 100 << 30}, Instances: corev1.Limit{Unlimited: true},
	}); err != nil {
		t.Fatalf("NodeResources.SyncAccount: %v", err)
	}
	binding := pgutil.Binding{NodeUID: nodeUID, AttemptID: "attempt-1", PlacementEpoch: 1}
	if _, err := nodes.OpenReservation(ctx, pgutil.PlacementIntent{
		Binding: binding, Owner: owner, Quantity: qty, ResourceRevision: 1,
		Ports: []corev1.PortRequest{{Name: "game", Protocol: "TCP", TargetPort: 25565, PublishedPort: 0}},
	}); err != nil {
		t.Fatalf("NodeResources.OpenReservation: %v", err)
	}
	reserveCmd := pgutil.Command{
		OperationID: "op-node-reserve-1", Domain: corev1.NodeDomain, Kind: corev1.Grant, AccountID: nodeUID,
		AllocationID: binding.AttemptID, Owner: owner, ExpectedVersion: 1, ResourceRevision: 1, Quantity: qty,
	}
	if _, err := nodes.RegisterCommand(ctx, reserveCmd); err != nil {
		t.Fatalf("NodeResources.RegisterCommand: %v", err)
	}
	nodeRes, err := nodes.TryReserve(ctx, reserveCmd.OperationID)
	if err != nil || len(nodeRes.AssignedPorts) != 1 || nodeRes.AssignedPorts[0].PublishedPort < 20000 {
		t.Fatalf("NodeResources.TryReserve: res=%+v err=%v", nodeRes, err)
	}
}

func TestPostgresBootstrapAndInstanceLifecycleReconcile(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	drivers := runtime.NewDefaultRegistry(t.TempDir())

	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("apiserver.Bootstrap: %v", err)
	}
	if _, err := tenantdb.NewDB(reg.Pool()).Create(ctx, &corev1.Tenant{
		Metadata: metav1.ObjectMeta{Name: "default"},
		Spec: corev1.TenantSpec{
			DisplayName: "Default Workspace",
			Type:        corev1.TenantTypeOrganization,
			Owner:       "admin",
			Quota: corev1.ResourceList{
				corev1.ResourceCPU:       "16000m",
				corev1.ResourceMemory:    "32Gi",
				corev1.ResourceStorage:   "200Gi",
				corev1.ResourceInstances: "10",
			},
		},
	}); err != nil {
		t.Fatalf("Create default Tenant: %v", err)
	}

	inst := fixtures.CreateActive(t, ctx, reg, drivers, "default", "node-01", &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "palworld-01"},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: "docker",
			Image:            "thijsvanloef/palworld-server-docker:latest",
			State:            corev1.InstanceStateRunning,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:     "2000m",
					corev1.ResourceMemory:  "4Gi",
					corev1.ResourceStorage: "20Gi",
				},
			},
			Ports: []corev1.InstancePort{
				{Name: "game", Protocol: "UDP", TargetPort: 8211, PublishedPort: 0},
			},
		},
	})
	if inst.Spec.NodeName != "node-01" {
		t.Fatalf("expected scheduled node-01, got %q", inst.Spec.NodeName)
	}
	if inst.Status.ResourceControl == nil || len(inst.Status.ResourceControl.ApprovedPorts) != 1 || inst.Status.ResourceControl.ApprovedPorts[0].PublishedPort == 0 {
		t.Fatalf("expected admitted dynamic host port in ApprovedPorts, got %+v", inst.Status.ResourceControl)
	}

	verifyBackupMutualExclusion(t, ctx, reg, inst)
	verifyInstanceSpecAndStatusDecoupling(t, ctx, reg, drivers, inst)
	verifyGlobalRBACAndDeclarativeAlerts(t, ctx, reg)
}

func verifyInstanceSpecAndStatusDecoupling(t *testing.T, ctx context.Context, reg *repo.Registry, drivers *runtime.Registry, initial *corev1.Instance) {
	t.Helper()
	instOp := instancemodel.NewOperator(reg, drivers, nil)
	instDB := instancedb.NewDB(reg.Pool())

	// User loads instance before background agent/schedule/backup status writes occur.
	userDraft, err := instOp.Get(ctx, initial.Metadata.Tenant, initial.Metadata.Name)
	if err != nil {
		t.Fatalf("Get userDraft: %v", err)
	}
	origRV := userDraft.Metadata.ResourceVersion

	// Background node-agent observation updates StatusVersion without bumping ResourceVersion.
	agentReport := *userDraft
	agentReport.Status.Phase = corev1.InstancePhaseRunning
	agentReport.Status.ObservedGeneration = userDraft.Metadata.Generation
	agentReport.Status.Usage = &corev1.InstanceUsage{CPUMillicores: 850, MemoryBytes: 1 << 30, StorageBytes: 2 << 30}
	afterObs, err := instOp.UpdateStatus(ctx, initial.Metadata.Tenant, initial.Metadata.Name, &agentReport)
	if err != nil {
		t.Fatalf("agent UpdateStatus failed: %v", err)
	}
	if afterObs.Metadata.ResourceVersion != origRV {
		t.Fatalf("expected UpdateStatus not to bump ResourceVersion (orig=%s got=%s)", origRV, afterObs.Metadata.ResourceVersion)
	}
	afterSched, err := instDB.UpdateScheduleStatuses(ctx, initial.Metadata.Tenant, initial.Metadata.Name, initial.Metadata.UID, afterObs.Metadata.StatusVersion, []corev1.InstanceScheduleStatus{
		{ID: "daily-save", LastResult: corev1.ScheduleResultSucceeded},
	})
	if err != nil {
		t.Fatalf("UpdateScheduleStatuses failed: %v", err)
	}

	// User saves Spec change (e.g. addons/env) with original ResourceVersion: must succeed without conflict!
	userDraft.Spec.Env = map[string]string{"MODS": "378160973,362175979"}
	savedSpec, err := instOp.Update(ctx, initial.Metadata.Tenant, initial.Metadata.Name, userDraft)
	if err != nil {
		t.Fatalf("expected user Spec Update not to conflict with background status updates, got: %v", err)
	}
	if savedSpec.Metadata.ResourceVersion == origRV || savedSpec.Spec.Env["MODS"] != "378160973,362175979" {
		t.Fatalf("expected bumped ResourceVersion and saved MODS env, got rv=%s env=%v", savedSpec.Metadata.ResourceVersion, savedSpec.Spec.Env)
	}
	if savedSpec.Status.Phase != corev1.InstancePhaseRunning || len(savedSpec.Status.ScheduleStatuses) != 1 || savedSpec.Metadata.StatusVersion != afterSched.Metadata.StatusVersion {
		t.Fatalf("expected runtime observation and schedule statuses preserved across Spec Update, got status=%+v sv=%s", savedSpec.Status, savedSpec.Metadata.StatusVersion)
	}

	// In-flight node-agent observation carrying older ResourceVersion still succeeds without overwriting new Spec!
	inFlightObs := *afterObs
	inFlightObs.Status.Usage = &corev1.InstanceUsage{CPUMillicores: 920, MemoryBytes: 1 << 30, StorageBytes: 2 << 30}
	afterSecondObs, err := instOp.UpdateStatus(ctx, initial.Metadata.Tenant, initial.Metadata.Name, &inFlightObs)
	if err != nil {
		t.Fatalf("expected in-flight UpdateStatus with older ResourceVersion to succeed via StatusVersion retry, got: %v", err)
	}
	if afterSecondObs.Spec.Env["MODS"] != "378160973,362175979" || afterSecondObs.Metadata.ResourceVersion != savedSpec.Metadata.ResourceVersion {
		t.Fatalf("expected Spec and ResourceVersion untouched by UpdateStatus, got rv=%s env=%v", afterSecondObs.Metadata.ResourceVersion, afterSecondObs.Spec.Env)
	}
}

func verifyGlobalRBACAndDeclarativeAlerts(t *testing.T, ctx context.Context, reg *repo.Registry) {
	t.Helper()
	adminUser := request.UserInfo{Name: "admin", Kind: request.KindUser, Groups: []string{request.GroupAuthenticated}}
	allowed, err := access.AllowsGlobal(ctx, reg, adminUser, corev1.GroupName, "tenants", "get", "*", nil)
	if err != nil || !allowed {
		t.Fatalf("expected bootstrapped admin to be allowed global tenants get, got allowed=%v err=%v", allowed, err)
	}
	guestUser := request.UserInfo{Name: "guest", Kind: request.KindUser, Groups: []string{request.GroupAuthenticated}}
	denied, err := access.AllowsGlobal(ctx, reg, guestUser, corev1.GroupName, "tenants/quota", "update", "default", nil)
	if err != nil || denied {
		t.Fatalf("expected guest to be denied global tenants/quota update, got allowed=%v err=%v", denied, err)
	}

	tenantDB := tenantdb.NewDB(reg.Pool())
	adminTenant, err := tenantDB.Get(ctx, "admin")
	if err != nil {
		t.Fatalf("Get admin Tenant: %v", err)
	}
	adminTenant.Spec.AlertRules = []corev1.AlertRule{
		{ID: "rule-custom-cpu", Title: "CPU 95%", Metric: corev1.AlertMetricCPUUsageRatio, Threshold: 95, Severity: corev1.AlertSeverityCritical, Scope: corev1.AlertScopeAll, Enabled: true},
	}
	adminTenant.Spec.PlatformAlertChannels = []corev1.AlertChannel{
		{ID: "pch-ops", Name: "Ops Webhook", Type: corev1.ChannelTypeWebhook, URL: "https://ops.example.com/hook", Enabled: true},
	}
	if _, err := tenantDB.Update(ctx, adminTenant); err != nil {
		t.Fatalf("Update admin Tenant declarative alert config: %v", err)
	}
	loadedRules := alertmodel.LoadClusterAlertRules(ctx, reg)
	if len(loadedRules) != 1 || loadedRules[0].ID != "rule-custom-cpu" || loadedRules[0].Threshold != 95 || loadedRules[0].ForSeconds != 180 {
		t.Fatalf("expected LoadClusterAlertRules to return declarative spec.alertRules with default ForSeconds=180, got %+v", loadedRules)
	}
	verifyWebhookBatchCoalescing(t, ctx, reg)
}

func verifyWebhookBatchCoalescing(t *testing.T, ctx context.Context, reg *repo.Registry) {
	t.Helper()
	store := auditdb.NewDB(reg.Pool())
	t0 := time.Date(2026, 11, 11, 16, 0, 0, 0, time.UTC)
	notBefore := t0.Add(30 * time.Second)
	buildNew := func() *corev1.Event {
		return &corev1.Event{
			Metadata:     metav1.ObjectMeta{Name: "ntf-batch-1", UID: "ntf-batch-1", Tenant: "admin", CreationTimestamp: t0},
			Actor:        "system:notification-controller",
			Verb:         "create",
			Resource:     "alerts",
			Subresource:  "webhook",
			ResourceName: "ch-1",
			Action:       corev1.WebhookActionPending,
			TargetName:   "Feishu",
			StatusCode:   102,
			Timestamp:    t0,
			Attributes:   map[string]string{"scope": corev1.AlertScopeTenant, "attempts": "0", "notBefore": notBefore.Format(time.RFC3339Nano), "count": "1"},
		}
	}
	ev1, err := store.MergeOrCreateWebhookBatch(ctx, "admin", corev1.AlertScopeTenant, "ch-1", t0, nil, buildNew)
	if err != nil || ev1 == nil || ev1.Metadata.Name != "ntf-batch-1" {
		t.Fatalf("initial MergeOrCreateWebhookBatch failed: ev=%+v err=%v", ev1, err)
	}
	ev2, err := store.MergeOrCreateWebhookBatch(ctx, "admin", corev1.AlertScopeTenant, "ch-1", t0.Add(5*time.Second), func(cur *corev1.Event) (*corev1.Event, bool) {
		cp := *cur
		cp.Attributes = map[string]string{"scope": corev1.AlertScopeTenant, "attempts": "0", "notBefore": cur.Attributes["notBefore"], "count": "2"}
		return &cp, true
	}, func() *corev1.Event {
		t.Fatal("expected merge into open batch rather than creating new record")
		return nil
	})
	if err != nil || ev2.Attributes["count"] != "2" {
		t.Fatalf("expected coalesced count=2, got ev=%+v err=%v", ev2, err)
	}
	if _, _, waitRem, claimed, err := store.ClaimWebhookDelivery(ctx, "ntf-batch-1", false, t0.Add(10*time.Second), 15*time.Second); err != nil || claimed || waitRem != 20*time.Second {
		t.Fatalf("expected premature claim to return waitRem=20s claimed=false, got wait=%v claimed=%v err=%v", waitRem, claimed, err)
	}
	claimedEv, attempt, waitRem, claimed, err := store.ClaimWebhookDelivery(ctx, "ntf-batch-1", false, notBefore, 15*time.Second)
	if err != nil || !claimed || attempt != 1 || waitRem != 0 || claimedEv.Attributes["count"] != "2" {
		t.Fatalf("expected claim at notBefore to succeed with count=2, got claimed=%v attempt=%d ev=%+v err=%v", claimed, attempt, claimedEv, err)
	}
}

func verifyBackupMutualExclusion(t *testing.T, ctx context.Context, reg *repo.Registry, inst *corev1.Instance) {
	t.Helper()
	backups := backupdb.NewDB(reg.Pool())
	owner := pgutil.Owner{TenantUID: inst.Status.ResourceControl.TenantUID, InstanceUID: inst.Metadata.UID, InstanceName: inst.Metadata.Name}
	binding := pgutil.Binding{NodeUID: inst.Status.ResourceControl.CurrentAttempt.NodeUID, AttemptID: inst.Status.ResourceControl.CurrentAttempt.ID, PlacementEpoch: inst.Status.ResourceControl.CurrentAttempt.Epoch}
	storage := s3.Binding{Endpoint: "http://127.0.0.1:9000", Region: "us-east-1", Bucket: "backups", UsePathStyle: true}

	cmd1 := backupdb.Command{
		OperationID: "backup-op-1", BackupUID: "backup-uid-1", AllocationID: "alloc-b1",
		Tenant: inst.Metadata.Tenant, Owner: owner, ObjectKey: "tenants/default/instances/palworld-01/b1.tar.gz",
		MaxBytes: 1 << 30, RequestedMaxBytes: 1 << 30, NodeName: inst.Spec.NodeName,
		Binding: binding, ApprovedRevision: 1, Storage: storage,
	}
	rec1, err := backups.Register(ctx, cmd1)
	if err != nil || rec1.Phase != backupdb.PendingQuota {
		t.Fatalf("first backup Register should succeed: rec=%+v err=%v", rec1, err)
	}

	// Idempotent replay of the exact same command must succeed
	if replay, err := backups.Register(ctx, cmd1); err != nil || replay.Command.OperationID != cmd1.OperationID {
		t.Fatalf("expected idempotent replay for same OperationID, got err=%v", err)
	}

	// Concurrent second backup on the same instance while first is still PendingQuota must conflict
	cmd2 := cmd1
	cmd2.OperationID = "backup-op-2"
	cmd2.BackupUID = "backup-uid-2"
	cmd2.AllocationID = "alloc-b2"
	cmd2.ObjectKey = "tenants/default/instances/palworld-01/b2.tar.gz"
	if _, err := backups.Register(ctx, cmd2); !errors.Is(err, backupdb.ErrConflict) {
		t.Fatalf("expected ErrConflict when registering second concurrent backup on same instance, got %v", err)
	}
}
