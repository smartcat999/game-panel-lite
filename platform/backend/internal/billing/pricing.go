package billing

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"time"
)

var allResourceKinds = []ResourceKind{ResourceCPU, ResourceMemory, ResourceInstanceDisk, ResourceBackupStorage, ResourceDedicatedIP}

func validateCatalog(catalog RegionCatalog) error {
	if catalog.ID == "" || catalog.RegionID == "" || catalog.Version < 1 || catalog.EffectiveAt.IsZero() || catalog.CreatedAt.IsZero() || !validRange(catalog.CPU) || !validRange(catalog.Memory) || !validRange(catalog.Disk) || len(catalog.EndpointDeliveryModes) == 0 {
		return ErrInvalidCatalog
	}
	seenModes := map[string]bool{}
	for _, mode := range catalog.EndpointDeliveryModes {
		if seenModes[mode] || mode != "gateway" && mode != "dedicated-ip" && mode != "node-direct" {
			return ErrInvalidCatalog
		}
		seenModes[mode] = true
	}
	switch catalog.Availability {
	case AvailabilityAvailable, AvailabilityLimited, AvailabilityUnavailable:
		return nil
	default:
		return ErrInvalidCatalog
	}
}

func validRange(value Range) bool {
	return value.Minimum > 0 && value.Maximum >= value.Minimum && value.Step > 0 && (value.Maximum-value.Minimum)%value.Step == 0
}

func validatePriceBook(book PriceBook) error {
	if book.ID == "" || book.RegionID == "" || book.Revision < 1 || book.Currency != CurrencyCNY || book.EffectiveAt.IsZero() || book.CreatedAt.IsZero() || len(book.UnitPrices) != len(allResourceKinds) {
		return ErrInvalidPriceBook
	}
	seen := map[ResourceKind]bool{}
	for _, item := range book.UnitPrices {
		if item.PriceMinor < 0 || item.UnitQuantity <= 0 || item.Unit == "" || seen[item.ResourceKind] {
			return ErrInvalidPriceBook
		}
		seen[item.ResourceKind] = true
	}
	for _, kind := range allResourceKinds {
		if !seen[kind] {
			return ErrInvalidPriceBook
		}
	}
	return nil
}

func validateSpec(catalog RegionCatalog, spec ResourceSpec) error {
	if !rangeContains(catalog.CPU, spec.CPUMilli) || !rangeContains(catalog.Memory, spec.MemoryMiB) || !rangeContains(catalog.Disk, spec.DiskGiB) {
		return ErrInvalidResourceSpec
	}
	return nil
}

func rangeContains(bounds Range, value int64) bool {
	return value >= bounds.Minimum && value <= bounds.Maximum && (value-bounds.Minimum)%bounds.Step == 0
}

func hourlyPrice(book PriceBook, spec ResourceSpec, dedicatedIP bool) (int64, error) {
	quantities := map[ResourceKind]int64{ResourceCPU: spec.CPUMilli, ResourceMemory: spec.MemoryMiB, ResourceInstanceDisk: spec.DiskGiB}
	if dedicatedIP {
		quantities[ResourceDedicatedIP] = 1
	}
	var total int64
	for kind, quantity := range quantities {
		amount, err := intervalPrice(book, kind, quantity, time.Hour)
		if err != nil {
			return 0, err
		}
		if total > math.MaxInt64-amount {
			return 0, ErrInvalidPriceBook
		}
		total += amount
	}
	return total, nil
}

func intervalPrice(book PriceBook, kind ResourceKind, quantity int64, duration time.Duration) (int64, error) {
	var price *UnitPrice
	for index := range book.UnitPrices {
		if book.UnitPrices[index].ResourceKind == kind {
			price = &book.UnitPrices[index]
			break
		}
	}
	if price == nil || quantity < 0 || duration <= 0 {
		return 0, ErrInvalidPriceBook
	}
	numerator, ok := safeMultiply(price.PriceMinor, quantity)
	if !ok {
		return 0, ErrInvalidPriceBook
	}
	numerator, ok = safeMultiply(numerator, int64(duration))
	if !ok {
		return 0, ErrInvalidPriceBook
	}
	denominator, ok := safeMultiply(price.UnitQuantity, int64(time.Hour))
	if !ok || denominator <= 0 || numerator > math.MaxInt64-denominator+1 {
		return 0, ErrInvalidPriceBook
	}
	return (numerator + denominator - 1) / denominator, nil
}

func safeMultiply(left, right int64) (int64, bool) {
	if left < 0 || right < 0 || left != 0 && right > math.MaxInt64/left {
		return 0, false
	}
	return left * right, true
}

func safeAdd(left, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right || right < 0 && left < math.MinInt64-right {
		return 0, false
	}
	return left + right, true
}

func usageRecordID(instanceID string, kind ResourceKind, start, end time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d", instanceID, kind, start.UTC().UnixNano(), end.UTC().UnixNano())))
	return "use_" + hex.EncodeToString(sum[:12])
}
