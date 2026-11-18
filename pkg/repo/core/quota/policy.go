package quota

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// ErrPolicySyncPending indicates the quota ledger has not caught up with the
// tenant's desired resourceVersion yet; callers should wake Tenant reconciliation and retry.
var ErrPolicySyncPending = errors.New("quota policy sync pending")

const maxPolicySyncPasses = 2

func invalidPolicyField(field, value, reason string) error {
	return &corev1.ValidationError{Field: field, Value: value, Reason: reason}
}

// QuotaPolicy captures effective tenant limits and optional expiration timestamp.
type QuotaPolicy struct {
	Limits    corev1.Limits `json:"limits"`
	ExpiresAt *time.Time    `json:"expiresAt,omitempty"`
}

// Normalize returns a canonical copy truncated to PostgreSQL TIMESTAMPTZ microsecond precision.
func (p QuotaPolicy) Normalize() QuotaPolicy {
	out := QuotaPolicy{Limits: p.Limits}
	if p.ExpiresAt != nil {
		t := p.ExpiresAt.UTC().Truncate(time.Microsecond)
		out.ExpiresAt = &t
	}
	return out
}

// Validate verifies finite limits invariants.
func (p QuotaPolicy) Validate() error {
	return p.Limits.Validate()
}

// Equal reports whether two normalized quota policies are identical.
func (p QuotaPolicy) Equal(other QuotaPolicy) bool {
	a, b := p.Normalize(), other.Normalize()
	if a.Limits != b.Limits {
		return false
	}
	if a.ExpiresAt == nil || b.ExpiresAt == nil {
		return a.ExpiresAt == nil && b.ExpiresAt == nil
	}
	return a.ExpiresAt.Equal(*b.ExpiresAt)
}

// QuotaPolicyChangePlan is the immutable JSON payload stored in quota_policy_changes.plan.
type QuotaPolicyChangePlan struct {
	ChangeID              string                       `json:"changeID"`
	TenantUID             string                       `json:"tenantUID"`
	Kind                  corev1.QuotaPolicyChangeKind `json:"kind"`
	FromVersion           int64                        `json:"fromVersion"`
	ToVersion             int64                        `json:"toVersion"`
	TenantResourceVersion int64                        `json:"tenantResourceVersion"`
	Old                   QuotaPolicy                  `json:"old"`
	Target                QuotaPolicy                  `json:"target"`
}

func validPolicyChangeKind(kind corev1.QuotaPolicyChangeKind) bool {
	switch kind {
	case corev1.QuotaPolicyChangeKindBootstrap,
		corev1.QuotaPolicyChangeKindAdminAdjust,
		corev1.QuotaPolicyChangeKindExpiry:
		return true
	default:
		return false
	}
}

// Validate checks structural invariants of an immutable quota policy change plan.
func (p QuotaPolicyChangePlan) Validate() error {
	if err := corev1.ValidateIdentifier("changeID", p.ChangeID); err != nil {
		return err
	}
	if err := corev1.ValidateIdentifier("tenantUID", p.TenantUID); err != nil {
		return err
	}
	if !validPolicyChangeKind(p.Kind) {
		return invalidPolicyField("kind", string(p.Kind), "unknown quota policy change kind")
	}
	if p.FromVersion < 0 || p.ToVersion != p.FromVersion+1 {
		return invalidPolicyField("toVersion", "", "must equal fromVersion + 1")
	}
	if p.TenantResourceVersion < 1 {
		return invalidPolicyField("tenantResourceVersion", "", "must be positive")
	}
	if p.FromVersion == 0 && (p.Kind != corev1.QuotaPolicyChangeKindBootstrap || !p.Old.Equal(QuotaPolicy{})) {
		return invalidPolicyField("old", "", "initial policy change must use Bootstrap kind with empty old snapshot")
	}
	if err := p.Old.Validate(); err != nil {
		return err
	}
	return p.Target.Validate()
}

// Fingerprint computes the immutable SHA-256 digest of this change plan.
func (p QuotaPolicyChangePlan) Fingerprint() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	canonical := p
	canonical.Old = p.Old.Normalize()
	canonical.Target = p.Target.Normalize()
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("resource-control/quota-policy-change/1\n"), raw...))
	return hex.EncodeToString(sum[:]), nil
}

// PolicyChange represents one persisted record in quota_policy_changes.
type PolicyChange struct {
	Plan          QuotaPolicyChangePlan
	SourceVersion int64
	ExportedAt    *time.Time
	CreatedAt     time.Time
}

// PolicySync requests reconciling quota_accounts to the target policy at an upstream Tenant revision.
type PolicySync struct {
	TenantUID       string
	UpstreamVersion int64
	Target          QuotaPolicy
	Kind            corev1.QuotaPolicyChangeKind
}

// SyncPolicy atomically records and posts an effective quota policy change without cross-table transactions.
func (s *DB) SyncPolicy(ctx context.Context, in PolicySync) (Account, *PolicyChange, error) {
	norm, err := validatePolicySync(in)
	if err != nil {
		return Account{}, nil, err
	}
	for range maxPolicySyncPasses {
		acct, chg, done, passErr := s.syncPolicyPass(ctx, norm)
		if passErr != nil {
			return Account{}, nil, passErr
		}
		if done {
			return acct, chg, nil
		}
	}
	return Account{}, nil, failure(corev1.RevisionChanged, "", "concurrent policy changes exceeded retry budget", nil)
}

func validatePolicySync(in PolicySync) (PolicySync, error) {
	if err := corev1.ValidateIdentifier("tenantUid", in.TenantUID); err != nil {
		return PolicySync{}, err
	}
	if in.UpstreamVersion < 1 {
		return PolicySync{}, invalidPolicyField("upstreamVersion", "", "must be positive")
	}
	in.Target = in.Target.Normalize()
	if err := in.Target.Validate(); err != nil {
		return PolicySync{}, err
	}
	if in.Kind == "" {
		in.Kind = corev1.QuotaPolicyChangeKindAdminAdjust
	}
	if !validPolicyChangeKind(in.Kind) {
		return PolicySync{}, invalidPolicyField("kind", string(in.Kind), "unknown quota policy change kind")
	}
	return in, nil
}

func (s *DB) syncPolicyPass(ctx context.Context, in PolicySync) (Account, *PolicyChange, bool, error) {
	acct, err := s.ensureAndGetAccount(ctx, in.TenantUID)
	if err != nil {
		return Account{}, nil, false, err
	}
	pending, err := s.getPolicyChangeBySource(ctx, in.TenantUID, acct.PolicyVersion)
	if err == nil {
		updated, applyErr := s.applyPolicyChange(ctx, pending)
		if applyErr != nil {
			return Account{}, nil, false, applyErr
		}
		return finishPendingRecovery(updated, pending, in)
	}
	if !errors.Is(err, ErrNotFound) {
		return Account{}, nil, false, err
	}
	if in.UpstreamVersion < acct.UpstreamVersion {
		return Account{}, nil, false, failure(corev1.RevisionChanged, "", "a newer upstream tenant version is already applied", nil)
	}
	if acct.PolicyVersion > 0 && in.UpstreamVersion == acct.UpstreamVersion && !acct.Policy().Equal(in.Target) {
		return Account{}, nil, false, failure(corev1.IdentityConflict, "", "same upstream version has different quota policy", nil)
	}
	if acct.PolicyVersion > 0 && acct.Policy().Equal(in.Target) {
		return s.finishUnchangedPolicy(ctx, acct, in.UpstreamVersion)
	}
	return s.recordAndApplyPolicy(ctx, acct, in)
}

func finishPendingRecovery(updated Account, recovered PolicyChange, in PolicySync) (Account, *PolicyChange, bool, error) {
	if in.UpstreamVersion < updated.UpstreamVersion {
		return Account{}, nil, false, failure(corev1.RevisionChanged, "", "a newer upstream tenant version is already applied", nil)
	}
	if in.UpstreamVersion == updated.UpstreamVersion && !updated.Policy().Equal(in.Target) {
		return Account{}, nil, false, failure(corev1.IdentityConflict, "", "same upstream version has different quota policy", nil)
	}
	if updated.Policy().Equal(in.Target) && updated.UpstreamVersion >= in.UpstreamVersion {
		return updated, &recovered, true, nil
	}
	return updated, nil, false, nil
}

func (s *DB) finishUnchangedPolicy(ctx context.Context, acct Account, upstreamVersion int64) (Account, *PolicyChange, bool, error) {
	if acct.UpstreamVersion >= upstreamVersion {
		return acct, nil, true, nil
	}
	res := s.orm.WithContext(ctx).Model(&QuotaAccountRecord{}).
		Where("tenant_uid = ? AND upstream_version < ?", acct.TenantUID, upstreamVersion).
		Updates(map[string]any{"upstream_version": upstreamVersion, "updated_at": gorm.Expr("NOW()")})
	if res.Error != nil {
		return Account{}, nil, false, pgutil.WriteError(res.Error, "")
	}
	updated, err := s.GetAccount(ctx, acct.TenantUID)
	if err != nil {
		return Account{}, nil, false, err
	}
	if upstreamVersion < updated.UpstreamVersion {
		return Account{}, nil, false, failure(corev1.RevisionChanged, "", "a newer upstream tenant version is already applied", nil)
	}
	return updated, nil, true, nil
}

func (s *DB) recordAndApplyPolicy(ctx context.Context, acct Account, in PolicySync) (Account, *PolicyChange, bool, error) {
	change, matched, err := s.insertPolicyChange(ctx, acct, in)
	if err != nil {
		return Account{}, nil, false, err
	}
	updated, err := s.applyPolicyChange(ctx, change)
	if err != nil {
		return Account{}, nil, false, err
	}
	if !matched {
		return finishPendingRecovery(updated, change, in)
	}
	return updated, &change, true, nil
}

func (s *DB) ensureAndGetAccount(ctx context.Context, tenantUID string) (Account, error) {
	acct := QuotaAccountRecord{TenantUID: tenantUID}
	if err := s.orm.WithContext(ctx).Select("tenant_uid").Clauses(clause.OnConflict{DoNothing: true}).Create(&acct).Error; err != nil {
		return Account{}, pgutil.WriteError(err, "")
	}
	return s.GetAccount(ctx, tenantUID)
}

func (s *DB) getPolicyChangeBySource(ctx context.Context, tenantUID string, sourceVersion int64) (PolicyChange, error) {
	var rec QuotaPolicyChangeRecord
	if err := s.orm.WithContext(ctx).Where("tenant_uid = ? AND source_version = ?", tenantUID, sourceVersion).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return PolicyChange{}, notFound("quota policy change", fmt.Sprintf("%s@%d", tenantUID, sourceVersion))
		}
		return PolicyChange{}, pgutil.Error(err, "")
	}
	return toPolicyChange(rec)
}

func (s *DB) insertPolicyChange(ctx context.Context, acct Account, in PolicySync) (PolicyChange, bool, error) {
	kind, oldPolicy := in.Kind, acct.Policy()
	if acct.PolicyVersion == 0 {
		kind = corev1.QuotaPolicyChangeKindBootstrap
		oldPolicy = QuotaPolicy{}
	}
	plan := QuotaPolicyChangePlan{
		ChangeID:              fmt.Sprintf("policy:%s:%d", in.TenantUID, acct.PolicyVersion),
		TenantUID:             in.TenantUID,
		Kind:                  kind,
		FromVersion:           acct.PolicyVersion,
		ToVersion:             acct.PolicyVersion + 1,
		TenantResourceVersion: in.UpstreamVersion,
		Old:                   oldPolicy,
		Target:                in.Target,
	}
	hash, err := plan.Fingerprint()
	if err != nil {
		return PolicyChange{}, false, err
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return PolicyChange{}, false, err
	}
	rec := QuotaPolicyChangeRecord{
		ChangeID: plan.ChangeID, TenantUID: plan.TenantUID, RequestHash: hash,
		Plan: raw, SourceVersion: acct.PolicyVersion,
	}
	err = s.orm.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_uid"}, {Name: "source_version"}},
			DoUpdates: clause.Assignments(map[string]any{"change_id": gorm.Expr("quota_policy_changes.change_id")}),
		},
		clause.Returning{},
	).Create(&rec).Error
	if err != nil {
		return PolicyChange{}, false, pgutil.WriteError(err, plan.ChangeID)
	}
	parsed, err := toPolicyChange(rec)
	if err != nil {
		return PolicyChange{}, false, err
	}
	return parsed, rec.RequestHash == hash, nil
}

func (s *DB) applyPolicyChange(ctx context.Context, chg PolicyChange) (Account, error) {
	p := chg.Plan
	l := p.Target.Limits
	res := s.orm.WithContext(ctx).Model(&QuotaAccountRecord{}).
		Where("tenant_uid = ? AND policy_version = ?", p.TenantUID, p.FromVersion).
		Updates(map[string]any{
			"policy_version":       p.ToVersion,
			"upstream_version":     gorm.Expr("GREATEST(upstream_version, ?)", p.TenantResourceVersion),
			"expires_at":           p.Target.ExpiresAt,
			"limit_cpu":            limitPtr(l.CPU),
			"limit_mem":            limitPtr(l.Memory),
			"limit_storage":        limitPtr(l.Storage),
			"limit_instances":      limitPtr(l.Instances),
			"limit_backups":        limitPtr(l.Backups),
			"limit_backup_storage": limitPtr(l.BackupStorage),
			"updated_at":           gorm.Expr("NOW()"),
		})
	if res.Error != nil {
		return Account{}, pgutil.WriteError(res.Error, p.ChangeID)
	}
	acct, err := s.GetAccount(ctx, p.TenantUID)
	if err != nil {
		return Account{}, err
	}
	if res.RowsAffected == 0 && acct.PolicyVersion < p.ToVersion {
		return Account{}, failure(corev1.RevisionChanged, p.ChangeID, "account policy version changed concurrently", nil)
	}
	return acct, nil
}

func toPolicyChange(rec QuotaPolicyChangeRecord) (PolicyChange, error) {
	chg := PolicyChange{SourceVersion: rec.SourceVersion, ExportedAt: rec.ExportedAt, CreatedAt: rec.CreatedAt}
	if err := json.Unmarshal(rec.Plan, &chg.Plan); err != nil {
		return chg, failure(corev1.InvariantViolation, rec.ChangeID, "invalid quota policy change plan", err)
	}
	actual, err := chg.Plan.Fingerprint()
	if err != nil || actual != rec.RequestHash || chg.Plan.ChangeID != rec.ChangeID ||
		chg.Plan.TenantUID != rec.TenantUID || chg.Plan.FromVersion != rec.SourceVersion {
		return chg, failure(corev1.InvariantViolation, rec.ChangeID, "quota policy change digest or identity is corrupt", err)
	}
	return chg, nil
}

// RequireAccountAt verifies the quota ledger has initialized and caught up to upstreamVersion
// (or matches expected policy when only non-quota fields changed on the Tenant).
func (s *DB) RequireAccountAt(ctx context.Context, tenantUID string, upstreamVersion int64, expected QuotaPolicy) (Account, error) {
	acct, err := s.GetAccount(ctx, tenantUID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Account{}, fmt.Errorf("%w: account for tenant %s does not exist", ErrPolicySyncPending, tenantUID)
		}
		return Account{}, err
	}
	if acct.PolicyVersion == 0 {
		return Account{}, fmt.Errorf("%w: account for tenant %s is not initialized", ErrPolicySyncPending, tenantUID)
	}
	if acct.UpstreamVersion < upstreamVersion && !acct.Policy().Equal(expected.Normalize()) {
		return acct, fmt.Errorf("%w: account upstreamVersion %d < tenant resourceVersion %d", ErrPolicySyncPending, acct.UpstreamVersion, upstreamVersion)
	}
	return acct, nil
}

// ListPolicyChanges returns committed ledger records strictly below the account's posted policy_version.
func (s *DB) ListPolicyChanges(ctx context.Context, tenantUID string, limit, offset int) ([]PolicyChange, int, int64, error) {
	acct, err := s.GetAccount(ctx, tenantUID)
	if errors.Is(err, ErrNotFound) || (err == nil && acct.PolicyVersion == 0) {
		return []PolicyChange{}, 0, 0, nil
	}
	if err != nil {
		return nil, 0, 0, err
	}
	if limit <= 0 {
		limit = 50
	} else if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	base := s.orm.WithContext(ctx).Model(&QuotaPolicyChangeRecord{}).
		Where("tenant_uid = ? AND source_version < ?", tenantUID, acct.PolicyVersion)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, 0, pgutil.Error(err, "")
	}
	var recs []QuotaPolicyChangeRecord
	if err := base.Order("source_version DESC").Offset(offset).Limit(limit).Find(&recs).Error; err != nil {
		return nil, 0, 0, pgutil.Error(err, "")
	}
	items := make([]PolicyChange, 0, len(recs))
	for _, rec := range recs {
		chg, err := toPolicyChange(rec)
		if err != nil {
			return nil, 0, 0, err
		}
		items = append(items, chg)
	}
	return items, int(total), acct.PolicyVersion, nil
}

// ListUnexportedPolicyChanges returns committed ledger changes strictly below policy_version with exported_at IS NULL.
func (s *DB) ListUnexportedPolicyChanges(ctx context.Context, tenantUID string, limit int) ([]PolicyChange, error) {
	acct, err := s.GetAccount(ctx, tenantUID)
	if errors.Is(err, ErrNotFound) || (err == nil && acct.PolicyVersion == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var recs []QuotaPolicyChangeRecord
	if err := s.orm.WithContext(ctx).
		Where("tenant_uid = ? AND source_version < ? AND exported_at IS NULL", tenantUID, acct.PolicyVersion).
		Order("source_version ASC").
		Limit(limit).
		Find(&recs).Error; err != nil {
		return nil, pgutil.Error(err, "")
	}
	out := make([]PolicyChange, 0, len(recs))
	for _, rec := range recs {
		chg, decErr := toPolicyChange(rec)
		if decErr != nil {
			return nil, decErr
		}
		out = append(out, chg)
	}
	return out, nil
}

// MarkPolicyChangeExported stamps exported_at on a committed policy ledger entry idempotently.
func (s *DB) MarkPolicyChangeExported(ctx context.Context, changeID string, exportedAt time.Time) error {
	ts := exportedAt.UTC().Truncate(time.Microsecond)
	res := s.orm.WithContext(ctx).Model(&QuotaPolicyChangeRecord{}).
		Where("change_id = ? AND exported_at IS NULL", changeID).
		Update("exported_at", ts)
	if res.Error != nil {
		return pgutil.WriteError(res.Error, changeID)
	}
	return nil
}

// VerifyPolicyChain checks ledger invariants I1 (gapless linked chain) and I2 (tail equals account state).
func (s *DB) VerifyPolicyChain(ctx context.Context, tenantUID string) (corev1.QuotaPolicyVerification, error) {
	acct, err := s.GetAccount(ctx, tenantUID)
	if errors.Is(err, ErrNotFound) {
		acct = Account{TenantUID: tenantUID}
	} else if err != nil {
		return corev1.QuotaPolicyVerification{}, err
	}
	var recs []QuotaPolicyChangeRecord
	if err := s.orm.WithContext(ctx).
		Where("tenant_uid = ? AND source_version < ?", tenantUID, acct.PolicyVersion).
		Order("source_version ASC").Find(&recs).Error; err != nil {
		return corev1.QuotaPolicyVerification{}, pgutil.Error(err, "")
	}
	changes, corruptMsg := decodeAllPolicyChanges(recs)
	i1 := checkPolicyChainOrder(acct.PolicyVersion, len(recs), changes, corruptMsg)
	i2 := checkPolicyChainTail(acct, changes, corruptMsg)
	return corev1.QuotaPolicyVerification{
		PolicyVersion:   acct.PolicyVersion,
		UpstreamVersion: acct.UpstreamVersion,
		CheckedChanges:  len(recs),
		Valid:           i1.Passed && i2.Passed,
		Checks:          []corev1.QuotaPolicyInvariantCheck{i1, i2},
	}, nil
}

func decodeAllPolicyChanges(recs []QuotaPolicyChangeRecord) ([]PolicyChange, string) {
	changes := make([]PolicyChange, 0, len(recs))
	for _, rec := range recs {
		chg, err := toPolicyChange(rec)
		if err != nil {
			return nil, err.Error()
		}
		changes = append(changes, chg)
	}
	return changes, ""
}

func checkPolicyChainOrder(policyVersion int64, rawCount int, changes []PolicyChange, corruptMsg string) corev1.QuotaPolicyInvariantCheck {
	if corruptMsg != "" {
		return corev1.QuotaPolicyInvariantCheck{ID: "I1", Passed: false, Message: corruptMsg}
	}
	if int64(rawCount) != policyVersion {
		return corev1.QuotaPolicyInvariantCheck{
			ID: "I1", Passed: false,
			Message: fmt.Sprintf("expected %d committed changes for policyVersion %d, got %d", policyVersion, policyVersion, rawCount),
		}
	}
	for i, chg := range changes {
		if chg.SourceVersion != int64(i) || chg.Plan.FromVersion != int64(i) || chg.Plan.ToVersion != int64(i+1) {
			return corev1.QuotaPolicyInvariantCheck{ID: "I1", Passed: false, Message: fmt.Sprintf("version gap or mismatch at index %d", i)}
		}
		if i == 0 && (chg.Plan.Kind != corev1.QuotaPolicyChangeKindBootstrap || !chg.Plan.Old.Equal(QuotaPolicy{})) {
			return corev1.QuotaPolicyInvariantCheck{ID: "I1", Passed: false, Message: "first entry must be Bootstrap from empty snapshot"}
		}
		if i > 0 && !chg.Plan.Old.Equal(changes[i-1].Plan.Target) {
			return corev1.QuotaPolicyInvariantCheck{ID: "I1", Passed: false, Message: fmt.Sprintf("target->old discontinuity between v%d and v%d", i, i+1)}
		}
	}
	return corev1.QuotaPolicyInvariantCheck{ID: "I1", Passed: true, Message: "contiguous linked chain"}
}

func checkPolicyChainTail(acct Account, changes []PolicyChange, corruptMsg string) corev1.QuotaPolicyInvariantCheck {
	if corruptMsg != "" {
		return corev1.QuotaPolicyInvariantCheck{ID: "I2", Passed: false, Message: corruptMsg}
	}
	if acct.PolicyVersion == 0 {
		return corev1.QuotaPolicyInvariantCheck{ID: "I2", Passed: len(changes) == 0, Message: "uninitialized account has zero changes"}
	}
	if len(changes) == 0 {
		return corev1.QuotaPolicyInvariantCheck{ID: "I2", Passed: false, Message: "missing ledger entries for active policyVersion"}
	}
	last := changes[len(changes)-1]
	if last.Plan.ToVersion != acct.PolicyVersion || !last.Plan.Target.Equal(acct.Policy()) {
		return corev1.QuotaPolicyInvariantCheck{ID: "I2", Passed: false, Message: "last change target does not match quota_accounts"}
	}
	return corev1.QuotaPolicyInvariantCheck{ID: "I2", Passed: true, Message: "last target matches current account policy"}
}
