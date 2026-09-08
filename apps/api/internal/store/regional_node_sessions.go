package store

import (
	"context"
	"errors"
	"math"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type regionalNodeSessionRow struct {
	NodeID          string
	Epoch, Sequence int64
	Architecture    string
	RuntimeReady    bool
	LastSeenMS      int64
}

func lockRegionalNode(tx *gorm.DB, nodeID string) (regional.Node, error) {
	var node regional.Node
	err := tx.Table("regional_nodes").Where("id = ?", nodeID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return node, regional.ErrNodeUnavailable
	}
	return node, err
}

// StartRegionalNodeSession requires a verified Node identity. Session rotation
// invalidates older heartbeat writers, not their physical workload execution.
func (s *RegionalStore) StartRegionalNodeSession(ctx context.Context, nodeID string) (regional.NodeSession, error) {
	var session regional.NodeSession
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockRegionalNode(tx, nodeID); err != nil {
			return err
		}
		var previous regionalNodeSessionRow
		err := tx.Table("regional_node_sessions").Where("node_id = ?", nodeID).Take(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if previous.Epoch == math.MaxInt64 {
			return regional.ErrNodeUnavailable
		}
		row := regionalNodeSessionRow{NodeID: nodeID, Epoch: previous.Epoch + 1}
		if err := tx.Table("regional_node_sessions").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "node_id"}}, DoUpdates: clause.AssignmentColumns([]string{"epoch", "sequence", "architecture", "runtime_ready", "last_seen_ms"})}).Create(&row).Error; err != nil {
			return err
		}
		session.Epoch = row.Epoch
		return nil
	})
	if err != nil {
		return regional.NodeSession{}, err
	}
	return session, nil
}

func (s *RegionalStore) RecordRegionalNodeHeartbeat(ctx context.Context, nodeID string, heartbeat regional.NodeHeartbeat) error {
	if heartbeat.SessionEpoch < 1 || heartbeat.Sequence < 1 || heartbeat.Architecture == "" || len(heartbeat.Architecture) > 128 {
		return regional.ErrInvalidNode
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		node, err := lockRegionalNode(tx, nodeID)
		if err != nil {
			return err
		}
		if node.Architecture != heartbeat.Architecture {
			return regional.ErrInvalidNode
		}
		var row regionalNodeSessionRow
		err = tx.Table("regional_node_sessions").Where("node_id = ?", nodeID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return regional.ErrNodeHeartbeatStale
		}
		if err != nil {
			return err
		}
		if row.Epoch != heartbeat.SessionEpoch || row.Sequence > heartbeat.Sequence {
			return regional.ErrNodeHeartbeatStale
		}
		if row.Sequence == heartbeat.Sequence {
			if row.Architecture != heartbeat.Architecture || row.RuntimeReady != heartbeat.RuntimeReady {
				return regional.ErrNodeHeartbeatStale
			}
			return nil // Duplicate acknowledgments do not extend observed liveness.
		}
		now, err := outboxNow(tx)
		if err != nil {
			return err
		}
		return tx.Table("regional_node_sessions").Where("node_id = ?", nodeID).Updates(map[string]any{"sequence": heartbeat.Sequence, "architecture": heartbeat.Architecture, "runtime_ready": heartbeat.RuntimeReady, "last_seen_ms": now}).Error
	})
}
