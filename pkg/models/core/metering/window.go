package metering

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

type factCheckpoint struct {
	timestamp        time.Time
	action           string
	phase            string
	cpu              corev1.Quantity
	memory           corev1.Quantity
	storage          corev1.Quantity
	runtimeClassName string
	nodeName         string
	game             string
	name             string
}

func (o *operator) computeWindowFromInputs(window string, inputs *meteringInputs) ([]*corev1.TenantMetering, error) {
	normWindow, dur := parseWindowDuration(window)
	now := inputs.now
	windowStart := now.Add(-dur)

	var items []*corev1.TenantMetering
	for _, t := range inputs.tenants {
		if t == nil {
			continue
		}
		tm, err := o.computeTenantWindowMetering(t, normWindow, windowStart, now, inputs)
		if err != nil {
			return nil, err
		}
		items = append(items, tm)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].CPUCoreHours != items[j].CPUCoreHours {
			return items[i].CPUCoreHours > items[j].CPUCoreHours
		}
		return items[i].Metadata.Name < items[j].Metadata.Name
	})

	if items == nil {
		items = []*corev1.TenantMetering{}
	}
	return items, nil
}

type tenantMeteringAcc struct {
	activeCount      int
	allocCores       decimal.Decimal
	allocMemGiB      decimal.Decimal
	allocDiskGiB     decimal.Decimal
	totalCoreHours   decimal.Decimal
	totalMemGiBHours decimal.Decimal
	totalDiskHours   decimal.Decimal
	totalBackups     int
	totalBackupBytes int64
	instItems        []corev1.InstanceMeteringItem
	processedKeys    map[string]bool
}

func (o *operator) computeTenantWindowMetering(t *corev1.Tenant, normWindow string, windowStart, now time.Time, inputs *meteringInputs) (*corev1.TenantMetering, error) {
	tName := t.Metadata.Name
	insts := inputs.byTenant[tName]
	sort.Slice(insts, func(i, j int) bool {
		return insts[i].Metadata.Name < insts[j].Metadata.Name
	})

	tenantFacts := inputs.factsByTenant[tName]
	recentEvents := o.collectRecentCloudEventsFromBatch(tName, t.Metadata.UID, windowStart, tenantFacts)
	factsByKey := groupFactsByInstanceKey(tenantFacts)

	acc := &tenantMeteringAcc{
		instItems:     make([]corev1.InstanceMeteringItem, 0, len(insts)),
		processedKeys: make(map[string]bool),
	}
	for _, inst := range insts {
		if err := accumulateLiveInstanceUsage(inst, factsByKey, windowStart, now, acc); err != nil {
			return nil, err
		}
	}
	accumulateDeletedInstancesUsage(factsByKey, windowStart, now, acc)

	return &corev1.TenantMetering{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "TenantMetering"},
		Metadata: metav1.ObjectMeta{
			Name: t.Metadata.Name, UID: t.Metadata.UID, Tenant: t.Metadata.Name,
			CreationTimestamp: t.Metadata.CreationTimestamp, Labels: t.Metadata.Labels,
		},
		DisplayName:        t.Spec.DisplayName,
		Type:               t.Spec.Type,
		Owner:              t.Spec.Owner,
		Window:             normWindow,
		Phase:              t.Status.Phase,
		ActiveInstances:    acc.activeCount,
		TotalInstances:     len(insts),
		AllocatedCPU:       formatDecimalCores(acc.allocCores),
		AllocatedMemory:    formatDecimalGiB(acc.allocMemGiB),
		AllocatedStorage:   formatDecimalGiB(acc.allocDiskGiB),
		QuotaCPU:           string(t.Spec.Quota[corev1.ResourceCPU]),
		QuotaMemory:        string(t.Spec.Quota[corev1.ResourceMemory]),
		QuotaStorage:       string(t.Spec.Quota[corev1.ResourceStorage]),
		CPUCoreHours:       toFloat2(acc.totalCoreHours),
		MemoryGiBHours:     toFloat2(acc.totalMemGiBHours),
		StorageGiBHours:    toFloat2(acc.totalDiskHours),
		BackupCount:        acc.totalBackups,
		BackupStorageBytes: acc.totalBackupBytes,
		CloudEventsCount:   len(recentEvents),
		Instances:          acc.instItems,
		RecentCloudEvents:  recentEvents,
	}, nil
}

func groupFactsByInstanceKey(tenantFacts []corev1.InstanceExecutionFactRecord) map[string][]factCheckpoint {
	factsByKey := make(map[string][]factCheckpoint)
	for _, rec := range tenantFacts {
		instName := rec.InstanceName
		if instName == "" {
			continue
		}
		key := firstNonEmpty(strings.TrimSpace(rec.InstanceUID), instName)
		factsByKey[key] = append(factsByKey[key], factCheckpoint{
			timestamp:        rec.OccurredAt.UTC(),
			action:           rec.FactType,
			phase:            rec.Attributes["phase"],
			cpu:              corev1.Quantity(rec.Attributes["cpu"]),
			memory:           corev1.Quantity(rec.Attributes["memory"]),
			storage:          corev1.Quantity(rec.Attributes["storage"]),
			runtimeClassName: rec.Attributes["runtimeClassName"],
			nodeName:         rec.Attributes["nodeName"],
			game:             rec.Attributes["game"],
			name:             instName,
		})
	}
	for k := range factsByKey {
		pts := factsByKey[k]
		sort.SliceStable(pts, func(i, j int) bool {
			return pts[i].timestamp.Before(pts[j].timestamp)
		})
		factsByKey[k] = pts
	}
	return factsByKey
}

func accumulateLiveInstanceUsage(inst *corev1.Instance, factsByKey map[string][]factCheckpoint, windowStart, now time.Time, acc *tenantMeteringAcc) error {
	key := firstNonEmpty(strings.TrimSpace(inst.Metadata.UID), inst.Metadata.Name)
	acc.processedKeys[key] = true

	allocated := allocatedResources(inst)
	acc.allocCores = acc.allocCores.Add(parseDecimalCores(allocated[corev1.ResourceCPU]))
	acc.allocMemGiB = acc.allocMemGiB.Add(parseDecimalGiB(allocated[corev1.ResourceMemory]))
	acc.allocDiskGiB = acc.allocDiskGiB.Add(parseDecimalGiB(allocated[corev1.ResourceStorage]))

	observed, hasObservation, err := observedSnapshot(inst)
	if err != nil {
		return err
	}
	resources := observed.resources
	if !hasObservation {
		resources = allocated
	}
	pts := factsByKey[key]
	if len(pts) == 0 && key != inst.Metadata.Name && len(factsByKey[inst.Metadata.Name]) > 0 {
		pts = factsByKey[inst.Metadata.Name]
		acc.processedKeys[inst.Metadata.Name] = true
	}
	if hasObservation && inst.Metadata.DeletionTimestamp == nil && inst.Status.Phase != corev1.InstancePhaseTerminating && hasActiveLatestCheckpoint(pts, now) {
		acc.activeCount++
	}

	var runHours, coreH, memH, diskH decimal.Decimal
	if len(pts) > 0 {
		runHours, coreH, memH, diskH = integrateCheckpoints(pts, inst, windowStart, now)
	}
	acc.totalCoreHours = acc.totalCoreHours.Add(coreH)
	acc.totalMemGiBHours = acc.totalMemGiBHours.Add(memH)
	acc.totalDiskHours = acc.totalDiskHours.Add(diskH)

	var bCount int
	var bBytes int64
	for _, b := range inst.Status.Backups {
		bCount++
		bBytes += b.SizeBytes
	}
	acc.totalBackups += bCount
	acc.totalBackupBytes += bBytes

	game := instanceGameName(inst)
	runtimeClass, node := executionLocation(inst)
	acc.instItems = append(acc.instItems, corev1.InstanceMeteringItem{
		Name: inst.Metadata.Name, Game: game, RuntimeClassName: runtimeClass, NodeName: node,
		Phase: string(inst.Status.Phase), CPU: string(resources[corev1.ResourceCPU]),
		Memory: string(resources[corev1.ResourceMemory]), Storage: string(resources[corev1.ResourceStorage]),
		RunningHours: toFloat2(runHours), CPUCoreHours: toFloat2(coreH), MemoryGiBHours: toFloat2(memH),
		StorageGiBHours: toFloat2(diskH), BackupCount: bCount, BackupBytes: bBytes, StartedAt: inst.Status.StartedAt,
	})
	return nil
}

func hasActiveLatestCheckpoint(pts []factCheckpoint, now time.Time) bool {
	for index := len(pts) - 1; index >= 0; index-- {
		if pts[index].timestamp.After(now) {
			continue
		}
		return isRunningCheckpoint(pts[index])
	}
	return false
}

func accumulateDeletedInstancesUsage(factsByKey map[string][]factCheckpoint, windowStart, now time.Time, acc *tenantMeteringAcc) {
	var deletedKeys []string
	for k := range factsByKey {
		if !acc.processedKeys[k] {
			deletedKeys = append(deletedKeys, k)
		}
	}
	sort.Strings(deletedKeys)
	for _, k := range deletedKeys {
		pts := factsByKey[k]
		if len(pts) == 0 {
			continue
		}
		runHours, coreH, memH, diskH := integrateCheckpoints(pts, nil, windowStart, now)
		if coreH.IsZero() && memH.IsZero() && diskH.IsZero() {
			continue
		}
		acc.totalCoreHours = acc.totalCoreHours.Add(coreH)
		acc.totalMemGiBHours = acc.totalMemGiBHours.Add(memH)
		acc.totalDiskHours = acc.totalDiskHours.Add(diskH)

		last := pts[len(pts)-1]
		acc.instItems = append(acc.instItems, corev1.InstanceMeteringItem{
			Name: last.name, Game: firstNonEmpty(last.game, DefaultGameName),
			RuntimeClassName: last.runtimeClassName, NodeName: last.nodeName, Phase: string(corev1.InstancePhaseDeleted),
			CPU: string(last.cpu), Memory: string(last.memory), Storage: string(last.storage),
			RunningHours: toFloat2(runHours), CPUCoreHours: toFloat2(coreH),
			MemoryGiBHours: toFloat2(memH), StorageGiBHours: toFloat2(diskH),
		})
	}
}

func (o *operator) loadTenantMeteringFacts(ctx context.Context, tenant, tenantUID string, start time.Time) ([]corev1.InstanceExecutionFactRecord, error) {
	if o.instanceDB == nil || tenant == "" || tenantUID == "" {
		return nil, nil
	}
	return o.instanceDB.ListWindowExecutionFacts(ctx, tenant, tenantUID, start)
}

// integrateCheckpoints integrates resource usage intervals [t_i, t_{i+1}) across persisted metering facts
// within [windowStart, now]. Stopping, resizing, and deleting an instance form explicit usage intervals.
func integrateCheckpoints(pts []factCheckpoint, liveInst *corev1.Instance, windowStart, now time.Time) (runHours, coreH, memH, diskH decimal.Decimal) {
	if len(pts) == 0 {
		return decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	}
	var provHours, lastRunningCores, lastRunningMem, lastDisk decimal.Decimal
	var sawRunningSegment, sawProvisionedSegment bool

	for i, pt := range pts {
		if isDeletedCheckpoint(pt) {
			break
		}
		segStart, segEnd, ok := clampCheckpointSegment(pts, i, liveInst != nil, windowStart, now)
		if !ok {
			continue
		}
		cores, memG, diskG := parseDecimalCores(pt.cpu), parseDecimalGiB(pt.memory), parseDecimalGiB(pt.storage)
		h := exactDecimalHoursBetween(segStart, segEnd)
		if h.IsZero() || (cores.IsZero() && memG.IsZero() && diskG.IsZero()) {
			continue
		}
		sawProvisionedSegment, lastDisk = true, diskG
		provHours = provHours.Add(h)
		diskH = diskH.Add(h.Mul(diskG))
		if isRunningCheckpoint(pt) {
			sawRunningSegment, lastRunningCores, lastRunningMem = true, cores, memG
			runHours = runHours.Add(h)
			coreH = coreH.Add(h.Mul(cores))
			memH = memH.Add(h.Mul(memG))
		}
	}
	if sawProvisionedSegment && provHours.LessThan(minBillableHrs) {
		diskH = diskH.Add(minBillableHrs.Sub(provHours).Mul(lastDisk))
	}
	if sawRunningSegment && runHours.LessThan(minBillableHrs) {
		topUp := minBillableHrs.Sub(runHours)
		coreH = coreH.Add(topUp.Mul(lastRunningCores))
		memH = memH.Add(topUp.Mul(lastRunningMem))
		runHours = minBillableHrs
	}
	return runHours.RoundBank(2), coreH.RoundBank(2), memH.RoundBank(2), diskH.RoundBank(2)
}

func clampCheckpointSegment(pts []factCheckpoint, i int, isLive bool, windowStart, now time.Time) (time.Time, time.Time, bool) {
	segStart := pts[i].timestamp
	segEnd := segStart
	if i+1 < len(pts) {
		segEnd = pts[i+1].timestamp
	} else if isLive {
		segEnd = now
	}
	if segEnd.Before(windowStart) || !segStart.Before(now) {
		return time.Time{}, time.Time{}, false
	}
	if segStart.Before(windowStart) {
		segStart = windowStart
	}
	if segEnd.After(now) {
		segEnd = now
	}
	return segStart, segEnd, true
}

func isDeletedCheckpoint(pt factCheckpoint) bool {
	return pt.action == FactInstanceDeleted || strings.EqualFold(pt.phase, string(corev1.InstancePhaseDeleted))
}

func isRunningCheckpoint(pt factCheckpoint) bool {
	if isDeletedCheckpoint(pt) || pt.action == FactInstanceStopped || strings.EqualFold(pt.phase, string(corev1.InstancePhaseStopped)) {
		return false
	}
	return pt.action == FactInstanceRunning || strings.EqualFold(pt.phase, string(corev1.InstancePhaseRunning))
}

func (o *operator) collectRecentCloudEventsFromBatch(tenant, tenantUID string, windowStart time.Time, batch []corev1.InstanceExecutionFactRecord) []corev1.MeteringCloudEventRecord {
	seen := make(map[string]bool)
	var out []corev1.MeteringCloudEventRecord

	for _, rec := range batch {
		ts := rec.OccurredAt.UTC()
		if firstNonEmpty(rec.TenantUID, rec.Attributes["tenantUID"]) != tenantUID || ts.Before(windowStart) {
			continue
		}
		if seen[rec.FactID] {
			continue
		}
		seen[rec.FactID] = true
		out = append(out, corev1.MeteringCloudEventRecord{
			ID:        rec.FactID,
			Type:      rec.FactType,
			Subject:   tenant + "/" + rec.InstanceName,
			Timestamp: ts,
			Phase:     rec.Attributes["phase"],
			CPU:       rec.Attributes["cpu"],
			Memory:    rec.Attributes["memory"],
			Storage:   rec.Attributes["storage"],
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

const meteringCacheTTL = 10 * time.Second

func meteringSnapshotKey(window, tenant string) string {
	if tenant == "" {
		return "metering:snap:" + window + ":_all"
	}
	return "metering:snap:" + window + ":" + tenant
}

func invalidateMeteringTenant(_ *repo.Registry, tenant string, _ ...string) {
	store := cache.Default()
	if store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, window := range DefaultMeteringWindows {
		_ = store.Delete(ctx, meteringSnapshotKey(window, ""))
		if tenant != "" {
			_ = store.Delete(ctx, meteringSnapshotKey(window, tenant))
		}
	}
}

func (o *operator) windowSnapshot(ctx context.Context, window, tenant string) ([]*corev1.TenantMetering, error) {
	if o.tenantDB == nil {
		return []*corev1.TenantMetering{}, nil
	}
	store := cache.Default()
	cacheKey := meteringSnapshotKey(window, tenant)
	if store != nil {
		if raw, err := store.Get(ctx, cacheKey); err == nil && raw != "" {
			var cached []*corev1.TenantMetering
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return cached, nil
			}
		}
	}
	tenants, err := o.loadMeteringTenants(ctx, tenant)
	if err != nil {
		return nil, err
	}
	if len(tenants) == 0 {
		return []*corev1.TenantMetering{}, nil
	}
	_, duration := parseWindowDuration(window)
	inputs, err := o.loadMeteringInputs(ctx, tenant, tenants, time.Now().UTC(), duration)
	if err != nil {
		return nil, err
	}
	items, err := o.computeWindowFromInputs(window, inputs)
	if err != nil {
		return nil, err
	}
	if store != nil {
		if encoded, err := json.Marshal(items); err == nil {
			_ = store.Set(ctx, cacheKey, string(encoded), meteringCacheTTL)
		}
	}
	publishMeteringMetrics(window, items)
	return items, nil
}

func publishMeteringMetrics(window string, items []*corev1.TenantMetering) {
	for _, tm := range items {
		if tm == nil {
			continue
		}
		tenant := tm.Metadata.Name
		metrics.TenantMeteringCPUCoreHours.WithLabelValues(tenant, window).Set(tm.CPUCoreHours)
		metrics.TenantMeteringMemoryGiBHours.WithLabelValues(tenant, window).Set(tm.MemoryGiBHours)
		metrics.TenantMeteringStorageGiBHours.WithLabelValues(tenant, window).Set(tm.StorageGiBHours)
	}
}

func RefreshMeteringSnapshots(ctx context.Context, reg *repo.Registry) {
	_ = refreshMeteringSnapshots(ctx, reg, "")
}

func refreshMeteringSnapshots(ctx context.Context, reg *repo.Registry, tenant string) error {
	o := newMeteringOperator(reg)
	if o.tenantDB == nil {
		return nil
	}
	tenants, err := o.loadMeteringTenants(ctx, tenant)
	if err != nil {
		return err
	}
	inputs, err := o.loadMeteringInputs(ctx, tenant, tenants, time.Now().UTC(), Duration30d)
	if err != nil {
		return err
	}
	store := cache.Default()
	for _, window := range DefaultMeteringWindows {
		items, err := o.computeWindowFromInputs(window, inputs)
		if err != nil {
			return err
		}
		if store != nil {
			if encoded, err := json.Marshal(items); err == nil {
				_ = store.Set(ctx, meteringSnapshotKey(window, tenant), string(encoded), meteringCacheTTL)
			}
		}
		publishMeteringMetrics(window, items)
	}
	return nil
}

func RefreshMeteringSnapshotsAsync(reg *repo.Registry) {
	RefreshTenantMeteringSnapshotsAsync(reg, "")
}

func RefreshTenantMeteringSnapshotsAsync(reg *repo.Registry, tenant string) {
	if reg == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultMeteringRefreshTimeout)
		defer cancel()
		_ = refreshMeteringSnapshots(ctx, reg, tenant)
	}()
}

type meteringInputs struct {
	now           time.Time
	tenants       []*corev1.Tenant
	byTenant      map[string][]*corev1.Instance
	factsByTenant map[string][]corev1.InstanceExecutionFactRecord
}

func (o *operator) loadMeteringTenants(ctx context.Context, tenant string) ([]*corev1.Tenant, error) {
	if tenant != "" {
		item, err := o.tenantDB.Get(ctx, tenant)
		if errors.Is(err, db.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if item == nil || item.Metadata.UID == "" {
			return nil, nil
		}
		return []*corev1.Tenant{item}, nil
	}
	list, err := o.tenantDB.List(ctx, db.ListOptions{SkipTotalCount: true})
	if err != nil || list == nil {
		return nil, err
	}
	return list.Items, nil
}

func (o *operator) loadMeteringInputs(ctx context.Context, tenant string, tenants []*corev1.Tenant, now time.Time, duration time.Duration) (*meteringInputs, error) {
	inputs := &meteringInputs{
		now: now, tenants: tenants, byTenant: make(map[string][]*corev1.Instance),
		factsByTenant: make(map[string][]corev1.InstanceExecutionFactRecord),
	}
	currentUIDs := make(map[string]string, len(tenants))
	for _, item := range tenants {
		if item != nil {
			currentUIDs[item.Metadata.Name] = item.Metadata.UID
		}
	}
	if len(inputs.tenants) == 0 {
		return inputs, nil
	}
	list, err := o.instanceDB.List(ctx, db.ListOptions{Tenant: tenant, SkipTotalCount: true})
	if err != nil {
		return nil, err
	}
	if list != nil {
		for _, inst := range list.Items {
			if inst == nil || inst.Status.ResourceControl == nil {
				continue
			}
			uid := currentUIDs[inst.Metadata.Tenant]
			if uid != "" && inst.Status.ResourceControl.TenantUID == uid && (tenant == "" || inst.Metadata.Tenant == tenant) {
				inputs.byTenant[inst.Metadata.Tenant] = append(inputs.byTenant[inst.Metadata.Tenant], inst)
			}
		}
	}
	for _, item := range inputs.tenants {
		if item == nil {
			continue
		}
		facts, err := o.loadTenantMeteringFacts(ctx, item.Metadata.Name, item.Metadata.UID, now.Add(-duration))
		if err != nil {
			return nil, err
		}
		inputs.factsByTenant[item.Metadata.Name] = facts
	}
	return inputs, nil
}
