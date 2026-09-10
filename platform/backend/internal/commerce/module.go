package commerce

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

var (
	ErrPlanNotFound      = errors.New("plan version not found")
	ErrOrderNotFound     = errors.New("order not found")
	ErrPaymentUnverified = errors.New("payment notification is not verified")
)

type PlanVersion struct {
	ID              contract.PlanVersionID `json:"id"`
	PlanID          contract.PlanID        `json:"planId"`
	Version         int                    `json:"version"`
	Name            string                 `json:"name"`
	PriceMinor      int64                  `json:"priceMinor"`
	Currency        string                 `json:"currency"`
	BillingPeriod   string                 `json:"billingPeriod"`
	RegionIDs       []contract.RegionID    `json:"regionIds"`
	MemoryMegabytes int                    `json:"memoryMegabytes"`
	CPUUnits        int                    `json:"cpuUnits"`
}

type OrderStatus string

const (
	OrderPendingPayment OrderStatus = "pending_payment"
	OrderPaid           OrderStatus = "paid"
)

type Order struct {
	ID                contract.OrderID           `json:"id"`
	WorkspaceID       contract.WorkspaceID       `json:"workspaceId"`
	LogicalInstanceID contract.LogicalInstanceID `json:"logicalInstanceId"`
	PlanVersionID     contract.PlanVersionID     `json:"planVersionId"`
	Status            OrderStatus                `json:"status"`
	ExpiresAt         time.Time                  `json:"expiresAt"`
}

type Payment struct {
	ID                     contract.PaymentID `json:"id"`
	OrderID                contract.OrderID   `json:"orderId"`
	ProviderNotificationID string             `json:"providerNotificationId"`
	VerifiedAt             time.Time          `json:"verifiedAt"`
}

type Entitlement struct {
	ID                contract.EntitlementID     `json:"id"`
	WorkspaceID       contract.WorkspaceID       `json:"workspaceId"`
	LogicalInstanceID contract.LogicalInstanceID `json:"logicalInstanceId"`
	PlanVersionID     contract.PlanVersionID     `json:"planVersionId"`
	EffectiveAt       time.Time                  `json:"effectiveAt"`
	ExpiresAt         time.Time                  `json:"expiresAt"`
	Active            bool                       `json:"active"`
}

type PreparedOrder struct{ Order Order }

type Module struct {
	mu              sync.RWMutex
	plans           map[contract.PlanVersionID]PlanVersion
	orders          map[contract.OrderID]Order
	payments        map[string]Payment
	entitlements    map[contract.LogicalInstanceID]Entitlement
	nextOrder       int
	nextPayment     int
	nextEntitlement int
}

func New(plans []PlanVersion) *Module {
	module := &Module{
		plans:        make(map[contract.PlanVersionID]PlanVersion),
		orders:       make(map[contract.OrderID]Order),
		payments:     make(map[string]Payment),
		entitlements: make(map[contract.LogicalInstanceID]Entitlement),
	}
	for _, plan := range plans {
		plan.RegionIDs = append([]contract.RegionID(nil), plan.RegionIDs...)
		module.plans[plan.ID] = plan
	}
	return module
}

func (m *Module) Plans(_ context.Context) []PlanVersion {
	m.mu.RLock()
	defer m.mu.RUnlock()
	plans := make([]PlanVersion, 0, len(m.plans))
	for _, plan := range m.plans {
		plans = append(plans, plan)
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].ID < plans[j].ID })
	return plans
}

func (m *Module) Plan(_ context.Context, planVersionID contract.PlanVersionID) (PlanVersion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	plan, ok := m.plans[planVersionID]
	if !ok {
		return PlanVersion{}, ErrPlanNotFound
	}
	return plan, nil
}

func (m *Module) PrepareOrder(workspaceID contract.WorkspaceID, instanceID contract.LogicalInstanceID, planVersionID contract.PlanVersionID, now time.Time) (PreparedOrder, error) {
	if _, err := m.Plan(context.Background(), planVersionID); err != nil {
		return PreparedOrder{}, err
	}
	m.mu.Lock()
	m.nextOrder++
	nextOrder := m.nextOrder
	m.mu.Unlock()
	return PrepareOrderWithID(contract.OrderID(formatID("ord", nextOrder)), workspaceID, instanceID, planVersionID, now), nil
}

func PrepareOrderWithID(orderID contract.OrderID, workspaceID contract.WorkspaceID, instanceID contract.LogicalInstanceID, planVersionID contract.PlanVersionID, now time.Time) PreparedOrder {
	return PreparedOrder{Order: Order{ID: orderID, WorkspaceID: workspaceID, LogicalInstanceID: instanceID, PlanVersionID: planVersionID, Status: OrderPendingPayment, ExpiresAt: now.Add(30 * time.Minute)}}
}

func (m *Module) CommitOrder(prepared PreparedOrder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders[prepared.Order.ID] = prepared.Order
}

func (m *Module) OrdersForWorkspace(_ context.Context, workspaceID contract.WorkspaceID) []Order {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var orders []Order
	for _, order := range m.orders {
		if order.WorkspaceID == workspaceID {
			orders = append(orders, order)
		}
	}
	sort.Slice(orders, func(i, j int) bool { return orders[i].ID < orders[j].ID })
	return orders
}

func (m *Module) AllOrders(_ context.Context) []Order {
	m.mu.RLock()
	defer m.mu.RUnlock()
	orders := make([]Order, 0, len(m.orders))
	for _, order := range m.orders {
		orders = append(orders, order)
	}
	sort.Slice(orders, func(i, j int) bool { return orders[i].ID < orders[j].ID })
	return orders
}

func (m *Module) VerifyPayment(orderID contract.OrderID, notificationID string, verified bool, now time.Time) (Payment, Entitlement, error) {
	if !verified {
		return Payment{}, Entitlement{}, ErrPaymentUnverified
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if payment, ok := m.payments[notificationID]; ok {
		return payment, m.entitlements[m.orders[payment.OrderID].LogicalInstanceID], nil
	}
	order, ok := m.orders[orderID]
	if !ok {
		return Payment{}, Entitlement{}, ErrOrderNotFound
	}
	m.nextPayment++
	m.nextEntitlement++
	payment := Payment{ID: contract.PaymentID(formatID("pay", m.nextPayment)), OrderID: orderID, ProviderNotificationID: notificationID, VerifiedAt: now}
	entitlement := Entitlement{
		ID: contract.EntitlementID(formatID("ent", m.nextEntitlement)), WorkspaceID: order.WorkspaceID,
		LogicalInstanceID: order.LogicalInstanceID, PlanVersionID: order.PlanVersionID, EffectiveAt: now,
		ExpiresAt: now.Add(30 * 24 * time.Hour), Active: true,
	}
	order.Status = OrderPaid
	m.orders[orderID] = order
	m.payments[notificationID] = payment
	m.entitlements[order.LogicalInstanceID] = entitlement
	return payment, entitlement, nil
}

func (m *Module) ActiveEntitlement(instanceID contract.LogicalInstanceID, now time.Time) (Entitlement, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entitlement, ok := m.entitlements[instanceID]
	return entitlement, ok && entitlement.Active && entitlement.EffectiveAt.After(now) == false && entitlement.ExpiresAt.After(now)
}

func (m *Module) CountOrders() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.orders)
}

func formatID(prefix string, value int) string {
	return fmt.Sprintf("%s_%06d", prefix, value)
}
