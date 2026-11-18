package quota

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

func checkChangeOwner(h pgutil.Holding, p pgutil.ResourceChange) error {
	if p.AllocationID != h.Allocation.ID || p.Owner != h.Allocation.Owner {
		return failure(corev1.IdentityConflict, "", "resource plan belongs to another allocation", nil)
	}
	if h.Allocation.State == corev1.AllocationClosing || h.Allocation.State == corev1.AllocationClosed {
		return failure(corev1.Closed, "", "resource plan cannot reopen a closed gate", nil)
	}
	return nil
}

func checkChangeStart(h pgutil.Holding, p pgutil.ResourceChange) error {
	if err := checkChangeOwner(h, p); err != nil {
		return err
	}
	if h.Allocation.State != corev1.AllocationActive || h.Allocation.ResourceRevision != p.FromRevision || h.Allocation.Current != p.Old || h.ChangePlanID != "" {
		return failure(corev1.RevisionChanged, "", "new resource plan must start from the current settled snapshot", nil)
	}
	return nil
}

func checkChangeCommand(h pgutil.Holding, c pgutil.Command, change Change) error {
	p := change.Plan
	if err := checkChangeOwner(h, p); err != nil {
		return err
	}
	if c.ChangePlanID != p.ChangeID || c.ResourceRevision != p.ToRevision {
		return failure(corev1.IdentityConflict, c.OperationID, "command changed the resource plan identity or target", nil)
	}
	grow, shrink, err := corev1.GrowthAndShrink(p.Old, p.Target)
	if err != nil {
		return err
	}
	if c.Kind == corev1.Increase && c.Quantity != grow || c.Kind == corev1.SettleDecrease && c.Quantity != shrink {
		return failure(corev1.IdentityConflict, c.OperationID, "amount differs from the immutable resource plan", nil)
	}
	expected := change.SourceVersion
	if c.Kind == corev1.SettleDecrease && !grow.IsZero() {
		if expected == math.MaxInt64 {
			return failure(corev1.InvariantViolation, c.OperationID, "resource plan version overflow", nil)
		}
		expected++
	}
	if c.ExpectedVersion != expected {
		return failure(corev1.IdentityConflict, c.OperationID, "command source version differs from its registered resource plan", nil)
	}
	return nil
}

func checkChangeExecution(h pgutil.Holding, c pgutil.Command, change Change) error {
	if err := checkChangeCommand(h, c, change); err != nil {
		return err
	}
	p := change.Plan
	if h.Allocation.ResourceRevision != p.FromRevision {
		return failure(corev1.RevisionChanged, c.OperationID, "resource plan source revision is stale", nil)
	}
	expected := p.Old
	if c.Kind == corev1.SettleDecrease {
		grow, _, err := corev1.GrowthAndShrink(p.Old, p.Target)
		if err != nil {
			return err
		}
		expected, err = expected.Add(grow)
		if err != nil {
			return err
		}
		if !grow.IsZero() && h.ChangePlanID != p.ChangeID {
			return failure(corev1.RevisionChanged, c.OperationID, "positive budget must be granted before settlement", nil)
		}
	}
	if h.Allocation.Current != expected {
		return failure(corev1.RevisionChanged, c.OperationID, "holding does not match this plan's execution phase", nil)
	}
	return nil
}

func (s *DB) RegisterChange(ctx context.Context, p pgutil.ResourceChange) (Change, error) {
	hash, err := p.Fingerprint()
	if err != nil {
		return Change{}, err
	}
	if existing, err := getChange(s.orm.WithContext(ctx), p.ChangeID); err == nil {
		original, _ := existing.Plan.Fingerprint()
		if original != hash {
			return existing, failure(corev1.IdentityConflict, "", "resource plan is immutable", nil)
		}
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Change{}, err
	}
	h, err := s.GetAllocation(ctx, p.AllocationID)
	if err != nil {
		return Change{}, err
	}
	if err := checkChangeStart(h, p); err != nil {
		return Change{}, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return Change{}, err
	}
	rec := QuotaChangeRecord{
		ChangeID:      p.ChangeID,
		AllocationID:  p.AllocationID,
		RequestHash:   hash,
		Plan:          raw,
		SourceVersion: h.Allocation.Version,
	}
	err = s.orm.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "change_id"}},
			DoUpdates: clause.Assignments(map[string]any{"change_id": gorm.Expr("quota_changes.change_id")}),
		},
		clause.Returning{},
	).Create(&rec).Error
	if err != nil {
		return Change{}, pgutil.WriteError(err, "")
	}
	if rec.RequestHash != hash {
		return Change{}, failure(corev1.IdentityConflict, "", "resource plan is immutable", nil)
	}
	return Change{Plan: p, SourceVersion: rec.SourceVersion}, nil
}

func getChange(tx *gorm.DB, id string) (Change, error) {
	var rec QuotaChangeRecord
	if err := tx.Where("change_id = ?", id).Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Change{}, notFound("resource change", id)
		}
		return Change{}, pgutil.Error(err, "")
	}
	change := Change{SourceVersion: rec.SourceVersion}
	if err := json.Unmarshal(rec.Plan, &change.Plan); err != nil {
		return change, failure(corev1.InvariantViolation, "", "invalid resource plan", err)
	}
	p := change.Plan
	actual, err := p.Fingerprint()
	if err != nil || actual != rec.RequestHash || p.ChangeID != id || p.AllocationID != rec.AllocationID || change.SourceVersion < 1 {
		return change, failure(corev1.InvariantViolation, "", "resource plan identity is corrupt", err)
	}
	return change, nil
}
