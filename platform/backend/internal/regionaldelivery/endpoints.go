package regionaldelivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"sort"
	"strconv"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

func chooseEndpoints(state State, pools []EndpointPool, used map[string]bool, now time.Time) ([]allocationRow, error) {
	var result []allocationRow
	for _, listener := range state.ListenerRequirements {
		var candidates []EndpointPool
		for _, pool := range pools {
			if listener.AddressMode == "ip-only" && pool.DeliveryMode == "dedicated-ip" || listener.AddressMode == "ip-port" && (pool.DeliveryMode == "gateway" || pool.DeliveryMode == "node-direct") {
				candidates = append(candidates, pool)
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			return modeRank(candidates[i].DeliveryMode) < modeRank(candidates[j].DeliveryMode)
		})
		var chosen *allocationRow
		for _, pool := range candidates {
			if listener.AddressMode == "ip-only" {
				key := "ip:" + pool.Address
				if used[key] {
					continue
				}
				row, err := newAllocation(state, listener, pool, nil, key, now)
				if err != nil {
					return nil, err
				}
				chosen = &row
				break
			}
			start, end := *pool.PortStart, *pool.PortEnd
			if listener.ExternalPortPolicy == "default-required" {
				if listener.InternalPort < start || listener.InternalPort > end {
					continue
				}
				start, end = listener.InternalPort, listener.InternalPort
			}
			for port := start; port <= end; port++ {
				key := pool.Address + ":" + strconv.Itoa(port)
				if used[key] {
					continue
				}
				value := port
				row, err := newAllocation(state, listener, pool, &value, key, now)
				if err != nil {
					return nil, err
				}
				chosen = &row
				break
			}
			if chosen != nil {
				break
			}
		}
		if chosen == nil {
			return nil, ErrNoEndpoint
		}
		used[chosen.AllocationKey] = true
		result = append(result, *chosen)
	}
	return result, nil
}

type allocationRow struct {
	ID                 string   `json:"id"`
	RegionalDeliveryID string   `json:"regionalDeliveryId"`
	LogicalInstanceID  string   `json:"logicalInstanceId"`
	PoolID             string   `json:"poolId"`
	ListenerName       string   `json:"listenerName"`
	Purpose            string   `json:"purpose"`
	Address            string   `json:"address"`
	Port               *int     `json:"port"`
	Transports         []string `json:"transports"`
	Stability          string   `json:"stability"`
	DisplayAddress     string   `json:"displayAddress"`
	IsPrimary          bool     `json:"isPrimary"`
	AllocationKey      string   `json:"allocationKey"`
}

func newAllocation(state State, listener deliverycontrol.ListenerRequirement, pool EndpointPool, port *int, key string, _ time.Time) (allocationRow, error) {
	id, err := persistence.NewID("ep")
	if err != nil {
		return allocationRow{}, err
	}
	display := pool.Address
	if port != nil {
		display = net.JoinHostPort(pool.Address, strconv.Itoa(*port))
	}
	return allocationRow{ID: id, RegionalDeliveryID: state.ID, LogicalInstanceID: state.LogicalInstanceID, PoolID: pool.ID, ListenerName: listener.Name, Purpose: listener.Purpose, Address: pool.Address, Port: port, Transports: append([]string(nil), listener.Transports...), Stability: pool.Stability, DisplayAddress: display, IsPrimary: listener.Primary, AllocationKey: key}, nil
}

func modeRank(mode string) int {
	if mode == "gateway" {
		return 0
	}
	if mode == "dedicated-ip" {
		return 1
	}
	return 2
}

func loadPools(ctx context.Context, tx *sql.Tx, regionID string) ([]EndpointPool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,region_id,delivery_mode,address,port_start,port_end,stability,active FROM endpoint_pools WHERE region_id=$1 AND active=true ORDER BY id FOR UPDATE`, regionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []EndpointPool
	for rows.Next() {
		var pool EndpointPool
		var start, end sql.NullInt64
		if err := rows.Scan(&pool.ID, &pool.RegionID, &pool.DeliveryMode, &pool.Address, &start, &end, &pool.Stability, &pool.Active); err != nil {
			return nil, err
		}
		if start.Valid {
			v := int(start.Int64)
			pool.PortStart = &v
		}
		if end.Valid {
			v := int(end.Int64)
			pool.PortEnd = &v
		}
		result = append(result, pool)
	}
	return result, rows.Err()
}

func loadActiveAllocationKeys(ctx context.Context, tx *sql.Tx, poolIDs []string) (map[string]bool, error) {
	result := map[string]bool{}
	if len(poolIDs) == 0 {
		return result, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT allocation_key FROM endpoint_allocations WHERE pool_id=ANY($1) AND active=true`, poolIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = true
	}
	return result, rows.Err()
}

func endpointsFor(ctx context.Context, query persistence.DBTX, deliveryID string) ([]deliverycontrol.EndpointBinding, error) {
	rows, err := query.QueryContext(ctx, `SELECT listener_name,purpose,address,port,transports,stability,display_address,is_primary FROM endpoint_allocations WHERE regional_delivery_id=$1 AND active=true ORDER BY is_primary DESC,listener_name`, deliveryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []deliverycontrol.EndpointBinding
	for rows.Next() {
		var item deliverycontrol.EndpointBinding
		var port sql.NullInt64
		var transports []byte
		if err := rows.Scan(&item.Name, &item.Purpose, &item.Address, &port, &transports, &item.Stability, &item.DisplayAddress, &item.Primary); err != nil {
			return nil, err
		}
		if port.Valid {
			v := int(port.Int64)
			item.Port = &v
		}
		if err := json.Unmarshal(transports, &item.Transports); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
