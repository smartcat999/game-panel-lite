package store

import (
	"context"
	"errors"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"gorm.io/gorm"
)

// RegionalAllocation reads a prior receipt from this region's own database.
// The coordinator must verify scope and recheck the receipt through admission;
// this read alone does not validate its ports or authorize runtime execution.
func (s *RegionalStore) RegionalAllocation(ctx context.Context, deploymentID string) (*regional.Allocation, error) {
	if deploymentID == "" || len(deploymentID) > 128 || strings.TrimSpace(deploymentID) != deploymentID || strings.ContainsAny(deploymentID, "\x00\r\n") {
		return nil, regional.ErrAllocationConflict
	}
	var row regionalAllocationRow
	err := s.db.WithContext(ctx).Table("regional_allocations").Where("deployment_id = ? AND status = ?", deploymentID, "reserved").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	allocation, err := row.allocation()
	if err != nil {
		return nil, err
	}
	return &allocation, nil
}
