package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/deploymentstatus"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ regional.ExecutionRepository = (*RegionalStore)(nil)

type regionalExecutionLeaseRow struct {
	ServerID, TaskID, NodeID, HolderID string
	SessionEpoch, Generation           int64
	Fence, GrantedAtMS, ExpiresAtMS    int64
}

type regionalObservationRow struct {
	TaskID, ServerID, NodeID, HolderID string
	Generation, Fence                  int64
	InputToken, ObservationToken       string
	PayloadSHA256                      string
	Payload                            string
	ObservedAt, UpdatedAt              time.Time
}

func (s *RegionalStore) NextExecutionCandidate(ctx context.Context, nodeID string, sessionEpoch int64) (*regional.ExecutionCandidate, error) {
	if !regionalExecutionIdentifier(nodeID) || sessionEpoch < 1 {
		return nil, regional.ErrExecutionUnavailable
	}
	var result *regional.ExecutionCandidate
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		var tasks []regional.NodeTask
		if err := tx.Table("regional_node_tasks").Where("region_id = ? AND node_id = ? AND session_epoch = ? AND status = ?", s.regionID, nodeID, sessionEpoch, "awaiting_authority").Order("id").Limit(32).Find(&tasks).Error; err != nil {
			return err
		}
		if len(tasks) < 32 {
			var activeTasks []regional.NodeTask
			if err := tx.Table("regional_node_tasks").Where("region_id = ? AND node_id = ? AND session_epoch = ? AND status = ?", s.regionID, nodeID, sessionEpoch, "active").Order("id").Limit(32 - len(tasks)).Find(&activeTasks).Error; err != nil {
				return err
			}
			tasks = append(tasks, activeTasks...)
		}
		if len(tasks) == 0 {
			return nil
		}
		servers := make([]string, 0, len(tasks))
		for _, task := range tasks {
			servers = append(servers, task.ServerID)
		}
		var leases []regionalExecutionLeaseRow
		if err := tx.Table("regional_execution_leases").Where("server_id IN ?", servers).Find(&leases).Error; err != nil {
			return err
		}
		active := make(map[string]bool, len(leases))
		for _, lease := range leases {
			if lease.ExpiresAtMS > now {
				active[lease.ServerID] = true
			}
		}
		for _, task := range tasks {
			if active[task.ServerID] {
				continue
			}
			candidate, err := loadRegionalExecutionCandidate(tx, task.ID, nodeID, sessionEpoch, false)
			if err != nil {
				return err
			}
			result = &candidate
			return nil
		}
		return nil
	})
	return result, err
}

func (s *RegionalStore) ExecutionCandidate(ctx context.Context, taskID, nodeID string, sessionEpoch int64) (regional.ExecutionCandidate, error) {
	if !regionalExecutionIdentifier(taskID) || !regionalExecutionIdentifier(nodeID) || sessionEpoch < 1 {
		return regional.ExecutionCandidate{}, regional.ErrExecutionUnavailable
	}
	var candidate regional.ExecutionCandidate
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		candidate, err = loadRegionalExecutionCandidate(tx, taskID, nodeID, sessionEpoch, false)
		if err == nil && candidate.Task.RegionID != s.regionID {
			return regional.ErrExecutionUnavailable
		}
		return err
	})
	return candidate, err
}

func loadRegionalExecutionCandidate(tx *gorm.DB, taskID, nodeID string, sessionEpoch int64, lock bool) (regional.ExecutionCandidate, error) {
	var candidate regional.ExecutionCandidate
	query := tx.Table("regional_node_tasks").Where("id = ? AND node_id = ? AND session_epoch = ? AND status IN ?", taskID, nodeID, sessionEpoch, []string{"awaiting_authority", "active"})
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Take(&candidate.Task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return candidate, regional.ErrExecutionUnavailable
		}
		return candidate, err
	}
	var allocationRow regionalAllocationRow
	if err := tx.Table("regional_allocations").Where("id = ? AND status = ?", candidate.Task.AllocationID, "reserved").Take(&allocationRow).Error; err != nil {
		return candidate, regional.ErrExecutionUnavailable
	}
	allocation, err := allocationRow.allocation()
	if err != nil {
		return candidate, err
	}
	candidate.Allocation = allocation
	var deployment regionalSchedulingRow
	if err := tx.Table("regional_deployments").Where("id = ?", candidate.Task.DeploymentID).Take(&deployment).Error; err != nil {
		return candidate, regional.ErrExecutionUnavailable
	}
	if deployment.OrganizationID != candidate.Task.OrganizationID || deployment.ServerID != candidate.Task.ServerID || deployment.PlacementEpoch != candidate.Task.PlacementEpoch || deployment.RevisionID != candidate.Task.RevisionID || deployment.SpecGeneration != candidate.Task.SpecGeneration || deployment.IntentVersion != candidate.Task.IntentVersion || deployment.DesiredState != "running" || deployment.SchedulingStatus != "reserved" {
		return candidate, regional.ErrExecutionUnavailable
	}
	var revisionTask struct{ Status, Snapshot string }
	if err := tx.Table("regional_revision_tasks").Select("status,CASE WHEN octet_length(snapshot) <= ? THEN snapshot ELSE '' END AS snapshot", 4<<20).Where("operation_id = ?", deployment.RevisionOperationID).Take(&revisionTask).Error; err != nil {
		return candidate, regional.ErrExecutionUnavailable
	}
	if revisionTask.Status != "assets_prepared" || json.Unmarshal([]byte(revisionTask.Snapshot), &candidate.Snapshot) != nil || candidate.Validate(nodeID, sessionEpoch) != nil {
		return candidate, regional.ErrExecutionUnavailable
	}
	return candidate, nil
}

func (s *RegionalStore) AcquireRegionalExecutionLease(ctx context.Context, candidate regional.ExecutionCandidate, holderID string, ttl, maxHeartbeatAge time.Duration) (regional.ExecutionLease, error) {
	return s.changeRegionalExecutionLease(ctx, candidate, holderID, 0, ttl, maxHeartbeatAge, "acquire")
}

func (s *RegionalStore) RenewRegionalExecutionLease(ctx context.Context, candidate regional.ExecutionCandidate, holderID string, fence int64, ttl, maxHeartbeatAge time.Duration) (regional.ExecutionLease, error) {
	return s.changeRegionalExecutionLease(ctx, candidate, holderID, fence, ttl, maxHeartbeatAge, "renew")
}

func (s *RegionalStore) ReleaseRegionalExecutionLease(ctx context.Context, candidate regional.ExecutionCandidate, holderID string, fence int64) error {
	_, err := s.changeRegionalExecutionLease(ctx, candidate, holderID, fence, 0, 0, "release")
	return err
}

func (s *RegionalStore) changeRegionalExecutionLease(ctx context.Context, candidate regional.ExecutionCandidate, holderID string, fence int64, ttl, maxHeartbeatAge time.Duration, action string) (regional.ExecutionLease, error) {
	if candidate.Task.RegionID != s.regionID || candidate.Validate(candidate.Task.NodeID, candidate.Task.SessionEpoch) != nil || !regionalExecutionIdentifier(holderID) || (action != "release" && (ttl < time.Second || ttl > 5*time.Minute || maxHeartbeatAge < time.Second || maxHeartbeatAge > time.Hour)) || ((action == "renew" || action == "release") && fence < 1) {
		return regional.ExecutionLease{}, regional.ErrExecutionLeaseLost
	}
	var result regional.ExecutionLease
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := loadRegionalExecutionCandidate(tx, candidate.Task.ID, candidate.Task.NodeID, candidate.Task.SessionEpoch, true)
		if err != nil || !reflect.DeepEqual(current, candidate) {
			return regional.ErrExecutionLeaseLost
		}
		if err := validateRegionalExecutionNode(tx, current, maxHeartbeatAge, action != "release"); err != nil {
			return err
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		seed := regionalExecutionLeaseRow{ServerID: current.Task.ServerID, TaskID: current.Task.ID, NodeID: current.Task.NodeID, SessionEpoch: current.Task.SessionEpoch, Generation: current.Task.SpecGeneration}
		if err := tx.Table("regional_execution_leases").Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		query := tx.Table("regional_execution_leases").Where("server_id = ?", current.Task.ServerID)
		updates := map[string]any{}
		switch action {
		case "acquire":
			query = query.Where("expires_at_ms <= ?", now)
			updates = map[string]any{"task_id": current.Task.ID, "node_id": current.Task.NodeID, "session_epoch": current.Task.SessionEpoch, "generation": current.Task.SpecGeneration, "holder_id": holderID, "fence": gorm.Expr("fence + 1"), "granted_at_ms": now, "expires_at_ms": now + ttl.Milliseconds()}
		case "renew":
			query = query.Where("task_id = ? AND node_id = ? AND session_epoch = ? AND generation = ? AND holder_id = ? AND fence = ? AND expires_at_ms > ?", current.Task.ID, current.Task.NodeID, current.Task.SessionEpoch, current.Task.SpecGeneration, holderID, fence, now)
			updates = map[string]any{"granted_at_ms": now, "expires_at_ms": now + ttl.Milliseconds()}
		case "release":
			var observation regionalObservationRow
			if err := tx.Table("regional_workload_observations").Where("task_id = ? AND holder_id = ? AND fence = ?", current.Task.ID, holderID, fence).Take(&observation).Error; err != nil {
				return regional.ErrExecutionLeaseLost
			}
			var report workload.Observation
			if json.Unmarshal([]byte(observation.Payload), &report) != nil || report.LastError != "" || report.ActualState != "running" || report.ObservedGeneration != int(current.Task.SpecGeneration) {
				return regional.ErrExecutionLeaseLost
			}
			query = query.Where("task_id = ? AND node_id = ? AND session_epoch = ? AND generation = ? AND holder_id = ? AND fence = ? AND expires_at_ms > ?", current.Task.ID, current.Task.NodeID, current.Task.SessionEpoch, current.Task.SpecGeneration, holderID, fence, now)
			updates = map[string]any{"expires_at_ms": now}
		default:
			return regional.ErrExecutionLeaseLost
		}
		changed := query.Updates(updates)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return regional.ErrExecutionLeaseLost
		}
		if action == "acquire" && current.Task.Status == "awaiting_authority" {
			if err := tx.Table("regional_node_tasks").Where("id = ? AND status = ?", current.Task.ID, "awaiting_authority").Update("status", "active").Error; err != nil {
				return err
			}
		}
		if action == "release" {
			if err := tx.Table("regional_node_tasks").Where("id = ? AND status = ?", current.Task.ID, "active").Update("status", "succeeded").Error; err != nil {
				return err
			}
		}
		var row regionalExecutionLeaseRow
		if err := tx.Table("regional_execution_leases").Where("server_id = ?", current.Task.ServerID).Take(&row).Error; err != nil {
			return err
		}
		var observation regionalObservationRow
		if err := tx.Table("regional_workload_observations").Select("observation_token").Where("task_id = ?", current.Task.ID).Take(&observation).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		result = regional.ExecutionLease{TaskID: row.TaskID, ServerID: row.ServerID, NodeID: row.NodeID, HolderID: row.HolderID, Generation: int(row.Generation), Fence: row.Fence, GrantedAtMS: row.GrantedAtMS, ExpiresAtMS: row.ExpiresAtMS, ObservationToken: observation.ObservationToken}
		return nil
	})
	return result, err
}

func validateRegionalExecutionNode(tx *gorm.DB, candidate regional.ExecutionCandidate, maxHeartbeatAge time.Duration, requireFresh bool) error {
	node, err := lockRegionalNode(tx, candidate.Task.NodeID)
	if err != nil || node.Version != candidate.Task.NodeVersion || !node.Schedulable {
		return regional.ErrExecutionUnavailable
	}
	var session regionalNodeSessionRow
	if err := tx.Table("regional_node_sessions").Where("node_id = ?", node.ID).Take(&session).Error; err != nil || session.Epoch != candidate.Task.SessionEpoch || !session.RuntimeReady || session.Architecture != node.Architecture {
		return regional.ErrExecutionUnavailable
	}
	if requireFresh {
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		if session.LastSeenMS < 1 || now-session.LastSeenMS > maxHeartbeatAge.Milliseconds() {
			return regional.ErrExecutionUnavailable
		}
	}
	return nil
}

func (s *RegionalStore) SaveRegionalExecutionObservation(ctx context.Context, candidate regional.ExecutionCandidate, holderID string, fence int64, observation workload.Observation) error {
	if candidate.Task.RegionID != s.regionID || candidate.Validate(candidate.Task.NodeID, candidate.Task.SessionEpoch) != nil || !regionalExecutionIdentifier(holderID) || fence < 1 || observation.LeaseHolderID != holderID || observation.LeaseFence != fence || observation.ObservedGeneration != int(candidate.Task.SpecGeneration) || observation.ObservedAt.IsZero() || observation.ObservedAt.UnixMilli() < 1 || !regionalActualState(observation.ActualState) {
		return regional.ErrExecutionLeaseLost
	}
	payload, err := json.Marshal(observation)
	if err != nil || len(payload) > 60<<10 {
		return regional.ErrExecutionLeaseLost
	}
	digest := sha256.Sum256(payload)
	payloadSHA256 := fmt.Sprintf("%x", digest[:])
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := loadRegionalExecutionCandidate(tx, candidate.Task.ID, candidate.Task.NodeID, candidate.Task.SessionEpoch, true)
		if err != nil || !reflect.DeepEqual(current, candidate) {
			return regional.ErrExecutionLeaseLost
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		locked := tx.Table("regional_execution_leases").Where("server_id = ? AND task_id = ? AND node_id = ? AND session_epoch = ? AND generation = ? AND holder_id = ? AND fence = ? AND expires_at_ms > ?", current.Task.ServerID, current.Task.ID, current.Task.NodeID, current.Task.SessionEpoch, current.Task.SpecGeneration, holderID, fence, now).UpdateColumn("fence", gorm.Expr("fence"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return regional.ErrExecutionLeaseLost
		}
		var existing regionalObservationRow
		err = tx.Table("regional_workload_observations").Where("task_id = ?", current.Task.ID).Take(&existing).Error
		if err == nil {
			if observation.ObservationToken == existing.InputToken && existing.HolderID == holderID && existing.Fence == fence && existing.PayloadSHA256 == payloadSHA256 {
				return nil
			}
			if observation.ObservationToken != existing.ObservationToken {
				return regional.ErrExecutionLeaseLost
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else if observation.ObservationToken != "" {
			return regional.ErrExecutionLeaseLost
		}
		row := regionalObservationRow{TaskID: current.Task.ID, ServerID: current.Task.ServerID, NodeID: current.Task.NodeID, Generation: current.Task.SpecGeneration, HolderID: holderID, Fence: fence, InputToken: observation.ObservationToken, ObservationToken: uuid.NewString(), PayloadSHA256: payloadSHA256, Payload: string(payload), ObservedAt: observation.ObservedAt.UTC()}
		if err := tx.Table("regional_workload_observations").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "task_id"}}, DoUpdates: clause.AssignmentColumns([]string{"holder_id", "fence", "input_token", "observation_token", "payload_sha256", "payload", "observed_at", "updated_at"})}).Create(&row).Error; err != nil {
			return err
		}
		outcome := "succeeded"
		if observation.LastError != "" || observation.ActualState != "running" || observation.RuntimeID == "" {
			outcome = "failed"
		}
		event := deploymentstatus.Event{SchemaVersion: 1, EventID: uuid.NewString(), RegionID: current.Task.RegionID, OrganizationID: current.Task.OrganizationID, OperationID: current.Snapshot.Event.OperationID, ServerID: current.Task.ServerID, RevisionID: current.Task.RevisionID, TaskID: current.Task.ID, NodeID: current.Task.NodeID, PlacementEpoch: current.Task.PlacementEpoch, SpecGeneration: current.Task.SpecGeneration, IntentVersion: current.Task.IntentVersion, Fence: fence, ActualState: observation.ActualState, Outcome: outcome, RuntimeID: observation.RuntimeID, ObservedAtMS: observation.ObservedAt.UnixMilli()}
		if event.Validate() != nil {
			return regional.ErrExecutionLeaseLost
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		outbox := map[string]any{"id": event.EventID, "server_id": event.ServerID, "task_id": event.TaskID, "fence": event.Fence, "event_type": "deployment.status.observed", "payload": string(encoded)}
		return tx.Table("regional_deployment_status_outbox").Create(outbox).Error
	})
}

func regionalExecutionIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' || ch == '.' || ch == ':') {
			return false
		}
	}
	return true
}

func regionalActualState(state string) bool {
	return state == "running" || state == "stopped" || state == "missing" || state == "unknown"
}
