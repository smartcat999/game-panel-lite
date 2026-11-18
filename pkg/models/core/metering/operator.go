package metering

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

type Operator interface {
	List(ctx context.Context, window string, opts db.ListOptions) (*metav1.List[*corev1.TenantMetering], error)
}

const (
	Window24h   = "24h"
	Window1dAlt = "1d"
	Window7d    = "7d"
	Window30d   = "30d"

	Duration24h = 24 * time.Hour
	Duration7d  = 7 * 24 * time.Hour
	Duration30d = 30 * 24 * time.Hour

	DefaultMeteringRefreshTimeout = 5 * time.Second

	FactInstanceCreated = "io.gamepanel.instance.created"
	FactInstanceRunning = "io.gamepanel.instance.running"
	FactInstanceStopped = "io.gamepanel.instance.stopped"
	FactInstanceResized = "io.gamepanel.instance.resized"
	FactInstanceDeleted = "io.gamepanel.instance.deleted"

	DefaultGameName = "custom-container"
)

var DefaultMeteringWindows = []string{Window24h, Window7d, Window30d}

var (
	secondsPerHour = decimal.NewFromInt(3600)
	milliDivisor   = decimal.NewFromInt(1000)
	bytesPerGiB    = decimal.NewFromInt(1024 * 1024 * 1024)
	minBillableHrs = decimal.NewFromFloat(0.1)
)

// RecordInstanceFact persists a usage metering business fact durably into instance_execution_facts
// with a deterministic idempotent factID (`transitionKey` when supplied) and invalidates the CloudEvent cache.
func RecordInstanceFact(ctx context.Context, reg *repo.Registry, factType string, inst *corev1.Instance, ts time.Time, transitionKeys ...string) (mq.Event, error) {
	pf, ok, err := PrepareInstanceFact(factType, inst, ts, transitionKeys...)
	if err != nil || !ok {
		return mq.Event{}, err
	}
	if reg != nil && reg.Pool() != nil {
		instDB := instancedb.NewDB(reg.Pool())
		if err := instDB.RecordExecutionFacts(ctx, []instancedb.ExecutionFactEntry{pf.Entry}); err != nil {
			return mq.Event{}, err
		}
		if persisted, err := instDB.GetExecutionFact(ctx, pf.Entry.FactID); err == nil && persisted != nil {
			pf.Event = CloudEventFromFactRecord(*persisted)
		}
	}
	RecordCloudEvent(pf.Event, reg)
	return pf.Event, nil
}

func instanceGameName(inst *corev1.Instance) string {
	if inst == nil {
		return DefaultGameName
	}
	if v := strings.TrimSpace(inst.Metadata.Labels[corev1.LabelInstancePreset]); v != "" {
		return v
	}
	if v := strings.TrimSpace(inst.Metadata.Labels[corev1.LabelInstanceApp]); v != "" {
		return v
	}
	return DefaultGameName
}

func buildInstanceFactData(inst *corev1.Instance, factType string, snapshot observationSnapshot, ts time.Time, transitionKeys []string) (string, map[string]string) {
	reqs := snapshot.resources
	phase := effectiveFactPhase(inst, factType)
	runtimeClass, node := executionLocation(inst)
	game := instanceGameName(inst)
	uid := firstNonEmpty(inst.Metadata.UID, inst.Metadata.Name)
	var customKey string
	if len(transitionKeys) > 0 {
		customKey = strings.TrimSpace(transitionKeys[0])
	}
	idempotencyKey := fmt.Sprintf("%s-%s-%s-%d", inst.Metadata.Tenant, uid, factType, ts.UnixNano())
	if customKey != "" {
		idempotencyKey = fmt.Sprintf("%s-%s-%s-%s", inst.Metadata.Tenant, uid, factType, customKey)
	}
	tenantUID := ""
	if inst.Status.ResourceControl != nil {
		tenantUID = inst.Status.ResourceControl.TenantUID
	}
	data := map[string]string{
		"uid": uid, "tenantUID": tenantUID, "eventId": idempotencyKey, "occurredAt": ts.Format(time.RFC3339Nano),
		"generation": fmt.Sprintf("%d", inst.Metadata.Generation), "phase": phase, "state": phase,
		"cpu": string(reqs[corev1.ResourceCPU]), "memory": string(reqs[corev1.ResourceMemory]), "storage": string(reqs[corev1.ResourceStorage]),
		"attemptID": snapshot.attemptID, "epoch": fmt.Sprintf("%d", snapshot.epoch), "resourceRevision": fmt.Sprintf("%d", snapshot.revision),
		"runtimeClassName": runtimeClass, "nodeName": node, "game": game,
	}
	if inst.Status.StartedAt != nil && !inst.Status.StartedAt.IsZero() {
		data["startedAt"] = inst.Status.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if len(snapshot.ports) > 0 {
		if rawPorts, err := json.Marshal(snapshot.ports); err == nil {
			data["ports"] = string(rawPorts)
		}
	}
	return idempotencyKey, data
}

func effectiveFactPhase(inst *corev1.Instance, factType string) string {
	if factType == FactInstanceDeleted {
		return string(corev1.InstancePhaseDeleted)
	}
	if factType == FactInstanceStopped {
		return string(corev1.InstancePhaseStopped)
	}
	if factType == FactInstanceRunning {
		return string(corev1.InstancePhaseRunning)
	}
	if inst != nil {
		return string(inst.Status.Phase)
	}
	return ""
}

// RecordCloudEvent validates a CNCF CloudEvents 1.0 usage metering event via github.com/cloudevents/sdk-go/v2
// and invalidates the tenant's cached metering snapshot.
func RecordCloudEvent(ev mq.Event, registries ...*repo.Registry) {
	if ev.Tenant == "" {
		return
	}
	ce, err := mq.ParseMeteringCloudEvent(ev)
	if err != nil {
		return
	}
	var data map[string]string
	_ = ce.DataAs(&data)
	var reg *repo.Registry
	if len(registries) > 0 {
		reg = registries[0]
	}
	tenantUID := firstNonEmpty(data["tenantUID"], ev.Attributes["tenantUID"])
	if tenantUID == "" {
		return
	}
	invalidateMeteringTenant(reg, ev.Tenant, tenantUID)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type operator struct {
	reg        *repo.Registry
	tenantDB   *tenantdb.DB
	instanceDB *instancedb.DB
}

func newMeteringOperator(reg *repo.Registry) *operator {
	return &operator{
		reg:        reg,
		tenantDB:   tenantdb.NewDB(reg.Pool()),
		instanceDB: instancedb.NewDB(reg.Pool()),
	}
}

func NewOperator(reg *repo.Registry) Operator {
	return newMeteringOperator(reg)
}

func parseWindowDuration(window string) (string, time.Duration) {
	switch strings.ToLower(strings.TrimSpace(window)) {
	case Window24h, Window1dAlt:
		return Window24h, Duration24h
	case Window7d:
		return Window7d, Duration7d
	default:
		return Window30d, Duration30d
	}
}

func parseDecimalCores(q corev1.Quantity) decimal.Decimal {
	milli, err := corev1.ParseCPU(q)
	if err != nil || milli <= 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(milli).Div(milliDivisor)
}

func parseDecimalGiB(q corev1.Quantity) decimal.Decimal {
	b, err := corev1.ParseBytes(q)
	if err != nil || b <= 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(b).Div(bytesPerGiB)
}

func exactDecimalHoursBetween(start, end time.Time) decimal.Decimal {
	secs := end.Sub(start).Seconds()
	if secs <= 0 {
		return decimal.Zero
	}
	return decimal.NewFromFloat(secs).Div(secondsPerHour)
}

func formatDecimalCores(c decimal.Decimal) string {
	rounded := c.RoundBank(1)
	if rounded.Equal(rounded.Truncate(0)) {
		return rounded.Truncate(0).String()
	}
	return rounded.StringFixed(1)
}

func formatDecimalGiB(g decimal.Decimal) string {
	rounded := g.RoundBank(1)
	if rounded.Equal(rounded.Truncate(0)) {
		return rounded.Truncate(0).String() + "Gi"
	}
	return rounded.StringFixed(1) + "Gi"
}

func toFloat2(d decimal.Decimal) float64 {
	f, _ := d.RoundBank(2).Float64()
	return f
}

func (o *operator) List(ctx context.Context, window string, opts db.ListOptions) (*metav1.List[*corev1.TenantMetering], error) {
	normWindow, _ := parseWindowDuration(window)
	tenantFilter := strings.TrimSpace(opts.Tenant)
	all, err := o.windowSnapshot(ctx, normWindow, tenantFilter)
	if err != nil {
		return nil, err
	}
	var items []*corev1.TenantMetering
	for _, tm := range all {
		if tm == nil {
			continue
		}
		if tenantFilter != "" && tm.Metadata.Name != tenantFilter {
			continue
		}
		if opts.LabelSelector != nil && !opts.LabelSelector.Matches(tm.Metadata.Labels) {
			continue
		}
		items = append(items, tm)
	}

	totalCount := len(items)
	if opts.Offset > 0 {
		if opts.Offset >= len(items) {
			items = []*corev1.TenantMetering{}
		} else {
			items = items[opts.Offset:]
		}
	}
	if opts.Limit > 0 && len(items) > opts.Limit {
		items = items[:opts.Limit]
	}
	if items == nil {
		items = []*corev1.TenantMetering{}
	}

	return &metav1.List[*corev1.TenantMetering]{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "TenantMeteringList"},
		Metadata: metav1.ListMeta{TotalCount: totalCount},
		Items:    items,
	}, nil
}
