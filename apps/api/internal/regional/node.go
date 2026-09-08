package regional

import (
	"errors"
	"math"
	"strings"
)

var ErrInvalidNode = errors.New("invalid regional node configuration")
var ErrNodeVersionConflict = errors.New("regional node configuration version changed")

// NodeConfiguration is operator-owned capacity, not a heartbeat or proof of
// free resources. Scheduling must also check current liveness and reservations.
type NodeConfiguration struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Architecture string  `json:"architecture"`
	CPU          float64 `json:"cpu"`
	MemoryMB     int64   `json:"memoryMb"`
	Schedulable  bool    `json:"schedulable"`
}

func (n NodeConfiguration) Validate() error {
	for _, value := range []string{n.ID, n.Name, n.Architecture} {
		if value == "" || len(value) > 128 || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
			return ErrInvalidNode
		}
	}
	if n.CPU <= 0 || math.IsNaN(n.CPU) || math.IsInf(n.CPU, 0) || n.MemoryMB <= 0 {
		return ErrInvalidNode
	}
	return nil
}

type Node struct {
	NodeConfiguration
	Version int64 `json:"version"`
}
