package store

import (
	"context"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// Agent reports own liveness/measurements, never node credentials or placement.
// Conditional UPDATE cannot recreate a deleted node or undo token rotation.
func (s *Store) SaveAgentHeartbeat(ctx context.Context, before, after domain.ComputeNode) error {
	return s.saveAgentNodeReport(ctx, before, after, []string{"cpu_usage_percent", "memory_used_mb", "disk_used_gb", "running_count", "ping_latency_ms"})
}

func (s *Store) SaveAgentRegistration(ctx context.Context, before, after domain.ComputeNode) error {
	fields := []string{"cpu_cores", "memory_total_mb", "disk_total_gb", "docker_version", "agent_version", "os_info"}
	// An omitted IP report preserves a concurrent administrative address change.
	if after.PublicIP != before.PublicIP {
		fields = append(fields, "public_ip")
	}
	return s.saveAgentNodeReport(ctx, before, after, fields)
}

func (s *Store) saveAgentNodeReport(ctx context.Context, before, after domain.ComputeNode, fields []string) error {
	if before.ID == "" || before.Token == "" || after.ID != before.ID || after.Token != before.Token || after.LastHeartbeat.IsZero() {
		return ErrReconciliationSuperseded
	}
	fields = append(fields, "status", "last_heartbeat", "workload_capabilities", "updated_at")
	result := s.db.WithContext(ctx).Model(&domain.ComputeNode{}).
		Where("id = ? AND token = ? AND (last_heartbeat IS NULL OR last_heartbeat <= ?)", before.ID, before.Token, after.LastHeartbeat).
		Select(fields).Updates(&after)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrReconciliationSuperseded
	}
	return nil
}
