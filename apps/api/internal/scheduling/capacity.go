// Package scheduling owns placement rules shared by candidate selection and
// transactional admission. Callers supply the reservations that still hold capacity.
package scheduling

import (
	"errors"
	"fmt"
	"math"
)

var ErrCapacityUnavailable = errors.New("capacity unavailable")

type Resources struct {
	CPU      float64
	MemoryMB int64
}

// CheckCapacity returns the remaining capacity after accepting request. Every
// existing reservation must have finite positive limits; a desired stop/delete
// is not evidence of release. This function neither reads nor reserves resources.
func CheckCapacity(total, request Resources, reserved []Resources) (Resources, error) {
	if !finite(total) || !finite(request) {
		return Resources{}, fmt.Errorf("%w: known capacity and finite positive limits are required", ErrCapacityUnavailable)
	}
	remaining := total
	consume := func(resources Resources) bool {
		if !finite(resources) || resources.CPU > remaining.CPU || resources.MemoryMB > remaining.MemoryMB {
			return false
		}
		remaining.CPU -= resources.CPU
		remaining.MemoryMB -= resources.MemoryMB
		return true
	}
	for _, resources := range reserved {
		if !consume(resources) {
			return Resources{}, fmt.Errorf("%w: invalid or exhausted existing reservations", ErrCapacityUnavailable)
		}
	}
	if !consume(request) {
		return Resources{}, fmt.Errorf("%w: insufficient CPU or memory", ErrCapacityUnavailable)
	}
	return remaining, nil
}

func finite(resources Resources) bool {
	return resources.CPU > 0 && !math.IsNaN(resources.CPU) && !math.IsInf(resources.CPU, 0) && resources.MemoryMB > 0
}
