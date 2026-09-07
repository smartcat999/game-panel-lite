package store

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func (s *Store) RemoteArtifactsAvailable(ctx context.Context, nodeID string) (bool, error) {
	node, err := s.GetComputeNode(ctx, nodeID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	age := time.Since(node.LastHeartbeat)
	return node.Status == "online" && !node.LastHeartbeat.IsZero() && age >= 0 && age <= 45*time.Second && slices.Contains(node.WorkloadCapabilities, workload.ArtifactCapability), nil
}
