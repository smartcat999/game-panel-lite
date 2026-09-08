// Package entitlements owns global, versioned rights to run a logical instance.
// A record is commercial/operational policy, not a Node execution lease.
package entitlements

import (
	"errors"
	"math"
	"strings"
)

var ErrInvalid = errors.New("invalid server entitlement")
var ErrUnavailable = errors.New("server entitlement unavailable")
var ErrVersionConflict = errors.New("server entitlement version changed")
var ErrRequestConflict = errors.New("entitlement request already used with different parameters")
var ErrOperatorRequired = errors.New("platform administrator required for operator entitlement changes")

type Policy struct {
	OrganizationID string  `json:"organizationId"`
	ServerID       string  `json:"serverId"`
	CPU            float64 `json:"cpu"`
	MemoryMB       int64   `json:"memoryMb"`
	StartsAtMS     int64   `json:"startsAtMs"`
	EndsAtMS       int64   `json:"endsAtMs"`
	Status         string  `json:"status"`
}

type Record struct {
	Policy
	Version    int64  `json:"version"`
	SourceKind string `json:"sourceKind"`
	SourceID   string `json:"sourceId"`
}

type OperatorChange struct {
	Policy
	ExpectedVersion int64  `json:"expectedVersion"`
	RequestID       string `json:"requestId"`
	Reason          string `json:"reason"`
}

func identifier(s string) bool {
	return s != "" && len(s) <= 128 && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n")
}
func (p Policy) Validate() error {
	if !identifier(p.OrganizationID) || !identifier(p.ServerID) || p.CPU <= 0 || math.IsNaN(p.CPU) || math.IsInf(p.CPU, 0) || p.MemoryMB <= 0 || p.StartsAtMS < 0 || p.EndsAtMS <= p.StartsAtMS || (p.Status != "active" && p.Status != "suspended" && p.Status != "revoked") {
		return ErrInvalid
	}
	return nil
}
func (c OperatorChange) Validate() error {
	if c.Policy.Validate() != nil || c.ExpectedVersion < 0 || c.ExpectedVersion == math.MaxInt64 || !identifier(c.RequestID) || strings.TrimSpace(c.Reason) == "" || len(c.Reason) > 512 {
		return ErrInvalid
	}
	return nil
}
