// Package commerce owns prepaid product pricing and commercial service terms.
// Execution authorization and regional resource allocation are separate concerns.
package commerce

import (
	"errors"
	"math"
	"strings"
)

var ErrInvalidPlan = errors.New("invalid prepaid plan version")
var ErrInvalidQuote = errors.New("invalid prepaid quote")

// PlanVersion is an immutable catalog version. Currency uses an uppercase
// three-letter code; UnitAmountMinor uses that currency's configured minor unit.
// PeriodSeconds is an explicit fixed-duration product term, not a calendar month.
// Changing any commercial field requires publishing a new version.
type PlanVersion struct {
	PlanID               string  `json:"planId"`
	Version              int64   `json:"version"`
	ProviderKey          string  `json:"providerKey"`
	RegionID             string  `json:"regionId"`
	CPU                  float64 `json:"cpu"`
	MemoryMB             int64   `json:"memoryMb"`
	StorageBytes         int64   `json:"storageBytes"`
	BackupRetentionCount int64   `json:"backupRetentionCount"`
	Currency             string  `json:"currency"`
	UnitAmountMinor      int64   `json:"unitAmountMinor"`
	PeriodSeconds        int64   `json:"periodSeconds"`
}

func validID(s string) bool {
	return s != "" && len(s) <= 128 && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n")
}

func (p PlanVersion) Validate() error {
	if !validID(p.PlanID) || p.Version < 1 || !validID(p.ProviderKey) || !validID(p.RegionID) || p.CPU <= 0 || math.IsNaN(p.CPU) || math.IsInf(p.CPU, 0) || p.MemoryMB <= 0 || p.StorageBytes <= 0 || p.BackupRetentionCount < 0 || p.UnitAmountMinor < 0 || p.PeriodSeconds <= 0 || p.PeriodSeconds > math.MaxInt64/1000 || len(p.Currency) != 3 {
		return ErrInvalidPlan
	}
	for _, c := range p.Currency {
		if c < 'A' || c > 'Z' {
			return ErrInvalidPlan
		}
	}
	return nil
}

// Quote captures the complete commercial version so a later catalog change
// cannot silently change an order. It neither proves payment nor grants service.
type Quote struct {
	Plan        PlanVersion `json:"plan"`
	Periods     int64       `json:"periods"`
	AmountMinor int64       `json:"amountMinor"`
	DurationMS  int64       `json:"durationMs"`
}

// QuotePrepaid prices one instance for a positive number of fixed periods.
// Catalog lookup and whether a version is still sold belong to the caller;
// clients must not supply an untrusted PlanVersion as a checkout price.
func QuotePrepaid(plan PlanVersion, periods int64) (Quote, error) {
	if plan.Validate() != nil {
		return Quote{}, ErrInvalidPlan
	}
	if periods <= 0 || (plan.UnitAmountMinor > 0 && periods > math.MaxInt64/plan.UnitAmountMinor) || periods > math.MaxInt64/(plan.PeriodSeconds*1000) {
		return Quote{}, ErrInvalidQuote
	}
	return Quote{Plan: plan, Periods: periods, AmountMinor: plan.UnitAmountMinor * periods, DurationMS: plan.PeriodSeconds * 1000 * periods}, nil
}

var ErrPlanConflict = errors.New("published plan version has different terms")
var ErrPlanUnavailable = errors.New("plan version is not available for sale")
var ErrCatalogVersionConflict = errors.New("plan sale version changed")
var ErrOperatorRequired = errors.New("platform administrator required for catalog changes")
