package store

import (
	"context"
	"fmt"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type nodePortPool struct {
	NodeID string `gorm:"primaryKey"`
}
type nodePortReservation struct {
	NodeID   string `gorm:"primaryKey"`
	HostPort int    `gorm:"primaryKey"`
	ServerID string `gorm:"primaryKey"`
}

// Port ownership is conservative across TCP/UDP and persists across assignment
// replacement/deletion. A changed spec is not proof that old bindings are gone.
// Caller must hold a transaction, and acquire this lock before instance locks.
func (s *Store) lockNodePorts(ctx context.Context, nodeID string) error {
	pool := nodePortPool{NodeID: nodeID}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&pool).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&nodePortPool{}).Where("node_id = ?", nodeID).UpdateColumn("node_id", gorm.Expr("node_id")).Error
}

func (s *Store) reserveNodePorts(ctx context.Context, nodeID, serverID string, ports []int) error {
	if len(ports) == 0 {
		return nil
	}
	var conflicting int64
	if err := s.db.WithContext(ctx).Model(&nodePortReservation{}).Where("node_id = ? AND host_port IN ? AND server_id <> ?", nodeID, ports, serverID).Count(&conflicting).Error; err != nil {
		return err
	}
	if conflicting > 0 {
		return fmt.Errorf("%w: host port is retained by another instance", ErrNodeAllocationUnavailable)
	}
	// Also include instances created by older writers that have not published a
	// workload yet. Port-pool locking serializes current allocation writers.
	var instances []domain.GameServer
	if err := s.db.WithContext(ctx).Where("node_id = ? AND id <> ?", nodeID, serverID).Find(&instances).Error; err != nil {
		return err
	}
	for _, instance := range instances {
		host := instance.Spec.Network.HostPort
		if host == 0 {
			host = instance.Spec.Network.Port
		}
		for _, port := range ports {
			if host == port {
				return fmt.Errorf("%w: host port is assigned to another instance", ErrNodeAllocationUnavailable)
			}
		}
	}
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return workload.ErrInvalidNetwork
		}
		claim := nodePortReservation{NodeID: nodeID, ServerID: serverID, HostPort: port}
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&claim).Error; err != nil {
			return err
		}
	}
	return nil
}
