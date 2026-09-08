// Package assets describes globally owned, immutable published asset versions.
// Regional file locations and upload sessions do not belong in these models.
package assets

import (
	"encoding/hex"
	"errors"
	"strings"
)

var (
	ErrInvalidVersion  = errors.New("invalid published asset version")
	ErrVersionConflict = errors.New("asset identity or version conflicts")
	ErrUnavailable     = errors.New("asset version is unavailable to this tenant")
)

type PublishedVersion struct {
	AssetID        string `json:"assetId"`
	OrganizationID string `json:"organizationId"`
	Version        string `json:"version"`
	SHA256         string `json:"sha256"`
	SizeBytes      int64  `json:"sizeBytes"`
}

func (v PublishedVersion) Validate() error {
	for _, id := range []string{v.AssetID, v.OrganizationID, v.Version} {
		if id == "" || len(id) > 128 || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\x00\r\n") {
			return ErrInvalidVersion
		}
	}
	digest, err := hex.DecodeString(v.SHA256)
	if err != nil || len(digest) != 32 || v.SHA256 != strings.ToLower(v.SHA256) || v.SizeBytes < 0 {
		return ErrInvalidVersion
	}
	return nil
}
