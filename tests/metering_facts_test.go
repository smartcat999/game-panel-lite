package tests

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	meteringctrl "github.com/smartcat999/game-panel-cloud/pkg/controller/core/metering"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	meteringmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/metering"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
	"github.com/smartcat999/game-panel-cloud/tests/fixtures"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

type meteringFactsEnv struct {
	ctx     context.Context
	reg     *repo.Registry
	drivers *runtime.Registry
	instOp  instancemodel.Operator
	instDB  *instancedb.DB
}

func setupMeteringFactsEnv(t *testing.T) meteringFactsEnv {
	t.Helper()
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
	return meteringFactsEnv{
		ctx:     ctx,
		reg:     reg,
		drivers: drivers,
		instOp:  instancemodel.NewOperator(reg, drivers, nil),
		instDB:  instancedb.NewDB(reg.Pool()),
	}
}

func createMeteringTestInstance(t *testing.T, env meteringFactsEnv, name string, cpu corev1.Quantity) *corev1.Instance {
	t.Helper()
	if cpu == "" {
		cpu = "2000m"
	}
	return fixtures.CreateActive(t, env.ctx, env.reg, env.drivers, "default", "node-01", &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: name},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: "docker",
			Image:            "itzg/minecraft-server:latest",
			State:            corev1.InstanceStateRunning,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:     cpu,
					corev1.ResourceMemory:  "4Gi",
					corev1.ResourceStorage: "20Gi",
				},
			},
			Ports: []corev1.InstancePort{
				{Name: "game", Protocol: corev1.ProtocolTCP, TargetPort: 25565, PublishedPort: 0},
			},
		},
	})
}

func buildPhaseObservation(base *corev1.Instance, phase corev1.InstancePhase, startedAt *time.Time, observedAt time.Time, transitionID string, usage *corev1.InstanceUsage) *corev1.Instance {
	cp := *base
	oCopy := observedAt.UTC()
	cp.Status.Phase = phase
	cp.Status.ObservedGeneration = base.Metadata.Generation
	cp.Status.TransitionID = transitionID
	if startedAt != nil {
		sCopy := startedAt.UTC()
		cp.Status.StartedAt = &sCopy
	} else {
		cp.Status.StartedAt = nil
	}
	cp.Status.ObservedAt = &oCopy
	cp.Status.Usage = usage
	if base.Status.ResourceControl != nil {
		rc := *base.Status.ResourceControl
		if rc.CurrentAttempt != nil {
			att := *rc.CurrentAttempt
			rc.CurrentAttempt = &att
			rc.ObservedRevision = rc.ApprovedRevision
			rc.ObservedAttemptID = att.ID
			rc.ObservedEpoch = att.Epoch
		}
		if len(rc.ApprovedResources) > 0 {
			res := make(corev1.ResourceList, len(rc.ApprovedResources))
			for k, v := range rc.ApprovedResources {
				res[k] = v
			}
			rc.ObservedResources = res
		}
		if len(rc.ApprovedPorts) > 0 {
			rc.ObservedPorts = append([]corev1.InstancePort(nil), rc.ApprovedPorts...)
		}
		cp.Status.ResourceControl = &rc
	}
	return &cp
}

func buildRunningObservation(base *corev1.Instance, startedAt, observedAt time.Time, transitionID string, usage *corev1.InstanceUsage) *corev1.Instance {
	return buildPhaseObservation(base, corev1.InstancePhaseRunning, &startedAt, observedAt, transitionID, usage)
}

func countExecutionFactRows(t *testing.T, ctx context.Context, reg *repo.Registry, instanceUID, factType string) int {
	t.Helper()
	var total int
	q := `SELECT COUNT(*) FROM instance_execution_facts WHERE instance_uid = $1`
	args := []any{instanceUID}
	if factType != "" {
		q += ` AND fact_type = $2`
		args = append(args, factType)
	}
	if err := reg.Pool().QueryRow(ctx, q, args...).Scan(&total); err != nil {
		t.Fatalf("count execution fact rows: %v", err)
	}
	return total
}

func countMeteringAuditRows(t *testing.T, ctx context.Context, reg *repo.Registry) int {
	t.Helper()
	var total int
	if err := reg.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE resource = 'metering'`).Scan(&total); err != nil {
		t.Fatalf("count metering audit rows: %v", err)
	}
	return total
}

func TestMeteringFactsRollbackAndDeletedRetry(t *testing.T) {
	env := setupMeteringFactsEnv(t)
	inst := createMeteringTestInstance(t, env, "facts-rollback", "2000m")
	uid := inst.Metadata.UID
	origSV := inst.Metadata.StatusVersion
	t0 := time.Now().UTC().Add(-5 * time.Minute).Truncate(time.Millisecond)

	// (a) BeforeCommit error rolls back both instance_observations and instance_execution_facts.
	injectedErr := errors.New("injected pre-commit failure")
	hookCtx := instancedb.WithObservationWriteHooks(env.ctx, instancedb.ObservationWriteHooks{
		BeforeCommit: func(context.Context) error { return injectedErr },
	})
	rep := buildRunningObservation(inst, t0, t0, "precommit-fail", nil)
	if _, err := env.instOp.UpdateStatus(hookCtx, "default", inst.Metadata.Name, rep); !errors.Is(err, injectedErr) {
		t.Fatalf("expected injected pre-commit error, got %v", err)
	}
	afterFail, err := env.instDB.Get(env.ctx, "default", inst.Metadata.Name)
	if err != nil {
		t.Fatalf("Get after rollback: %v", err)
	}
	if afterFail.Metadata.StatusVersion != origSV || afterFail.Status.Phase == corev1.InstancePhaseRunning {
		t.Fatalf("expected StatusVersion=%s and non-Running phase after rollback, got sv=%s phase=%s", origSV, afterFail.Metadata.StatusVersion, afterFail.Status.Phase)
	}
	if total := countExecutionFactRows(t, env.ctx, env.reg, uid, meteringmodel.FactInstanceRunning); total != 0 {
		t.Fatalf("expected 0 execution fact rows after pre-commit rollback, got %d", total)
	}
	if audits := countMeteringAuditRows(t, env.ctx, env.reg); audits != 0 {
		t.Fatalf("expected 0 metering rows in audit_events, got %d", audits)
	}

	// Commit real Running observation at t0.
	validRep := buildRunningObservation(afterFail, t0, t0, "valid-start", nil)
	committed, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, validRep)
	if err != nil {
		t.Fatalf("valid UpdateStatus: %v", err)
	}

	// (b) Stale StatusVersion CAS rejection rolls back execution facts atomically.
	staleObs := *committed
	staleObs.Metadata.StatusVersion = origSV
	staleFact := []instancedb.ExecutionFactEntry{{
		FactID:       "stale-cas-fact",
		Tenant:       "default",
		InstanceUID:  uid,
		InstanceName: committed.Metadata.Name,
		FactType:     meteringmodel.FactInstanceRunning,
		OccurredAt:   t0,
		Attributes:   map[string]string{"uid": uid},
	}}
	if _, err := env.instDB.UpdateObservationWithFacts(env.ctx, &staleObs, staleFact); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("expected ErrConflict on stale SV CAS, got %v", err)
	}
	if _, err := env.instDB.GetExecutionFact(env.ctx, "stale-cas-fact"); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected rolled-back stale fact to be absent, got err=%v", err)
	}

	// Older ObservedAt retry via UpdateStatus must be rejected without creating duplicate facts.
	olderRep := buildRunningObservation(committed, t0, t0.Add(-1*time.Minute), "", nil)
	if _, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, olderRep); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("expected ErrConflict on older ObservedAt observation, got %v", err)
	}
	if total := countExecutionFactRows(t, env.ctx, env.reg, uid, meteringmodel.FactInstanceRunning); total != 1 {
		t.Fatalf("expected 1 Running execution fact after rejected older observation, got %d", total)
	}

	// (c) Idempotent FactInstanceDeleted retry with newer timestamp preserves original OccurredAt without IdentityConflict.
	tDel1 := time.Now().UTC().Truncate(time.Millisecond)
	tDel2 := tDel1.Add(5 * time.Second)
	ev1, err := meteringmodel.RecordInstanceFact(env.ctx, env.reg, meteringmodel.FactInstanceDeleted, committed, tDel1, "delete")
	if err != nil {
		t.Fatalf("first FactInstanceDeleted failed: %v", err)
	}
	ev2, err := meteringmodel.RecordInstanceFact(env.ctx, env.reg, meteringmodel.FactInstanceDeleted, committed, tDel2, "delete")
	if err != nil {
		t.Fatalf("second FactInstanceDeleted retry failed: %v", err)
	}
	if ev1.ID != ev2.ID || !ev2.Timestamp.UTC().Equal(tDel1) {
		t.Fatalf("expected identical fact_id and original timestamp across delete retries (%s/%s != %s/%s)", ev1.ID, tDel1, ev2.ID, ev2.Timestamp.UTC())
	}
	delRec, err := env.instDB.GetExecutionFact(env.ctx, ev1.ID)
	if err != nil {
		t.Fatalf("GetExecutionFact(delete): %v", err)
	}
	if !delRec.OccurredAt.UTC().Equal(tDel1) {
		t.Fatalf("expected original delete OccurredAt %s preserved, got %s", tDel1, delRec.OccurredAt.UTC())
	}
	if audits := countMeteringAuditRows(t, env.ctx, env.reg); audits != 0 {
		t.Fatalf("expected 0 metering rows in audit_events, got %d", audits)
	}
}

func TestMeteringFactsOutcomeUnknownRecovery(t *testing.T) {
	env := setupMeteringFactsEnv(t)
	t0 := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Millisecond)

	// Subcase A: commit succeeded in PG, CommitOverride simulates lost ACK -> OutcomeUnknown recovery preserves original fact_id.
	instA := createMeteringTestInstance(t, env, "facts-unknown-committed", "2000m")
	var firedA atomic.Bool
	var committedOpID string
	ctxA := instancedb.WithObservationWriteHooks(env.ctx, instancedb.ObservationWriteHooks{
		CommitOverride: func(ctx context.Context, tx *gorm.DB, operationID string) error {
			if !firedA.Swap(true) {
				committedOpID = operationID
				if err := pgutil.CommitGORM(ctx, tx, operationID); err != nil {
					return err
				}
				return &corev1.Error{Code: corev1.OutcomeUnknown, OperationID: operationID, Detail: "simulated lost ACK after commit"}
			}
			return pgutil.CommitGORM(ctx, tx, operationID)
		},
	})
	repA := buildRunningObservation(instA, t0, t0, "unknown-commit-ok", nil)
	updatedA, err := env.instOp.UpdateStatus(ctxA, "default", instA.Metadata.Name, repA)
	if err != nil {
		t.Fatalf("OutcomeUnknown committed recovery failed: %v", err)
	}
	if updatedA.Status.Phase != corev1.InstancePhaseRunning || committedOpID == "" {
		t.Fatalf("expected Running phase and non-empty operationID, got phase=%s op=%q", updatedA.Status.Phase, committedOpID)
	}
	if totalA := countExecutionFactRows(t, env.ctx, env.reg, instA.Metadata.UID, meteringmodel.FactInstanceRunning); totalA != 1 {
		t.Fatalf("expected 1 Running execution fact in subcase A, got %d", totalA)
	}
	if _, err := env.instDB.GetExecutionFact(env.ctx, committedOpID); err != nil {
		t.Fatalf("expected original fact_id %s persisted in instance_execution_facts: %v", committedOpID, err)
	}

	// Subcase B: transaction rolled back before commit, CommitOverride returns OutcomeUnknown -> retry preserves original fact_id.
	instB := createMeteringTestInstance(t, env, "facts-unknown-rollback", "2000m")
	var firedB atomic.Bool
	var mu sync.Mutex
	var opIDs []string
	ctxB := instancedb.WithObservationWriteHooks(env.ctx, instancedb.ObservationWriteHooks{
		CommitOverride: func(ctx context.Context, tx *gorm.DB, operationID string) error {
			mu.Lock()
			opIDs = append(opIDs, operationID)
			mu.Unlock()
			if !firedB.Swap(true) {
				_ = tx.Rollback()
				return &corev1.Error{Code: corev1.OutcomeUnknown, OperationID: operationID, Detail: "simulated unknown after rollback"}
			}
			return pgutil.CommitGORM(ctx, tx, operationID)
		},
	})
	repB := buildRunningObservation(instB, t0, t0, "unknown-rollback", nil)
	updatedB, err := env.instOp.UpdateStatus(ctxB, "default", instB.Metadata.Name, repB)
	if err != nil {
		t.Fatalf("OutcomeUnknown rollback retry failed: %v", err)
	}
	if updatedB.Status.Phase != corev1.InstancePhaseRunning {
		t.Fatalf("expected Running phase in subcase B, got %s", updatedB.Status.Phase)
	}
	if len(opIDs) != 2 || opIDs[0] == "" || opIDs[0] != opIDs[1] {
		t.Fatalf("expected recovery retry to reuse exact original fact_id, got %v", opIDs)
	}
	if totalB := countExecutionFactRows(t, env.ctx, env.reg, instB.Metadata.UID, meteringmodel.FactInstanceRunning); totalB != 1 {
		t.Fatalf("expected 1 Running execution fact in subcase B, got %d", totalB)
	}
	if audits := countMeteringAuditRows(t, env.ctx, env.reg); audits != 0 {
		t.Fatalf("expected 0 metering rows in audit_events, got %d", audits)
	}
}

func TestMeteringFactsAfterCommitBeforeGetAndRetryMetering(t *testing.T) {
	env := setupMeteringFactsEnv(t)
	inst := createMeteringTestInstance(t, env, "facts-after-commit-get", "2000m")
	origSV := inst.Metadata.StatusVersion
	startTime := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Millisecond)
	readErr := errors.New("injected post-commit read failure")

	hookCtx := instancedb.WithObservationWriteHooks(env.ctx, instancedb.ObservationWriteHooks{
		AfterCommitBeforeGet: func(context.Context) error { return readErr },
	})
	report1 := buildRunningObservation(inst, startTime, startTime, "start-after-commit-err", nil)
	if _, err := env.instOp.UpdateStatus(hookCtx, "default", inst.Metadata.Name, report1); !errors.Is(err, readErr) {
		t.Fatalf("expected post-commit Get failure %v, got %v", readErr, err)
	}

	// Execution fact must ALREADY exist durably in PostgreSQL before any retry or MQ wakeup!
	if totalBefore := countExecutionFactRows(t, env.ctx, env.reg, inst.Metadata.UID, meteringmodel.FactInstanceRunning); totalBefore != 1 {
		t.Fatalf("expected 1 durable execution fact immediately after post-commit read failure, got %d", totalBefore)
	}
	tmBefore, err := meteringmodel.NewOperator(env.reg).List(env.ctx, meteringmodel.Window24h, db.ListOptions{Tenant: "default"})
	if err != nil || tmBefore == nil || len(tmBefore.Items) != 1 || tmBefore.Items[0].CPUCoreHours <= 0 {
		t.Fatalf("expected TenantMetering to reflect committed execution fact immediately, got list=%+v err=%v", tmBefore, err)
	}

	// Identical agent retry carrying original stale StatusVersion and TransitionID recovers cleanly with 1 fact row.
	report2 := buildRunningObservation(inst, startTime, startTime, "start-after-commit-err", nil)
	report2.Metadata.StatusVersion = origSV
	retried, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, report2)
	if err != nil {
		t.Fatalf("identical agent retry failed: %v", err)
	}
	if retried.Status.Phase != corev1.InstancePhaseRunning {
		t.Fatalf("expected Running phase on retry, got %s", retried.Status.Phase)
	}
	if totalAfter := countExecutionFactRows(t, env.ctx, env.reg, inst.Metadata.UID, meteringmodel.FactInstanceRunning); totalAfter != 1 {
		t.Fatalf("expected strictly 1 Running execution fact after retry, got %d", totalAfter)
	}
	if audits := countMeteringAuditRows(t, env.ctx, env.reg); audits != 0 {
		t.Fatalf("expected 0 metering rows in audit_events, got %d", audits)
	}
}

func approveResizeToCPU(t *testing.T, env meteringFactsEnv, current *corev1.Instance, cpu corev1.Quantity) *corev1.Instance {
	t.Helper()
	req := *current
	resCopy := make(corev1.ResourceList, len(current.Spec.Resources.Limits))
	for k, v := range current.Spec.Resources.Limits {
		resCopy[k] = v
	}
	resCopy[corev1.ResourceCPU] = cpu
	reqCopy := make(corev1.ResourceList, len(current.Spec.Resources.Requests))
	for k, v := range current.Spec.Resources.Requests {
		reqCopy[k] = v
	}
	if len(reqCopy) > 0 {
		reqCopy[corev1.ResourceCPU] = cpu
	}
	req.Spec.Resources.Limits = resCopy
	req.Spec.Resources.Requests = reqCopy
	if _, err := env.instOp.Update(env.ctx, "default", current.Metadata.Name, &req); err != nil {
		t.Fatalf("Update resize CPU=%s: %v", cpu, err)
	}
	reconciler := instancemodel.NewStageReconciler(env.reg)
	for range 8 {
		next, err := reconciler.ReconcileStep(env.ctx, "default", current.Metadata.Name)
		if err != nil {
			t.Fatalf("ReconcileStep resize CPU=%s: %v", cpu, err)
		}
		if next != nil && next.Status.ResourceControl != nil &&
			next.Status.ResourceControl.ApprovedResources[corev1.ResourceCPU] == cpu &&
			next.Status.ResourceControl.Change != nil &&
			next.Status.ResourceControl.Change.Phase == instancemodel.ChangeApplying {
			return next
		}
	}
	t.Fatalf("resize to CPU=%s did not reach ChangeApplying", cpu)
	return nil
}

func TestMeteringFactsHistoricalResizeIntervalsWithoutMQ(t *testing.T) {
	env := setupMeteringFactsEnv(t)
	inst := createMeteringTestInstance(t, env, "facts-resize-history", "1000m")
	t3 := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	t2 := t3.Add(-1 * time.Hour)
	t1 := t2.Add(-1 * time.Hour)
	t0 := t1.Add(-1 * time.Hour)

	// Step 1: Running at 1000m for [t0, t1).
	obs1 := buildRunningObservation(inst, t0, t0, "step-1-run-1c", nil)
	cur, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, obs1)
	if err != nil {
		t.Fatalf("step 1 UpdateStatus: %v", err)
	}

	// Step 2: Approve resize to 4000m and observe at t1 -> [t1, t2).
	approved4c := approveResizeToCPU(t, env, cur, "4000m")
	obs2 := buildRunningObservation(approved4c, t0, t1, "step-2-resize-4c", nil)
	if _, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, obs2); err != nil {
		t.Fatalf("step 2 UpdateStatus: %v", err)
	}
	if _, err := instancemodel.NewStageReconciler(env.reg).ReconcileStep(env.ctx, "default", inst.Metadata.Name); err != nil {
		t.Fatalf("settle 4000m resize: %v", err)
	}
	cur, err = env.instDB.Get(env.ctx, "default", inst.Metadata.Name)
	if err != nil {
		t.Fatalf("Get after 4000m settle: %v", err)
	}

	// Step 3: Approve shrink to 2000m and observe at t2 -> [t2, t3).
	approved2c := approveResizeToCPU(t, env, cur, "2000m")
	obs3 := buildRunningObservation(approved2c, t0, t2, "step-3-resize-2c", nil)
	if _, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, obs3); err != nil {
		t.Fatalf("step 3 UpdateStatus: %v", err)
	}
	if _, err := instancemodel.NewStageReconciler(env.reg).ReconcileStep(env.ctx, "default", inst.Metadata.Name); err != nil {
		t.Fatalf("settle 2000m shrink: %v", err)
	}
	cur, err = env.instDB.Get(env.ctx, "default", inst.Metadata.Name)
	if err != nil {
		t.Fatalf("Get after 2000m settle: %v", err)
	}

	// Step 4: Observe Stopped at t3.
	obs4 := buildPhaseObservation(cur, corev1.InstancePhaseStopped, nil, t3, "step-4-stop", nil)
	if _, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, obs4); err != nil {
		t.Fatalf("step 4 UpdateStatus(Stopped): %v", err)
	}

	// Without MQ events, controller Resync + Operator.List must reconstruct 1*1 + 4*1 + 2*1 = 7.0 CPU core-hours.
	if _, err := (&meteringctrl.Controller{}).Resync(env.ctx, env.reg, nil); err != nil {
		t.Fatalf("metering Resync: %v", err)
	}
	tmList, err := meteringmodel.NewOperator(env.reg).List(env.ctx, meteringmodel.Window24h, db.ListOptions{Tenant: "default"})
	if err != nil || tmList == nil || len(tmList.Items) != 1 {
		t.Fatalf("expected 1 TenantMetering item, got list=%+v err=%v", tmList, err)
	}
	tm := tmList.Items[0]
	if math.Abs(tm.CPUCoreHours-7.0) > 0.05 {
		t.Fatalf("expected ~7.0 CPUCoreHours across 1c->4c->2c intervals, got %.2f (items=%+v)", tm.CPUCoreHours, tm.Instances)
	}
	if len(tm.Instances) != 1 || math.Abs(tm.Instances[0].RunningHours-3.0) > 0.05 {
		t.Fatalf("expected ~3.0 RunningHours, got %+v", tm.Instances)
	}
	if totalFacts := countExecutionFactRows(t, env.ctx, env.reg, cur.Metadata.UID, ""); totalFacts != 4 {
		t.Fatalf("expected 4 immutable execution facts (running, resized, resized, stopped), got %d", totalFacts)
	}
	if audits := countMeteringAuditRows(t, env.ctx, env.reg); audits != 0 {
		t.Fatalf("expected 0 metering rows in audit_events, got %d", audits)
	}
}

func TestMeteringFactsCacheOnlyHeartbeatFastPath(t *testing.T) {
	prevCache := cache.Default()
	cache.SetDefault(newMemTestCache())
	t.Cleanup(func() { cache.SetDefault(prevCache) })

	env := setupMeteringFactsEnv(t)
	inst := createMeteringTestInstance(t, env, "facts-cache-heartbeat", "2000m")
	t0 := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Millisecond)

	rep1 := buildRunningObservation(inst, t0, t0, "hb-start", &corev1.InstanceUsage{
		CPUMillicores: 250,
		MemoryBytes:   1 << 30,
		StorageBytes:  2 << 30,
	})
	updated1, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, rep1)
	if err != nil {
		t.Fatalf("initial Running UpdateStatus: %v", err)
	}
	sv1 := updated1.Metadata.StatusVersion

	// Second observation only updates Usage +10s later (len(facts) == 0 -> cache-only fast path preserves SV).
	rep2 := buildRunningObservation(updated1, t0, t0.Add(10*time.Second), "hb-start", &corev1.InstanceUsage{
		CPUMillicores: 900,
		MemoryBytes:   2 << 30,
		StorageBytes:  2 << 30,
	})
	updated2, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, rep2)
	if err != nil {
		t.Fatalf("heartbeat UpdateStatus: %v", err)
	}
	if updated2.Metadata.StatusVersion != sv1 {
		t.Fatalf("expected cache-only heartbeat to preserve StatusVersion %s, got %s", sv1, updated2.Metadata.StatusVersion)
	}
	if updated2.Status.Usage == nil || updated2.Status.Usage.CPUMillicores != 900 {
		t.Fatalf("expected updated cache Usage CPUMillicores=900, got %+v", updated2.Status.Usage)
	}
	persisted, err := env.instDB.Get(env.ctx, "default", inst.Metadata.Name)
	if err != nil {
		t.Fatalf("Get persisted instance: %v", err)
	}
	if persisted.Metadata.StatusVersion != sv1 || persisted.Status.Usage == nil || persisted.Status.Usage.CPUMillicores != 900 {
		t.Fatalf("expected Get to overlay cached 900m usage while DB StatusVersion stays %s, got sv=%s usage=%+v", sv1, persisted.Metadata.StatusVersion, persisted.Status.Usage)
	}
	if total := countExecutionFactRows(t, env.ctx, env.reg, inst.Metadata.UID, meteringmodel.FactInstanceRunning); total != 1 {
		t.Fatalf("expected 1 Running execution fact across heartbeat, got %d", total)
	}
}

func TestMeteringFactsConcurrentRunningUpdateStatus(t *testing.T) {
	env := setupMeteringFactsEnv(t)
	inst := createMeteringTestInstance(t, env, "facts-concurrent-8", "2000m")
	t0 := time.Now().UTC().Add(-1 * time.Minute).Truncate(time.Millisecond)

	const workers = 8
	ready := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := range workers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rep := buildRunningObservation(inst, t0, t0, fmt.Sprintf("conc-%d", idx), &corev1.InstanceUsage{
				CPUMillicores: 500,
				MemoryBytes:   1 << 30,
				StorageBytes:  2 << 30,
			})
			rep.Metadata.StatusVersion = ""
			<-ready
			_, err := env.instOp.UpdateStatus(env.ctx, "default", inst.Metadata.Name, rep)
			errs <- err
		}(i)
	}
	close(ready)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Running UpdateStatus returned unexpected error: %v", err)
		}
	}
	finalInst, err := env.instDB.Get(env.ctx, "default", inst.Metadata.Name)
	if err != nil {
		t.Fatalf("Get final instance: %v", err)
	}
	if finalInst.Status.Phase != corev1.InstancePhaseRunning {
		t.Fatalf("expected final Running phase, got %s", finalInst.Status.Phase)
	}
	if totalFacts := countExecutionFactRows(t, env.ctx, env.reg, inst.Metadata.UID, meteringmodel.FactInstanceRunning); totalFacts != 1 {
		t.Fatalf("expected strictly 1 Running execution fact across %d concurrent updates, got %d", workers, totalFacts)
	}
	if audits := countMeteringAuditRows(t, env.ctx, env.reg); audits != 0 {
		t.Fatalf("expected 0 metering rows in audit_events across concurrent updates, got %d", audits)
	}
}
