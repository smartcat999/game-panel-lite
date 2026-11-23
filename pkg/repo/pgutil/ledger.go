package pgutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

func invalid(field, value, reason string) error {
	return &corev1.ValidationError{Field: field, Value: value, Reason: reason}
}

// fingerprint is versioned and operates only on fixed-schema structs.
func fingerprint(schema string, value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("resource-control/"+schema+"/1\n"), encoded...))
	return hex.EncodeToString(sum[:]), nil
}

// ValidateOperationTransition prevents historical results from being rewritten.
func ValidateOperationTransition(from, to corev1.OperationState) error {
	valid := func(s corev1.OperationState) bool {
		return s == corev1.Accepted || s == corev1.Committed || s == corev1.Cancelled
	}
	if !valid(from) || !valid(to) {
		return &corev1.Error{Code: corev1.InvariantViolation, Detail: "unknown operation state"}
	}
	if from == to || from == corev1.Accepted {
		return nil
	}
	return &corev1.Error{Code: corev1.InvariantViolation, Detail: "historical command result cannot be rewritten"}
}

// NextState is the lifecycle contract for resource allocations.
func NextState(state corev1.AllocationState, kind corev1.Kind) (corev1.AllocationState, error) {
	if state != corev1.AllocationPending && state != corev1.AllocationActive && state != corev1.AllocationClosing && state != corev1.AllocationClosed {
		return "", &corev1.Error{Code: corev1.InvariantViolation, Detail: "unknown allocation state"}
	}
	switch kind {
	case corev1.RequestClose:
		if state == corev1.AllocationClosed {
			return corev1.AllocationClosed, nil
		}
		return corev1.AllocationClosing, nil
	case corev1.FinalizeClose:
		if state == corev1.AllocationClosing || state == corev1.AllocationClosed {
			return corev1.AllocationClosed, nil
		}
		return "", &corev1.Error{Code: corev1.RevisionChanged, Detail: "close must be requested before finalization"}
	case corev1.Grant, corev1.Increase, corev1.SettleDecrease:
		if state == corev1.AllocationClosing || state == corev1.AllocationClosed {
			return "", &corev1.Error{Code: corev1.Closed, Detail: "allocation admission gate is closed"}
		}
		if kind == corev1.Grant && state == corev1.AllocationPending {
			return corev1.AllocationActive, nil
		}
		if kind != corev1.Grant && state == corev1.AllocationActive {
			return corev1.AllocationActive, nil
		}
		return "", &corev1.Error{Code: corev1.RevisionChanged, Detail: "command does not match allocation lifecycle"}
	case corev1.CancelChange:
		if state == corev1.AllocationPending {
			return "", &corev1.Error{Code: corev1.RevisionChanged, Detail: "cancel change requires an Active or Closing allocation"}
		}
		return state, nil
	default:
		return "", invalid("command kind", string(kind), "unknown resource command")
	}
}

type Owner struct {
	TenantUID    string `json:"tenantUid"`
	InstanceUID  string `json:"instanceUid"`
	InstanceName string `json:"instanceName"`
}

func (o Owner) Validate() error {
	for _, field := range []struct{ name, value string }{
		{"tenantUid", o.TenantUID}, {"instanceUid", o.InstanceUID}, {"instanceName", o.InstanceName},
	} {
		if err := corev1.ValidateIdentifier(field.name, field.value); err != nil {
			return err
		}
	}
	return nil
}

// Binding fences placement, not individual server starts.
type Binding struct {
	NodeUID        string `json:"nodeUid"`
	AttemptID      string `json:"attemptId"`
	PlacementEpoch int64  `json:"placementEpoch"`
}

func (b Binding) Validate() error {
	if err := corev1.ValidateIdentifier("nodeUid", b.NodeUID); err != nil {
		return err
	}
	if err := corev1.ValidateIdentifier("attemptId", b.AttemptID); err != nil {
		return err
	}
	if b.PlacementEpoch < 1 {
		return invalid("placementEpoch", "", "must be positive")
	}
	return nil
}

// AllocationIntent is frozen before the first grant attempt.
type AllocationIntent struct {
	AllocationID     string           `json:"allocationId"`
	Owner            Owner            `json:"owner"`
	ResourceRevision int64            `json:"resourceRevision"`
	Quantity         corev1.Resources `json:"quantity"`
}

func (i AllocationIntent) Validate() error {
	if err := corev1.ValidateIdentifier("allocationId", i.AllocationID); err != nil {
		return err
	}
	if err := i.Owner.Validate(); err != nil {
		return err
	}
	if i.ResourceRevision < 1 {
		return invalid("resourceRevision", "", "must be positive")
	}
	if err := i.Quantity.Validate(); err != nil {
		return err
	}
	if i.Quantity.IsZero() {
		return invalid("quantity", "0", "initial allocation must request a resource")
	}
	return nil
}

func (i AllocationIntent) Fingerprint() (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	return fingerprint("allocation-intent", i)
}

type PlacementIntent struct {
	Owner            Owner                `json:"owner"`
	Binding          Binding              `json:"binding"`
	ResourceRevision int64                `json:"resourceRevision"`
	Quantity         corev1.Resources     `json:"quantity"`
	Ports            []corev1.PortRequest `json:"ports"`
}

func (p PlacementIntent) Canonical() (PlacementIntent, error) {
	if err := p.Owner.Validate(); err != nil {
		return PlacementIntent{}, err
	}
	if err := p.Binding.Validate(); err != nil {
		return PlacementIntent{}, err
	}
	if p.ResourceRevision < 1 || p.Quantity.IsZero() {
		return PlacementIntent{}, invalid("placement", "", "positive resource revision and non-zero budget required")
	}
	if err := p.Quantity.ValidateInstance(); err != nil {
		return PlacementIntent{}, err
	}
	ports, err := corev1.CanonicalPorts(p.Ports)
	if err != nil {
		return PlacementIntent{}, err
	}
	p.Ports = ports
	return p, nil
}

func (p PlacementIntent) Fingerprint() (string, error) {
	canonical, err := p.Canonical()
	if err != nil {
		return "", err
	}
	return fingerprint("placement-intent", canonical)
}

// ResourceChange is one immutable plan.
type ResourceChange struct {
	ChangeID     string           `json:"changeId"`
	AllocationID string           `json:"allocationId"`
	Owner        Owner            `json:"owner"`
	Binding      Binding          `json:"binding"`
	FromRevision int64            `json:"fromRevision"`
	ToRevision   int64            `json:"toRevision"`
	Old          corev1.Resources `json:"old"`
	Target       corev1.Resources `json:"target"`
}

func validateChangeResources(r corev1.Resources) error {
	if r.BackupsCount == 1 && r.CPUMillicores == 0 && r.MemoryBytes == 0 && r.StorageBytes == 0 && r.InstanceCount == 0 {
		return r.Validate()
	}
	return r.ValidateInstance()
}

func (c ResourceChange) Validate() error {
	for _, field := range []struct{ name, value string }{{"changeId", c.ChangeID}, {"allocationId", c.AllocationID}} {
		if err := corev1.ValidateIdentifier(field.name, field.value); err != nil {
			return err
		}
	}
	if err := c.Owner.Validate(); err != nil {
		return err
	}
	if err := c.Binding.Validate(); err != nil {
		return err
	}
	if c.FromRevision < 1 || c.ToRevision < 1 || c.ToRevision != c.FromRevision+1 {
		return invalid("resource revision", "", "one plan must advance exactly one resource revision")
	}
	if err := validateChangeResources(c.Old); err != nil {
		return err
	}
	if err := validateChangeResources(c.Target); err != nil {
		return err
	}
	if c.Old.InstanceCount != c.Target.InstanceCount || c.Old.BackupsCount != c.Target.BackupsCount || c.Old == c.Target {
		return invalid("resource change", "", "must change resources of one existing allocation")
	}
	return nil
}

func (c ResourceChange) Fingerprint() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	return fingerprint("resource-change", c)
}

// Command is an immutable resource mutation.
type Command struct {
	OperationID      string           `json:"operationId"`
	Domain           corev1.Domain    `json:"domain"`
	AccountID        string           `json:"accountId"`
	AllocationID     string           `json:"allocationId"`
	Owner            Owner            `json:"owner"`
	Kind             corev1.Kind      `json:"kind"`
	Quantity         corev1.Resources `json:"quantity"`
	ExpectedVersion  int64            `json:"expectedVersion"`
	ResourceRevision int64            `json:"resourceRevision"`
	ChangePlanID     string           `json:"changePlanId"`
	CloseIntentID    string           `json:"closeIntentId"`
}

func (c Command) Validate() error {
	for _, field := range []struct{ name, value string }{
		{"operationId", c.OperationID}, {"accountId", c.AccountID}, {"allocationId", c.AllocationID},
	} {
		if err := corev1.ValidateIdentifier(field.name, field.value); err != nil {
			return err
		}
	}
	if err := c.Owner.Validate(); err != nil {
		return err
	}
	if c.Domain != corev1.QuotaDomain && c.Domain != corev1.NodeDomain {
		return invalid("domain", string(c.Domain), "expected quota or node")
	}
	if c.Domain == corev1.QuotaDomain && c.AccountID != c.Owner.TenantUID {
		return invalid("accountId", c.AccountID, "quota account must belong to the owner tenant")
	}
	if err := c.Quantity.Validate(); err != nil {
		return err
	}
	if c.Domain == corev1.NodeDomain {
		if err := c.Quantity.ValidateNode(); err != nil {
			return err
		}
	}
	return c.validateKind()
}

func (c Command) validateKind() error {
	switch c.Kind {
	case corev1.Grant, corev1.Increase, corev1.SettleDecrease:
		if c.ExpectedVersion < 1 || c.ResourceRevision < 1 {
			return invalid("revision", "", "funding commands require positive allocation and resource versions")
		}
		if c.CloseIntentID != "" {
			return invalid("closeIntentId", c.CloseIntentID, "funding cannot authorize cancellation")
		}
		if c.Kind != corev1.SettleDecrease && c.Quantity.IsZero() {
			return invalid("quantity", "0", "grant and increase require a positive change")
		}
		if c.Kind == corev1.Grant && c.ChangePlanID != "" {
			return invalid("changePlanId", c.ChangePlanID, "initial grant is not a resource change")
		}
		if c.Kind != corev1.Grant {
			return corev1.ValidateIdentifier("changePlanId", c.ChangePlanID)
		}
		return nil
	case corev1.CancelChange:
		if !c.Quantity.IsZero() || c.ExpectedVersion != 0 || c.ResourceRevision != 0 || c.CloseIntentID != "" {
			return invalid("cancelChange command", "", "cancel change uses persisted plan; caller must not supply quantity, version, or close intent")
		}
		return corev1.ValidateIdentifier("changePlanId", c.ChangePlanID)
	case corev1.RequestClose, corev1.FinalizeClose:
		if !c.Quantity.IsZero() || c.ExpectedVersion != 0 || c.ResourceRevision != 0 || c.ChangePlanID != "" {
			return invalid("close command", "", "store reads the current holding; caller must not supply a refund amount or stale version")
		}
		return corev1.ValidateIdentifier("closeIntentId", c.CloseIntentID)
	default:
		return invalid("kind", string(c.Kind), "unknown resource command")
	}
}

func (c Command) Fingerprint() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	return fingerprint("command", c)
}

// MatchCommand rejects changed identities before any inventory mutation.
func MatchCommand(persisted, retry Command) error {
	expected, err := persisted.Fingerprint()
	if err != nil {
		return &corev1.Error{Code: corev1.InvariantViolation, OperationID: persisted.OperationID, Detail: "invalid persisted command", Cause: err}
	}
	actual, err := retry.Fingerprint()
	if err != nil {
		return err
	}
	if expected != actual {
		return &corev1.Error{Code: corev1.IdentityConflict, OperationID: retry.OperationID, Detail: "command identity or immutable payload differs"}
	}
	return nil
}

// Allocation is a value snapshot of current resource entitlement.
type Allocation struct {
	ID               string
	Owner            Owner
	State            corev1.AllocationState
	Current          corev1.Resources
	Version          int64
	ResourceRevision int64
	CloseIntentID    string
}

func (a Allocation) Validate() error {
	if err := corev1.ValidateIdentifier("allocationId", a.ID); err != nil {
		return err
	}
	if err := a.Owner.Validate(); err != nil {
		return err
	}
	if a.Version < 1 || a.ResourceRevision < 0 {
		return &corev1.Error{Code: corev1.InvariantViolation, Detail: "invalid allocation version"}
	}
	if err := a.Current.Validate(); err != nil {
		return err
	}
	switch a.State {
	case corev1.AllocationPending:
		if !a.Current.IsZero() || a.CloseIntentID != "" {
			return &corev1.Error{Code: corev1.InvariantViolation, Detail: "Pending must have zero holdings and an open gate"}
		}
	case corev1.AllocationActive:
		if a.CloseIntentID != "" || a.ResourceRevision < 1 || a.Current.IsZero() {
			return &corev1.Error{Code: corev1.InvariantViolation, Detail: "Active requires a holding, an open gate and approved resource revision"}
		}
	case corev1.AllocationClosing, corev1.AllocationClosed:
		if err := corev1.ValidateIdentifier("closeIntentId", a.CloseIntentID); err != nil {
			return &corev1.Error{Code: corev1.InvariantViolation, Detail: "closed gate requires a persistent close intent", Cause: err}
		}
		if a.State == corev1.AllocationClosed && !a.Current.IsZero() {
			return &corev1.Error{Code: corev1.InvariantViolation, Detail: "Closed must have zero holdings"}
		}
	default:
		return &corev1.Error{Code: corev1.InvariantViolation, Detail: "unknown allocation state"}
	}
	return nil
}

// MutationResult separates a historical commit from CURRENT entitlement.
type MutationResult struct {
	OperationID string
	State       corev1.OperationState
	Allocation  Allocation
	Replayed    bool
}

// Holding is the common accounting snapshot owned by one domain.
type Holding struct {
	Domain          corev1.Domain
	AccountID       string
	Allocation      Allocation
	IntentHash      string
	InitialQuantity corev1.Resources
	InitialRevision int64
	ChangePlanID    string
	ChangeRevision  int64
}

func (h Holding) Validate() error {
	if h.Domain != corev1.QuotaDomain && h.Domain != corev1.NodeDomain {
		return &corev1.Error{Code: corev1.InvariantViolation, Detail: "unknown holding domain"}
	}
	if err := corev1.ValidateIdentifier("accountId", h.AccountID); err != nil {
		return err
	}
	if err := h.Allocation.Validate(); err != nil {
		return err
	}
	if h.Domain == corev1.QuotaDomain && h.AccountID != h.Allocation.Owner.TenantUID {
		return &corev1.Error{Code: corev1.InvariantViolation, Detail: "holding account does not match tenant"}
	}
	if h.Domain == corev1.NodeDomain {
		if err := h.Allocation.Current.ValidateNode(); err != nil {
			return err
		}
	}
	if h.IntentHash == "" {
		if h.Allocation.State != corev1.AllocationClosing && h.Allocation.State != corev1.AllocationClosed {
			return &corev1.Error{Code: corev1.InvariantViolation, Detail: "only a close tombstone may lack an initial intent"}
		}
	} else if len(h.IntentHash) != 64 || h.InitialRevision < 1 || h.InitialQuantity.IsZero() {
		return &corev1.Error{Code: corev1.InvariantViolation, Detail: "invalid initial intent snapshot"}
	}
	if err := h.InitialQuantity.Validate(); err != nil {
		return err
	}
	if h.Domain == corev1.NodeDomain {
		if err := h.InitialQuantity.ValidateNode(); err != nil {
			return err
		}
	}
	return h.validateChangePlan()
}

func (h Holding) validateChangePlan() error {
	if h.ChangePlanID == "" {
		if h.ChangeRevision != 0 {
			return &corev1.Error{Code: corev1.InvariantViolation, Detail: "change revision has no plan"}
		}
		return nil
	}
	if err := corev1.ValidateIdentifier("changePlanId", h.ChangePlanID); err != nil {
		return err
	}
	if (h.Allocation.State != corev1.AllocationActive && h.Allocation.State != corev1.AllocationClosing) || h.Allocation.ResourceRevision == math.MaxInt64 || h.ChangeRevision != h.Allocation.ResourceRevision+1 {
		return &corev1.Error{Code: corev1.InvariantViolation, Detail: "inconsistent pending resource change"}
	}
	return nil
}

// Mutation is a pure accounting proposal, not a committed result.
type Mutation struct {
	Holding  Holding
	Increase corev1.Resources
	Decrease corev1.Resources
	Changed  bool
}

// PlanMutation contains only shared resource-accounting rules.
func PlanMutation(h Holding, c Command) (Mutation, error) {
	fail := func(code corev1.Code, detail string) (Mutation, error) {
		return Mutation{}, &corev1.Error{Code: code, OperationID: c.OperationID, Detail: detail}
	}
	if err := h.Validate(); err != nil {
		return Mutation{}, err
	}
	if err := c.Validate(); err != nil {
		return Mutation{}, err
	}
	if c.Domain != h.Domain || c.AccountID != h.AccountID || c.AllocationID != h.Allocation.ID || c.Owner != h.Allocation.Owner {
		return fail(corev1.IdentityConflict, "command does not own this holding")
	}
	a := h.Allocation
	next, err := NextState(a.State, c.Kind)
	if err != nil {
		return Mutation{}, err
	}
	funding := c.Kind == corev1.Grant || c.Kind == corev1.Increase || c.Kind == corev1.SettleDecrease
	if funding && a.Version != c.ExpectedVersion {
		return fail(corev1.RevisionChanged, "allocation version differs from the immutable command")
	}
	m, a, err := applyCommandToAllocation(h, a, c, next)
	if err != nil {
		return Mutation{}, err
	}
	if m.Changed {
		if a.Version == math.MaxInt64 {
			return fail(corev1.InvariantViolation, "allocation version overflow")
		}
		a.Version++
	}
	a.State = next
	m.Holding.Allocation = a
	if err := m.Holding.Validate(); err != nil {
		return Mutation{}, err
	}
	return m, nil
}

func applyCommandToAllocation(h Holding, a Allocation, c Command, next corev1.AllocationState) (Mutation, Allocation, error) {
	fail := func(code corev1.Code, detail string) (Mutation, Allocation, error) {
		return Mutation{}, a, &corev1.Error{Code: code, OperationID: c.OperationID, Detail: detail}
	}
	m := Mutation{Holding: h, Changed: true}
	var err error
	switch c.Kind {
	case corev1.Grant:
		if h.IntentHash == "" || c.Quantity != h.InitialQuantity || c.ResourceRevision != h.InitialRevision {
			return fail(corev1.IdentityConflict, "grant differs from the frozen initial intent")
		}
		a.Current, a.ResourceRevision = c.Quantity, c.ResourceRevision
		m.Increase = c.Quantity
	case corev1.Increase:
		if h.ChangePlanID != "" || a.ResourceRevision == math.MaxInt64 || c.ResourceRevision != a.ResourceRevision+1 {
			return fail(corev1.RevisionChanged, "another resource plan is pending or target revision is stale")
		}
		a.Current, err = a.Current.Add(c.Quantity)
		m.Increase = c.Quantity
		m.Holding.ChangePlanID, m.Holding.ChangeRevision = c.ChangePlanID, c.ResourceRevision
	case corev1.SettleDecrease:
		if a.ResourceRevision == math.MaxInt64 || c.ResourceRevision != a.ResourceRevision+1 {
			return fail(corev1.RevisionChanged, "settlement must advance the resource revision once")
		}
		if h.ChangePlanID != "" && (h.ChangePlanID != c.ChangePlanID || h.ChangeRevision != c.ResourceRevision) {
			return fail(corev1.RevisionChanged, "settlement belongs to another resource plan")
		}
		a.Current, err = a.Current.Subtract(c.Quantity)
		a.ResourceRevision = c.ResourceRevision
		m.Decrease = c.Quantity
		m.Holding.ChangePlanID, m.Holding.ChangeRevision = "", 0
	case corev1.RequestClose:
		if a.CloseIntentID != "" && a.CloseIntentID != c.CloseIntentID {
			return fail(corev1.IdentityConflict, "holding already belongs to another close intent")
		}
		m.Changed = a.State != next
		a.CloseIntentID = c.CloseIntentID
	case corev1.FinalizeClose:
		if a.CloseIntentID != c.CloseIntentID {
			return fail(corev1.IdentityConflict, "cleanup refers to another close intent")
		}
		m.Changed = a.State != corev1.AllocationClosed
		m.Decrease, a.Current = a.Current, corev1.Resources{}
		m.Holding.ChangePlanID, m.Holding.ChangeRevision = "", 0
	}
	return m, a, err
}

// ObservationRef is an identity check for runtime observation matching.
type ObservationRef struct {
	InstanceUID      string
	Binding          Binding
	ResourceRevision int64
}

func (o ObservationRef) Matches(owner Owner, binding Binding, revision int64) bool {
	return owner.Validate() == nil && binding.Validate() == nil && revision > 0 &&
		o.InstanceUID == owner.InstanceUID && o.Binding == binding && o.ResourceRevision == revision
}
