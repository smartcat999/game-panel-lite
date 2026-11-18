package instance

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	appmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/app"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

func newDeletedInstanceEvent(previous *corev1.Instance) mq.Event {
	ev := mq.NewEvent(mq.EventDeleted, corev1.GroupVersion, "Instance", previous.Metadata.Tenant, previous.Metadata.Name, previous.Metadata.Generation, previous.Metadata.ResourceVersion)
	attrs := map[string]string{
		"nodeName": previous.Spec.NodeName,
	}
	if previous.Metadata.UID != "" {
		attrs["uid"] = previous.Metadata.UID
	}
	labels := previous.Metadata.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	if raw, err := json.Marshal(labels); err == nil {
		attrs["labels"] = string(raw)
	}
	ev.Attributes = attrs
	return ev
}

const (
	instanceResyncBatchSize = 200
	instanceRetryMin        = time.Second
	instanceRetryMax        = time.Minute
	instanceReconcileTTL    = 2 * time.Minute
)

type reconcileEntry struct {
	running bool
	pending bool
	version string
	retryAt time.Time
	delay   time.Duration
}

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct {
	mu            sync.Mutex
	resyncOffsets map[string]int
	reconcileKeys map[string]*reconcileEntry
	backupRunning sync.Map // map[string]time.Time (instance UID -> last attempt time)
}

var (
	transferHTTPClient = &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: 45 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
	}
)

func (c *Controller) Name() string  { return "03-instance-scheduler" }
func (c *Controller) Topic() string { return mq.TopicInstance }

func (c *Controller) pruneBackupLocks(now time.Time) {
	c.backupRunning.Range(func(k, v any) bool {
		if t, ok := v.(time.Time); ok && now.Sub(t) > 10*time.Minute {
			c.backupRunning.Delete(k)
		}
		return true
	})
}

// Resync pages each lifecycle stage independently so pending work cannot starve
// behind stable bindings.
func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, _ *ctrlruntime.Config) ([]mq.Event, error) {
	now := time.Now()
	c.pruneBackupLocks(now)
	c.pruneReconcileKeys(now)

	instDB := instancedb.NewDB(reg.Pool())
	nodeDB := nodedb.NewDB(reg.Pool())
	tenantDB := tenantdb.NewDB(reg.Pool())
	seen := make(map[string]struct{})
	nodes := make(map[string]*corev1.Node)
	tenants := make(map[string]*corev1.Tenant)
	var events []mq.Event
	for _, stage := range []string{corev1.InstanceStagePendingQuota, corev1.InstanceStageScheduling, corev1.InstanceStageDeleting, corev1.InstanceStageReleasingQuota, corev1.InstanceStageActive} {
		offset := c.nextStageOffset(stage, -1)
		page, err := instDB.List(ctx, db.ListOptions{FieldSelector: map[string]string{"status.resourceControl.stage": stage}, Limit: instanceResyncBatchSize, Offset: offset, SkipTotalCount: true})
		if err != nil {
			return nil, err
		}
		c.nextStageOffset(stage, len(page.Items))
		for _, inst := range page.Items {
			if inst == nil {
				continue
			}
			cacheInstanceNode(ctx, nodeDB, inst, nodes)
			if !needsReconcileWithNodes(inst, nodes) && !tenantFreezeMismatch(ctx, tenantDB, inst, tenants) && !c.retryDue(inst, now) {
				continue
			}
			key := inst.Metadata.Tenant + "/" + inst.Metadata.Name
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			ev := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Instance", inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.Generation, inst.Metadata.ResourceVersion)
			ev.Topic = mq.TopicInstance
			events = append(events, ev)
		}
	}
	return events, nil
}

func cacheInstanceNode(ctx context.Context, nodeDB *nodedb.DB, inst *corev1.Instance, nodes map[string]*corev1.Node) {
	if !needsReconcile(inst) && inst.Spec.NodeName != "" && inst.Status.Phase != corev1.InstancePhaseUnknown {
		if _, cached := nodes[inst.Spec.NodeName]; !cached {
			if node, err := nodeDB.Get(ctx, inst.Spec.NodeName); err == nil {
				nodes[inst.Spec.NodeName] = node
			}
		}
	}
}

func tenantFreezeMismatch(ctx context.Context, tenantDB *tenantdb.DB, inst *corev1.Instance, tenants map[string]*corev1.Tenant) bool {
	if inst == nil || inst.Metadata.DeletionTimestamp != nil || inst.Status.ResourceControl == nil {
		return false
	}
	if inst.Status.ResourceControl.Stage != corev1.InstanceStageActive {
		return false
	}
	t, ok := tenants[inst.Metadata.Tenant]
	if !ok {
		if loaded, err := tenantDB.Get(ctx, inst.Metadata.Tenant); err == nil {
			t = loaded
		}
		tenants[inst.Metadata.Tenant] = t
	}
	if t == nil {
		return false
	}
	return (t.Status.Phase == corev1.TenantPhaseSuspended) != inst.Status.ResourceControl.Suspended
}

func (c *Controller) nextStageOffset(stage string, fetched int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resyncOffsets == nil {
		c.resyncOffsets = make(map[string]int)
	}
	cur := c.resyncOffsets[stage]
	if fetched >= 0 {
		if fetched == instanceResyncBatchSize {
			c.resyncOffsets[stage] = cur + instanceResyncBatchSize
		} else {
			c.resyncOffsets[stage] = 0
		}
	}
	return cur
}

func publishConditionChange(ctx context.Context, bus mq.Interface, previous, current *corev1.Instance) {
	if bus == nil || previous == nil || current == nil || previous.Metadata.UID != current.Metadata.UID {
		return
	}
	condSame := reflect.DeepEqual(previous.Status.Conditions, current.Status.Conditions)
	prevSusp := previous.Status.ResourceControl != nil && previous.Status.ResourceControl.Suspended
	curSusp := current.Status.ResourceControl != nil && current.Status.ResourceControl.Suspended
	if condSame && prevSusp == curSusp {
		return
	}
	ev := mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Instance", current.Metadata.Tenant, current.Metadata.Name, current.Metadata.Generation, current.Metadata.ResourceVersion)
	ev.Attributes = map[string]string{"statusOnly": "true", "prevPhase": string(previous.Status.Phase), "hasUsage": "false"}
	_ = bus.Publish(ctx, mq.TopicInstance, ev)
}

func isStatusOnlyEvent(event mq.Event) bool {
	return event.Type == mq.EventUpdated && event.Attributes != nil && event.Attributes["statusOnly"] == "true"
}

func (c *Controller) releaseAndPublish(ctx context.Context, bus mq.Interface, previous, current *corev1.Instance, progress, acquired bool, unlock func(), reconcileErr error) error {
	if unlock != nil {
		unlock()
	}
	rerun := c.finishReconcile(previous.Metadata.UID, current, progress, acquired, reconcileErr, time.Now())
	if bus == nil || !acquired {
		return nil
	}
	if current == nil {
		return bus.Publish(ctx, mq.TopicInstance, newDeletedInstanceEvent(previous))
	}
	if !progress && !rerun {
		return nil
	}
	if err := bus.Publish(ctx, mq.TopicInstance, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Instance", current.Metadata.Tenant, current.Metadata.Name, current.Metadata.Generation, current.Metadata.ResourceVersion)); err != nil {
		return err
	}
	if progress {
		_ = bus.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", current.Metadata.Tenant, 0, ""))
	}
	return nil
}

func (c *Controller) Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, _ *ctrlruntime.Config, event mq.Event) (reconcileErr error) {
	if event.Type == mq.EventDeleted {
		if bus != nil && event.Tenant != "" {
			_ = bus.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", event.Tenant, 0, ""))
		}
		return nil
	}
	if event.Tenant == "" && strings.HasPrefix(event.Name, corev1.InstanceSyncAppMetricsPrefix) {
		return appmodel.SyncAppInstanceMetrics(ctx, reg, bus, strings.TrimPrefix(event.Name, corev1.InstanceSyncAppMetricsPrefix))
	}
	if event.Tenant == "" || event.Name == "" {
		return nil
	}
	prevInst, err := instancedb.NewDB(reg.Pool()).Get(ctx, event.Tenant, event.Name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	statusOnly := isStatusOnlyEvent(event)
	if !c.beginReconcile(prevInst, statusOnly) {
		return nil
	}
	curInst := prevInst
	progress := false
	acquired := false
	var unlock func()
	defer func() {
		if pubErr := c.releaseAndPublish(ctx, bus, prevInst, curInst, progress, acquired, unlock, reconcileErr); reconcileErr == nil && pubErr != nil {
			reconcileErr = pubErr
		}
	}()
	unlock, acquired = cache.TryLock(ctx, "gamepanel:lock:instance:reconcile:"+prevInst.Metadata.UID, instanceReconcileTTL)
	if !acquired {
		return nil
	}
	if statusOnly {
		if !needsReconcile(prevInst) && !scheduleStatusesNeedPrune(prevInst) {
			return nil
		}
	}
	curInst, progress, err = c.runStageReconcileStep(ctx, reg, bus, prevInst, event.Tenant, event.Name)
	if err != nil || curInst == nil || curInst.Metadata.DeletionTimestamp != nil || curInst.Spec.NodeName == "" {
		return err
	}
	return c.reconcileActiveNodeAndSchedules(ctx, reg, bus, curInst)
}

func (c *Controller) runStageReconcileStep(ctx context.Context, reg *repo.Registry, bus mq.Interface, previous *corev1.Instance, tenant, name string) (*corev1.Instance, bool, error) {
	previousControl := previous.Status.ResourceControl
	inst, err := instancemodel.NewStageReconciler(reg).ReconcileStep(ctx, tenant, name)
	if err != nil {
		if errors.Is(err, quotadb.ErrPolicySyncPending) && bus != nil {
			wake := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", tenant, 0, "")
			wake.Topic = mq.TopicTenant
			_ = bus.Publish(ctx, mq.TopicTenant, wake)
		}
		if current, readErr := instancedb.NewDB(reg.Pool()).Get(ctx, tenant, name); readErr == nil {
			inst = current
			publishConditionChange(ctx, bus, previous, current)
		} else if inst == nil {
			inst = previous
		}
		return inst, false, err
	}
	progress := inst != nil && !reflect.DeepEqual(previousControl, inst.Status.ResourceControl)
	publishConditionChange(ctx, bus, previous, inst)
	return inst, progress, nil
}

func (c *Controller) reconcileActiveNodeAndSchedules(ctx context.Context, reg *repo.Registry, bus mq.Interface, inst *corev1.Instance) error {
	instDB := instancedb.NewDB(reg.Pool())
	node, nodeErr := nodedb.NewDB(reg.Pool()).Get(ctx, inst.Spec.NodeName)
	if nodeErr != nil && !errors.Is(nodeErr, db.ErrNotFound) {
		return nodeErr
	}
	if errors.Is(nodeErr, db.ErrNotFound) || node != nil && node.Status.Phase == corev1.NodePhaseNotReady {
		if inst.Status.Phase != corev1.InstancePhaseUnknown {
			inst.Status.Phase = corev1.InstancePhaseUnknown
			inst.Status.Endpoint = corev1.InstanceEndpoint{}
			metav1.SetCondition(&inst.Status.Conditions, metav1.Condition{Type: metav1.ConditionTypeReady, Status: metav1.ConditionUnknown, ObservedGeneration: inst.Metadata.Generation, Reason: corev1.ReasonNodeNotReady, Message: "Node " + inst.Spec.NodeName + " is not reporting; instance state is unknown"})
			if _, err := instDB.UpdateObservation(ctx, inst); err != nil {
				return err
			}
		}
		return nil
	}
	if scheduleStatusesNeedPrune(inst) {
		pruned := pruneControllerScheduleStatuses(inst.Spec.Schedules, inst.Status.ScheduleStatuses)
		updated, err := instDB.UpdateScheduleStatuses(ctx, inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.UID, inst.Metadata.StatusVersion, pruned)
		if err != nil {
			return err
		}
		publishScheduleStatus(ctx, bus, updated)
		inst = updated
	}
	if inst.Status.ResourceControl != nil && inst.Status.ResourceControl.Suspended {
		return nil
	}
	autoBackupErr := c.maybeRunAutoBackup(ctx, reg, bus, inst)
	schedErr := c.reconcileDueSchedules(ctx, reg, bus, inst)
	if latest, gErr := instDB.Get(ctx, inst.Metadata.Tenant, inst.Metadata.Name); gErr == nil && latest != nil {
		inst = latest
	}
	op := instancemodel.NewOperator(reg, nil, bus, instancemodel.WithHTTPClient(transferHTTPClient))
	return errors.Join(autoBackupErr, schedErr, op.ReconcileBackups(ctx, inst))
}

func (c *Controller) maybeRunAutoBackup(ctx context.Context, reg *repo.Registry, bus mq.Interface, inst *corev1.Instance) error {
	due, ready := autoBackupOccurrence(inst, time.Now())
	if !ready || config.Get() == nil || !config.Get().S3.IsConfigured() {
		return nil
	}
	key := inst.Metadata.UID
	now := time.Now()
	prev, exists := c.backupRunning.Load(key)
	lastAttempt, _ := prev.(time.Time)
	if exists && now.Sub(lastAttempt) < time.Minute {
		return nil
	}
	c.backupRunning.Store(key, now)
	op := instancemodel.NewOperator(reg, nil, bus, instancemodel.WithHTTPClient(transferHTTPClient))
	backupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	archivePath := buildAutoBackupArchivePath(inst.Metadata.Name, due)
	opID := "policy:" + inst.Metadata.UID + ":" + due.Format(time.RFC3339Nano)
	_, backupErr := op.Files(backupCtx, inst.Metadata.Tenant, inst.Metadata.Name, &corev1.InstanceFiles{
		Metadata:    metav1.ObjectMeta{UID: inst.Metadata.UID},
		Action:      corev1.FileActionBackup,
		OperationID: opID,
		Path:        archivePath,
	})
	if backupErr != nil {
		log.S().Warnw("auto-backup execution failed", "tenant", inst.Metadata.Tenant, "instance", inst.Metadata.Name, "err", backupErr)
	}
	return backupErr
}

func needsReconcileWithNodes(inst *corev1.Instance, nodes map[string]*corev1.Node) bool {
	if needsReconcile(inst) || scheduleStatusesNeedPrune(inst) {
		return true
	}
	if inst == nil || nodes == nil || inst.Spec.NodeName == "" || inst.Status.Phase == corev1.InstancePhaseUnknown {
		return false
	}
	node := nodes[inst.Spec.NodeName]
	return node == nil || node.Status.Phase == corev1.NodePhaseNotReady
}

func scheduleStatusesNeedPrune(inst *corev1.Instance) bool {
	if inst == nil || len(inst.Status.ScheduleStatuses) == 0 {
		return false
	}
	if len(inst.Spec.Schedules) == 0 {
		return true
	}
	allowed := make(map[string]struct{}, len(inst.Spec.Schedules))
	for _, s := range inst.Spec.Schedules {
		if id := strings.TrimSpace(s.ID); id != "" {
			allowed[id] = struct{}{}
		}
	}
	for _, st := range inst.Status.ScheduleStatuses {
		if _, ok := allowed[strings.TrimSpace(st.ID)]; !ok {
			return true
		}
	}
	return false
}

func pruneControllerScheduleStatuses(schedules []corev1.InstanceSchedule, statuses []corev1.InstanceScheduleStatus) []corev1.InstanceScheduleStatus {
	if len(statuses) == 0 || len(schedules) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(schedules))
	for _, s := range schedules {
		if id := strings.TrimSpace(s.ID); id != "" {
			allowed[id] = struct{}{}
		}
	}
	var kept []corev1.InstanceScheduleStatus
	for _, st := range statuses {
		if _, ok := allowed[strings.TrimSpace(st.ID)]; ok {
			kept = append(kept, st)
		}
	}
	return kept
}

func needsReconcile(inst *corev1.Instance) bool {
	if inst == nil {
		return false
	}
	if inst.Metadata.DeletionTimestamp != nil {
		return true
	}
	control := inst.Status.ResourceControl
	if control == nil || control.Stage != corev1.InstanceStageActive || control.Change != nil {
		return true
	}
	if control.ObservedRevision != control.ApprovedRevision {
		return true
	}
	if inst.Spec.NodeName == "" {
		return true
	}
	if isAutoBackupDue(inst) || hasPendingBackupWork(inst) {
		return true
	}
	for _, status := range inst.Status.ScheduleStatuses {
		if status.LastResult == corev1.ScheduleResultRunning && (status.ClaimedAt == nil || time.Since(*status.ClaimedAt) >= scheduleClaimTTL(inst, status.ID)) {
			return true
		}
	}
	return hasDueSchedules(inst, time.Now())
}

func hasPendingBackupWork(inst *corev1.Instance) bool {
	if inst == nil {
		return false
	}
	for _, b := range inst.Status.Backups {
		if b.Phase != "" && b.Phase != corev1.BackupPhaseCompleted && b.Phase != corev1.BackupPhaseInspectFailed {
			return true
		}
		if (b.Phase == corev1.BackupPhaseCompleted || b.Phase == corev1.BackupPhaseInspectFailed) && b.ReservedBytes > b.SizeBytes {
			return true
		}
		if b.Phase == corev1.BackupPhaseCompleted && b.Inspection != nil && b.Inspection.Phase == corev1.BackupInspectPhasePending {
			return true
		}
	}
	return false
}

func hasDueSchedules(inst *corev1.Instance, now time.Time) bool {
	if inst == nil || inst.Metadata.DeletionTimestamp != nil || inst.Status.Phase != corev1.InstancePhaseRunning {
		return false
	}
	for _, s := range inst.Spec.Schedules {
		if isScheduleDue(inst, s, now) {
			return true
		}
	}
	return false
}

func isAutoBackupDue(inst *corev1.Instance) bool {
	_, due := autoBackupOccurrence(inst, time.Now())
	return due
}

func autoBackupOccurrence(inst *corev1.Instance, now time.Time) (time.Time, bool) {
	if inst == nil || inst.Metadata.DeletionTimestamp != nil || inst.Spec.BackupPolicy == nil || !inst.Spec.BackupPolicy.Enabled || inst.Status.Phase != corev1.InstancePhaseRunning || hasPendingBackupWork(inst) {
		return time.Time{}, false
	}
	hours := inst.Spec.BackupPolicy.IntervalHours
	if hours <= 0 {
		hours = 24
	}
	if int64(hours) > int64((time.Duration(1<<63-1))/time.Hour) {
		return time.Time{}, false
	}
	interval := time.Duration(hours) * time.Hour
	base := inst.Metadata.CreationTimestamp
	if base.IsZero() && inst.Status.StartedAt != nil {
		base = *inst.Status.StartedAt
	}
	if base.IsZero() || now.Sub(base) < interval {
		return time.Time{}, false
	}
	due := base.Add(now.Sub(base) / interval * interval).UTC()
	for _, backup := range inst.Status.Backups {
		if backup.CompletedAt != nil && !backup.CompletedAt.Before(due) {
			return time.Time{}, false
		}
	}
	return due, true
}

var cstZone = time.FixedZone("CST", 8*3600)

func buildAutoBackupArchivePath(instanceName string, now time.Time) string {
	local := now.In(cstZone)
	labelHex := hex.EncodeToString([]byte(fmt.Sprintf("自动备份 %s", local.Format("01-02 15:04"))))
	return fmt.Sprintf("backups/%s-%s--%s.tar.gz", instanceName, local.Format("20060102-150405"), labelHex)
}

func (c *Controller) beginReconcile(inst *corev1.Instance, statusOnly bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reconcileKeys == nil {
		c.reconcileKeys = make(map[string]*reconcileEntry)
	}
	key := inst.Metadata.UID
	entry := c.reconcileKeys[key]
	if entry == nil {
		entry = &reconcileEntry{}
		c.reconcileKeys[key] = entry
	}
	if entry.running {
		if !statusOnly {
			entry.pending = true
		}
		return false
	}
	entry.running = true
	entry.pending = false
	return true
}

func (c *Controller) finishReconcile(uid string, inst *corev1.Instance, progress, acquired bool, err error, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.reconcileKeys[uid]
	if entry == nil {
		return false
	}
	pending := entry.pending
	entry.pending = false
	if inst == nil {
		delete(c.reconcileKeys, uid)
		return false
	}
	if err == nil && (progress || inst.Status.ResourceControl != nil && inst.Status.ResourceControl.Stage == corev1.InstanceStageActive && inst.Status.ResourceControl.Change == nil) {
		delete(c.reconcileKeys, uid)
		return acquired && (progress || pending)
	}
	entry.running = false
	entry.version = inst.Metadata.ResourceVersion
	if entry.delay == 0 {
		entry.delay = instanceRetryMin
	} else {
		entry.delay *= 2
		if entry.delay > instanceRetryMax {
			entry.delay = instanceRetryMax
		}
	}
	entry.retryAt = now.Add(entry.delay)
	return acquired && pending
}

func (c *Controller) retryDue(inst *corev1.Instance, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.reconcileKeys[inst.Metadata.UID]
	return entry != nil && !entry.running && (entry.version != inst.Metadata.ResourceVersion || !now.Before(entry.retryAt))
}

func (c *Controller) pruneReconcileKeys(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.reconcileKeys {
		if !entry.running && now.Sub(entry.retryAt) > 10*time.Minute {
			delete(c.reconcileKeys, key)
		}
	}
}
