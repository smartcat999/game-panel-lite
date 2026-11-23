package tests

import (
	"context"
	"errors"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	tenantctrl "github.com/smartcat999/game-panel-cloud/pkg/controller/core/tenant"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
	"github.com/smartcat999/game-panel-cloud/tests/fixtures"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

func setupRebudgetEnv(t *testing.T, tenantCPU, tenantMem, nodeCPU, nodeMem corev1.Quantity) (*repo.Registry, *corev1.Tenant, *corev1.Node) {
	t.Helper()
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	tenant, err := tenantdb.NewDB(reg.Pool()).Create(ctx, &corev1.Tenant{
		Metadata: metav1.ObjectMeta{Name: "ws-rebudget"},
		Spec: corev1.TenantSpec{
			DisplayName: "Rebudget Workspace",
			Type:        corev1.TenantTypeOrganization,
			Owner:       "admin",
			Quota: corev1.ResourceList{
				corev1.ResourceCPU:       tenantCPU,
				corev1.ResourceMemory:    tenantMem,
				corev1.ResourceStorage:   "200Gi",
				corev1.ResourceInstances: "10",
			},
		},
	})
	if err != nil {
		t.Fatalf("Create Tenant: %v", err)
	}
	if err := (&tenantctrl.Controller{}).Reconcile(ctx, reg, nil, nil, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", tenant.Metadata.Name, 0, "")); err != nil {
		t.Fatalf("Reconcile Tenant: %v", err)
	}
	node := fixtures.ReadyNode(t, ctx, reg, "node-rebudget")
	node.Status.Allocatable = corev1.ResourceList{
		corev1.ResourceCPU:     nodeCPU,
		corev1.ResourceMemory:  nodeMem,
		corev1.ResourceStorage: "100Gi",
	}
	node, err = nodedb.NewDB(reg.Pool()).UpdateStatus(ctx, node)
	if err != nil {
		t.Fatalf("UpdateStatus node allocatable: %v", err)
	}
	return reg, tenant, node
}

func makeTestInstance(name, nodeName string, cpu, mem, storage corev1.Quantity) *corev1.Instance {
	reqs := corev1.ResourceList{
		corev1.ResourceCPU:     cpu,
		corev1.ResourceMemory:  mem,
		corev1.ResourceStorage: storage,
	}
	return &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: name},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: "docker",
			Image:            "registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/runtime:latest",
			State:            corev1.InstanceStateStopped,
			NodeSelector:     map[string]string{fixtures.NodeLabel: nodeName},
			Resources:        corev1.ResourceRequirements{Requests: reqs, Limits: reqs},
			Ports:            []corev1.InstancePort{{Name: "game", Protocol: corev1.ProtocolUDP, TargetPort: 7777, PublishedPort: 0}},
		},
	}
}

func setInstanceResources(inst *corev1.Instance, cpu, mem, storage corev1.Quantity) {
	reqs := corev1.ResourceList{
		corev1.ResourceCPU:     cpu,
		corev1.ResourceMemory:  mem,
		corev1.ResourceStorage: storage,
	}
	inst.Spec.Resources = corev1.ResourceRequirements{Requests: reqs, Limits: reqs}
}

func TestSchedulingAdmissionDeniedRebudgetToActive(t *testing.T) {
	ctx := context.Background()
	reg, tenant, node := setupRebudgetEnv(t, "16000m", "32Gi", "4000m", "4Gi")
	op := instancemodel.NewOperator(reg, runtime.NewDefaultRegistry(t.TempDir()), nil)
	rec := instancemodel.NewStageReconciler(reg)
	quotas, nodes := quotadb.NewDB(reg.Pool()), nodedb.NewDB(reg.Pool())

	created, err := op.Create(ctx, tenant.Metadata.Name, makeTestInstance("sched-rebudget", node.Metadata.Name, "8000m", "8Gi", "40Gi"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldAllocID := created.Status.ResourceControl.AllocationID

	// Reconcile into StageScheduling where node AdmissionDenied keeps AttemptPending.
	var cur *corev1.Instance
	for range 6 {
		cur, err = rec.ReconcileStep(ctx, tenant.Metadata.Name, created.Metadata.Name)
		if err != nil {
			t.Fatalf("ReconcileStep initial: %v", err)
		}
	}
	if instancemodel.GetStage(cur) != instancemodel.StageScheduling || cur.Status.ResourceControl.CurrentAttempt == nil {
		t.Fatalf("expected StageScheduling with candidate attempt, got %+v", cur.Status.ResourceControl)
	}
	oldAttemptID := cur.Status.ResourceControl.CurrentAttempt.ID

	// User shrinks resources while stuck in StageScheduling; previously returned ErrConflict.
	setInstanceResources(cur, "2000m", "2Gi", "20Gi")
	updated, err := op.Update(ctx, tenant.Metadata.Name, cur.Metadata.Name, cur)
	if err != nil {
		t.Fatalf("Update shrink in StageScheduling: %v", err)
	}
	if !updated.Status.ResourceControl.Rebudgeting || updated.Status.ResourceControl.CurrentAttempt.Phase != instancemodel.AttemptClosing {
		t.Fatalf("expected Rebudgeting=true and AttemptClosing, got %+v", updated.Status.ResourceControl)
	}

	cur = fixtures.ReconcileActive(t, ctx, reg, tenant.Metadata.Name, created.Metadata.Name)
	if cur.Status.ResourceControl.Rebudgeting || cur.Status.ResourceControl.AllocationID == oldAllocID {
		t.Fatalf("expected rotated allocation and Rebudgeting=false, got %+v", cur.Status.ResourceControl)
	}
	if len(cur.Status.ResourceControl.ApprovedPorts) != 1 || cur.Status.ResourceControl.ApprovedPorts[0].PublishedPort == 0 {
		t.Fatalf("expected dynamic port assignment after rebudget, got %+v", cur.Status.ResourceControl.ApprovedPorts)
	}
	oldAlloc, err := quotas.GetAllocation(ctx, oldAllocID)
	if err != nil || oldAlloc.Allocation.State != corev1.AllocationClosed {
		t.Fatalf("expected old quota allocation closed, got %+v err=%v", oldAlloc, err)
	}
	oldAttempt, err := nodes.GetReservation(ctx, oldAttemptID)
	if err != nil || oldAttempt.Allocation.State != corev1.AllocationClosed {
		t.Fatalf("expected old node reservation closed, got %+v err=%v", oldAttempt, err)
	}
	qAcct, err := quotas.GetAccount(ctx, tenant.Metadata.UID)
	if err != nil {
		t.Fatalf("quota GetAccount: %v", err)
	}
	nAcct, err := nodes.GetAccount(ctx, node.Metadata.UID)
	if err != nil {
		t.Fatalf("node GetAccount: %v", err)
	}
	want := corev1.Resources{CPUMillicores: 2000, MemoryBytes: 2 << 30, StorageBytes: 20 << 30, InstanceCount: 1}
	if qAcct.Allocated != want || nAcct.Allocated != want {
		t.Fatalf("expected quota & node allocated %+v, got quota=%+v node=%+v", want, qAcct.Allocated, nAcct.Allocated)
	}
}

func TestPendingQuotaRebudgetAndDeleteDuringRebudget(t *testing.T) {
	ctx := context.Background()
	reg, tenant, node := setupRebudgetEnv(t, "2000m", "2Gi", "8000m", "8Gi")
	op := instancemodel.NewOperator(reg, runtime.NewDefaultRegistry(t.TempDir()), nil)
	rec := instancemodel.NewStageReconciler(reg)
	quotas, nodes := quotadb.NewDB(reg.Pool()), nodedb.NewDB(reg.Pool())

	// Subtest 1: PendingQuota AdmissionDenied -> Rebudget -> Active.
	inst, err := op.Create(ctx, tenant.Metadata.Name, makeTestInstance("pending-rebudget", node.Metadata.Name, "4000m", "4Gi", "30Gi"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	inst, err = rec.ReconcileStep(ctx, tenant.Metadata.Name, inst.Metadata.Name)
	if err != nil || instancemodel.GetStage(inst) != instancemodel.StagePendingQuota {
		t.Fatalf("expected StagePendingQuota after quota denial, got stage=%s err=%v", instancemodel.GetStage(inst), err)
	}
	setInstanceResources(inst, "1000m", "1Gi", "10Gi")
	inst, err = op.Update(ctx, tenant.Metadata.Name, inst.Metadata.Name, inst)
	if err != nil || !inst.Status.ResourceControl.Rebudgeting {
		t.Fatalf("expected Rebudgeting=true in StagePendingQuota, got rc=%+v err=%v", inst.Status.ResourceControl, err)
	}
	inst = fixtures.ReconcileActive(t, ctx, reg, tenant.Metadata.Name, inst.Metadata.Name)
	if inst.Status.ResourceControl.Rebudgeting {
		t.Fatalf("expected Rebudgeting=false once Active, got %+v", inst.Status.ResourceControl)
	}
	want1 := corev1.Resources{CPUMillicores: 1000, MemoryBytes: 1 << 30, StorageBytes: 10 << 30, InstanceCount: 1}
	if qAcct, err := quotas.GetAccount(ctx, tenant.Metadata.UID); err != nil || qAcct.Allocated != want1 {
		t.Fatalf("expected quota allocated %+v, got %+v err=%v", want1, qAcct.Allocated, err)
	}

	// Subtest 2: Rebudget triggered then instance deleted mid-rebudget (no CloseIntentID conflict, full refund to steady state).
	inst2, err := op.Create(ctx, tenant.Metadata.Name, makeTestInstance("delete-mid-rebudget", node.Metadata.Name, "1000m", "1Gi", "10Gi"))
	if err != nil {
		t.Fatalf("Create inst2: %v", err)
	}
	setInstanceResources(inst2, "500m", "512Mi", "5Gi")
	if _, err := op.Update(ctx, tenant.Metadata.Name, inst2.Metadata.Name, inst2); err != nil {
		t.Fatalf("Update inst2 during PendingQuota: %v", err)
	}
	// Delete while Rebudgeting=true without having completed rotation yet.
	if err := op.Delete(ctx, tenant.Metadata.Name, inst2.Metadata.Name); err != nil {
		t.Fatalf("Delete during Rebudgeting: %v", err)
	}
	for range 12 {
		cur, err := rec.ReconcileStep(ctx, tenant.Metadata.Name, inst2.Metadata.Name)
		if err != nil {
			t.Fatalf("ReconcileStep delete during rebudget: %v", err)
		}
		if cur == nil {
			break
		}
	}
	if _, err := op.Get(ctx, tenant.Metadata.Name, inst2.Metadata.Name); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected inst2 deleted cleanly, got err=%v", err)
	}
	if qAcct, err := quotas.GetAccount(ctx, tenant.Metadata.UID); err != nil || qAcct.Allocated != want1 {
		t.Fatalf("expected quota back to %+v after delete, got %+v err=%v", want1, qAcct.Allocated, err)
	}
	if nAcct, err := nodes.GetAccount(ctx, node.Metadata.UID); err != nil || nAcct.Allocated != want1 {
		t.Fatalf("expected node back to %+v after delete, got %+v err=%v", want1, nAcct.Allocated, err)
	}
}

func TestActiveChangeBudgetingCancelRevertRetargetAndFencing(t *testing.T) {
	ctx := context.Background()
	reg, tenant, node := setupRebudgetEnv(t, "8000m", "8Gi", "4000m", "4Gi")
	op := instancemodel.NewOperator(reg, runtime.NewDefaultRegistry(t.TempDir()), nil)
	rec := instancemodel.NewStageReconciler(reg)
	quotas, nodes := quotadb.NewDB(reg.Pool()), nodedb.NewDB(reg.Pool())

	if _, err := op.Create(ctx, tenant.Metadata.Name, makeTestInstance("active-cancel", node.Metadata.Name, "2000m", "2Gi", "20Gi")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	cur := fixtures.ReconcileActive(t, ctx, reg, tenant.Metadata.Name, "active-cancel")
	allocID := cur.Status.ResourceControl.AllocationID
	attempt := cur.Status.ResourceControl.CurrentAttempt
	owner := pgutil.Owner{TenantUID: tenant.Metadata.UID, InstanceUID: cur.Metadata.UID, InstanceName: cur.Metadata.Name}

	// Step 1: Request scale-up to 6000m/6Gi/40Gi -> Quota grants +4000m, Node denies (>4000m node capacity).
	setInstanceResources(cur, "6000m", "6Gi", "40Gi")
	if _, err := op.Update(ctx, tenant.Metadata.Name, cur.Metadata.Name, cur); err != nil {
		t.Fatalf("Update scale-up: %v", err)
	}
	cur, err := rec.ReconcileStep(ctx, tenant.Metadata.Name, cur.Metadata.Name)
	if err != nil {
		t.Fatalf("ReconcileStep budgetResize: %v", err)
	}
	if cur.Status.ResourceControl.Change == nil || cur.Status.ResourceControl.Change.Phase != instancemodel.ChangeBudgeting {
		t.Fatalf("expected stalled ChangeBudgeting after NodeDenied, got %+v", cur.Status.ResourceControl)
	}
	oldChangeID := cur.Status.ResourceControl.Change.ID
	heldDuringDenied := corev1.Resources{CPUMillicores: 6000, MemoryBytes: 6 << 30, StorageBytes: 40 << 30, InstanceCount: 1}
	if qAcct, err := quotas.GetAccount(ctx, tenant.Metadata.UID); err != nil || qAcct.Allocated != heldDuringDenied {
		t.Fatalf("expected quota pre-holding %+v before cancel, got %+v err=%v", heldDuringDenied, qAcct.Allocated, err)
	}

	// Step 2: Revert back to 2000m/2Gi/20Gi -> ChangeCancelling -> refunds exact difference.
	setInstanceResources(cur, "2000m", "2Gi", "20Gi")
	cur, err = op.Update(ctx, tenant.Metadata.Name, cur.Metadata.Name, cur)
	if err != nil || cur.Status.ResourceControl.Change.Phase != instancemodel.ChangeCancelling {
		t.Fatalf("expected ChangeCancelling on revert, got %+v err=%v", cur.Status.ResourceControl, err)
	}
	cur, err = rec.ReconcileStep(ctx, tenant.Metadata.Name, cur.Metadata.Name)
	if err != nil || cur.Status.ResourceControl.Change != nil || cur.Status.ResourceControl.DesiredRevision != 1 {
		t.Fatalf("expected cancelled change cleared and DesiredRevision=1, got %+v err=%v", cur.Status.ResourceControl, err)
	}
	wantBase := corev1.Resources{CPUMillicores: 2000, MemoryBytes: 2 << 30, StorageBytes: 20 << 30, InstanceCount: 1}
	if qAcct, err := quotas.GetAccount(ctx, tenant.Metadata.UID); err != nil || qAcct.Allocated != wantBase {
		t.Fatalf("expected quota refunded to %+v, got %+v err=%v", wantBase, qAcct.Allocated, err)
	}

	// Late/replayed commands referencing cancelled oldChangeID must be fenced with Closed.
	if _, err := nodes.TryIncrease(ctx, "node:Increase:"+oldChangeID); !errors.Is(err, corev1.Closed) {
		t.Fatalf("expected cancelled node:Increase to fail with Closed, got %v", err)
	}
	if _, err := quotas.RegisterCommand(ctx, pgutil.Command{
		OperationID: "quota:SettleDecrease:" + oldChangeID, Domain: corev1.QuotaDomain,
		AccountID: tenant.Metadata.UID, AllocationID: allocID, Owner: owner,
		Kind: corev1.SettleDecrease, ChangePlanID: oldChangeID, ExpectedVersion: 1, ResourceRevision: 2,
	}); !errors.Is(err, corev1.Closed) {
		t.Fatalf("expected cancelled quota SettleDecrease registration to fail with Closed, got %v", err)
	}
	_ = attempt

	// Step 3: Scale 2000m -> 6000m (NodeDenied), then re-target to 3000m/3Gi/30Gi -> approves ChangeApplying.
	setInstanceResources(cur, "6000m", "6Gi", "40Gi")
	cur, err = op.Update(ctx, tenant.Metadata.Name, cur.Metadata.Name, cur)
	if err != nil {
		t.Fatalf("Update 6000m: %v", err)
	}
	cur, err = rec.ReconcileStep(ctx, tenant.Metadata.Name, cur.Metadata.Name)
	if err != nil {
		t.Fatalf("ReconcileStep 6000m deny: %v", err)
	}
	setInstanceResources(cur, "3000m", "3Gi", "30Gi")
	cur, err = op.Update(ctx, tenant.Metadata.Name, cur.Metadata.Name, cur)
	if err != nil || cur.Status.ResourceControl.Change.Phase != instancemodel.ChangeCancelling {
		t.Fatalf("expected ChangeCancelling on re-target, got %+v err=%v", cur.Status.ResourceControl, err)
	}
	cur, err = rec.ReconcileStep(ctx, tenant.Metadata.Name, cur.Metadata.Name)
	if err != nil || cur.Status.ResourceControl.Change == nil || cur.Status.ResourceControl.Change.Phase != instancemodel.ChangeBudgeting {
		t.Fatalf("expected new ChangeBudgeting plan for 3000m, got %+v err=%v", cur.Status.ResourceControl, err)
	}
	cur, err = rec.ReconcileStep(ctx, tenant.Metadata.Name, cur.Metadata.Name)
	if err != nil || cur.Status.ResourceControl.Change == nil || cur.Status.ResourceControl.Change.Phase != instancemodel.ChangeApplying {
		t.Fatalf("expected approved ChangeApplying for 3000m, got %+v err=%v", cur.Status.ResourceControl, err)
	}
	want3c := corev1.Resources{CPUMillicores: 3000, MemoryBytes: 3 << 30, StorageBytes: 30 << 30, InstanceCount: 1}
	if qAcct, err := quotas.GetAccount(ctx, tenant.Metadata.UID); err != nil || qAcct.Allocated != want3c {
		t.Fatalf("expected quota %+v after re-target approval, got %+v err=%v", want3c, qAcct.Allocated, err)
	}
	if nAcct, err := nodes.GetAccount(ctx, node.Metadata.UID); err != nil || nAcct.Allocated != want3c {
		t.Fatalf("expected node %+v after re-target approval, got %+v err=%v", want3c, nAcct.Allocated, err)
	}

	// Step 4: While ChangeApplying is in progress, changing resources MUST still fail with ErrConflict.
	setInstanceResources(cur, "2500m", "2Gi", "20Gi")
	if _, err := op.Update(ctx, tenant.Metadata.Name, cur.Metadata.Name, cur); !errors.Is(err, models.ErrConflict) {
		t.Fatalf("expected ErrConflict when updating resources during ChangeApplying, got %v", err)
	}
}
