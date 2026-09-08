package regional

import (
	"errors"
	"strings"
)

var ErrNodeAccessDenied = errors.New("regional node access denied")
var ErrNodeAccessVersionConflict = errors.New("regional node access policy version changed")

// NodeAccessPolicy is regional operator-owned tenant access to named nodes.
// It is neither a commercial entitlement nor permission to execute a workload.
// Empty or disabled policies deny new admissions; no implicit public pool exists.
type NodeAccessPolicy struct {
	OrganizationID string   `json:"organizationId"`
	NodeIDs        []string `json:"nodeIds"`
	Enabled        bool     `json:"enabled"`
	Version        int64    `json:"version"`
}

func (p NodeAccessPolicy) Validate() error {
	validID := func(id string) bool {
		return id != "" && len(id) <= 128 && strings.TrimSpace(id) == id && !strings.ContainsAny(id, "\x00\r\n")
	}
	if !validID(p.OrganizationID) || len(p.NodeIDs) > 200 {
		return ErrNodeAccessDenied
	}
	seen := map[string]bool{}
	for _, id := range p.NodeIDs {
		if !validID(id) || seen[id] {
			return ErrNodeAccessDenied
		}
		seen[id] = true
	}
	return nil
}
