package regional

import (
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

// AssetSourceSnapshot binds the authorized reference to one observed source.
// It is point-in-time metadata, not a signed transfer or execution grant.
type AssetSourceSnapshot struct {
	Event   instances.RevisionAvailable `json:"event"`
	Asset   assets.PublishedVersion     `json:"asset"`
	Replica assets.Replica              `json:"replica"`
}

func (s AssetSourceSnapshot) ValidateFor(event instances.RevisionAvailable, ref instances.AssetVersion, replicaID string, replicaVersion int64) error {
	if event.Validate() != nil || s.Event != event || s.Asset.Validate() != nil || s.Asset.OrganizationID != event.OrganizationID || s.Asset.AssetID != ref.AssetID || s.Asset.Version != ref.Version ||
		s.Replica.Validate() != nil || !s.Replica.Available || s.Replica.ID != replicaID || s.Replica.Version != replicaVersion || s.Replica.AssetID != ref.AssetID || s.Replica.AssetVersion != ref.Version {
		return errors.New("asset source does not match authorized reference")
	}
	return nil
}
