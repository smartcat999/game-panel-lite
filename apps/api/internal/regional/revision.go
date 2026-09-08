package regional

import (
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

var ErrRevisionUnavailable = errors.New("revision is not available to this region")

// RevisionSnapshot is data, not an execution grant. CurrentSpecGeneration can
// be newer than Revision.SpecGeneration after a delayed notification arrives.
// A materializer still needs entitlement, intent and bounded execution authority.
type RevisionSnapshot struct {
	Event                 instances.RevisionAvailable `json:"event"`
	Revision              instances.Revision          `json:"revision"`
	CurrentSpecGeneration int64                       `json:"currentSpecGeneration"`
	DesiredState          string                      `json:"desiredState"`
	IntentVersion         int64                       `json:"intentVersion"`
	Assets                []assets.PublishedVersion   `json:"assets,omitempty"`
}

// ValidateFor checks snapshot identity and monotonic versions, not execution authority.
func (s RevisionSnapshot) ValidateFor(event instances.RevisionAvailable) error {
	if err := s.validateIdentity(event); err != nil {
		return err
	}
	return s.validateManifest(event)
}

// NeedsAssetManifest identifies a legacy snapshot eligible for authorized
// refetch. It is not permission to execute or to synthesize missing metadata.
func (s RevisionSnapshot) NeedsAssetManifest(event instances.RevisionAvailable) bool {
	return s.validateIdentity(event) == nil && len(s.Revision.Specification.Assets) > 0 && len(s.Assets) == 0
}

func (s RevisionSnapshot) validateIdentity(event instances.RevisionAvailable) error {
	if event.Validate() != nil || s.Event != event || s.Revision.ID != event.RevisionID ||
		s.Revision.ServerID != event.ServerID || s.Revision.SpecGeneration != event.SpecGeneration ||
		s.CurrentSpecGeneration < event.SpecGeneration || s.IntentVersion < 1 ||
		(s.DesiredState != "running" && s.DesiredState != "stopped") || s.Revision.Specification.Validate() != nil {
		return errors.New("revision snapshot does not match notification")
	}
	return nil
}

func (s RevisionSnapshot) validateManifest(event instances.RevisionAvailable) error {
	if len(s.Assets) != len(s.Revision.Specification.Assets) {
		return errors.New("revision asset manifest is incomplete")
	}
	byReference := make(map[instances.AssetVersion]bool, len(s.Revision.Specification.Assets))
	for _, ref := range s.Revision.Specification.Assets {
		byReference[ref] = true
	}
	for _, version := range s.Assets {
		ref := instances.AssetVersion{AssetID: version.AssetID, Version: version.Version}
		if version.Validate() != nil || version.OrganizationID != event.OrganizationID || !byReference[ref] {
			return errors.New("revision asset manifest does not match references")
		}
		delete(byReference, ref)
	}
	return nil
}
