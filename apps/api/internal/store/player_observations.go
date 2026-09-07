package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// SavePlayerCount discards observations if runtime status or placement changed
// during log collection. It never writes instance intent or inserts missing rows.
func (s *Store) SavePlayerCount(ctx context.Context, observed domain.GameServer, count int) error {
	previous, err := json.Marshal(observed.Status)
	if err != nil {
		return err
	}
	status := observed.Status
	status.PlayersOnline = count
	update := domain.GameServer{Status: status, UpdatedAt: time.Now().UTC()}
	return s.db.WithContext(ctx).Model(&domain.GameServer{}).
		Where("id = ? AND status = ? AND COALESCE(node_id, '') = ? AND COALESCE(organization_id, '') = ?", observed.ID, string(previous), observed.NodeID, observed.OrganizationID).
		Select("status", "updated_at").Updates(&update).Error
}
