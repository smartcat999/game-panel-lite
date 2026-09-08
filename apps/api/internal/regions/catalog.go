// Package regions owns global region directory metadata, not regional resources.
package regions

import (
	"errors"
	"strings"
)

var ErrInvalidRegion = errors.New("invalid region directory entry")
var ErrRegionConflict = errors.New("region directory entry already exists or changed")

type Entry struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	AcceptingCreates bool   `json:"acceptingCreates"`
	Version          int64  `json:"version"`
}

func ValidateIdentity(id, name string) error {
	if id == "" || len(id) > 128 || strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) || len(name) > 200 {
		return ErrInvalidRegion
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return ErrInvalidRegion
		}
	}
	return nil
}
