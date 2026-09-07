package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrExecutionLeaseUnavailable = errors.New("execution lease is held, expired, or superseded")

// ExecutionLease is retained across assignment replacement/deletion so its fence
// cannot restart at one for an earlier server identity. It does not stop a runtime.
type ExecutionLease struct {
	ServerID      string `gorm:"primaryKey"`
	AssignmentUID string
	NodeID        string
	Generation    int
	HolderID      string
	Fence         int64
	ExpiresAtMS   int64
}

func (ExecutionLease) TableName() string { return "workload_execution_leases" }

// ExecutionLeaseRequest identifies one process incarnation and exact assignment.
// A process must choose a fresh HolderID on restart, never a shared node ID.
type ExecutionLeaseRequest struct {
	NodeID, NodeToken, AssignmentUID, HolderID string
	Generation                                 int
}

// AcquireExecutionLease never extends an existing claim, even for the same
// holder. Retry uncertain acquisitions through RenewExecutionLease with the
// returned fence; callers without a confirmed fence must wait for expiration.
func (s *Store) AcquireExecutionLease(ctx context.Context, request ExecutionLeaseRequest, ttl time.Duration) (ExecutionLease, error) {
	return s.changeExecutionLease(ctx, request, 0, ttl, false)
}

func (s *Store) RenewExecutionLease(ctx context.Context, request ExecutionLeaseRequest, fence int64, ttl time.Duration) (ExecutionLease, error) {
	if fence <= 0 {
		return ExecutionLease{}, ErrExecutionLeaseUnavailable
	}
	return s.changeExecutionLease(ctx, request, fence, ttl, false)
}

func (s *Store) ReleaseExecutionLease(ctx context.Context, request ExecutionLeaseRequest, fence int64) error {
	if fence <= 0 {
		return ErrExecutionLeaseUnavailable
	}
	_, err := s.changeExecutionLease(ctx, request, fence, 0, true)
	return err
}

func (s *Store) changeExecutionLease(ctx context.Context, request ExecutionLeaseRequest, fence int64, ttl time.Duration, release bool) (ExecutionLease, error) {
	if request.NodeID == "" || request.NodeToken == "" || request.AssignmentUID == "" || request.HolderID == "" || len(request.HolderID) > 128 || request.Generation <= 0 {
		return ExecutionLease{}, ErrExecutionLeaseUnavailable
	}
	if !release && (ttl < time.Second || ttl > 5*time.Minute) {
		return ExecutionLease{}, fmt.Errorf("execution lease TTL must be between one second and five minutes")
	}
	var lease ExecutionLease
	err := s.Transaction(ctx, func(tx *Store) error {
		// Lock order: node, instance, assignment, lease. Token rotation and deletion
		// must serialize with authorization, not merely precede an HTTP handler.
		locked := tx.db.WithContext(ctx).Model(&domain.ComputeNode{}).Where("id = ? AND token = ?", request.NodeID, request.NodeToken).UpdateColumn("updated_at", gorm.Expr("updated_at"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return ErrExecutionLeaseUnavailable
		}
		assignment, err := tx.GetWorkloadAssignmentByUID(ctx, request.AssignmentUID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return ErrExecutionLeaseUnavailable
			}
			return err
		}
		locked = tx.db.WithContext(ctx).Model(&domain.GameServer{}).Where("id = ? AND node_id = ?", assignment.ServerID, request.NodeID).UpdateColumn("updated_at", gorm.Expr("updated_at"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return ErrExecutionLeaseUnavailable
		}
		server, err := tx.GetGameServer(ctx, assignment.ServerID)
		if err != nil {
			return err
		}
		if server.Spec.Generation != request.Generation || server.Spec.DesiredState != assignment.DesiredState {
			return ErrExecutionLeaseUnavailable
		}
		locked = tx.db.WithContext(ctx).Model(&domain.WorkloadAssignment{}).Where("uid = ? AND server_id = ? AND node_id = ? AND generation = ?", request.AssignmentUID, assignment.ServerID, request.NodeID, request.Generation).UpdateColumn("updated_at", gorm.Expr("updated_at"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return ErrExecutionLeaseUnavailable
		}
		// Use the database clock after acquiring locks; control-plane hosts need not
		// agree on wall time, and time spent waiting for locks cannot renew a stale claim.
		var now int64
		clockSQL := "SELECT CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER)"
		if tx.db.Dialector.Name() == "postgres" {
			clockSQL = "SELECT CAST(EXTRACT(EPOCH FROM clock_timestamp()) * 1000 AS BIGINT)"
		}
		if err := tx.db.Raw(clockSQL).Scan(&now).Error; err != nil {
			return err
		}
		seed := ExecutionLease{ServerID: assignment.ServerID}
		if err := tx.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		query := tx.db.Model(&ExecutionLease{}).Where("server_id = ?", assignment.ServerID)
		updates := map[string]any{"expires_at_ms": now + ttl.Milliseconds()}
		if fence == 0 {
			query = query.Where("expires_at_ms <= ?", now)
			updates["assignment_uid"] = request.AssignmentUID
			updates["node_id"] = request.NodeID
			updates["generation"] = request.Generation
			updates["holder_id"] = request.HolderID
			updates["fence"] = gorm.Expr("fence + 1")
		} else {
			query = query.Where("assignment_uid = ? AND node_id = ? AND generation = ? AND holder_id = ? AND fence = ? AND expires_at_ms > ?", request.AssignmentUID, request.NodeID, request.Generation, request.HolderID, fence, now)
		}
		if release {
			updates["expires_at_ms"] = now
		} else if fence > 0 {
			// A shorter renewal must not invalidate the previously granted window.
			expiry := now + ttl.Milliseconds()
			updates["expires_at_ms"] = gorm.Expr("CASE WHEN expires_at_ms > ? THEN expires_at_ms ELSE ? END", expiry, expiry)
		}
		result := query.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrExecutionLeaseUnavailable
		}
		return tx.db.First(&lease, "server_id = ?", assignment.ServerID).Error
	})
	return lease, err
}
