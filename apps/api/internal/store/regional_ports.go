package store

import (
	"encoding/json"
	"slices"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"gorm.io/gorm"
)

type regionalAllocationRow struct {
	ID string
	regional.CapacityRequest
	CPU      float64
	MemoryMB int64
	Status   string
	Ports    string
}

func (row regionalAllocationRow) allocation() (regional.Allocation, error) {
	var ports []workload.Port
	if json.Unmarshal([]byte(row.Ports), &ports) != nil {
		return regional.Allocation{}, regional.ErrAllocationConflict
	}
	return regional.Allocation{ID: row.ID, CapacityRequest: row.CapacityRequest, CPU: row.CPU, MemoryMB: row.MemoryMB, Status: row.Status, Ports: ports}, nil
}

type regionalPortRow struct {
	AllocationID, NodeID    string
	HostPort, ContainerPort int
	Protocol, Status        string
}

type regionalPortKey struct {
	HostPort int
	Protocol string
}

func regionalBindings(network workload.Network) ([]workload.Port, error) {
	if len(network.AdditionalPorts) > 128 {
		return nil, workload.ErrInvalidNetwork
	}
	ports, err := workload.ResolvePortBindings(network)
	if err != nil {
		return nil, err
	}
	if len(ports) > 128 {
		return nil, workload.ErrInvalidNetwork
	}
	return ports, nil
}

// Read requested host ports once for a bounded candidate page and plan set.
func regionalReservedPorts(tx *gorm.DB, nodeIDs []string, ports []workload.Port) (map[string]map[regionalPortKey]bool, error) {
	reserved := map[string]map[regionalPortKey]bool{}
	if len(ports) == 0 {
		return reserved, nil
	}
	hosts := []int{}
	seenHosts := map[int]bool{}
	wanted := make(map[regionalPortKey]bool, len(ports))
	for _, port := range ports {
		if !seenHosts[port.HostPort] {
			hosts = append(hosts, port.HostPort)
			seenHosts[port.HostPort] = true
		}
		wanted[regionalPortKey{port.HostPort, port.Protocol}] = true
	}
	var rows []regionalPortRow
	if err := tx.Table("regional_port_reservations").Select("node_id,host_port,protocol").Where("node_id IN ? AND host_port IN ? AND status = ?", nodeIDs, hosts, "reserved").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		key := regionalPortKey{row.HostPort, row.Protocol}
		if wanted[key] {
			if reserved[row.NodeID] == nil {
				reserved[row.NodeID] = map[regionalPortKey]bool{}
			}
			reserved[row.NodeID][key] = true
		}
	}
	return reserved, nil
}

func regionalPortConflicts(tx *gorm.DB, nodeIDs []string, ports []workload.Port) (map[string]bool, error) {
	reserved, err := regionalReservedPorts(tx, nodeIDs, ports)
	if err != nil {
		return nil, err
	}
	conflicts := map[string]bool{}
	for node, bindings := range reserved {
		conflicts[node] = len(bindings) > 0
	}
	return conflicts, nil
}

func saveRegionalPorts(tx *gorm.DB, allocation regional.Allocation) error {
	if len(allocation.Ports) == 0 {
		return nil
	}
	rows := make([]regionalPortRow, 0, len(allocation.Ports))
	for _, port := range allocation.Ports {
		rows = append(rows, regionalPortRow{AllocationID: allocation.ID, NodeID: allocation.NodeID, HostPort: port.HostPort, ContainerPort: port.Port, Protocol: port.Protocol, Status: "reserved"})
	}
	return tx.Table("regional_port_reservations").Create(&rows).Error
}

func checkRegionalPortReceipt(tx *gorm.DB, allocation regional.Allocation, ports []workload.Port) error {
	if !slices.Equal(allocation.Ports, ports) {
		return regional.ErrAllocationConflict
	}
	var rows []regionalPortRow
	if err := tx.Table("regional_port_reservations").Where("allocation_id = ? AND status = ?", allocation.ID, "reserved").Order("host_port,protocol").Limit(129).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(ports) {
		return regional.ErrAllocationConflict
	}
	for i, row := range rows {
		if row.NodeID != allocation.NodeID || row.HostPort != ports[i].HostPort || row.ContainerPort != ports[i].Port || row.Protocol != ports[i].Protocol {
			return regional.ErrAllocationConflict
		}
	}
	return nil
}
