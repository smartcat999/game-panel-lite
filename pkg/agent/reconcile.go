package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

// frozenRuntimeClass consumes only the persisted backend identity. Missing
// identity cannot be reconstructed from a mutable class name, even for cleanup.
func frozenRuntimeClass(inst *corev1.Instance) (*corev1.RuntimeClass, error) {
	if inst == nil || inst.Status.ResourceControl == nil {
		return nil, errors.New("instance has no frozen runtime binding")
	}
	if binding := inst.Status.ResourceControl.RuntimeClass; binding != nil {
		if binding.UID == "" || binding.Name == "" || binding.Handler == "" {
			return nil, errors.New("frozen runtime binding is incomplete")
		}
		params := make(map[string]string, len(binding.Parameters))
		for key, value := range binding.Parameters {
			params[key] = value
		}
		return &corev1.RuntimeClass{
			Metadata:   metav1.ObjectMeta{UID: binding.UID, Name: binding.Name},
			Handler:    binding.Handler,
			Parameters: params,
		}, nil
	}
	return nil, errors.New("instance has no frozen runtime binding")
}

type instanceWorkItem struct {
	listed *corev1.Instance
	orphan *localInstance
}

func (a *Agent) reconcileAssignedInstances(ctx context.Context) error {
	cli, err := a.apiClient()
	if err != nil {
		return err
	}
	lookupCtx, cancel := a.operationContext(ctx)
	list, err := cli.CoreV1().Instances("").List(lookupCtx, client.ListOptions{NodeName: a.cfg.NodeName})
	if err != nil {
		cancel()
		return err
	}
	node, err := cli.CoreV1().Nodes().Get(lookupCtx, a.cfg.NodeName)
	cancel()
	if err != nil {
		return err
	}
	localNodeUID := strings.TrimSpace(node.Metadata.UID)
	hostIP := a.resolveHostIPFromNode(node)
	workItems := a.buildInstanceWorkItems(list.Items)
	return a.executeInstanceWorkPool(ctx, cli, workItems, localNodeUID, hostIP)
}

func (a *Agent) resolveHostIPFromNode(node *corev1.Node) string {
	hostIP := strings.TrimSpace(a.cfg.ExternalIP)
	if hostIP == "" {
		hostIP = strings.TrimSpace(node.Spec.ExternalIP)
	}
	if hostIP == "" {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeAddressExternalIP && strings.TrimSpace(addr.Address) != "" {
				hostIP = strings.TrimSpace(addr.Address)
				break
			}
		}
	}
	if hostIP == "" {
		hostIP = DetectInternalIP()
	}
	return hostIP
}

func (a *Agent) buildInstanceWorkItems(listedItems []corev1.Instance) []instanceWorkItem {
	var workItems []instanceWorkItem
	seen := make(map[string]bool, len(listedItems))
	for i := range listedItems {
		inst := &listedItems[i]
		seen[inst.Metadata.Tenant+"/"+inst.Metadata.Name] = true
		workItems = append(workItems, instanceWorkItem{listed: inst})
	}
	a.instancesMu.RLock()
	for key, local := range a.instances {
		if !seen[key] && local != nil && local.instance != nil && local.driver != nil {
			workItems = append(workItems, instanceWorkItem{orphan: local})
		}
	}
	a.instancesMu.RUnlock()
	return workItems
}

func (a *Agent) executeInstanceWorkPool(ctx context.Context, cli client.Interface, workItems []instanceWorkItem, localNodeUID, hostIP string) error {
	jobs := make(chan instanceWorkItem)
	var wg sync.WaitGroup
	var errorsMu sync.Mutex
	var workErrors []error
	workers := min(agentRuntimeWorkers, len(workItems))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				var err error
				if item.listed != nil {
					err = a.reconcileListedInstance(ctx, cli, item.listed, localNodeUID, hostIP)
				} else {
					err = a.reconcileOrphan(ctx, cli, item.orphan)
				}
				if err != nil {
					errorsMu.Lock()
					workErrors = append(workErrors, err)
					errorsMu.Unlock()
				}
			}
		}()
	}
	for _, item := range workItems {
		select {
		case jobs <- item:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	return errors.Join(workErrors...)
}

func (a *Agent) reconcileListedInstance(parent context.Context, cli client.Interface, listed *corev1.Instance, nodeUID, hostIP string) error {
	if listed.Metadata.UID == "" {
		return nil
	}
	ctx, cancel := a.operationContext(parent)
	defer cancel()
	gate := a.gateForUID(listed.Metadata.UID)
	deleting := listed.Metadata.DeletionTimestamp != nil
	if deleting {
		gate.closeAdmission()
	}
	release, err := gate.acquire(ctx, deleting)
	if err != nil {
		return err
	}
	lease := &executionLease{releases: []func(){release}}
	defer lease.finish()
	current, err := cli.CoreV1().Instances(listed.Metadata.Tenant).Get(ctx, listed.Metadata.Name)
	if err != nil {
		if client.IsNotFound(err) {
			return nil
		}
		return err
	}
	if current.Metadata.DeletionTimestamp != nil {
		gate.closeAdmission()
	}
	inst, err := approvedInstance(current, a.cfg.NodeName, nodeUID)
	if err != nil || !sameExecution(listed, current) {
		return nil
	}
	key := inst.Metadata.Tenant + "/" + inst.Metadata.Name
	a.instancesMu.RLock()
	previous := a.instances[key]
	a.instancesMu.RUnlock()
	if previous != nil && previous.instance != nil && previous.instance.Metadata.UID != inst.Metadata.UID {
		if err := a.cleanupPriorUID(ctx, previous); err != nil {
			return err
		}
	}
	releaseSlot, err := a.acquireRuntimeSlot(ctx)
	if err != nil {
		return err
	}
	lease.releases = append(lease.releases, releaseSlot)
	rc, rcErr := frozenRuntimeClass(inst)
	var drv runtime.Driver
	if rcErr == nil && rc != nil {
		drv, _ = a.drivers.Get(rc.Handler)
	}
	if inst.Metadata.DeletionTimestamp != nil {
		gate.closeAdmission()
		return a.reconcileDeletingInstance(parent, ctx, cli, lease, inst, rc, drv, key, nodeUID)
	}
	return a.reconcileActiveInstance(parent, ctx, cli, lease, inst, rc, drv, key, nodeUID, hostIP)
}

func (a *Agent) reconcileDeletingInstance(parent, ctx context.Context, cli client.Interface, lease *executionLease, inst *corev1.Instance, rc *corev1.RuntimeClass, drv runtime.Driver, key, nodeUID string) error {
	if drv == nil {
		a.recordRuntimeHealth(inst.Metadata.UID, errors.New("cleanup driver unavailable"))
		a.reportCleanupFailure(ctx, cli, inst, nodeUID, "DriverUnavailable", "Cleanup is waiting for the instance runtime driver")
		return nil
	}
	_, err := runRuntimeOperation(ctx, lease, func() (struct{}, error) { return struct{}{}, drv.Destroy(ctx, inst, rc) })
	if err != nil {
		a.recordRuntimeHealth(inst.Metadata.UID, err)
		a.reportCleanupFailure(parent, cli, inst, nodeUID, "RuntimeCleanupFailed", "Runtime cleanup failed; resources remain reserved for retry")
		return nil
	}
	if err := ctx.Err(); err != nil {
		a.recordRuntimeHealth(inst.Metadata.UID, err)
		return err
	}
	latest, err := cli.CoreV1().Instances(inst.Metadata.Tenant).Get(ctx, inst.Metadata.Name)
	if err != nil || !sameExecution(inst, latest) {
		return nil
	}
	if _, err := approvedInstance(latest, a.cfg.NodeName, nodeUID); err != nil {
		return nil
	}
	if err := a.cleanupInstanceStorage(ctx, inst.Metadata.UID, inst.Metadata.Tenant, inst.Metadata.Name, rc); err != nil {
		a.reportCleanupFailure(ctx, cli, inst, nodeUID, "WorkDirCleanupFailed", "Instance data cleanup failed; resources remain reserved for retry")
		return nil
	}
	for _, condition := range latest.Status.Conditions {
		if condition.Type == metav1.ConditionTypeRuntimeCleanup && condition.Status == metav1.ConditionFalse {
			metav1.SetCondition(&latest.Status.Conditions, metav1.Condition{Type: metav1.ConditionTypeRuntimeCleanup, Status: metav1.ConditionTrue, Reason: corev1.ReasonCleanupSucceeded, Message: "Runtime and instance data cleanup completed", ObservedGeneration: latest.Metadata.Generation})
			latest, err = cli.CoreV1().Instances(latest.Metadata.Tenant).UpdateStatus(ctx, latest)
			break
		}
	}
	if err != nil {
		return err
	}
	latest.Metadata.Finalizers = removeString(latest.Metadata.Finalizers, corev1.FinalizerInstanceCleanup)
	if _, err := cli.CoreV1().Instances(latest.Metadata.Tenant).Update(ctx, latest); err != nil {
		return err
	}
	a.forgetInstance(key, inst)
	a.recordRuntimeHealth(inst.Metadata.UID, nil)
	return nil
}

func (a *Agent) reconcileActiveInstance(parent, _ context.Context, cli client.Interface, lease *executionLease, inst *corev1.Instance, rc *corev1.RuntimeClass, drv runtime.Driver, key, nodeUID, hostIP string) error {
	a.reportRestartingStatusIfNeeded(parent, cli, inst, nodeUID)
	runCtx, runCancel := a.runtimeOperationContext(parent)
	defer runCancel()
	obs, runErr := a.executeInstanceDriverReconcile(runCtx, lease, inst, rc, drv, key, nodeUID)
	a.recordRuntimeHealth(inst.Metadata.UID, runErr)
	reportCtx, reportCancel := a.operationContext(parent)
	defer reportCancel()
	latest, err := cli.CoreV1().Instances(inst.Metadata.Tenant).Get(reportCtx, inst.Metadata.Name)
	if err != nil || !sameExecution(inst, latest) {
		return nil
	}
	if _, err := approvedInstance(latest, a.cfg.NodeName, nodeUID); err != nil {
		return nil
	}
	a.applyRuntimeObservationToStatus(inst, latest, obs, runErr, hostIP)
	if !lease.detached && runErr == nil {
		metricsInstance := *inst
		metricsInstance.Status = latest.Status
		metricCtx, metricCancel := context.WithTimeout(reportCtx, 3*time.Second)
		usage, metricErr := runRuntimeOperation(metricCtx, lease, func() (*corev1.InstanceUsage, error) {
			collectAndRecordInstanceUsage(a.cfg.NodeName, a.cfg.WorkDir, &metricsInstance, drv)
			return metricsInstance.Status.Usage, nil
		})
		metricCancel()
		if metricErr == nil {
			latest.Status.Usage = usage
		}
	}
	latest.Status.Backups, latest.Status.ScheduleStatuses = nil, nil
	if reported, err := cli.CoreV1().Instances(inst.Metadata.Tenant).UpdateStatus(reportCtx, latest); err != nil {
		log.S().Warnw("failed to report instance observation", "instance", key, "error", err)
	} else if drv != nil && sameExecution(inst, reported) {
		if approved, err := approvedInstance(reported, a.cfg.NodeName, nodeUID); err == nil {
			a.cacheInstance(key, &localInstance{instance: approved, rc: rc, driver: drv})
		}
	}
	exportInstance := *inst
	exportInstance.Status = latest.Status
	metrics.ExportInstancePrometheus(a.cfg.NodeName, &exportInstance, a.cfg.PerInstancePrometheusMetrics)
	return nil
}

func (a *Agent) reportRestartingStatusIfNeeded(parent context.Context, cli client.Interface, inst *corev1.Instance, nodeUID string) {
	reason, message, transitionID, ok := a.pendingStatusTransition(inst)
	if !ok {
		return
	}
	ctx, cancel := a.operationContext(parent)
	defer cancel()
	latest, err := cli.CoreV1().Instances(inst.Metadata.Tenant).Get(ctx, inst.Metadata.Name)
	if err != nil || !sameExecution(inst, latest) {
		return
	}
	if _, err := approvedInstance(latest, a.cfg.NodeName, nodeUID); err != nil {
		return
	}
	latest.Status.Phase = corev1.InstancePhaseStarting
	metav1.SetCondition(&latest.Status.Conditions, metav1.Condition{
		Type:               metav1.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: inst.Metadata.Generation,
		Reason:             reason,
		Message:            message,
	})
	latest.Status.TransitionID = transitionID
	latest.Status.Backups, latest.Status.ScheduleStatuses = nil, nil
	_, _ = cli.CoreV1().Instances(inst.Metadata.Tenant).UpdateStatus(ctx, latest)
}

func (a *Agent) pendingStatusTransition(inst *corev1.Instance) (reason, message, transitionID string, ok bool) {
	if inst == nil || corev1.EffectiveInstanceState(inst) != corev1.InstanceStateRunning {
		return "", "", "", false
	}
	if inst.Status.Phase == corev1.InstancePhasePending {
		return corev1.ReasonStarting, "正在准备服务端资源与运行环境", fmt.Sprintf("agent-starting-gen%d", inst.Metadata.Generation), true
	}
	if inst.Status.Phase == corev1.InstancePhaseRunning && inst.Status.ObservedGeneration > 0 && inst.Metadata.Generation > inst.Status.ObservedGeneration {
		return corev1.ReasonRestarting, "正在应用新配置并重启容器", fmt.Sprintf("agent-restarting-gen%d", inst.Metadata.Generation), true
	}
	restartAt := strings.TrimSpace(inst.Metadata.Annotations[corev1.AnnotationRestartRequestedAt])
	reqTime, err := time.Parse(time.RFC3339Nano, restartAt)
	if restartAt == "" || err != nil {
		return "", "", "", false
	}
	switch inst.Status.Phase {
	case corev1.InstancePhaseRunning:
		if inst.Status.StartedAt != nil && reqTime.After(*inst.Status.StartedAt) {
			return corev1.ReasonRestarting, "正在重启容器", fmt.Sprintf("agent-restarting-%d", reqTime.UnixNano()), true
		}
	case corev1.InstancePhaseFailed:
		condTime := readyConditionTransitionTime(inst)
		if (condTime.IsZero() || reqTime.After(condTime)) && a.claimRestartReport(inst.Metadata.UID, restartAt) {
			return corev1.ReasonRestarting, "正在重新启动容器", fmt.Sprintf("agent-restarting-%d", reqTime.UnixNano()), true
		}
	}
	return "", "", "", false
}

func readyConditionTransitionTime(inst *corev1.Instance) time.Time {
	for _, c := range inst.Status.Conditions {
		if c.Type == metav1.ConditionTypeReady {
			return c.LastTransitionTime
		}
	}
	return time.Time{}
}

func (a *Agent) claimRestartReport(uid, restartAt string) bool {
	if uid == "" || restartAt == "" {
		return false
	}
	a.gatesMu.Lock()
	defer a.gatesMu.Unlock()
	if a.reportedRestarts == nil {
		a.reportedRestarts = make(map[string]string)
	}
	if a.reportedRestarts[uid] == restartAt {
		return false
	}
	a.reportedRestarts[uid] = restartAt
	return true
}

func (a *Agent) executeInstanceDriverReconcile(ctx context.Context, lease *executionLease, inst *corev1.Instance, rc *corev1.RuntimeClass, drv runtime.Driver, key, nodeUID string) (*runtime.RuntimeObservation, error) {
	if drv == nil {
		return nil, fmt.Errorf("runtime class %q has no registered driver", inst.Spec.RuntimeClassName)
	}
	started := time.Now()
	a.cacheInstance(key, &localInstance{instance: inst, rc: rc, driver: drv})
	executionInst, _ := approvedInstance(inst, a.cfg.NodeName, nodeUID)
	obs, runErr := runRuntimeOperation(ctx, lease, func() (*runtime.RuntimeObservation, error) { return drv.Reconcile(ctx, executionInst, rc) })
	if runErr == nil && obs == nil {
		runErr = errors.New("runtime returned no observation")
	}
	if runErr == nil {
		runErr = ctx.Err()
	}
	if runErr == nil && (obs.Phase == corev1.InstancePhaseRunning || obs.Phase == corev1.InstancePhaseStopped) {
		runErr = verifiedObservation(inst, obs, started)
	}
	return obs, runErr
}

func (a *Agent) applyRuntimeObservationToStatus(inst, latest *corev1.Instance, obs *runtime.RuntimeObservation, runErr error, hostIP string) {
	if runErr != nil {
		latest.Status.Phase = corev1.InstancePhaseFailed
		metav1.SetCondition(&latest.Status.Conditions, metav1.Condition{Type: metav1.ConditionTypeReady, Status: metav1.ConditionFalse, ObservedGeneration: inst.Metadata.Generation, Reason: corev1.ReasonReconcileError, Message: runErr.Error()})
	} else {
		latest.Status.Phase, latest.Status.RuntimeID = obs.Phase, obs.RuntimeID
		latest.Status.Endpoint, latest.Status.StartedAt = obs.Endpoint, obs.StartedAt
		if obs.Phase == corev1.InstancePhaseRunning {
			if latest.Status.Endpoint.HostIP == "" || latest.Status.Endpoint.HostIP == "127.0.0.1" {
				latest.Status.Endpoint.HostIP = hostIP
			}
		} else {
			latest.Status.Endpoint = corev1.InstanceEndpoint{}
		}
		ready := metav1.ConditionFalse
		if obs.Phase == corev1.InstancePhaseRunning {
			ready = metav1.ConditionTrue
		}
		reason, msg := obs.Reason, obs.Message
		if obs.Phase == corev1.InstancePhaseStopped && inst.Status.ResourceControl != nil && inst.Status.ResourceControl.Suspended && inst.Spec.State == corev1.InstanceStateRunning {
			reason = corev1.ReasonInstanceSuspendedByTenant
			msg = "工作区订阅已到期冻结，服务器已安全停服保号"
		}
		metav1.SetCondition(&latest.Status.Conditions, metav1.Condition{Type: metav1.ConditionTypeReady, Status: ready, ObservedGeneration: inst.Metadata.Generation, Reason: reason, Message: msg})
		switch obs.Phase {
		case corev1.InstancePhaseStarting:
			time.AfterFunc(agentStartingFollowupDelay, a.triggerReconcile)
		case corev1.InstancePhaseRunning, corev1.InstancePhaseStopped:
			latest.Status.ObservedGeneration = inst.Metadata.Generation
			control := latest.Status.ResourceControl
			control.ObservedRevision = control.ApprovedRevision
			control.ObservedAttemptID = control.CurrentAttempt.ID
			control.ObservedEpoch = control.CurrentAttempt.Epoch
			control.ObservedResources = obs.AppliedResources
			control.ObservedPorts = append([]corev1.InstancePort(nil), obs.AppliedPorts...)
			latest.Status.ObservedAt = obs.ObservedAt
		}
	}
	if latest.Status.Phase == corev1.InstancePhaseRunning && latest.Status.StartedAt != nil && !latest.Status.StartedAt.IsZero() {
		latest.Status.TransitionID = fmt.Sprintf("agent-running-%d", latest.Status.StartedAt.UnixNano())
	} else if latest.Status.StartedAt != nil && !latest.Status.StartedAt.IsZero() {
		latest.Status.TransitionID = fmt.Sprintf("agent-%s-gen%d-%d", latest.Status.Phase, inst.Metadata.Generation, latest.Status.StartedAt.UnixNano())
	} else {
		latest.Status.TransitionID = fmt.Sprintf("agent-%s-gen%d", latest.Status.Phase, inst.Metadata.Generation)
	}
}

func (a *Agent) cacheInstance(key string, local *localInstance) {
	a.instancesMu.Lock()
	defer a.instancesMu.Unlock()
	if a.instances == nil {
		a.instances = make(map[string]*localInstance)
	}
	a.instances[key] = local
}

func (a *Agent) forgetInstance(key string, inst *corev1.Instance) {
	a.instancesMu.Lock()
	if local := a.instances[key]; local != nil && sameBinding(local.instance, inst) {
		delete(a.instances, key)
	}
	a.instancesMu.Unlock()
	a.deleteInstanceMetrics(inst)
}

func (a *Agent) cleanupPriorUID(ctx context.Context, local *localInstance) error {
	gate := a.gateForUID(local.instance.Metadata.UID)
	gate.closeAdmission()
	release, err := gate.acquire(ctx, true)
	if err != nil {
		return err
	}
	lease := &executionLease{releases: []func(){release}}
	defer lease.finish()
	if local.driver == nil {
		return errors.New("prior instance cleanup driver unavailable")
	}
	releaseSlot, err := a.acquireRuntimeSlot(ctx)
	if err != nil {
		return err
	}
	lease.releases = append(lease.releases, releaseSlot)
	_, err = runRuntimeOperation(ctx, lease, func() (struct{}, error) { return struct{}{}, local.driver.Destroy(ctx, local.instance, local.rc) })
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return a.cleanupInstanceStorage(ctx, local.instance.Metadata.UID, local.instance.Metadata.Tenant, local.instance.Metadata.Name, local.rc)
}

func (a *Agent) reconcileOrphan(parent context.Context, cli client.Interface, local *localInstance) error {
	ctx, cancel := a.operationContext(parent)
	defer cancel()
	current, err := cli.CoreV1().Instances(local.instance.Metadata.Tenant).Get(ctx, local.instance.Metadata.Name)
	if err != nil && !client.IsNotFound(err) {
		return err
	}
	if err == nil && current.Metadata.UID == local.instance.Metadata.UID {
		return nil
	}
	if err := a.cleanupPriorUID(ctx, local); err != nil {
		return err
	}
	a.forgetInstance(local.instance.Metadata.Tenant+"/"+local.instance.Metadata.Name, local.instance)
	a.recordRuntimeHealth(local.instance.Metadata.UID, nil)
	return nil
}
