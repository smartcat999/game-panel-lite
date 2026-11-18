package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"gorm.io/gorm"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/network"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

var ErrNotFound = errors.New("node record not found")

type Account struct {
	NodeUID       string
	PolicyVersion int64
	Limits        corev1.Limits
	Allocated     corev1.Resources
}

type Change struct {
	Plan          pgutil.ResourceChange
	SourceVersion int64
}

type Result struct {
	pgutil.MutationResult
	Binding       pgutil.Binding
	AssignedPorts []corev1.PortRequest
}

type Operation struct {
	Command   pgutil.Command
	State     corev1.OperationState
	Result    Result
	HasResult bool
}

type (
	NodeAccountRecord     = corev1.NodeAccountRecord
	NodeReservationRecord = corev1.NodeReservationRecord
	NodeOperationRecord   = corev1.NodeOperationRecord
	NodeChangeRecord      = corev1.NodeChangeRecord
	NodePortRecord        = corev1.NodePortRecord
)

// Reservation is CURRENT state. Binding is zero for a close-before-open tombstone;
// such a tombstone never invents a placement epoch or execution authorization.
type Reservation struct {
	pgutil.Holding
	Binding       pgutil.Binding
	Ports         []corev1.PortRequest
	AssignedPorts []corev1.PortRequest
}

type Options struct {
	DynamicPortMin, DynamicPortMax int32
	MaxPortProbes                  int
}

func defaultOptions() Options {
	pr := network.DefaultPortRange()
	return Options{DynamicPortMin: pr.Min, DynamicPortMax: pr.Max, MaxPortProbes: pr.MaxProbes}
}
func (o Options) portRange() network.PortRange {
	return network.PortRange{Min: o.DynamicPortMin, Max: o.DynamicPortMax, MaxProbes: o.MaxPortProbes}
}
func (o Options) validate() error {
	if err := o.portRange().Validate(); err != nil {
		return &corev1.ValidationError{Field: "options", Reason: corev1.ValidationReasonInvalidPortOptions}
	}
	return nil
}

func failure(code corev1.Code, id, detail string, cause error) error {
	return &corev1.Error{Code: code, OperationID: id, Detail: detail, Cause: cause}
}
func validateLimits(l corev1.Limits) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if l.Backups != (corev1.Limit{}) || l.BackupStorage != (corev1.Limit{}) {
		return &corev1.ValidationError{Field: "limits", Reason: corev1.ValidationReasonNodeBackupLimits}
	}
	return nil
}
func validateCommand(c pgutil.Command, kind corev1.Kind) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Domain != corev1.NodeDomain || kind != "" && c.Kind != kind {
		return failure(corev1.IdentityConflict, c.OperationID, "wrong domain or method for command", nil)
	}
	return nil
}
func funding(k corev1.Kind) bool {
	return k == corev1.Grant || k == corev1.Increase || k == corev1.SettleDecrease
}

func (r Reservation) Validate() error {
	if err := r.Holding.Validate(); err != nil {
		return err
	}
	if r.IntentHash == "" {
		if r.Binding != (pgutil.Binding{}) || len(r.Ports) != 0 || len(r.AssignedPorts) != 0 {
			return failure(corev1.InvariantViolation, "", "tombstone has fabricated placement", nil)
		}
		return nil
	}
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if r.Binding.NodeUID != r.AccountID || r.Binding.AttemptID != r.Allocation.ID {
		return failure(corev1.InvariantViolation, "", "binding does not match reservation", nil)
	}
	p := pgutil.PlacementIntent{Owner: r.Allocation.Owner, Binding: r.Binding, ResourceRevision: r.InitialRevision, Quantity: r.InitialQuantity, Ports: r.Ports}
	canonical, err := p.Canonical()
	if err != nil || !slices.Equal(canonical.Ports, r.Ports) {
		return failure(corev1.InvariantViolation, "", "persisted port requests are not canonical", err)
	}
	hash, err := p.Fingerprint()
	if err != nil || hash != r.IntentHash {
		return failure(corev1.InvariantViolation, "", "invalid immutable placement", err)
	}
	if r.Allocation.State == corev1.AllocationPending || r.Allocation.State == corev1.AllocationClosed || r.Allocation.State == corev1.AllocationClosing && r.Allocation.Current.IsZero() {
		if len(r.AssignedPorts) != 0 {
			return failure(corev1.InvariantViolation, "", "unfunded reservation owns ports", nil)
		}
		return nil
	}
	return validateAssignedPorts(r)
}

func validateAssignedPorts(r Reservation) error {
	if len(r.Ports) != len(r.AssignedPorts) {
		return failure(corev1.InvariantViolation, "", "assigned ports do not match request", nil)
	}
	seen := map[portKey]bool{}
	for n, p := range r.AssignedPorts {
		request := r.Ports[n]
		if p.PublishedPort < 1 || p.PublishedPort > 65535 || p.Name != request.Name || p.Protocol != request.Protocol || p.TargetPort != request.TargetPort || request.PublishedPort != 0 && request.PublishedPort != p.PublishedPort {
			return failure(corev1.InvariantViolation, "", "invalid assigned port", nil)
		}
		k := portKey{r.AccountID, p.Protocol, p.PublishedPort}
		if seen[k] {
			return failure(corev1.InvariantViolation, "", "duplicate assigned port", nil)
		}
		seen[k] = true
	}
	return nil
}

func cloneReservation(r Reservation) Reservation {
	r.Ports = slices.Clone(r.Ports)
	r.AssignedPorts = slices.Clone(r.AssignedPorts)
	return r
}
func currentResult(op Operation, r Reservation, replayed bool) Result {
	return Result{MutationResult: pgutil.MutationResult{OperationID: op.Command.OperationID, State: op.State, Allocation: r.Allocation, Replayed: replayed}, Binding: r.Binding, AssignedPorts: slices.Clone(r.AssignedPorts)}
}
func matchIntent(r Reservation, i pgutil.PlacementIntent, hash string) error {
	if r.AccountID != i.Binding.NodeUID || r.Allocation.Owner != i.Owner {
		return failure(corev1.IdentityConflict, "", "reservation belongs to another owner or node", nil)
	}
	if r.Allocation.State == corev1.AllocationClosing || r.Allocation.State == corev1.AllocationClosed {
		return failure(corev1.Closed, "", "reservation admission gate is closed", nil)
	}
	if r.IntentHash != hash {
		return failure(corev1.IdentityConflict, "", "initial placement is immutable", nil)
	}
	return nil
}
func replay(op Operation, r Reservation) (Result, error) {
	out := currentResult(op, r, true)
	if op.State == corev1.Cancelled || funding(op.Command.Kind) && (r.Allocation.State == corev1.AllocationClosing || r.Allocation.State == corev1.AllocationClosed) {
		return out, failure(corev1.Closed, op.Command.OperationID, "historical result does not grant current entitlement", nil)
	}
	return out, nil
}

func notFound(kind, id string) error { return fmt.Errorf("%w: %s %s", ErrNotFound, kind, id) }
func isClosed(err error) bool        { return errors.Is(err, corev1.Closed) }

type portKey struct {
	NodeUID, Protocol string
	Port              int32
}

func checkContext(ctx context.Context, op string) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return failure(corev1.OutcomeUnknown, op, "context ended before node outcome was known", ctx.Err())
}

func checkRegistration(r Reservation, c pgutil.Command) error {
	if r.AccountID != c.AccountID || r.Allocation.Owner != c.Owner {
		return failure(corev1.IdentityConflict, c.OperationID, "command owner or account does not match reservation", nil)
	}
	if r.Allocation.State == corev1.AllocationClosing || r.Allocation.State == corev1.AllocationClosed {
		return failure(corev1.Closed, c.OperationID, "reservation admission gate is closed", nil)
	}
	if c.Kind == corev1.Grant {
		if r.Allocation.State != corev1.AllocationPending {
			return failure(corev1.RevisionChanged, c.OperationID, "initial reserve requires a Pending reservation", nil)
		}
		if c.Quantity != r.InitialQuantity || c.ResourceRevision != r.InitialRevision {
			return failure(corev1.IdentityConflict, c.OperationID, "reserve command does not match placement intent", nil)
		}
	} else if r.Allocation.State != corev1.AllocationActive {
		return failure(corev1.RevisionChanged, c.OperationID, "resource change requires an Active reservation", nil)
	}
	if c.ExpectedVersion != r.Allocation.Version {
		return failure(corev1.RevisionChanged, c.OperationID, "reservation version changed", nil)
	}
	return nil
}

func toAccount(rec NodeAccountRecord) (Account, error) {
	if rec.PolicyVersion < 0 {
		return Account{}, failure(corev1.InvariantViolation, "", "invalid persisted capacity policy version", nil)
	}
	a := Account{
		NodeUID:       rec.NodeUID,
		PolicyVersion: rec.PolicyVersion,
		Limits: pgutil.Limits([6]*int64{
			rec.LimitCPU, rec.LimitMem, rec.LimitStorage,
			rec.LimitInstances, rec.LimitBackups, rec.LimitBackupStorage,
		}),
		Allocated: corev1.Resources{
			CPUMillicores:      rec.AllocatedCPU,
			MemoryBytes:        rec.AllocatedMem,
			StorageBytes:       rec.AllocatedStorage,
			InstanceCount:      rec.AllocatedInstances,
			BackupsCount:       rec.AllocatedBackups,
			BackupStorageBytes: rec.AllocatedBackupStorage,
		},
	}
	if err := validateLimits(a.Limits); err != nil {
		return a, failure(corev1.InvariantViolation, "", "invalid persisted limits", err)
	}
	if err := a.Allocated.ValidateNode(); err != nil {
		return a, failure(corev1.InvariantViolation, "", "invalid persisted account", err)
	}
	return a, nil
}

func toHolding(rec NodeReservationRecord) (Reservation, error) {
	h := Reservation{
		Holding: pgutil.Holding{
			Domain:         corev1.NodeDomain,
			AccountID:      rec.NodeUID,
			IntentHash:     rec.IntentHash,
			ChangePlanID:   rec.ChangePlanID,
			ChangeRevision: rec.ChangeRevision,
			Allocation: pgutil.Allocation{
				ID: rec.AllocationID,
				Owner: pgutil.Owner{
					TenantUID:    rec.TenantUID,
					InstanceUID:  rec.InstanceUID,
					InstanceName: rec.InstanceName,
				},
				State: rec.State,
				Current: corev1.Resources{
					CPUMillicores:      rec.CurrentCPU,
					MemoryBytes:        rec.CurrentMem,
					StorageBytes:       rec.CurrentStorage,
					InstanceCount:      rec.CurrentInstances,
					BackupsCount:       rec.CurrentBackups,
					BackupStorageBytes: rec.CurrentBackupStorage,
				},
				Version:          rec.Version,
				ResourceRevision: rec.ResourceRevision,
				CloseIntentID:    rec.CloseIntentID,
			},
		},
	}
	if err := json.Unmarshal(rec.AssignedPorts, &h.AssignedPorts); err != nil {
		return h, failure(corev1.InvariantViolation, "", "invalid assigned ports", err)
	}
	if len(rec.InitialIntent) > 0 {
		var intent pgutil.PlacementIntent
		if err := json.Unmarshal(rec.InitialIntent, &intent); err != nil {
			return h, failure(corev1.InvariantViolation, "", "invalid persisted intent", err)
		}
		hash, err := intent.Fingerprint()
		if err != nil || hash != h.IntentHash || intent.Binding.AttemptID != h.Allocation.ID || intent.Binding.NodeUID != h.AccountID || intent.Owner != h.Allocation.Owner {
			return h, failure(corev1.InvariantViolation, "", "intent identity or digest is corrupt", err)
		}
		h.InitialQuantity, h.InitialRevision = intent.Quantity, intent.ResourceRevision
		h.Binding, h.Ports = intent.Binding, intent.Ports
	}
	return h, h.Validate()
}

func toOperation(rec NodeOperationRecord) (Operation, error) {
	op := Operation{State: rec.State}
	if err := json.Unmarshal(rec.Command, &op.Command); err != nil {
		return op, failure(corev1.InvariantViolation, rec.OperationID, "invalid persisted command", err)
	}
	actual, err := op.Command.Fingerprint()
	if err != nil || actual != rec.RequestHash || op.Command.Domain != corev1.NodeDomain ||
		op.Command.OperationID != rec.OperationID || op.Command.AllocationID != rec.AllocationID ||
		op.Command.Kind != rec.Kind || op.Command.ChangePlanID != rec.ChangePlanID || op.Command.CloseIntentID != rec.CloseIntentID {
		return op, failure(corev1.InvariantViolation, rec.OperationID, "command identity or digest is corrupt", err)
	}
	if err := pgutil.ValidateOperationTransition(op.State, op.State); err != nil {
		return op, err
	}
	if len(rec.Result) > 0 {
		if err := json.Unmarshal(rec.Result, &op.Result); err != nil {
			return op, failure(corev1.InvariantViolation, rec.OperationID, "invalid historical result", err)
		}
		op.HasResult = true
		if err := validateHistorical(op); err != nil {
			return op, err
		}
	}
	if op.HasResult != (op.State == corev1.Committed) {
		return op, failure(corev1.InvariantViolation, rec.OperationID, "result and operation state disagree", nil)
	}
	return op, nil
}

func getAccountRow(tx *gorm.DB, id string) (Account, error) {
	var rec NodeAccountRecord
	if err := tx.Where("node_uid = ?", id).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Account{}, notFound("account", id)
		}
		return Account{}, pgutil.Error(err, "")
	}
	return toAccount(rec)
}

func increaseAccountGORM(tx *gorm.DB, nodeUID string, inc corev1.Resources) *gorm.DB {
	return tx.Model(&NodeAccountRecord{}).
		Where(`node_uid = ?
 AND allocated_cpu <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_cpu IS NULL OR allocated_cpu <= limit_cpu - ?::bigint)
 AND allocated_mem <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_mem IS NULL OR allocated_mem <= limit_mem - ?::bigint)
 AND allocated_storage <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_storage IS NULL OR allocated_storage <= limit_storage - ?::bigint)
 AND allocated_instances <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_instances IS NULL OR allocated_instances <= limit_instances - ?::bigint)
 AND allocated_backups <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_backups IS NULL OR allocated_backups <= limit_backups - ?::bigint)
 AND allocated_backup_storage <= 9223372036854775807 - ?::bigint AND (?::bigint = 0 OR limit_backup_storage IS NULL OR allocated_backup_storage <= limit_backup_storage - ?::bigint)`,
			nodeUID,
			inc.CPUMillicores, inc.CPUMillicores, inc.CPUMillicores,
			inc.MemoryBytes, inc.MemoryBytes, inc.MemoryBytes,
			inc.StorageBytes, inc.StorageBytes, inc.StorageBytes,
			inc.InstanceCount, inc.InstanceCount, inc.InstanceCount,
			inc.BackupsCount, inc.BackupsCount, inc.BackupsCount,
			inc.BackupStorageBytes, inc.BackupStorageBytes, inc.BackupStorageBytes,
		).
		Updates(map[string]any{
			"allocated_cpu":            gorm.Expr("allocated_cpu + ?", inc.CPUMillicores),
			"allocated_mem":            gorm.Expr("allocated_mem + ?", inc.MemoryBytes),
			"allocated_storage":        gorm.Expr("allocated_storage + ?", inc.StorageBytes),
			"allocated_instances":      gorm.Expr("allocated_instances + ?", inc.InstanceCount),
			"allocated_backups":        gorm.Expr("allocated_backups + ?", inc.BackupsCount),
			"allocated_backup_storage": gorm.Expr("allocated_backup_storage + ?", inc.BackupStorageBytes),
			"updated_at":               gorm.Expr("NOW()"),
		})
}

func decreaseAccountGORM(tx *gorm.DB, nodeUID string, dec corev1.Resources) *gorm.DB {
	return tx.Model(&NodeAccountRecord{}).
		Where(`node_uid = ? AND allocated_cpu >= ? AND allocated_mem >= ? AND allocated_storage >= ?
 AND allocated_instances >= ? AND allocated_backups >= ? AND allocated_backup_storage >= ?`,
			nodeUID,
			dec.CPUMillicores, dec.MemoryBytes, dec.StorageBytes,
			dec.InstanceCount, dec.BackupsCount, dec.BackupStorageBytes,
		).
		Updates(map[string]any{
			"allocated_cpu":            gorm.Expr("allocated_cpu - ?", dec.CPUMillicores),
			"allocated_mem":            gorm.Expr("allocated_mem - ?", dec.MemoryBytes),
			"allocated_storage":        gorm.Expr("allocated_storage - ?", dec.StorageBytes),
			"allocated_instances":      gorm.Expr("allocated_instances - ?", dec.InstanceCount),
			"allocated_backups":        gorm.Expr("allocated_backups - ?", dec.BackupsCount),
			"allocated_backup_storage": gorm.Expr("allocated_backup_storage - ?", dec.BackupStorageBytes),
			"updated_at":               gorm.Expr("NOW()"),
		})
}

func updateHoldingGORM(tx *gorm.DB, expectedVersion int64, next pgutil.Holding) *gorm.DB {
	a := next.Allocation
	updates := map[string]any{
		"state":                  a.State,
		"current_cpu":            a.Current.CPUMillicores,
		"current_mem":            a.Current.MemoryBytes,
		"current_storage":        a.Current.StorageBytes,
		"current_instances":      a.Current.InstanceCount,
		"current_backups":        a.Current.BackupsCount,
		"current_backup_storage": a.Current.BackupStorageBytes,
		"version":                a.Version,
		"resource_revision":      a.ResourceRevision,
		"close_intent_id":        a.CloseIntentID,
		"change_plan_id":         next.ChangePlanID,
		"change_revision":        next.ChangeRevision,
		"updated_at":             gorm.Expr("NOW()"),
	}
	if a.State == corev1.AllocationClosed {
		updates["assigned_ports"] = gorm.Expr("'[]'::jsonb")
	}
	return tx.Model(&NodeReservationRecord{}).
		Where("allocation_id = ? AND version = ?", a.ID, expectedVersion).
		Updates(updates)
}

// MaxDynamicPortProbes caps SQL round trips per dynamic request.
const MaxDynamicPortProbes = network.MaxDynamicPortProbes

func assignPorts(options Options, attemptID string, requests []corev1.PortRequest, claim func(corev1.PortRequest, int32) (bool, error)) ([]corev1.PortRequest, error) {
	return network.AssignPortsInRange(options.portRange(), attemptID, requests, claim)
}

func validateHistorical(op Operation) error {
	c, r := op.Command, op.Result
	fail := func(detail string, cause error) error {
		return failure(corev1.InvariantViolation, c.OperationID, detail, cause)
	}
	if err := r.Allocation.Validate(); err != nil {
		return fail("invalid historical allocation", err)
	}
	if err := r.Allocation.Current.ValidateNode(); err != nil {
		return fail("invalid historical node budget", err)
	}
	if r.OperationID != c.OperationID || r.State != corev1.Committed || r.Replayed || r.Allocation.ID != c.AllocationID || r.Allocation.Owner != c.Owner {
		return fail("historical result identity is corrupt", nil)
	}
	if r.Binding == (pgutil.Binding{}) {
		if funding(c.Kind) || !r.Allocation.Current.IsZero() || len(r.AssignedPorts) != 0 {
			return fail("historical result lacks its placement binding", nil)
		}
	} else {
		if err := r.Binding.Validate(); err != nil {
			return fail("invalid historical binding", err)
		}
		if r.Binding.NodeUID != c.AccountID || r.Binding.AttemptID != c.AllocationID {
			return fail("historical binding belongs to another attempt", nil)
		}
	}
	switch c.Kind {
	case corev1.Grant, corev1.Increase, corev1.SettleDecrease:
		if r.Allocation.State != corev1.AllocationActive {
			return fail("funding result has no active historical holding", nil)
		}
	case corev1.RequestClose:
		if r.Allocation.State != corev1.AllocationClosing && r.Allocation.State != corev1.AllocationClosed {
			return fail("historical close did not close admission", nil)
		}
	case corev1.FinalizeClose:
		if r.Allocation.State != corev1.AllocationClosed {
			return fail("historical finalization is not closed", nil)
		}
	}
	if r.Allocation.Current.IsZero() && len(r.AssignedPorts) != 0 {
		return fail("historical zero holding has port claims", nil)
	}
	if _, err := corev1.CanonicalPorts(r.AssignedPorts); err != nil {
		return fail("invalid historical port claims", err)
	}
	for _, p := range r.AssignedPorts {
		if p.PublishedPort < 1 || p.Protocol != corev1.ProtocolTCP && p.Protocol != corev1.ProtocolUDP {
			return fail("non-concrete historical port claim", nil)
		}
	}
	return nil
}
