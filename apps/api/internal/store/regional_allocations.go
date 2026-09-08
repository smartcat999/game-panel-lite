package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/scheduling"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReserveRegionalResources atomically reserves compute and the complete network
// rendered for this revision by the trusted coordinator. Node choice and runtime
// execution require separate authorization; expiry never releases these rows.
func (s *RegionalStore) ReserveRegionalResources(ctx context.Context, request regional.CapacityRequest, network workload.Network, maxHeartbeatAge time.Duration) (regional.Allocation, error) {
	if request.RegionID != s.regionID {
		return regional.Allocation{}, ErrRegionMismatch
	}
	for _, id := range []string{request.OrganizationID, request.DeploymentID, request.ServerID, request.RevisionID, request.NodeID} {
		if id == "" || len(id) > 128 || id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n") {
			return regional.Allocation{}, regional.ErrAllocationConflict
		}
	}
	if request.PlacementEpoch < 1 || request.SpecGeneration < 1 || request.IntentVersion < 1 || request.NodeVersion < 1 || request.SessionEpoch < 1 || maxHeartbeatAge < time.Millisecond || maxHeartbeatAge > time.Hour {
		return regional.Allocation{}, regional.ErrAllocationConflict
	}
	ports, err := regionalBindings(network)
	if err != nil {
		return regional.Allocation{}, err
	}
	var allocation regional.Allocation
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var hint regional.Deployment
		if err := tx.Table("regional_deployments").Where("id = ?", request.DeploymentID).Take(&hint).Error; err != nil {
			return deploymentReadError(err)
		}
		// Match asset completion's task -> deployment lock order. Recheck the hint
		// once locked because another completion may have advanced its revision.
		var task struct{ Status, Snapshot string }
		if err := tx.Table("regional_revision_tasks").Select("status,CASE WHEN octet_length(snapshot) <= ? THEN snapshot ELSE '' END AS snapshot", 4<<20).Where("operation_id = ?", hint.RevisionOperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&task).Error; err != nil {
			return deploymentReadError(err)
		}
		var deployment regional.Deployment
		if err := tx.Table("regional_deployments").Where("id = ?", request.DeploymentID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&deployment).Error; err != nil {
			return deploymentReadError(err)
		}
		if deployment.RevisionOperationID != hint.RevisionOperationID {
			return regional.ErrDeploymentConflict
		}
		var existingRow regionalAllocationRow
		err := tx.Table("regional_allocations").Where("deployment_id = ? AND status = ?", deployment.ID, "reserved").Take(&existingRow).Error
		if err == nil {
			existing, err := existingRow.allocation()
			if err != nil {
				return err
			}
			if existing.CapacityRequest != request {
				return regional.ErrAllocationConflict
			}
			if err := checkRegionalPortReceipt(tx, existing, ports); err != nil {
				return err
			}
			allocation = existing
			return nil // Receipt replay, not renewed scheduling authority.
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if deployment.OrganizationID != request.OrganizationID || deployment.ServerID != request.ServerID || deployment.PlacementEpoch != request.PlacementEpoch || deployment.RevisionID != request.RevisionID || deployment.SpecGeneration != request.SpecGeneration || deployment.IntentVersion != request.IntentVersion || deployment.DesiredState != "running" || deployment.Status != "awaiting_authority" {
			return regional.ErrDeploymentConflict
		}
		var snapshot regional.RevisionSnapshot
		if task.Status != "assets_prepared" || json.Unmarshal([]byte(task.Snapshot), &snapshot) != nil || snapshot.ValidateFor(snapshot.Event) != nil {
			return regional.ErrDeploymentConflict
		}
		event := snapshot.Event
		if event.OperationID != deployment.RevisionOperationID || event.OrganizationID != request.OrganizationID || event.RegionID != s.regionID || event.ServerID != request.ServerID || event.PlacementEpoch != request.PlacementEpoch || event.RevisionID != request.RevisionID || event.SpecGeneration != request.SpecGeneration {
			return regional.ErrDeploymentConflict
		}
		access, err := regionalNodeAccess(tx, request.OrganizationID, true)
		if err != nil {
			return err
		}
		if !access[request.NodeID] {
			return regional.ErrNodeAccessDenied
		}
		node, err := lockRegionalNode(tx, request.NodeID)
		if err != nil {
			return err
		}
		if !node.Schedulable || node.Version != request.NodeVersion {
			return regional.ErrNodeUnavailable
		}
		var observed regionalNodeSessionRow
		if err := tx.Table("regional_node_sessions").Where("node_id = ?", node.ID).Take(&observed).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return regional.ErrNodeUnavailable
			}
			return err
		}
		if observed.Epoch != request.SessionEpoch || !observed.RuntimeReady || observed.Architecture != node.Architecture {
			return regional.ErrNodeUnavailable
		}
		// Every reservation writer holds the Node lock. Aggregation is restricted to
		// that node's active rows and reuses the same pure admission rule as selection.
		var used struct {
			CPU      float64
			MemoryMB int64
			Count    int64
		}
		if err := tx.Table("regional_allocations").Select("COALESCE(SUM(cpu),0) AS cpu,COALESCE(SUM(memory_mb),0) AS memory_mb,COUNT(*) AS count").Where("node_id = ? AND status = ?", node.ID, "reserved").Take(&used).Error; err != nil {
			return err
		}
		reserved := []scheduling.Resources{}
		if used.Count > 0 {
			reserved = append(reserved, scheduling.Resources{CPU: used.CPU, MemoryMB: used.MemoryMB})
		}
		resources := snapshot.Revision.Specification.Resources
		if _, err := scheduling.CheckCapacity(scheduling.Resources{CPU: node.CPU, MemoryMB: node.MemoryMB}, scheduling.Resources{CPU: resources.CPU, MemoryMB: resources.MemoryMB}, reserved); err != nil {
			return err
		}
		conflicts, err := regionalPortConflicts(tx, []string{node.ID}, ports)
		if err != nil {
			return err
		}
		if conflicts[node.ID] {
			return regional.ErrPortsUnavailable
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		if !regionalNodeReady(node, observed, now, maxHeartbeatAge) {
			return regional.ErrNodeUnavailable
		}
		allocation = regional.Allocation{ID: uuid.NewString(), CapacityRequest: request, CPU: resources.CPU, MemoryMB: resources.MemoryMB, Status: "reserved", Ports: ports}
		encoded, err := json.Marshal(ports)
		if err != nil {
			return err
		}
		row := regionalAllocationRow{ID: allocation.ID, CapacityRequest: request, CPU: allocation.CPU, MemoryMB: allocation.MemoryMB, Status: allocation.Status, Ports: string(encoded)}
		if err := tx.Table("regional_allocations").Create(&row).Error; err != nil {
			return err
		}
		return saveRegionalPorts(tx, allocation)
	})
	if err != nil {
		return regional.Allocation{}, err
	}
	return allocation, nil
}

func deploymentReadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return regional.ErrDeploymentConflict
	}
	return err
}
