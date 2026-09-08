package assets

import (
	"errors"
	"strings"
)

var ErrReplicaConflict = errors.New("asset replica identity or version conflicts")

// Replica locates immutable bytes by regional storage identity, never a URL or
// host path. Available is a catalog observation, not permission to download.
type Replica struct {
	ID           string `json:"id"`
	AssetID      string `json:"assetId"`
	AssetVersion string `json:"assetVersion"`
	RegionID     string `json:"regionId"`
	StorageID    string `json:"storageId"`
	Available    bool   `json:"available"`
	Version      int64  `json:"version"`
}

func (r Replica) ValidateRegistration() error {
	if r.Available || r.Version != 0 {
		return ErrInvalidVersion
	}
	return r.validateIdentity()
}

func (r Replica) Validate() error {
	if r.Version < 1 {
		return ErrInvalidVersion
	}
	return r.validateIdentity()
}

func (r Replica) validateIdentity() error {
	if r.AssetID == "" || r.AssetVersion == "" || len(r.AssetID) > 128 || len(r.AssetVersion) > 128 {
		return ErrInvalidVersion
	}
	for _, id := range []string{r.AssetID, r.AssetVersion} {
		if strings.TrimSpace(id) != id || strings.ContainsAny(id, "\x00\r\n") {
			return ErrInvalidVersion
		}
	}
	for _, id := range []string{r.ID, r.RegionID, r.StorageID} {
		if id == "" || len(id) > 128 {
			return ErrInvalidVersion
		}
		for _, c := range id {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return ErrInvalidVersion
			}
		}
	}
	return nil
}
