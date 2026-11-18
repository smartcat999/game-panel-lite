package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
)

type Stage string

const (
	StagePendingQuota   Stage = corev1.InstanceStagePendingQuota
	StageScheduling     Stage = corev1.InstanceStageScheduling
	StageActive         Stage = corev1.InstanceStageActive
	StageDeleting       Stage = corev1.InstanceStageDeleting
	StageReleasingQuota Stage = corev1.InstanceStageReleasingQuota
	FinalizerResources        = corev1.FinalizerResourceControl
	AttemptPending            = corev1.AttemptPhasePending
	AttemptBound              = corev1.AttemptPhaseBound
	AttemptClosing            = corev1.AttemptPhaseClosing
	AttemptClosed             = corev1.AttemptPhaseClosed
	ChangeBudgeting           = corev1.ResourceChangePhaseBudgeting
	ChangeApplying            = corev1.ResourceChangePhaseApplying
)

func AddFinalizer(m *metav1.ObjectMeta, f string) {
	if !HasFinalizer(m, f) {
		m.Finalizers = append(m.Finalizers, f)
	}
}
func HasFinalizer(m *metav1.ObjectMeta, f string) bool { return slices.Contains(m.Finalizers, f) }
func RemoveFinalizer(m *metav1.ObjectMeta, f string) {
	out := make([]string, 0, len(m.Finalizers))
	for _, v := range m.Finalizers {
		if v != f {
			out = append(out, v)
		}
	}
	m.Finalizers = out
}
func GetStage(i *corev1.Instance) Stage {
	if i == nil || i.Status.ResourceControl == nil {
		return ""
	}
	return Stage(i.Status.ResourceControl.Stage)
}

func conflict(detail string) error { return fmt.Errorf("%w: %s", db.ErrConflict, detail) }
func clone(i *corev1.Instance) (*corev1.Instance, error) {
	if i == nil {
		return nil, conflict("instance is required")
	}
	raw, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}
	var out corev1.Instance
	err = json.Unmarshal(raw, &out)
	return &out, err
}
func snapshot(list corev1.ResourceList) (corev1.Resources, error) {
	q, err := corev1.ParseResources(list)
	if err == nil {
		err = q.ValidateInstance()
	}
	return q, err
}

func CreateIntent(ctx context.Context, d *instancedb.DB, input *corev1.Instance) (*corev1.Instance, error) {
	i, err := clone(input)
	if err != nil {
		return nil, err
	}
	if _, err := canonicalPorts(i.Spec.Ports); err != nil {
		return nil, err
	}
	i.DefaultMeta()
	if i.Status.ResourceControl == nil {
		return nil, conflict("trusted tenant reference is required")
	}
	tenantUID := i.Status.ResourceControl.TenantUID
	if err := corev1.ValidateIdentifier("tenantUID", tenantUID); err != nil {
		return nil, err
	}
	q, err := corev1.NormalizeInstanceResources(i.Spec.Resources, nil)
	if err != nil {
		return nil, err
	}
	canonical, err := canonicalPorts(i.Spec.Ports)
	if err != nil {
		return nil, err
	}
	i.Spec.Ports = canonical
	if len(i.Status.ResourceControl.InitialResources) > 0 {
		initial, err := snapshot(i.Status.ResourceControl.InitialResources)
		if err != nil {
			return nil, err
		}
		if initial != q {
			return nil, conflict("initial resources must equal normalized desired resources")
		}
	}
	uid := db.NewUID()
	i.Metadata.UID = uid
	i.Metadata.ResourceVersion = ""
	i.Metadata.DeletionTimestamp = nil
	i.Status = corev1.InstanceStatus{Phase: corev1.InstancePhasePending, ResourceControl: &corev1.InstanceResourceControl{TenantUID: tenantUID, AllocationID: db.NewUID(), Stage: string(StagePendingQuota), InitialResources: corev1.CanonicalResources(q), DesiredRevision: 1}}
	i.Metadata.Finalizers = nil
	AddFinalizer(&i.Metadata, FinalizerResources)
	return d.Create(ctx, i)
}

func CommitControl(ctx context.Context, d *instancedb.DB, input *corev1.Instance) (*corev1.Instance, error) {
	i, err := clone(input)
	if err != nil {
		return nil, err
	}
	if i.Metadata.UID == "" || i.Metadata.ResourceVersion == "" {
		return nil, conflict("UID and resource version are required")
	}
	old, err := d.Get(ctx, i.Metadata.Tenant, i.Metadata.Name)
	if err != nil {
		return nil, err
	}
	if old.Metadata.UID != i.Metadata.UID || old.Metadata.ResourceVersion != i.Metadata.ResourceVersion {
		return nil, conflict("instance changed before commit")
	}
	if i.Metadata.StatusVersion != "" && i.Metadata.StatusVersion != old.Metadata.StatusVersion {
		syncObservationForControlCommit(old, i)
	}
	if err := validateTransition(old, i); err != nil {
		return nil, err
	}
	if reflect.DeepEqual(old, i) {
		return old, nil
	}
	return d.UpdateObject(ctx, i)
}

func syncObservationForControlCommit(old, next *corev1.Instance) {
	next.Metadata.StatusVersion = old.Metadata.StatusVersion
	next.Status.ObservedGeneration = old.Status.ObservedGeneration
	next.Status.RuntimeID = old.Status.RuntimeID
	next.Status.Endpoint = old.Status.Endpoint
	next.Status.Usage = old.Status.Usage
	next.Status.Backups = old.Status.Backups
	next.Status.ScheduleStatuses = old.Status.ScheduleStatuses
	next.Status.StartedAt = old.Status.StartedAt
	next.Status.ObservedAt = old.Status.ObservedAt
	next.Status.TransitionID = old.Status.TransitionID
	if next.Metadata.DeletionTimestamp == nil {
		next.Status.Phase = old.Status.Phase
	}
	if old.Status.ResourceControl != nil && next.Status.ResourceControl != nil {
		next.Status.ResourceControl.ObservedRevision = old.Status.ResourceControl.ObservedRevision
		next.Status.ResourceControl.ObservedAttemptID = old.Status.ResourceControl.ObservedAttemptID
		next.Status.ResourceControl.ObservedEpoch = old.Status.ResourceControl.ObservedEpoch
		next.Status.ResourceControl.ObservedResources = old.Status.ResourceControl.ObservedResources
		next.Status.ResourceControl.ObservedPorts = append([]corev1.InstancePort(nil), old.Status.ResourceControl.ObservedPorts...)
	}
	merged := make([]metav1.Condition, 0, len(next.Status.Conditions)+len(old.Status.Conditions))
	for _, c := range next.Status.Conditions {
		if isControlCondition(c.Type) {
			merged = append(merged, c)
		}
	}
	for _, c := range old.Status.Conditions {
		if !isControlCondition(c.Type) {
			merged = append(merged, c)
		}
	}
	if len(merged) > 0 {
		next.Status.Conditions = merged
	} else {
		next.Status.Conditions = nil
	}
}

func isControlCondition(t string) bool {
	switch t {
	case metav1.ConditionTypeQuotaReserved,
		metav1.ConditionTypeScheduled,
		metav1.ConditionTypeResourcesApproved,
		metav1.ConditionTypeResourcesSettled,
		metav1.ConditionTypeReconciled:
		return true
	default:
		return false
	}
}

func RequestDelete(ctx context.Context, d *instancedb.DB, tenant, name, uid string) (*corev1.Instance, error) {
	if uid == "" {
		return nil, conflict("delete requires expected UID")
	}
	i, err := d.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if i.Metadata.UID != uid {
		return nil, conflict("delete UID differs")
	}
	if i.Status.ResourceControl == nil {
		return nil, conflict("instance lacks resource-control intent")
	}
	if GetStage(i) == StageDeleting || GetStage(i) == StageReleasingQuota {
		return i, nil
	}
	now := time.Now().UTC()
	i.Metadata.DeletionTimestamp = &now
	i.Status.Phase = corev1.InstancePhaseTerminating
	i.Status.ResourceControl.Stage = string(StageDeleting)
	return CommitControl(ctx, d, i)
}

func DeleteInstanceIfUID(ctx context.Context, d *instancedb.DB, tenant, name, uid string) error {
	if uid == "" {
		return conflict("physical delete requires expected UID")
	}
	i, err := d.Get(ctx, tenant, name)
	if err != nil {
		return err
	}
	if i.Metadata.UID != uid || len(i.Metadata.Finalizers) != 0 || i.Metadata.DeletionTimestamp == nil || GetStage(i) != StageReleasingQuota {
		return conflict("physical delete requires matching UID and completed finalizers")
	}
	return d.DeleteIfUID(ctx, tenant, name, uid)
}

func validateTransition(old, next *corev1.Instance) error {
	a, b := old.Status.ResourceControl, next.Status.ResourceControl
	if a == nil || b == nil {
		return conflict("resource control is mandatory")
	}
	if err := validateControlIdentityAndStage(old, next, a, b); err != nil {
		return err
	}
	if err := validateRuntimeClass(a, b, old, next); err != nil {
		return err
	}
	to := GetStage(next)
	if err := validateAttempt(a, b, to, next.Spec.NodeName); err != nil {
		return err
	}
	if b.CurrentAttempt != nil && (a.CurrentAttempt == nil || a.CurrentAttempt.ID != b.CurrentAttempt.ID) {
		if err := validateNewPorts(b.CurrentAttempt, next.Spec.Ports); err != nil {
			return err
		}
	}
	if err := validateApprovedPorts(b); err != nil {
		return err
	}
	if err := validateControlRevisionsAndSnapshots(old, next, a, b); err != nil {
		return err
	}
	return validateChange(a, b, old, next)
}

func validateControlIdentityAndStage(old, next *corev1.Instance, a, b *corev1.InstanceResourceControl) error {
	if a.TenantUID != b.TenantUID || a.AllocationID != b.AllocationID || !reflect.DeepEqual(a.InitialResources, b.InitialResources) {
		return conflict("creation identity and initial budget are immutable")
	}
	for name, value := range map[string]string{"tenantUID": b.TenantUID, "allocationID": b.AllocationID} {
		if err := corev1.ValidateIdentifier(name, value); err != nil {
			return err
		}
	}
	if _, err := snapshot(b.InitialResources); err != nil {
		return err
	}
	if old.Metadata.DeletionTimestamp != nil && (next.Metadata.DeletionTimestamp == nil || !old.Metadata.DeletionTimestamp.Equal(*next.Metadata.DeletionTimestamp)) {
		return conflict("deletion intent is immutable")
	}
	from, to := GetStage(old), GetStage(next)
	valid := from == to || (from == StagePendingQuota && to == StageScheduling) || (from == StageScheduling && to == StageActive) || (from != StageReleasingQuota && to == StageDeleting) || (from == StageDeleting && to == StageReleasingQuota)
	if !valid {
		return conflict("invalid lifecycle stage transition")
	}
	if to == StageDeleting || to == StageReleasingQuota {
		if next.Metadata.DeletionTimestamp == nil {
			return conflict("closing lifecycle needs deletion intent")
		}
	} else if next.Metadata.DeletionTimestamp != nil {
		return conflict("deleted object cannot reopen lifecycle")
	}
	if HasFinalizer(&old.Metadata, FinalizerResources) && !HasFinalizer(&next.Metadata, FinalizerResources) && to != StageReleasingQuota {
		return conflict("inventory finalizer cannot be removed before release")
	}
	if HasFinalizer(&old.Metadata, corev1.FinalizerInstanceCleanup) && !HasFinalizer(&next.Metadata, corev1.FinalizerInstanceCleanup) && next.Metadata.DeletionTimestamp == nil {
		return conflict("runtime cleanup finalizer cannot be removed from a live binding")
	}
	if !reflect.DeepEqual(old.Spec.Ports, next.Spec.Ports) {
		return conflict("port changes require a dedicated placement plan")
	}
	return nil
}

func validateControlRevisionsAndSnapshots(old, next *corev1.Instance, a, b *corev1.InstanceResourceControl) error {
	if b.DesiredRevision < 1 || b.ApprovedRevision < 0 || b.ObservedRevision < 0 || b.ApprovedRevision > b.DesiredRevision || b.ObservedRevision > b.ApprovedRevision {
		return conflict("invalid resource revisions")
	}
	if b.ApprovedRevision < a.ApprovedRevision || b.ObservedRevision < a.ObservedRevision {
		return conflict("approved and observed revisions cannot regress")
	}
	if a.ApprovedRevision == b.ApprovedRevision && !reflect.DeepEqual(a.ApprovedResources, b.ApprovedResources) {
		return conflict("approved budget cannot change without advancing its frozen plan")
	}
	if a.ApprovedRevision > 0 && !reflect.DeepEqual(a.ApprovedPorts, b.ApprovedPorts) {
		return conflict("resource changes must preserve assigned ports")
	}
	if b.ObservedRevision > 0 && (b.CurrentAttempt == nil || b.ObservedAttemptID != b.CurrentAttempt.ID || b.ObservedEpoch != b.CurrentAttempt.Epoch) {
		return conflict("observation does not match current placement")
	}
	if b.ObservedRevision > a.ObservedRevision {
		observed, err := snapshot(b.ObservedResources)
		if err != nil {
			return conflict("observation requires a valid complete resource snapshot")
		}
		approved, err := snapshot(b.ApprovedResources)
		if err != nil {
			return err
		}
		if b.ObservedRevision != b.ApprovedRevision || observed != approved || !slices.Equal(b.ObservedPorts, b.ApprovedPorts) {
			return conflict("observation must include the complete approved execution snapshot")
		}
	} else if !reflect.DeepEqual(a.ObservedResources, b.ObservedResources) || !slices.Equal(a.ObservedPorts, b.ObservedPorts) {
		return conflict("observed execution snapshot cannot change without advancing its revision")
	}
	if b.ApprovedRevision > 0 {
		if b.CurrentAttempt == nil || next.Spec.NodeName != b.CurrentAttempt.NodeName {
			return conflict("approved execution lacks matching placement")
		}
		if _, err := snapshot(b.ApprovedResources); err != nil {
			return err
		}
	}
	from, to := GetStage(old), GetStage(next)
	if to == StageActive && (b.ApprovedRevision < 1 || b.CurrentAttempt == nil || b.CurrentAttempt.Phase != AttemptBound || !HasFinalizer(&next.Metadata, corev1.FinalizerInstanceCleanup)) {
		return conflict("Active requires an approved bound placement")
	}
	if from == StageScheduling && to == StageActive && (b.ApprovedRevision != 1 || !reflect.DeepEqual(b.InitialResources, b.ApprovedResources)) {
		return conflict("initial binding must approve the frozen initial budget")
	}
	return nil
}
func validateRuntimeClass(a, b *corev1.InstanceResourceControl, old, next *corev1.Instance) error {
	if a.RuntimeClass != nil {
		if !reflect.DeepEqual(a.RuntimeClass, b.RuntimeClass) || old.Spec.RuntimeClassName != next.Spec.RuntimeClassName {
			return conflict("runtime backend identity and parameters are immutable")
		}
	} else if b.RuntimeClass != nil {
		if (GetStage(old) != StagePendingQuota && GetStage(old) != StageScheduling) || a.CurrentAttempt != nil || a.ApprovedRevision != 0 {
			return conflict("runtime backend must freeze before placement")
		}
	}
	if b.RuntimeClass == nil {
		if GetStage(next) == StageScheduling || GetStage(next) == StageActive || b.CurrentAttempt != nil {
			return conflict("placement requires a frozen runtime backend")
		}
		return nil
	}
	for field, value := range map[string]string{"runtimeClass UID": b.RuntimeClass.UID, "runtimeClass name": b.RuntimeClass.Name, "runtimeClass handler": b.RuntimeClass.Handler} {
		if err := corev1.ValidateIdentifier(field, value); err != nil {
			return err
		}
	}
	if next.Spec.RuntimeClassName != b.RuntimeClass.Name {
		return conflict("desired runtime class differs from frozen backend")
	}
	return nil
}

func validateAttempt(a, b *corev1.InstanceResourceControl, stage Stage, nodeName string) error {
	old, next := a.CurrentAttempt, b.CurrentAttempt
	if next == nil {
		if old != nil {
			return conflict("attempt reference cannot be discarded")
		}
		return nil
	}
	if err := corev1.ValidateIdentifier("attemptID", next.ID); err != nil {
		return err
	}
	if err := corev1.ValidateIdentifier("nodeUID", next.NodeUID); err != nil {
		return err
	}
	if err := corev1.ValidateIdentifier("nodeName", next.NodeName); err != nil {
		return err
	}
	if next.Epoch < 1 || next.ResourceRevision != 1 {
		return conflict("initial placement version is invalid")
	}
	if _, err := snapshot(next.Resources); err != nil {
		return err
	}
	if old == nil || old.ID != next.ID {
		if stage != StageScheduling || next.Phase != AttemptPending || b.ApprovedRevision != 0 {
			return conflict("new attempts start Pending during Scheduling")
		}
		expected := int64(1)
		if old != nil {
			if old.Phase != AttemptClosed || old.Epoch == math.MaxInt64 {
				return conflict("old attempt must be Closed before replacement")
			}
			expected = old.Epoch + 1
		}
		if next.Epoch != expected || !reflect.DeepEqual(next.Resources, b.InitialResources) {
			return conflict("new attempt changed frozen initial resource identity")
		}
		return nil
	}
	x, y := *old, *next
	x.Phase = ""
	y.Phase = ""
	if !reflect.DeepEqual(x, y) {
		return conflict("same attempt changed immutable placement payload")
	}
	if old.Phase != next.Phase {
		valid := old.Phase == AttemptPending && (next.Phase == AttemptBound || next.Phase == AttemptClosing) || old.Phase == AttemptBound && next.Phase == AttemptClosing || old.Phase == AttemptClosing && next.Phase == AttemptClosed
		if !valid {
			return conflict("invalid attempt phase transition")
		}
	}
	if next.Phase == AttemptBound && nodeName != next.NodeName {
		return conflict("bound attempt and node name differ")
	}
	return nil
}
func validateChange(a, b *corev1.InstanceResourceControl, old, next *corev1.Instance) error {
	if err := validateChangeTransition(a, b, old, next); err != nil {
		return err
	}
	y := b.Change
	if y != nil {
		return validateActiveChangePayload(b, next, y)
	}
	if b.DesiredRevision != a.DesiredRevision || (b.ApprovedRevision != a.ApprovedRevision && (a.ApprovedRevision != 0 || b.ApprovedRevision != 1 || GetStage(next) != StageActive)) {
		return conflict("revision changed without a frozen plan")
	}
	desired, err := corev1.NormalizeInstanceResources(next.Spec.Resources, nil)
	if err != nil {
		return err
	}
	reference := b.InitialResources
	if b.ApprovedRevision > 0 {
		reference = b.ApprovedResources
	}
	approved, err := snapshot(reference)
	if err != nil {
		return err
	}
	if desired != approved {
		return conflict("resources changed without a frozen plan")
	}
	return nil
}

func validateChangeTransition(a, b *corev1.InstanceResourceControl, old, next *corev1.Instance) error {
	x, y := a.Change, b.Change
	if x != nil && y == nil {
		if x.Phase != ChangeApplying || b.ApprovedRevision != x.ToRevision || b.ObservedRevision != x.ToRevision || b.CurrentAttempt == nil || b.ObservedAttemptID != b.CurrentAttempt.ID || b.ObservedEpoch != b.CurrentAttempt.Epoch {
			return conflict("change cannot clear before matching execution observation")
		}
	} else if x != nil && y != nil {
		p, q := *x, *y
		p.Phase, q.Phase = "", ""
		if !reflect.DeepEqual(p, q) {
			return conflict("resource change identity and complete plan are immutable")
		}
		if x.Phase != y.Phase && (x.Phase != ChangeBudgeting || y.Phase != ChangeApplying) {
			return conflict("invalid resource change phase")
		}
	} else if x == nil && y != nil {
		if GetStage(old) != StageActive || GetStage(next) != StageActive || a.ApprovedRevision == math.MaxInt64 || y.FromRevision != a.ApprovedRevision || y.ToRevision != a.ApprovedRevision+1 || y.Phase != ChangeBudgeting || !reflect.DeepEqual(y.OldResources, a.ApprovedResources) {
			return conflict("new change must freeze current approved budget")
		}
		if err := corev1.ValidateIdentifier("changeID", y.ID); err != nil {
			return err
		}
	}
	return nil
}

func validateActiveChangePayload(b *corev1.InstanceResourceControl, next *corev1.Instance, y *corev1.InstanceResourceChange) error {
	before, err := snapshot(y.OldResources)
	if err != nil {
		return err
	}
	target, err := snapshot(y.TargetResources)
	if err != nil {
		return err
	}
	desired, err := corev1.NormalizeInstanceResources(next.Spec.Resources, nil)
	if err != nil {
		return err
	}
	if before == target || target != desired || b.DesiredRevision != y.ToRevision {
		return conflict("change target differs from declared resources or revision")
	}
	switch y.Phase {
	case ChangeApplying:
		if b.ApprovedRevision != y.ToRevision || !reflect.DeepEqual(b.ApprovedResources, y.TargetResources) {
			return conflict("Applying must publish the complete target budget")
		}
	case ChangeBudgeting:
		if b.ApprovedRevision != y.FromRevision || !reflect.DeepEqual(b.ApprovedResources, y.OldResources) {
			return conflict("Budgeting cannot publish executable resources")
		}
	default:
		return conflict("unknown change phase")
	}
	return nil
}

func canonicalPorts(input []corev1.InstancePort) ([]corev1.InstancePort, error) {
	requests := make([]corev1.PortRequest, len(input))
	for n, p := range input {
		if p.TargetPort < 1 || p.TargetPort > 65535 || p.PublishedPort < 0 || p.PublishedPort > 65535 {
			return nil, &corev1.ValidationError{Field: "ports", Reason: corev1.ValidationReasonPortOutOfRange}
		}
		requests[n] = corev1.PortRequest{Name: p.Name, Protocol: p.Protocol, TargetPort: int32(p.TargetPort), PublishedPort: int32(p.PublishedPort)}
	}
	canonical, err := corev1.CanonicalPorts(requests)
	if err != nil {
		return nil, err
	}
	out := make([]corev1.InstancePort, len(canonical))
	for n, p := range canonical {
		out[n] = corev1.InstancePort{Name: p.Name, Protocol: p.Protocol, TargetPort: int(p.TargetPort), PublishedPort: int(p.PublishedPort)}
	}
	return out, nil
}
func validateApprovedPorts(control *corev1.InstanceResourceControl) error {
	if control.ApprovedRevision == 0 {
		if len(control.ApprovedResources) != 0 || len(control.ApprovedPorts) != 0 {
			return conflict("unbound instance has an approved execution snapshot")
		}
		return nil
	}
	a := control.CurrentAttempt
	if a == nil {
		return conflict("approved ports lack placement")
	}
	if len(a.Ports) != len(control.ApprovedPorts) {
		return conflict("approved port set differs from frozen placement")
	}
	if _, err := canonicalPorts(control.ApprovedPorts); err != nil {
		return err
	}
	for n, assigned := range control.ApprovedPorts {
		request := a.Ports[n]
		if assigned.PublishedPort < 1 || assigned.Name != request.Name || assigned.Protocol != request.Protocol || assigned.TargetPort != request.TargetPort || request.PublishedPort != 0 && request.PublishedPort != assigned.PublishedPort {
			return conflict("approved port assignment differs from requested placement")
		}
	}
	return nil
}
func validateNewPorts(attempt *corev1.InstancePlacementAttempt, spec []corev1.InstancePort) error {
	canonical, err := canonicalPorts(spec)
	if err != nil {
		return err
	}
	if !slices.Equal(attempt.Ports, canonical) {
		return conflict("new attempt must freeze canonical requested ports")
	}
	return nil
}
