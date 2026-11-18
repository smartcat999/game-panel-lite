package metering

import (
	"encoding/json"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
)

// PreparedFact holds a deterministic CloudEvent and its immutable execution fact entry before commit.
type PreparedFact struct {
	Event      mq.Event
	Entry      instancedb.ExecutionFactEntry
	OccurredAt time.Time
}

// PrepareInstanceFact computes a deterministic CloudEvent and immutable execution fact entry without writing to the database.
func PrepareInstanceFact(factType string, inst *corev1.Instance, ts time.Time, transitionKeys ...string) (PreparedFact, bool, error) {
	if inst == nil {
		return PreparedFact{}, false, nil
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	} else {
		ts = ts.UTC()
	}
	snapshot, observed, err := observedSnapshot(inst)
	if err != nil || (!observed && factType != FactInstanceDeleted) {
		return PreparedFact{}, false, err
	}
	idempotencyKey, data := buildInstanceFactData(inst, factType, snapshot, ts, transitionKeys)
	ceEvent := mq.NewMeteringCloudEvent(idempotencyKey, factType, inst.Metadata.Tenant, inst.Metadata.Name, data)
	ceEvent.Timestamp = ts
	if ce, err := mq.ParseMeteringCloudEvent(ceEvent); err == nil {
		ce.SetTime(ts)
		if raw, err := json.Marshal(ce); err == nil {
			ceEvent.Attributes["cloudevent_json"] = string(raw)
		}
	}
	tenantUID := ""
	if inst.Status.ResourceControl != nil {
		tenantUID = inst.Status.ResourceControl.TenantUID
	}
	entry := instancedb.ExecutionFactEntry{
		FactID:       idempotencyKey,
		Tenant:       inst.Metadata.Tenant,
		TenantUID:    tenantUID,
		InstanceUID:  firstNonEmpty(inst.Metadata.UID, inst.Metadata.Name),
		InstanceName: inst.Metadata.Name,
		FactType:     factType,
		OccurredAt:   ts,
		Attributes:   ceEvent.Attributes,
	}
	return PreparedFact{Event: ceEvent, Entry: entry, OccurredAt: ts}, true, nil
}

// ToExecutionFactEntries converts prepared metering facts into repository execution fact entries.
func ToExecutionFactEntries(facts []PreparedFact) []instancedb.ExecutionFactEntry {
	if len(facts) == 0 {
		return nil
	}
	entries := make([]instancedb.ExecutionFactEntry, 0, len(facts))
	for _, f := range facts {
		entries = append(entries, f.Entry)
	}
	return entries
}

// FactsToMQEvents extracts mq.Event values from prepared metering facts.
func FactsToMQEvents(facts []PreparedFact) []mq.Event {
	if len(facts) == 0 {
		return nil
	}
	events := make([]mq.Event, 0, len(facts))
	for _, f := range facts {
		events = append(events, f.Event)
	}
	return events
}

// CloudEventFromFactRecord converts an immutable execution fact row into an mq.Event compatible with ParseMeteringCloudEvent.
func CloudEventFromFactRecord(rec corev1.InstanceExecutionFactRecord) mq.Event {
	attrs := make(map[string]string, len(rec.Attributes))
	for k, v := range rec.Attributes {
		attrs[k] = v
	}
	return mq.Event{
		ID:         rec.FactID,
		Topic:      mq.TopicMetering,
		Type:       mq.EventType(rec.FactType),
		APIVersion: "metering.gamepanel.io/v1",
		Kind:       "CloudEvent",
		Tenant:     rec.Tenant,
		Name:       rec.InstanceName,
		Timestamp:  rec.OccurredAt.UTC(),
		Attributes: attrs,
	}
}

// InvalidateTenantCache removes cached TenantMetering window snapshots for tenant.
func InvalidateTenantCache(reg *repo.Registry, tenant string) {
	invalidateMeteringTenant(reg, tenant)
}
