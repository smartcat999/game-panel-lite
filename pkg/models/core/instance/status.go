package instance

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	promclient "github.com/smartcat999/game-panel-cloud/pkg/infra/prometheus"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
)

func (o *operator) UpdateStatus(ctx context.Context, tenant, name string, inst *corev1.Instance) (*corev1.Instance, error) {
	inst.Metadata.Tenant = tenant
	inst.Metadata.Name = name
	existing, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if err := o.authorizeAndValidateObservation(ctx, existing, inst); err != nil {
		return nil, err
	}
	targetUID := existing.Metadata.UID
	inst.Metadata.UID = targetUID

	observedAt, hasExplicitObservedAt, err := o.resolveObservedAt(inst)
	if err != nil {
		return nil, err
	}
	transitionID := strings.TrimSpace(inst.Status.TransitionID)
	if transitionID != "" && existing.Status.TransitionID == transitionID && existing.Status.Phase == inst.Status.Phase &&
		inst.Metadata.StatusVersion != "" && inst.Metadata.StatusVersion != existing.Metadata.StatusVersion {
		return existing, nil
	}
	if err := o.rejectStaleObservation(existing, inst, observedAt, hasExplicitObservedAt, transitionID); err != nil {
		return nil, err
	}
	return o.commitAndPublishStatusObservation(ctx, tenant, name, existing, inst, targetUID, observedAt, hasExplicitObservedAt, transitionID)
}

func (o *operator) authorizeAndValidateObservation(ctx context.Context, existing, inst *corev1.Instance) error {
	closing := existing.Metadata.DeletionTimestamp != nil && existing.Status.ResourceControl != nil && existing.Status.ResourceControl.CurrentAttempt != nil && existing.Status.ResourceControl.CurrentAttempt.Phase == AttemptClosing
	if closing {
		if err := authorizeInstanceCleanup(ctx, o.nodeDB, existing); err != nil {
			return err
		}
		if changesExecutionObservation(existing, inst) {
			return fmt.Errorf("%w: closing binding accepts cleanup diagnostics only", db.ErrConflict)
		}
	} else if err := authorizeInstanceObservation(ctx, o.nodeDB, existing); err != nil {
		return err
	}
	if existing.Status.ResourceControl != nil && inst.Metadata.UID != existing.Metadata.UID {
		return fmt.Errorf("%w: controlled status requires current UID", db.ErrConflict)
	}
	if inst.Metadata.UID != "" && existing.Metadata.UID != "" && inst.Metadata.UID != existing.Metadata.UID {
		return fmt.Errorf("%w: instance UID mismatch (expected %s, got %s)", models.ErrConflict, existing.Metadata.UID, inst.Metadata.UID)
	}
	return preserveIntentObservation(existing, inst)
}

const maxObservationFutureSkew = 2 * time.Minute

func (o *operator) resolveObservedAt(inst *corev1.Instance) (time.Time, bool, error) {
	now := o.now().UTC()
	if inst.Status.ObservedAt != nil && !inst.Status.ObservedAt.IsZero() {
		obs := inst.Status.ObservedAt.UTC()
		if obs.After(now.Add(maxObservationFutureSkew)) {
			return time.Time{}, true, fmt.Errorf("%w: status.observedAt is too far in the future", models.ErrBadRequest)
		}
		return obs, true, nil
	}
	return now, false, nil
}

func (o *operator) rejectStaleObservation(existing, inst *corev1.Instance, observedAt time.Time, hasExplicitObservedAt bool, transitionID string) error {
	isOlderGen := inst.Status.ObservedGeneration > 0 && existing.Status.ObservedGeneration > 0 && inst.Status.ObservedGeneration < existing.Status.ObservedGeneration
	isOlderTime := hasExplicitObservedAt && existing.Status.ObservedAt != nil && !existing.Status.ObservedAt.IsZero() && observedAt.Before(*existing.Status.ObservedAt)
	if isOlderGen || isOlderTime {
		return fmt.Errorf("%w: stale status observation rejected (observedGeneration=%d, current=%d)", db.ErrConflict, inst.Status.ObservedGeneration, existing.Status.ObservedGeneration)
	}
	if inst.Status.ObservedGeneration > 0 && inst.Status.ObservedGeneration < existing.Metadata.Generation &&
		corev1.EffectiveInstanceState(existing) == corev1.InstanceStateStopped && inst.Status.Phase == corev1.InstancePhaseRunning && transitionID == "" {
		return fmt.Errorf("%w: stale Running status observation for generation %d (current generation %d is Stopped)", db.ErrConflict, inst.Status.ObservedGeneration, existing.Metadata.Generation)
	}
	return nil
}

const maxStatusConflictRetries = 8

func (o *operator) commitAndPublishStatusObservation(ctx context.Context, tenant, name string, existing, inst *corev1.Instance, targetUID string, observedAt time.Time, hasExplicitObservedAt bool, transitionID string) (*corev1.Instance, error) {
	obsCopy := observedAt
	inst.Status.ObservedAt = &obsCopy

	callerOmittedSV := inst.Metadata.StatusVersion == ""
	if callerOmittedSV {
		inst.Metadata.StatusVersion = existing.Metadata.StatusVersion
		defer func() { inst.Metadata.StatusVersion = "" }()
	}
	updated, err := o.writeStatusWithConflictCheck(ctx, tenant, name, existing, inst, targetUID, transitionID, observedAt, hasExplicitObservedAt)
	if err != nil {
		return nil, err
	}
	o.publishStatusObservationEffects(ctx, tenant, name, existing, updated)
	return updated, nil
}

func (o *operator) writeStatusWithConflictCheck(ctx context.Context, tenant, name string, existing, inst *corev1.Instance, targetUID, transitionID string, observedAt time.Time, hasExplicitObservedAt bool) (*corev1.Instance, error) {
	current := existing
	for range maxStatusConflictRetries {
		updated, err := o.db.UpdateObservation(ctx, inst)
		if err == nil {
			return updated, nil
		}
		if errors.Is(err, corev1.OutcomeUnknown) {
			return o.recoverUnknownObservationWrite(ctx, tenant, name, current, inst, targetUID, transitionID, err)
		}
		if !errors.Is(err, db.ErrConflict) {
			return nil, err
		}
		latest, stop, rErr := o.prepareConflictRetry(ctx, tenant, name, inst, targetUID, transitionID, observedAt, hasExplicitObservedAt, err)
		if rErr != nil {
			return nil, rErr
		}
		if stop {
			return latest, nil
		}
		current = latest
	}
	return nil, db.ErrConflict
}

func (o *operator) prepareConflictRetry(ctx context.Context, tenant, name string, inst *corev1.Instance, targetUID, transitionID string, observedAt time.Time, hasExplicitObservedAt bool, conflictErr error) (*corev1.Instance, bool, error) {
	latest, gErr := o.db.Get(ctx, tenant, name)
	if gErr != nil || latest == nil || latest.Metadata.UID != targetUID {
		return nil, false, conflictErr
	}
	if transitionID != "" && latest.Status.TransitionID == transitionID && latest.Status.Phase == inst.Status.Phase {
		return latest, true, nil
	}
	if err := o.authorizeAndValidateObservation(ctx, latest, inst); err != nil {
		return nil, false, err
	}
	isOlderGen := inst.Status.ObservedGeneration > 0 && latest.Status.ObservedGeneration > 0 && inst.Status.ObservedGeneration < latest.Status.ObservedGeneration
	isOlderTime := hasExplicitObservedAt && latest.Status.ObservedAt != nil && !latest.Status.ObservedAt.IsZero() && observedAt.Before(*latest.Status.ObservedAt)
	if isOlderGen || isOlderTime {
		return nil, false, conflictErr
	}
	inst.Metadata.StatusVersion = latest.Metadata.StatusVersion
	return latest, false, nil
}

func (o *operator) recoverUnknownObservationWrite(ctx context.Context, tenant, name string, existing, inst *corev1.Instance, targetUID, transitionID string, cause error) (*corev1.Instance, error) {
	latest, err := o.db.Get(ctx, tenant, name)
	if err != nil || latest == nil || latest.Metadata.UID != targetUID {
		return nil, cause
	}
	if o.observationAlreadyCommitted(existing, latest, inst, transitionID) {
		return latest, nil
	}
	if latest.Metadata.StatusVersion != existing.Metadata.StatusVersion {
		return nil, cause
	}
	inst.Metadata.StatusVersion = latest.Metadata.StatusVersion
	return o.db.UpdateObservation(ctx, inst)
}

func (o *operator) observationAlreadyCommitted(existing, latest, inst *corev1.Instance, transitionID string) bool {
	return transitionID != "" && latest.Status.TransitionID == transitionID && latest.Status.Phase == inst.Status.Phase &&
		latest.Metadata.StatusVersion != existing.Metadata.StatusVersion
}

func (o *operator) publishStatusObservationEffects(ctx context.Context, tenant, name string, existing, updated *corev1.Instance) {
	if updated.Status.Usage != nil {
		metrics.ExportInstanceUsage(ctx, tenant, name, metrics.DefaultControlPlaneNodeLabel, updated.Status.Usage)
		_ = cache.SetJSON(ctx, nil, instanceMetaCacheKey(tenant, name), cachedInstanceMeta{
			Metadata:      updated.Metadata,
			CustomMetrics: updated.Status.Usage.CustomMetrics,
		}, DefaultInstanceMetaTTL)
	}
	phaseOrBackupsChanged := updated.Status.Phase != existing.Status.Phase || len(updated.Status.Backups) != len(existing.Status.Backups)
	if phaseOrBackupsChanged || resourceObservationChanged(existing.Status.ResourceControl, updated.Status.ResourceControl) ||
		!reflect.DeepEqual(updated.Status.StartedAt, existing.Status.StartedAt) ||
		!reflect.DeepEqual(updated.Status.Endpoint, existing.Status.Endpoint) ||
		!reflect.DeepEqual(updated.Status.Usage, existing.Status.Usage) ||
		!reflect.DeepEqual(updated.Status.ScheduleStatuses, existing.Status.ScheduleStatuses) ||
		!reflect.DeepEqual(updated.Status.Conditions, existing.Status.Conditions) {
		if o.mq != nil {
			ev := mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Instance", updated.Metadata.Tenant, updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion)
			ev.Attributes = map[string]string{
				"runtimeClassName": updated.Spec.RuntimeClassName,
				"state":            string(updated.Spec.State),
				"statusOnly":       "true",
				"statusVersion":    updated.Metadata.StatusVersion,
				"prevPhase":        string(existing.Status.Phase),
				"hasUsage":         strconv.FormatBool(updated.Status.Usage != nil),
			}
			_ = o.mq.Publish(ctx, mq.TopicInstance, ev)
			if phaseOrBackupsChanged {
				_ = o.mq.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", updated.Metadata.Tenant, 0, ""))
			}
		}
	}
}

func sameIntentExecution(observed, current *corev1.InstanceResourceControl) bool {
	if observed == nil || current == nil || observed.CurrentAttempt == nil || current.CurrentAttempt == nil {
		return false
	}
	a, b := observed.CurrentAttempt, current.CurrentAttempt
	return observed.AllocationID == current.AllocationID && observed.TenantUID == current.TenantUID &&
		observed.ApprovedRevision == current.ApprovedRevision && a.ID == b.ID && a.NodeUID == b.NodeUID && a.NodeName == b.NodeName && a.Epoch == b.Epoch
}

func preserveIntentObservation(current, incoming *corev1.Instance) error {
	control := current.Status.ResourceControl
	report := incoming.Status.ResourceControl
	if report != nil {
		if err := validateReportedObservation(current, incoming, control, report); err != nil {
			return err
		}
	}
	if control != nil {
		cp, err := intentCopy(current)
		if err != nil {
			return err
		}
		incoming.Status.ResourceControl = cp.Status.ResourceControl
		if report != nil && report.ObservedRevision > 0 {
			incoming.Status.ResourceControl.ObservedRevision = report.ObservedRevision
			incoming.Status.ResourceControl.ObservedAttemptID = report.ObservedAttemptID
			incoming.Status.ResourceControl.ObservedEpoch = report.ObservedEpoch
			incoming.Status.ResourceControl.ObservedResources = report.ObservedResources
			incoming.Status.ResourceControl.ObservedPorts = append([]corev1.InstancePort(nil), report.ObservedPorts...)
		}
	}
	return nil
}

func validateReportedObservation(current, incoming *corev1.Instance, control, report *corev1.InstanceResourceControl) error {
	if incoming.Metadata.UID != current.Metadata.UID || !sameIntentExecution(report, control) {
		return fmt.Errorf("%w: observation source does not match the current binding", db.ErrConflict)
	}
	if report.ObservedRevision < control.ObservedRevision {
		return db.ErrConflict
	}
	if report.ObservedRevision == control.ObservedRevision {
		if report.ObservedAttemptID != control.ObservedAttemptID || report.ObservedEpoch != control.ObservedEpoch ||
			!reflect.DeepEqual(report.ObservedResources, control.ObservedResources) || !reflect.DeepEqual(report.ObservedPorts, control.ObservedPorts) {
			return fmt.Errorf("%w: recorded execution observations are immutable", db.ErrConflict)
		}
		return nil
	}
	if report.ObservedRevision != control.ApprovedRevision || report.ObservedAttemptID != control.CurrentAttempt.ID || report.ObservedEpoch != control.CurrentAttempt.Epoch {
		return fmt.Errorf("%w: observation revision or epoch is not currently approved", db.ErrConflict)
	}
	actual, err := corev1.ParseResources(report.ObservedResources)
	if err != nil {
		return fmt.Errorf("%w: invalid applied resource observation", db.ErrConflict)
	}
	approved, err := corev1.ParseResources(control.ApprovedResources)
	if err != nil || actual != approved {
		return fmt.Errorf("%w: applied resources do not match approval", db.ErrConflict)
	}
	actualPorts, err := ports(report.ObservedPorts)
	if err != nil {
		return fmt.Errorf("%w: invalid applied ports", db.ErrConflict)
	}
	approvedPorts, err := ports(control.ApprovedPorts)
	if err != nil || !reflect.DeepEqual(actualPorts, approvedPorts) {
		return fmt.Errorf("%w: applied ports do not match approval", db.ErrConflict)
	}
	return nil
}

func resourceObservationChanged(a, b *corev1.InstanceResourceControl) bool {
	if a == nil || b == nil {
		return a != b
	}
	return a.ObservedRevision != b.ObservedRevision || a.ObservedAttemptID != b.ObservedAttemptID || a.ObservedEpoch != b.ObservedEpoch
}

func changesExecutionObservation(current, incoming *corev1.Instance) bool {
	if current == nil || incoming == nil || (current.Status.Phase != incoming.Status.Phase && incoming.Status.Phase != corev1.InstancePhaseTerminating) ||
		current.Status.RuntimeID != incoming.Status.RuntimeID || !reflect.DeepEqual(current.Status.Endpoint, incoming.Status.Endpoint) ||
		!reflect.DeepEqual(current.Status.StartedAt, incoming.Status.StartedAt) {
		return true
	}
	a, b := current.Status.ResourceControl, incoming.Status.ResourceControl
	if a == nil || b == nil {
		return a != b
	}
	return a.ObservedRevision != b.ObservedRevision || a.ObservedAttemptID != b.ObservedAttemptID || a.ObservedEpoch != b.ObservedEpoch ||
		!reflect.DeepEqual(a.ObservedResources, b.ObservedResources) || !reflect.DeepEqual(a.ObservedPorts, b.ObservedPorts)
}

func authorizeInstanceObservation(ctx context.Context, nodeDB *nodedb.DB, inst *corev1.Instance) error {
	return authorizeInstanceNode(ctx, nodeDB, inst, false)
}

func authorizeInstanceCleanup(ctx context.Context, nodeDB *nodedb.DB, inst *corev1.Instance) error {
	return authorizeInstanceNode(ctx, nodeDB, inst, true)
}

func authorizeInstanceNode(ctx context.Context, nodeDB *nodedb.DB, inst *corev1.Instance, cleanup bool) error {
	user, explicit := iamv1.UserFrom(ctx)
	if !explicit || (user.Kind == iamv1.SubjectKindSystem && !user.IsAnonymous()) {
		return nil
	}
	if user.Kind != iamv1.SubjectKindNode || user.IsAnonymous() {
		return fmt.Errorf("%w: instance execution and cleanup facts require the bound node", models.ErrForbidden)
	}
	if inst == nil || inst.Metadata.UID == "" || inst.Status.ResourceControl == nil {
		return fmt.Errorf("%w: instance has no current execution binding", models.ErrForbidden)
	}
	control := inst.Status.ResourceControl
	attempt := control.CurrentAttempt
	if attempt == nil || attempt.ID == "" || attempt.NodeUID == "" || attempt.NodeName == "" || attempt.Epoch <= 0 || control.AllocationID == "" || control.TenantUID == "" || control.ApprovedRevision <= 0 {
		return fmt.Errorf("%w: instance has no approved execution binding", models.ErrForbidden)
	}
	if attempt.Phase != AttemptBound && (!cleanup || attempt.Phase != AttemptClosing) {
		return fmt.Errorf("%w: instance execution binding is not active", models.ErrForbidden)
	}
	if user.Name != attempt.NodeName || inst.Spec.NodeName != attempt.NodeName {
		return fmt.Errorf("%w: caller is not the current instance node", models.ErrForbidden)
	}
	if nodeDB == nil {
		return fmt.Errorf("%w: current node identity cannot be verified", models.ErrForbidden)
	}
	node, err := nodeDB.Get(ctx, attempt.NodeName)
	if err != nil || node == nil || node.Metadata.UID != attempt.NodeUID {
		return fmt.Errorf("%w: current node UID does not match the instance binding", models.ErrForbidden)
	}
	return nil
}

func (o *operator) GetMetrics(ctx context.Context, tenant, name string, window time.Duration) (*corev1.InstanceMetrics, error) {
	mc := o.metricsClient
	if mc == nil {
		mc = promclient.DefaultClient()
	}
	samples, _ := mc.QueryInstanceSamples(ctx, tenant, name, window)

	if cachedMeta, ok := cache.GetJSON[cachedInstanceMeta](ctx, nil, instanceMetaCacheKey(tenant, name)); ok &&
		cachedMeta.Metadata.Name != "" && len(samples) > 0 && len(cachedMeta.CustomMetrics) > 0 {
		return &corev1.InstanceMetrics{
			TypeMeta:      metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceMetrics"},
			Metadata:      cachedMeta.Metadata,
			Samples:       samples,
			CustomMetrics: cachedMeta.CustomMetrics,
		}, nil
	}

	existing, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 && existing.Status.Usage != nil {
		samples = append(samples, seedMetricSampleFromUsage(ctx, tenant, name, existing.Status.Usage))
	}
	o.hydrateInstanceMetricsSpec(ctx, existing)
	customMetrics := resolveInstanceCustomMetrics(existing)
	_ = cache.SetJSON(ctx, nil, instanceMetaCacheKey(tenant, name), cachedInstanceMeta{
		Metadata:      existing.Metadata,
		CustomMetrics: customMetrics,
	}, DefaultInstanceMetaTTL)

	return &corev1.InstanceMetrics{
		TypeMeta:      metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceMetrics"},
		Metadata:      existing.Metadata,
		Samples:       samples,
		CustomMetrics: customMetrics,
	}, nil
}

func seedMetricSampleFromUsage(ctx context.Context, tenant, name string, usage *corev1.InstanceUsage) corev1.InstanceMetricSample {
	var customMap map[string]float64
	if len(usage.CustomMetrics) > 0 {
		customMap = make(map[string]float64, len(usage.CustomMetrics))
		for _, cm := range usage.CustomMetrics {
			customMap[cm.ID] = cm.Value
		}
	}
	metrics.ExportInstanceUsage(ctx, tenant, name, metrics.DefaultControlPlaneNodeLabel, usage)
	return corev1.InstanceMetricSample{
		Timestamp:            time.Now().UTC(),
		CPUMillicores:        usage.CPUMillicores,
		MemoryBytes:          usage.MemoryBytes,
		StorageBytes:         usage.StorageBytes,
		NetworkRxBytesPerSec: usage.NetworkRxBytesPerSec,
		NetworkTxBytesPerSec: usage.NetworkTxBytesPerSec,
		CustomMetrics:        customMap,
	}
}

func resolveInstanceCustomMetrics(existing *corev1.Instance) []corev1.InstanceCustomMetric {
	if existing.Status.Usage != nil && len(existing.Status.Usage.CustomMetrics) > 0 {
		return existing.Status.Usage.CustomMetrics
	}
	if len(existing.Spec.Metrics) == 0 {
		return nil
	}
	customMetrics := make([]corev1.InstanceCustomMetric, 0, len(existing.Spec.Metrics))
	for _, m := range existing.Spec.Metrics {
		customMetrics = append(customMetrics, corev1.InstanceCustomMetric{
			ID: m.ID, Label: m.Label, Unit: m.Unit, Value: 0, Display: "0",
			Max: m.MaxValue, BindParam: m.BindParam, QuickActionIDs: m.QuickActionIDs, ItemActions: m.ItemActions,
		})
	}
	return customMetrics
}
