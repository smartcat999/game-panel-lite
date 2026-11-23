package instance

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	backupdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/backup"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	runtimeclassdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/runtimeclass"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

type StageReconciler struct {
	db       *instancedb.DB
	tenantDB *tenantdb.DB
	rcDB     *runtimeclassdb.DB
	nodeDB   *nodedb.DB
	quotaDB  *quotadb.DB
	backupDB *backupdb.DB
	reg      *repo.Registry
}

func NewStageReconciler(reg *repo.Registry) *StageReconciler {
	return &StageReconciler{
		db:       instancedb.NewDB(reg.Pool()),
		tenantDB: tenantdb.NewDB(reg.Pool()),
		rcDB:     runtimeclassdb.NewDB(reg.Pool()),
		nodeDB:   nodedb.NewDB(reg.Pool()),
		quotaDB:  quotadb.NewDB(reg.Pool()),
		backupDB: backupdb.NewDB(reg.Pool()),
		reg:      reg,
	}
}
func controlError(code corev1.Code, detail string) error {
	return &corev1.Error{Code: code, Detail: detail}
}
func budget(list corev1.ResourceList) (corev1.Resources, error) {
	q, err := corev1.ParseResources(list)
	if err == nil {
		err = q.ValidateInstance()
	}
	return q, err
}
func owner(i *corev1.Instance) pgutil.Owner {
	return pgutil.Owner{TenantUID: i.Status.ResourceControl.TenantUID, InstanceUID: i.Metadata.UID, InstanceName: i.Metadata.Name}
}
func binding(i *corev1.Instance) pgutil.Binding {
	a := i.Status.ResourceControl.CurrentAttempt
	return pgutil.Binding{NodeUID: a.NodeUID, AttemptID: a.ID, PlacementEpoch: a.Epoch}
}
func version(rv string) (int64, error) {
	v, err := strconv.ParseInt(rv, 10, 64)
	if err != nil || v < 1 {
		return 0, controlError(corev1.IdentityConflict, "source object has no valid persistent version")
	}
	return v, nil
}
func quotaCommand(i *corev1.Instance, kind corev1.Kind) pgutil.Command {
	c := pgutil.Command{Domain: corev1.QuotaDomain, AccountID: i.Status.ResourceControl.TenantUID, AllocationID: i.Status.ResourceControl.AllocationID, Owner: owner(i), Kind: kind}
	c.OperationID = "quota:" + string(kind) + ":" + c.AllocationID
	if kind == corev1.RequestClose || kind == corev1.FinalizeClose {
		if i.Status.ResourceControl.Rebudgeting {
			c.CloseIntentID = "rebudget:" + c.AllocationID
		} else {
			c.CloseIntentID = "delete:" + i.Metadata.UID
		}
	}
	return c
}

func cancelChangeCommand(i *corev1.Instance, plan pgutil.ResourceChange, node bool) pgutil.Command {
	c := quotaCommand(i, corev1.CancelChange)
	if node {
		c = nodeCommand(i, corev1.CancelChange)
	}
	c.OperationID = string(c.Domain) + ":" + string(corev1.CancelChange) + ":" + plan.ChangeID
	c.ChangePlanID = plan.ChangeID
	return c
}

func removeConditionsByType(list *[]metav1.Condition, types ...string) {
	if list == nil || len(*list) == 0 {
		return
	}
	kept := (*list)[:0]
	for _, c := range *list {
		if !slices.Contains(types, c.Type) {
			kept = append(kept, c)
		}
	}
	if len(kept) == 0 {
		*list = nil
		return
	}
	*list = kept
}
func nodeCommand(i *corev1.Instance, kind corev1.Kind) pgutil.Command {
	a := i.Status.ResourceControl.CurrentAttempt
	c := pgutil.Command{Domain: corev1.NodeDomain, AccountID: a.NodeUID, AllocationID: a.ID, Owner: owner(i), Kind: kind, OperationID: "node:" + string(kind) + ":" + a.ID}
	if kind == corev1.RequestClose || kind == corev1.FinalizeClose {
		c.CloseIntentID = "candidate:" + a.ID
		if i.Status.ResourceControl.ApprovedRevision > 0 {
			c.CloseIntentID = "delete:" + i.Metadata.UID
		}
	}
	return c
}
func ports(input []corev1.InstancePort) ([]corev1.PortRequest, error) {
	out := make([]corev1.PortRequest, len(input))
	for n, p := range input {
		if p.TargetPort < 1 || p.TargetPort > 65535 || p.PublishedPort < 0 || p.PublishedPort > 65535 {
			return nil, &corev1.ValidationError{Field: "ports", Reason: corev1.ValidationReasonPortOutOfRange}
		}
		out[n] = corev1.PortRequest{Name: p.Name, Protocol: p.Protocol, TargetPort: int32(p.TargetPort), PublishedPort: int32(p.PublishedPort)}
	}
	return corev1.CanonicalPorts(out)
}
func apiPorts(input []corev1.PortRequest) []corev1.InstancePort {
	out := make([]corev1.InstancePort, len(input))
	for n, p := range input {
		out[n] = corev1.InstancePort{Name: p.Name, Protocol: p.Protocol, TargetPort: int(p.TargetPort), PublishedPort: int(p.PublishedPort)}
	}
	return out
}
func (r *StageReconciler) condition(ctx context.Context, i *corev1.Instance, kind, status, reason, message string) (*corev1.Instance, error) {
	metav1.SetCondition(&i.Status.Conditions, metav1.Condition{Type: kind, Status: status, Reason: reason, Message: message, ObservedGeneration: i.Metadata.Generation})
	return CommitControl(ctx, r.db, i)
}

func (r *StageReconciler) ReconcileStep(ctx context.Context, tenant, name string) (*corev1.Instance, error) {
	if r == nil || r.reg == nil {
		return nil, fmt.Errorf("resource stores are required")
	}
	i, err := r.db.Get(ctx, tenant, name)
	if errors.Is(err, db.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if i.Status.ResourceControl == nil {
		return i, controlError(corev1.IdentityConflict, "instance lacks resource-control intent")
	}
	uid, rv := i.Metadata.UID, i.Metadata.ResourceVersion
	out, stepErr := r.reconcileStage(ctx, i)
	if stepErr != nil {
		if !errors.Is(stepErr, db.ErrConflict) && ctx.Err() == nil {
			if current, err := r.db.Get(ctx, tenant, name); err == nil && current.Metadata.UID == uid && current.Metadata.ResourceVersion == rv {
				reason := "ReconcileError"
				message := "Resource reconciliation failed; held resources are retained and the same intent will be retried"
				if GetStage(current) == StageDeleting && len(current.Status.Backups) > 0 {
					reason = "BackupCleanupFailed"
					message = "Backup cleanup failed; backup records and held resources are retained for retry"
				}
				if updated, err := r.condition(ctx, current, metav1.ConditionTypeReconciled, metav1.ConditionFalse, reason, message); err == nil {
					out = updated
				}
			}
		}
		return out, stepErr
	}
	if out != nil {
		for _, condition := range out.Status.Conditions {
			if condition.Type == metav1.ConditionTypeReconciled && condition.Status == metav1.ConditionFalse {
				return r.condition(ctx, out, metav1.ConditionTypeReconciled, metav1.ConditionTrue, "ReconcileSucceeded", "A resource-control reconciliation step completed successfully")
			}
		}
	}
	return out, nil
}

func (r *StageReconciler) reconcileStage(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	switch GetStage(i) {
	case StagePendingQuota:
		return r.pendingQuota(ctx, i)
	case StageScheduling:
		return r.scheduling(ctx, i)
	case StageActive:
		return r.reconcileActive(ctx, i)
	case StageDeleting:
		return r.deleting(ctx, i)
	case StageReleasingQuota:
		return r.releaseQuota(ctx, i)
	default:
		return i, controlError(corev1.IdentityConflict, "unknown persisted instance stage")
	}
}

func (r *StageReconciler) reconcileActive(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	tenant, err := r.tenantDB.Get(ctx, i.Metadata.Tenant)
	if err != nil {
		return i, err
	}
	if tenant.Metadata.UID != i.Status.ResourceControl.TenantUID {
		return i, controlError(corev1.IdentityConflict, "tenant name now refers to a different UID")
	}
	wantSuspended := tenant.Status.Phase == corev1.TenantPhaseSuspended
	if i.Status.ResourceControl.Suspended != wantSuspended {
		i.Status.ResourceControl.Suspended = wantSuspended
		return CommitControl(ctx, r.db, i)
	}
	if i.Status.ResourceControl.Change != nil && (!wantSuspended || i.Status.ResourceControl.Change.Phase == ChangeApplying || i.Status.ResourceControl.Change.Phase == ChangeCancelling) {
		return r.resize(ctx, i)
	}
	return i, nil
}
func (r *StageReconciler) syncQuota(ctx context.Context, i *corev1.Instance) error {
	tenant, err := r.tenantDB.Get(ctx, i.Metadata.Tenant)
	if err != nil {
		return err
	}
	if tenant.Metadata.UID != i.Status.ResourceControl.TenantUID {
		return controlError(corev1.IdentityConflict, "tenant name now refers to a different UID")
	}
	if tenant.Metadata.DeletionTimestamp != nil || tenant.Status.Phase == corev1.TenantPhaseSuspended || tenant.Status.Phase == corev1.TenantPhaseTerminating {
		return controlError(corev1.AdmissionDenied, "tenant is not admitting resources")
	}
	limits, err := corev1.NormalizeLimits(tenant.Spec.Quota, tenant.Spec.UnlimitedQuota)
	if err != nil {
		return err
	}
	rv, err := version(tenant.Metadata.ResourceVersion)
	if err != nil {
		return err
	}
	_, err = r.quotaDB.RequireAccountAt(ctx, tenant.Metadata.UID, rv, quotadb.QuotaPolicy{Limits: limits})
	return err
}
func (r *StageReconciler) pendingQuota(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	if i.Status.ResourceControl.Rebudgeting {
		return r.rebudgetUnbound(ctx, i)
	}
	class, err := r.runtimeClass(ctx, i)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) || errors.Is(err, corev1.AdmissionDenied) {
			return r.condition(ctx, i, metav1.ConditionTypeScheduled, metav1.ConditionFalse, "RuntimeUnavailable", "The requested runtime class is unavailable")
		}
		return i, err
	}
	if err := r.syncQuota(ctx, i); err != nil {
		if errors.Is(err, corev1.AdmissionDenied) {
			return r.condition(ctx, i, metav1.ConditionTypeQuotaReserved, metav1.ConditionFalse, "AdmissionDenied", err.Error())
		}
		return i, err
	}
	q, err := budget(i.Status.ResourceControl.InitialResources)
	if err != nil {
		return i, err
	}
	_, err = r.quotaDB.OpenAllocation(ctx, pgutil.AllocationIntent{AllocationID: i.Status.ResourceControl.AllocationID, Owner: owner(i), ResourceRevision: 1, Quantity: q})
	if err != nil {
		return i, err
	}
	c := quotaCommand(i, corev1.Grant)
	c.Quantity = q
	c.ExpectedVersion = 1
	c.ResourceRevision = 1
	if _, err = r.quotaDB.RegisterCommand(ctx, c); err != nil {
		return i, err
	}
	result, err := r.quotaDB.TryGrant(ctx, c.OperationID)
	if err != nil {
		if errors.Is(err, corev1.AdmissionDenied) {
			return r.condition(ctx, i, metav1.ConditionTypeQuotaReserved, metav1.ConditionFalse, "AdmissionDenied", err.Error())
		}
		return i, err
	}
	if result.Allocation.State != corev1.AllocationActive {
		return i, controlError(corev1.Closed, "quota is not currently active")
	}
	if i.Status.ResourceControl.RuntimeClass == nil {
		i.Status.ResourceControl.RuntimeClass = &corev1.InstanceRuntimeBinding{UID: class.Metadata.UID, Name: class.Metadata.Name, Handler: class.Handler, Parameters: maps.Clone(class.Parameters)}
		i.Spec.RuntimeClassName = class.Metadata.Name
	}
	i.Status.ResourceControl.Stage = string(StageScheduling)
	return r.condition(ctx, i, metav1.ConditionTypeQuotaReserved, metav1.ConditionTrue, "Granted", "Initial tenant budget is held")
}
func nodePolicyVersion(n *corev1.Node) (int64, error) {
	rv, err := version(n.Metadata.ResourceVersion)
	if err != nil {
		return 0, err
	}
	sv := int64(1)
	if n.Metadata.StatusVersion != "" {
		sv, err = version(n.Metadata.StatusVersion)
		if err != nil {
			return 0, err
		}
	}
	return rv + sv - 1, nil
}
func (r *StageReconciler) syncNode(ctx context.Context, i *corev1.Instance) (*corev1.Node, error) {
	a := i.Status.ResourceControl.CurrentAttempt
	n, err := r.nodeDB.Get(ctx, a.NodeName)
	if err != nil {
		return nil, err
	}
	if n.Metadata.UID != a.NodeUID {
		return nil, controlError(corev1.IdentityConflict, "node name now refers to a different UID")
	}
	limits, err := corev1.NormalizeNodeLimits(n.Status.Capacity, n.Status.Allocatable)
	if err != nil {
		return nil, err
	}
	pv, err := nodePolicyVersion(n)
	if err != nil {
		return nil, err
	}
	_, err = r.nodeDB.SyncAccount(ctx, a.NodeUID, pv, limits)
	return n, err
}
func (r *StageReconciler) runtimeClass(ctx context.Context, i *corev1.Instance) (*corev1.RuntimeClass, error) {
	if frozen := i.Status.ResourceControl.RuntimeClass; frozen != nil {
		class, err := r.rcDB.Get(ctx, frozen.Name)
		if err != nil {
			return nil, err
		}
		if class.Metadata.UID != frozen.UID || class.Metadata.DeletionTimestamp != nil {
			return nil, controlError(corev1.AdmissionDenied, "the frozen runtime class identity is unavailable")
		}
		class.Handler = frozen.Handler
		class.Parameters = maps.Clone(frozen.Parameters)
		return class, nil
	}
	if i.Spec.RuntimeClassName != "" {
		class, err := r.rcDB.Get(ctx, i.Spec.RuntimeClassName)
		if err == nil && class.Metadata.DeletionTimestamp != nil {
			return nil, controlError(corev1.AdmissionDenied, "runtime class is deleting")
		}
		return class, err
	}
	list, err := r.rcDB.List(ctx, db.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, class := range list.Items {
		if class.Metadata.DeletionTimestamp == nil && class.IsDefault {
			return class, nil
		}
	}
	return nil, controlError(corev1.AdmissionDenied, "no default runtime class is available")
}
func eligible(n *corev1.Node, i *corev1.Instance, class *corev1.RuntimeClass) bool {
	if n.Metadata.UID == "" || n.Metadata.DeletionTimestamp != nil || n.Spec.Unschedulable || n.Status.Phase != corev1.NodePhaseReady {
		return false
	}
	found := false
	for _, runtime := range n.Status.Runtimes {
		if runtime.Handler == class.Handler {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	for key, value := range i.Spec.NodeSelector {
		if n.Metadata.Labels[key] != value {
			return false
		}
	}
	if class.Scheduling != nil {
		for key, value := range class.Scheduling.NodeSelector {
			if n.Metadata.Labels[key] != value {
				return false
			}
		}
	}
	if !n.AllowsInstance(i) {
		return false
	}
	return true
}
func (r *StageReconciler) choose(ctx context.Context, i *corev1.Instance) (*corev1.Node, error) {
	class, err := r.runtimeClass(ctx, i)
	if err != nil {
		return nil, err
	}
	list, err := r.nodeDB.List(ctx, db.ListOptions{})
	if err != nil {
		return nil, err
	}
	q, err := budget(i.Status.ResourceControl.InitialResources)
	if err != nil {
		return nil, err
	}
	var candidates []*corev1.Node
	fits := make(map[string]bool)
	for _, n := range list.Items {
		if i.Spec.NodeName != "" && n.Metadata.Name != i.Spec.NodeName {
			continue
		}
		if eligible(n, i, class) {
			limits, err := corev1.NormalizeNodeLimits(n.Status.Capacity, n.Status.Allocatable)
			if err != nil {
				return nil, err
			}
			account, err := r.nodeDB.GetAccount(ctx, n.Metadata.UID)
			if err != nil && !errors.Is(err, nodedb.ErrNotFound) {
				return nil, err
			}
			fits[n.Metadata.UID] = limits.CheckIncrease(account.Allocated, q) == nil
			candidates = append(candidates, n)
		}
	}
	sortCandidatesByFitAndCache(candidates, fits, instancePackageKey(i))
	if len(candidates) == 0 {
		return nil, controlError(corev1.AdmissionDenied, "no Ready node matches runtime and selectors")
	}
	previous := i.Status.ResourceControl.CurrentAttempt
	if previous != nil {
		for n, candidate := range candidates {
			if candidate.Metadata.UID == previous.NodeUID {
				return candidates[(n+1)%len(candidates)], nil
			}
		}
	}
	return candidates[0], nil
}
func (r *StageReconciler) scheduling(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	control := i.Status.ResourceControl
	if control.Rebudgeting {
		return r.rebudgetUnbound(ctx, i)
	}
	if control.CurrentAttempt != nil && control.CurrentAttempt.Phase == AttemptClosing {
		return r.closeCandidate(ctx, i)
	}
	if err := r.syncQuota(ctx, i); err != nil {
		if errors.Is(err, corev1.AdmissionDenied) {
			return r.condition(ctx, i, metav1.ConditionTypeScheduled, metav1.ConditionFalse, "TenantUnavailable", err.Error())
		}
		return i, err
	}
	h, err := r.quotaDB.GetAllocation(ctx, control.AllocationID)
	if err != nil {
		return i, err
	}
	if h.Allocation.State != corev1.AllocationActive {
		return i, controlError(corev1.Closed, "placement requires current tenant budget")
	}
	a := control.CurrentAttempt
	if a == nil || a.Phase == AttemptClosed {
		return r.selectAndPersistCandidateAttempt(ctx, i)
	}
	if a.Phase != AttemptPending {
		return i, controlError(corev1.IdentityConflict, "Scheduling has an invalid attempt phase")
	}
	return r.bindPendingCandidateAttempt(ctx, i)
}

func (r *StageReconciler) selectAndPersistCandidateAttempt(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	control := i.Status.ResourceControl
	a := control.CurrentAttempt
	node, err := r.choose(ctx, i)
	if err != nil {
		if errors.Is(err, corev1.AdmissionDenied) || errors.Is(err, db.ErrNotFound) {
			return r.condition(ctx, i, metav1.ConditionTypeScheduled, metav1.ConditionFalse, "Unschedulable", err.Error())
		}
		return i, err
	}
	requested, err := ports(i.Spec.Ports)
	if err != nil {
		return i, err
	}
	epoch := int64(1)
	if a != nil {
		if a.Epoch == math.MaxInt64 {
			return i, controlError(corev1.InvariantViolation, "placement epoch overflow")
		}
		epoch = a.Epoch + 1
	}
	q, err := budget(control.InitialResources)
	if err != nil {
		return i, err
	}
	control.CurrentAttempt = &corev1.InstancePlacementAttempt{
		ID: db.NewUID(), NodeUID: node.Metadata.UID, NodeName: node.Metadata.Name,
		Epoch: epoch, ResourceRevision: 1, Resources: corev1.CanonicalResources(q),
		Ports: apiPorts(requested), Phase: AttemptPending,
	}
	return CommitControl(ctx, r.db, i)
}

func (r *StageReconciler) bindPendingCandidateAttempt(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	control := i.Status.ResourceControl
	a := control.CurrentAttempt
	n, err := r.nodeDB.Get(ctx, a.NodeName)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return i, err
	}
	class, classErr := r.runtimeClass(ctx, i)
	if classErr != nil && !errors.Is(classErr, db.ErrNotFound) && !errors.Is(classErr, corev1.AdmissionDenied) {
		return i, classErr
	}
	if err != nil || n.Metadata.UID != a.NodeUID || classErr != nil || !eligible(n, i, class) {
		a.Phase = AttemptClosing
		return r.condition(ctx, i, metav1.ConditionTypeScheduled, metav1.ConditionFalse, "CandidateChanged", "Candidate identity or eligibility changed")
	}
	if _, err = r.syncNode(ctx, i); err != nil {
		return i, err
	}
	q, err := budget(a.Resources)
	if err != nil {
		return i, err
	}
	requested, err := ports(a.Ports)
	if err != nil {
		return i, err
	}
	if _, err = r.nodeDB.OpenReservation(ctx, pgutil.PlacementIntent{Owner: owner(i), Binding: binding(i), ResourceRevision: a.ResourceRevision, Quantity: q, Ports: requested}); err != nil {
		return i, err
	}
	c := nodeCommand(i, corev1.Grant)
	c.Quantity, c.ExpectedVersion, c.ResourceRevision = q, 1, a.ResourceRevision
	if _, err = r.nodeDB.RegisterCommand(ctx, c); err != nil {
		return i, err
	}
	result, err := r.nodeDB.TryReserve(ctx, c.OperationID)
	if err != nil {
		if errors.Is(err, corev1.AdmissionDenied) {
			return r.condition(ctx, i, metav1.ConditionTypeScheduled, metav1.ConditionFalse, "AdmissionDenied", err.Error())
		}
		return i, err
	}
	if result.Allocation.State != corev1.AllocationActive || result.Binding != binding(i) {
		return i, controlError(corev1.Closed, "historical reservation is not current placement authority")
	}
	a.Phase = AttemptBound
	control.Stage = string(StageActive)
	control.ApprovedRevision = 1
	control.ApprovedResources = corev1.CanonicalResources(q)
	control.ApprovedPorts = apiPorts(result.AssignedPorts)
	i.Spec.NodeName = a.NodeName
	AddFinalizer(&i.Metadata, corev1.FinalizerInstanceCleanup)
	return r.condition(ctx, i, metav1.ConditionTypeScheduled, metav1.ConditionTrue, "Bound", "Node budget and ports are held for the persisted placement")
}
func (r *StageReconciler) closeCandidate(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	if i.Status.ResourceControl.ApprovedRevision != 0 {
		return i, controlError(corev1.IdentityConflict, "a bound placement requires runtime cleanup")
	}
	if _, err := r.nodeDB.RequestClose(ctx, nodeCommand(i, corev1.RequestClose)); err != nil {
		return i, err
	}
	result, err := r.nodeDB.FinalizeClose(ctx, nodeCommand(i, corev1.FinalizeClose))
	if err != nil {
		return i, err
	}
	if result.Allocation.State != corev1.AllocationClosed {
		return i, controlError(corev1.InvariantViolation, "candidate close is not confirmed")
	}
	i.Status.ResourceControl.CurrentAttempt.Phase = AttemptClosed
	return CommitControl(ctx, r.db, i)
}

func changePlan(i *corev1.Instance, node bool) (pgutil.ResourceChange, error) {
	control := i.Status.ResourceControl
	change := control.Change
	old, err := budget(change.OldResources)
	if err != nil {
		return pgutil.ResourceChange{}, err
	}
	target, err := budget(change.TargetResources)
	if err != nil {
		return pgutil.ResourceChange{}, err
	}
	allocation := control.AllocationID
	if node {
		allocation = control.CurrentAttempt.ID
	}
	return pgutil.ResourceChange{ChangeID: change.ID, AllocationID: allocation, Owner: owner(i), Binding: binding(i), FromRevision: change.FromRevision, ToRevision: change.ToRevision, Old: old, Target: target}, nil
}
func changeCommand(i *corev1.Instance, plan pgutil.ResourceChange, source int64, kind corev1.Kind, node bool) (pgutil.Command, error) {
	grow, shrink, err := corev1.GrowthAndShrink(plan.Old, plan.Target)
	if err != nil {
		return pgutil.Command{}, err
	}
	c := quotaCommand(i, kind)
	if node {
		c = nodeCommand(i, kind)
	}
	c.OperationID = string(c.Domain) + ":" + string(kind) + ":" + plan.ChangeID
	c.ChangePlanID = plan.ChangeID
	c.ResourceRevision = plan.ToRevision
	c.ExpectedVersion = source
	c.Quantity = grow
	if kind == corev1.SettleDecrease {
		c.Quantity = shrink
		if !grow.IsZero() {
			if source == math.MaxInt64 {
				return pgutil.Command{}, controlError(corev1.InvariantViolation, "change source version overflow")
			}
			c.ExpectedVersion++
		}
	}
	return c, nil
}
func (r *StageReconciler) rebudgetUnbound(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	control := i.Status.ResourceControl
	if a := control.CurrentAttempt; a != nil && a.Phase != AttemptClosed {
		if a.Phase == AttemptPending {
			a.Phase = AttemptClosing
			return CommitControl(ctx, r.db, i)
		}
		if a.Phase == AttemptClosing {
			var err error
			i, err = r.closeCandidate(ctx, i)
			if err != nil {
				return i, err
			}
			control = i.Status.ResourceControl
		}
	}
	if _, err := r.quotaDB.RequestClose(ctx, quotaCommand(i, corev1.RequestClose)); err != nil {
		return i, err
	}
	result, err := r.quotaDB.FinalizeClose(ctx, quotaCommand(i, corev1.FinalizeClose))
	if err != nil {
		return i, err
	}
	if result.Allocation.State != corev1.AllocationClosed {
		return i, controlError(corev1.InvariantViolation, "rebudget quota close is not confirmed")
	}
	desired, err := normalizedIntentResources(i)
	if err != nil {
		return i, err
	}
	control.AllocationID = db.NewUID()
	control.InitialResources = corev1.CanonicalResources(desired)
	control.Stage = string(StagePendingQuota)
	control.Rebudgeting = false
	removeConditionsByType(&i.Status.Conditions, metav1.ConditionTypeQuotaReserved, metav1.ConditionTypeScheduled)
	return CommitControl(ctx, r.db, i)
}

func (r *StageReconciler) resize(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	control := i.Status.ResourceControl
	if control.CurrentAttempt == nil || control.CurrentAttempt.Phase != AttemptBound {
		return i, controlError(corev1.IdentityConflict, "resource change requires the current bound placement")
	}
	qp, err := changePlan(i, false)
	if err != nil {
		return i, err
	}
	np, err := changePlan(i, true)
	if err != nil {
		return i, err
	}
	switch control.Change.Phase {
	case ChangeBudgeting:
		return r.budgetResize(ctx, i, qp, np)
	case ChangeCancelling:
		return r.cancelResize(ctx, i, qp, np)
	case ChangeApplying:
		return r.settleResize(ctx, i, qp, np)
	default:
		return i, controlError(corev1.IdentityConflict, "unknown resource change phase")
	}
}

func (r *StageReconciler) cancelResize(ctx context.Context, i *corev1.Instance, qp, np pgutil.ResourceChange) (*corev1.Instance, error) {
	if _, err := r.nodeDB.CancelChange(ctx, cancelChangeCommand(i, np, true)); err != nil {
		return i, err
	}
	if _, err := r.quotaDB.CancelChange(ctx, cancelChangeCommand(i, qp, false)); err != nil {
		return i, err
	}
	control := i.Status.ResourceControl
	desired, err := normalizedIntentResources(i)
	if err != nil {
		return i, err
	}
	approved, err := budget(control.ApprovedResources)
	if err != nil {
		return i, err
	}
	if desired == approved {
		control.Change = nil
		control.DesiredRevision = control.ApprovedRevision
		return r.condition(ctx, i, metav1.ConditionTypeResourcesApproved, metav1.ConditionTrue, "ChangeCancelled", "Pending resource resize was cancelled and reserved budget difference was released")
	}
	control.DesiredRevision = control.ApprovedRevision + 1
	control.Change = &corev1.InstanceResourceChange{
		ID: "change:" + db.NewUID(), FromRevision: control.ApprovedRevision, ToRevision: control.DesiredRevision,
		OldResources: control.ApprovedResources, TargetResources: corev1.CanonicalResources(desired), Phase: ChangeBudgeting,
	}
	removeConditionsByType(&i.Status.Conditions, metav1.ConditionTypeResourcesApproved)
	return CommitControl(ctx, r.db, i)
}

func (r *StageReconciler) budgetResize(ctx context.Context, i *corev1.Instance, qp, np pgutil.ResourceChange) (*corev1.Instance, error) {
	control := i.Status.ResourceControl
	if err := r.syncQuota(ctx, i); err != nil {
		return i, err
	}
	if _, err := r.syncNode(ctx, i); err != nil {
		return i, err
	}
	quotaChange, err := r.quotaDB.RegisterChange(ctx, qp)
	if err != nil {
		return i, err
	}
	nodeChange, err := r.nodeDB.RegisterChange(ctx, np)
	if err != nil {
		return i, err
	}
	grow, _, err := corev1.GrowthAndShrink(qp.Old, qp.Target)
	if err != nil {
		return i, err
	}
	if !grow.IsZero() {
		qc, err := changeCommand(i, qp, quotaChange.SourceVersion, corev1.Increase, false)
		if err != nil {
			return i, err
		}
		if _, err = r.quotaDB.RegisterCommand(ctx, qc); err == nil {
			_, err = r.quotaDB.TryIncrease(ctx, qc.OperationID)
		}
		if err != nil {
			if errors.Is(err, corev1.AdmissionDenied) {
				return r.condition(ctx, i, metav1.ConditionTypeResourcesApproved, metav1.ConditionFalse, "QuotaDenied", err.Error())
			}
			return i, err
		}
		nc, err := changeCommand(i, np, nodeChange.SourceVersion, corev1.Increase, true)
		if err != nil {
			return i, err
		}
		if _, err = r.nodeDB.RegisterCommand(ctx, nc); err == nil {
			_, err = r.nodeDB.TryIncrease(ctx, nc.OperationID)
		}
		if err != nil {
			if errors.Is(err, corev1.AdmissionDenied) {
				return r.condition(ctx, i, metav1.ConditionTypeResourcesApproved, metav1.ConditionFalse, "NodeDenied", err.Error())
			}
			return i, err
		}
	}
	control.ApprovedResources = corev1.CanonicalResources(qp.Target)
	control.ApprovedRevision = qp.ToRevision
	control.Change.Phase = ChangeApplying
	return r.condition(ctx, i, metav1.ConditionTypeResourcesApproved, metav1.ConditionTrue, "BudgetGranted", "Target resource budget is held in both domains")
}

func (r *StageReconciler) settleResize(ctx context.Context, i *corev1.Instance, qp, np pgutil.ResourceChange) (*corev1.Instance, error) {
	control := i.Status.ResourceControl
	if control.ObservedRevision != qp.ToRevision || control.ObservedAttemptID != control.CurrentAttempt.ID || control.ObservedEpoch != control.CurrentAttempt.Epoch {
		return i, nil
	}
	observed, err := budget(control.ObservedResources)
	if err != nil || observed != qp.Target || !slices.Equal(control.ObservedPorts, control.ApprovedPorts) {
		return i, controlError(corev1.IdentityConflict, "settlement requires the complete matching execution observation")
	}
	quotaChange, err := r.quotaDB.RegisterChange(ctx, qp)
	if err != nil {
		return i, err
	}
	nodeChange, err := r.nodeDB.RegisterChange(ctx, np)
	if err != nil {
		return i, err
	}
	qc, err := changeCommand(i, qp, quotaChange.SourceVersion, corev1.SettleDecrease, false)
	if err != nil {
		return i, err
	}
	if _, err = r.quotaDB.RegisterCommand(ctx, qc); err == nil {
		_, err = r.quotaDB.SettleDecrease(ctx, qc.OperationID)
	}
	if err != nil {
		return i, err
	}
	nc, err := changeCommand(i, np, nodeChange.SourceVersion, corev1.SettleDecrease, true)
	if err != nil {
		return i, err
	}
	if _, err = r.nodeDB.RegisterCommand(ctx, nc); err == nil {
		_, err = r.nodeDB.SettleDecrease(ctx, nc.OperationID)
	}
	if err != nil {
		return i, err
	}
	control.Change = nil
	return r.condition(ctx, i, metav1.ConditionTypeResourcesSettled, metav1.ConditionTrue, "Observed", "Matching runtime observation settled both domain holdings")
}
func (r *StageReconciler) deleting(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	if _, err := r.quotaDB.RequestClose(ctx, quotaCommand(i, corev1.RequestClose)); err != nil {
		return i, err
	}
	if i.Status.ResourceControl.CurrentAttempt != nil {
		if _, err := r.nodeDB.RequestClose(ctx, nodeCommand(i, corev1.RequestClose)); err != nil {
			return i, err
		}
	}
	cleaned, err := r.cleanupBackups(ctx, i)
	if err != nil {
		return i, err
	}
	if cleaned.Metadata.ResourceVersion != i.Metadata.ResourceVersion {
		return cleaned, nil
	}
	if cleaned.Metadata.StatusVersion != i.Metadata.StatusVersion {
		i = cleaned
	}
	control := i.Status.ResourceControl
	a := control.CurrentAttempt
	if a != nil {
		if a.Phase != AttemptClosing && a.Phase != AttemptClosed {
			a.Phase = AttemptClosing
			return CommitControl(ctx, r.db, i)
		}
		if control.ApprovedRevision > 0 && HasFinalizer(&i.Metadata, corev1.FinalizerInstanceCleanup) {
			return i, nil
		}
		result, err := r.nodeDB.FinalizeClose(ctx, nodeCommand(i, corev1.FinalizeClose))
		if err != nil {
			return i, err
		}
		if result.Allocation.State != corev1.AllocationClosed {
			return i, controlError(corev1.InvariantViolation, "node cleanup is not closed")
		}
		a.Phase = AttemptClosed
	}
	control.Stage = string(StageReleasingQuota)
	return CommitControl(ctx, r.db, i)
}
func (r *StageReconciler) releaseQuota(ctx context.Context, i *corev1.Instance) (*corev1.Instance, error) {
	control := i.Status.ResourceControl
	if control.CurrentAttempt != nil && control.CurrentAttempt.Phase != AttemptClosed {
		return i, controlError(corev1.IdentityConflict, "quota release must follow confirmed node cleanup")
	}
	if HasFinalizer(&i.Metadata, corev1.FinalizerInstanceCleanup) {
		return i, nil
	}
	result, err := r.quotaDB.FinalizeClose(ctx, quotaCommand(i, corev1.FinalizeClose))
	if err != nil {
		return i, err
	}
	if result.Allocation.State != corev1.AllocationClosed {
		return i, controlError(corev1.InvariantViolation, "quota close is not confirmed")
	}
	for _, finalizer := range i.Metadata.Finalizers {
		if finalizer != FinalizerResources {
			return i, nil
		}
	}
	if HasFinalizer(&i.Metadata, FinalizerResources) {
		RemoveFinalizer(&i.Metadata, FinalizerResources)
		i, err = CommitControl(ctx, r.db, i)
		if err != nil {
			return i, err
		}
	}
	if len(i.Metadata.Finalizers) > 0 {
		return i, nil
	}
	err = DeleteInstanceIfUID(ctx, r.db, i.Metadata.Tenant, i.Metadata.Name, i.Metadata.UID)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return i, err
	}
	return nil, nil
}

// sortCandidatesByFitAndCache orders nodes by capacity fit first, then cache affinity (nodes that already
// hold the instance's app:version package in their local cache start the server without any download),
// then by name for determinism.
func sortCandidatesByFitAndCache(candidates []*corev1.Node, fits map[string]bool, pkgKey string) {
	cached := make(map[string]bool, len(candidates))
	for _, n := range candidates {
		cached[n.Metadata.UID] = nodeHasCachedPackage(n, pkgKey)
	}
	sort.SliceStable(candidates, func(a, b int) bool {
		ua, ub := candidates[a].Metadata.UID, candidates[b].Metadata.UID
		if fits[ua] != fits[ub] {
			return fits[ua]
		}
		if cached[ua] != cached[ub] {
			return cached[ua]
		}
		return candidates[a].Metadata.Name < candidates[b].Metadata.Name
	})
}

func nodeHasCachedPackage(n *corev1.Node, pkgKey string) bool {
	if n == nil || pkgKey == "" {
		return false
	}
	for _, item := range n.Status.CachedArtifacts {
		if strings.TrimSpace(item) == pkgKey {
			return true
		}
	}
	return false
}

// instancePackageKey returns the "<repo>.<pkg>.<version>" artifact coordinate of an instance for cache-affinity scheduling.
func instancePackageKey(i *corev1.Instance) string {
	if i == nil || i.Spec.ServerArtifact == nil {
		return ""
	}
	return i.Spec.ServerArtifact.Coordinate()
}
