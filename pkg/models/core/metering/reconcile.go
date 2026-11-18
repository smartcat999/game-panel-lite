package metering

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
)

type observationSnapshot struct {
	attemptID string
	epoch     int64
	revision  int64
	resources corev1.ResourceList
	quantity  corev1.Resources
	ports     []corev1.InstancePort
}

// observedSnapshot reads a durable execution snapshot, never the desired spec.
// Older observations remain useful while a newer approved revision is applying.
// A missing resource snapshot can only use approval for that exact observation.
func observedSnapshot(inst *corev1.Instance) (observationSnapshot, bool, error) {
	if inst == nil || inst.Status.ResourceControl == nil {
		return observationSnapshot{}, false, nil
	}
	c := inst.Status.ResourceControl
	if c.ObservedRevision <= 0 || c.ObservedAttemptID == "" || c.ObservedEpoch <= 0 {
		return observationSnapshot{}, false, nil
	}
	resources, ports := c.ObservedResources, c.ObservedPorts
	if len(resources) == 0 {
		if !matchesApprovedObservation(c) {
			return observationSnapshot{}, false, nil
		}
		resources, ports = c.ApprovedResources, c.ApprovedPorts
	}
	if len(resources) == 0 {
		return observationSnapshot{}, false, nil
	}
	quantity, err := corev1.ParseResources(resources)
	if err != nil {
		return observationSnapshot{}, false, err
	}
	if err := quantity.ValidateInstance(); err != nil {
		return observationSnapshot{}, false, err
	}
	return observationSnapshot{
		attemptID: c.ObservedAttemptID,
		epoch:     c.ObservedEpoch,
		revision:  c.ObservedRevision,
		resources: resources,
		quantity:  quantity,
		ports:     canonicalObservationPorts(ports),
	}, true, nil
}

func matchesApprovedObservation(c *corev1.InstanceResourceControl) bool {
	return c != nil && c.CurrentAttempt != nil && c.ApprovedRevision > 0 &&
		c.ObservedRevision == c.ApprovedRevision && c.ObservedAttemptID == c.CurrentAttempt.ID &&
		c.ObservedEpoch == c.CurrentAttempt.Epoch
}

func canonicalObservationPorts(ports []corev1.InstancePort) []corev1.InstancePort {
	if len(ports) == 0 {
		return nil
	}
	out := append([]corev1.InstancePort(nil), ports...)
	for i := range out {
		out[i].Protocol = strings.ToUpper(strings.TrimSpace(out[i].Protocol))
		if out[i].Protocol == "" {
			out[i].Protocol = corev1.ProtocolTCP
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		if a.PublishedPort != b.PublishedPort {
			return a.PublishedPort < b.PublishedPort
		}
		if a.TargetPort != b.TargetPort {
			return a.TargetPort < b.TargetPort
		}
		return a.Name < b.Name
	})
	return out
}

func sameObservation(a, b observationSnapshot) bool {
	return a.attemptID == b.attemptID && a.epoch == b.epoch && a.revision == b.revision &&
		a.quantity == b.quantity && reflect.DeepEqual(a.ports, b.ports)
}

// PrepareObservationFacts computes durable execution metering facts for an incoming
// observation transition without mutating the database, so the caller can commit them
// atomically in the same transaction as instance_observations.
//
// Running/Stopped require a matching approved attempt, epoch, revision, and
// resource snapshot. First observations and phase changes record Running or
// Stopped; a changed execution snapshot records Resized even when StartedAt and
// phase are unchanged. A repeated heartbeat or a desired-only edit records no
// fact. Fact identities exclude heartbeat timestamps and object resource versions.
func PrepareObservationFacts(ctx context.Context, reg *repo.Registry, previous, incoming *corev1.Instance, observedAt time.Time, transitionID string) ([]PreparedFact, error) {
	snapshot, ok, err := validateIncomingObservation(previous, incoming)
	if err != nil || !ok {
		return nil, err
	}
	prior, hasPrior, priorPhase, priorStartedAt, priorObservedAt, err := resolvePriorObservationState(ctx, reg, previous, incoming, snapshot, observedAt)
	if err != nil {
		return nil, err
	}
	phaseChanged := !hasPrior || priorPhase != incoming.Status.Phase
	restarted := hasPrior && incoming.Status.Phase == corev1.InstancePhaseRunning &&
		priorStartedAt != nil && incoming.Status.StartedAt != nil &&
		!priorStartedAt.Equal(*incoming.Status.StartedAt)
	if hasPrior && !phaseChanged && !restarted && sameObservation(prior, snapshot) {
		return nil, nil
	}
	if err := verifyObservationMatchesApproval(incoming.Status.ResourceControl, snapshot); err != nil {
		return nil, err
	}
	factType := FactInstanceRunning
	if incoming.Status.Phase == corev1.InstancePhaseStopped {
		factType = FactInstanceStopped
	}
	if hasPrior && !phaseChanged && !restarted {
		factType = FactInstanceResized
	}
	boundary := computeObservationBoundary(factType, transitionID, hasPrior, phaseChanged, priorPhase, priorObservedAt, priorStartedAt, incoming)
	identity, err := json.Marshal(struct {
		AttemptID string
		Epoch     int64
		Revision  int64
		Quantity  corev1.Resources
		Ports     []corev1.InstancePort
		Phase     corev1.InstancePhase
		Boundary  string
	}{snapshot.attemptID, snapshot.epoch, snapshot.revision, snapshot.quantity, snapshot.ports, incoming.Status.Phase, boundary})
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("observation-%x", sha256.Sum256(identity))
	pf, ok, err := PrepareInstanceFact(factType, incoming, observedAt, key)
	if err != nil || !ok {
		return nil, err
	}
	return []PreparedFact{pf}, nil
}

// RecordObservationFacts prepares, persists into instance_execution_facts, and invalidates
// metering cache for stale or standalone observation reconciliation.
func RecordObservationFacts(ctx context.Context, reg *repo.Registry, previous, incoming *corev1.Instance, observedAt time.Time, transitionID string) ([]mq.Event, error) {
	facts, err := PrepareObservationFacts(ctx, reg, previous, incoming, observedAt, transitionID)
	if err != nil || len(facts) == 0 {
		return nil, err
	}
	if reg != nil && reg.Pool() != nil {
		if err := instancedb.NewDB(reg.Pool()).RecordExecutionFacts(ctx, ToExecutionFactEntries(facts)); err != nil {
			return nil, err
		}
	}
	for _, f := range facts {
		RecordCloudEvent(f.Event, reg)
	}
	return FactsToMQEvents(facts), nil
}

func validateIncomingObservation(previous, incoming *corev1.Instance) (observationSnapshot, bool, error) {
	if incoming == nil || (incoming.Status.Phase != corev1.InstancePhaseRunning && incoming.Status.Phase != corev1.InstancePhaseStopped) {
		return observationSnapshot{}, false, nil
	}
	if previous != nil && (previous.Metadata.UID != incoming.Metadata.UID || previous.Metadata.Tenant != incoming.Metadata.Tenant || previous.Metadata.Name != incoming.Metadata.Name) {
		return observationSnapshot{}, false, fmt.Errorf("%w: metering observation identity changed", db.ErrConflict)
	}
	snapshot, ok, err := observedSnapshot(incoming)
	if err != nil || !ok {
		return observationSnapshot{}, false, err
	}
	if strings.TrimSpace(incoming.Metadata.UID) == "" {
		return observationSnapshot{}, false, fmt.Errorf("%w: metering observation requires an instance UID", db.ErrConflict)
	}
	c := incoming.Status.ResourceControl
	if c.TenantUID == "" || (previous != nil && previous.Status.ResourceControl != nil && previous.Status.ResourceControl.TenantUID != c.TenantUID) {
		return observationSnapshot{}, false, fmt.Errorf("%w: metering observation requires the frozen tenant UID", db.ErrConflict)
	}
	return snapshot, true, nil
}

func resolvePriorObservationState(
	ctx context.Context, reg *repo.Registry, previous, incoming *corev1.Instance, incomingSnap observationSnapshot, observedAt time.Time,
) (observationSnapshot, bool, corev1.InstancePhase, *time.Time, *time.Time, error) {
	isOutOfOrder := previous != nil && previous.Status.ObservedAt != nil && !previous.Status.ObservedAt.IsZero() &&
		!observedAt.IsZero() && observedAt.Before(*previous.Status.ObservedAt)
	if isOutOfOrder {
		return resolveOutOfOrderPrior(ctx, reg, incoming, incomingSnap, observedAt)
	}
	prior, hasPrior, err := observedSnapshot(previous)
	if err != nil {
		return observationSnapshot{}, false, "", nil, nil, err
	}
	var priorPhase corev1.InstancePhase
	var priorStartedAt, priorObservedAt *time.Time
	if previous != nil {
		priorPhase, priorStartedAt, priorObservedAt = previous.Status.Phase, previous.Status.StartedAt, previous.Status.ObservedAt
	}
	if hasPrior && priorPhase != corev1.InstancePhaseRunning && priorPhase != corev1.InstancePhaseStopped {
		fact, found, err := latestExecutionFact(ctx, reg, incoming, observedAt)
		if err != nil {
			return observationSnapshot{}, false, "", nil, nil, err
		}
		if found {
			if fact.phase == corev1.InstancePhaseDeleted {
				return observationSnapshot{}, false, "", nil, nil, fmt.Errorf("%w: execution observation follows confirmed deletion", db.ErrConflict)
			}
			priorPhase, priorObservedAt = fact.phase, &fact.observedAt
			if fact.startedAt != nil {
				priorStartedAt = fact.startedAt
			}
		}
	}
	return prior, hasPrior, priorPhase, priorStartedAt, priorObservedAt, nil
}

func resolveOutOfOrderPrior(
	ctx context.Context, reg *repo.Registry, incoming *corev1.Instance, incomingSnap observationSnapshot, observedAt time.Time,
) (observationSnapshot, bool, corev1.InstancePhase, *time.Time, *time.Time, error) {
	fact, found, err := latestExecutionFact(ctx, reg, incoming, observedAt)
	if err != nil || !found {
		return observationSnapshot{}, false, "", nil, nil, err
	}
	if fact.phase == corev1.InstancePhaseDeleted {
		return observationSnapshot{}, false, "", nil, nil, fmt.Errorf("%w: execution observation follows confirmed deletion", db.ErrConflict)
	}
	snap := fact.snapshot
	if fact.hasSnapshot && len(snap.ports) == 0 && len(incomingSnap.ports) > 0 &&
		snap.attemptID == incomingSnap.attemptID && snap.epoch == incomingSnap.epoch && snap.revision == incomingSnap.revision {
		snap.ports = incomingSnap.ports
	}
	obsCopy := fact.observedAt
	return snap, fact.hasSnapshot, fact.phase, fact.startedAt, &obsCopy, nil
}

func verifyObservationMatchesApproval(c *corev1.InstanceResourceControl, snapshot observationSnapshot) error {
	if !matchesApprovedObservation(c) {
		return fmt.Errorf("%w: metering observation does not match current approval", db.ErrConflict)
	}
	approved, err := corev1.ParseResources(c.ApprovedResources)
	if err != nil {
		return err
	}
	if snapshot.quantity != approved || !reflect.DeepEqual(snapshot.ports, canonicalObservationPorts(c.ApprovedPorts)) {
		return fmt.Errorf("%w: metering resources or ports differ from approval", db.ErrConflict)
	}
	return nil
}

func computeObservationBoundary(factType, transitionID string, hasPrior, phaseChanged bool, priorPhase corev1.InstancePhase, priorObservedAt, priorStartedAt *time.Time, incoming *corev1.Instance) string {
	if factType == FactInstanceResized {
		return ""
	}
	if b := strings.TrimSpace(transitionID); b != "" {
		return b
	}
	if hasPrior && phaseChanged && incoming.Status.ObservedAt != nil && !incoming.Status.ObservedAt.IsZero() {
		return "phase-observed:" + incoming.Status.ObservedAt.UTC().Format(time.RFC3339Nano)
	}
	if priorPhase == corev1.InstancePhaseStopped && incoming.Status.Phase == corev1.InstancePhaseRunning && priorObservedAt != nil {
		return "resume-after:" + priorObservedAt.UTC().Format(time.RFC3339Nano)
	}
	if incoming.Status.Phase == corev1.InstancePhaseStopped {
		if runStart := nonZeroTimePtr(incoming.Status.StartedAt, priorStartedAt); runStart != nil {
			return "after-run:" + runStart.UTC().Format(time.RFC3339Nano)
		}
	}
	if incoming.Status.StartedAt != nil && !incoming.Status.StartedAt.IsZero() {
		return incoming.Status.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if priorObservedAt != nil {
		return string(priorPhase) + ":" + priorObservedAt.UTC().Format(time.RFC3339Nano)
	}
	return "initial"
}

func nonZeroTimePtr(primary, fallback *time.Time) *time.Time {
	if primary != nil && !primary.IsZero() {
		return primary
	}
	if fallback != nil && !fallback.IsZero() {
		return fallback
	}
	return nil
}

type executionFact struct {
	phase       corev1.InstancePhase
	observedAt  time.Time
	startedAt   *time.Time
	snapshot    observationSnapshot
	hasSnapshot bool
}

// latestExecutionFact is only needed when a diagnostic phase obscures the last
// confirmed phase or when an observation arrives out of order. The normal
// heartbeat path never needs a history read.
func latestExecutionFact(ctx context.Context, reg *repo.Registry, inst *corev1.Instance, before time.Time) (executionFact, bool, error) {
	if reg == nil || reg.Pool() == nil || inst == nil || inst.Metadata.UID == "" || inst.Status.ResourceControl == nil {
		return executionFact{}, false, nil
	}
	if before.IsZero() {
		before = time.Now().UTC()
	}
	rows, err := instancedb.NewDB(reg.Pool()).ListInstanceExecutionFactsBefore(
		ctx, inst.Metadata.Tenant, inst.Status.ResourceControl.TenantUID, inst.Metadata.UID, before, 8,
	)
	if err != nil {
		return executionFact{}, false, err
	}
	for _, rec := range rows {
		phase := resolvedFactPhase(rec)
		if phase != corev1.InstancePhaseRunning && phase != corev1.InstancePhaseStopped && phase != corev1.InstancePhaseDeleted {
			continue
		}
		snap, hasSnap := parseFactSnapshot(rec)
		out := executionFact{phase: phase, observedAt: rec.OccurredAt.UTC(), snapshot: snap, hasSnapshot: hasSnap}
		if startedAt, err := time.Parse(time.RFC3339Nano, rec.Attributes["startedAt"]); err == nil {
			out.startedAt = &startedAt
		}
		return out, true, nil
	}
	return executionFact{}, false, nil
}

func parseFactSnapshot(rec corev1.InstanceExecutionFactRecord) (observationSnapshot, bool) {
	attemptID := strings.TrimSpace(rec.Attributes["attemptID"])
	epoch, errEpoch := strconv.ParseInt(strings.TrimSpace(rec.Attributes["epoch"]), 10, 64)
	revision, errRev := strconv.ParseInt(strings.TrimSpace(rec.Attributes["resourceRevision"]), 10, 64)
	if attemptID == "" || errEpoch != nil || epoch <= 0 || errRev != nil || revision <= 0 {
		return observationSnapshot{}, false
	}
	resources := corev1.ResourceList{corev1.ResourceInstances: "1"}
	for _, k := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory, corev1.ResourceStorage} {
		if v := strings.TrimSpace(rec.Attributes[string(k)]); v != "" {
			resources[k] = corev1.Quantity(v)
		}
	}
	quantity, err := corev1.ParseResources(resources)
	if err != nil || quantity.ValidateInstance() != nil {
		return observationSnapshot{}, false
	}
	var ports []corev1.InstancePort
	if rawPorts := strings.TrimSpace(rec.Attributes["ports"]); rawPorts != "" {
		if err := json.Unmarshal([]byte(rawPorts), &ports); err == nil {
			ports = canonicalObservationPorts(ports)
		}
	}
	return observationSnapshot{
		attemptID: attemptID,
		epoch:     epoch,
		revision:  revision,
		resources: resources,
		quantity:  quantity,
		ports:     ports,
	}, true
}

func resolvedFactPhase(rec corev1.InstanceExecutionFactRecord) corev1.InstancePhase {
	switch rec.FactType {
	case FactInstanceRunning:
		return corev1.InstancePhaseRunning
	case FactInstanceStopped:
		return corev1.InstancePhaseStopped
	case FactInstanceDeleted:
		return corev1.InstancePhaseDeleted
	default:
		return corev1.InstancePhase(rec.Attributes["phase"])
	}
}

// allocatedResources reports approved capacity, or the frozen initial tenant
// admission while scheduling. Pending intent is never an allocation.
func allocatedResources(inst *corev1.Instance) corev1.ResourceList {
	if inst == nil || inst.Status.ResourceControl == nil {
		return nil
	}
	c := inst.Status.ResourceControl
	if c.Stage == corev1.InstanceStagePendingQuota {
		return nil
	}
	if c.ApprovedRevision > 0 && len(c.ApprovedResources) > 0 {
		return c.ApprovedResources
	}
	if c.Stage == corev1.InstanceStageScheduling {
		return c.InitialResources
	}
	return nil
}

func executionLocation(inst *corev1.Instance) (runtimeClass, node string) {
	if inst == nil || inst.Status.ResourceControl == nil {
		return "", ""
	}
	c := inst.Status.ResourceControl
	if c.RuntimeClass != nil {
		runtimeClass = c.RuntimeClass.Name
	}
	if c.CurrentAttempt != nil {
		node = c.CurrentAttempt.NodeName
	}
	return runtimeClass, node
}
