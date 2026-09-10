package globalproduct

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/commerce"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instancecontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regiondirectory"
)

var (
	ErrPlanUnavailableInRegion = errors.New("plan is unavailable in region")
	ErrActiveEntitlementNeeded = errors.New("active entitlement required")
)

type CreateCommand struct {
	Identity      contract.CommandIdentity
	WorkspaceID   contract.WorkspaceID
	PlanVersionID contract.PlanVersionID
	RegionID      contract.RegionID
	Name          string
	GameKey       string
	GameVersion   string
	Configuration map[string]any
}

type CheckoutResult struct {
	Instance instancecontrol.LogicalInstance `json:"instance"`
	Order    commerce.Order                  `json:"order"`
}

type ActivationResult struct {
	Payment     commerce.Payment     `json:"payment"`
	Entitlement commerce.Entitlement `json:"entitlement"`
}

type Module struct {
	mu          sync.Mutex
	regions     *regiondirectory.Module
	commerce    *commerce.Module
	instances   *instancecontrol.Module
	messaging   *messaging.Module
	checkouts   map[contract.IdempotencyKey]CheckoutResult
	activations map[string]ActivationResult
}

func New(regions *regiondirectory.Module, commerceModule *commerce.Module, instances *instancecontrol.Module, messages *messaging.Module) *Module {
	return &Module{
		regions: regions, commerce: commerceModule, instances: instances, messaging: messages,
		checkouts: make(map[contract.IdempotencyKey]CheckoutResult), activations: make(map[string]ActivationResult),
	}
}

func (m *Module) CreateCheckout(ctx context.Context, command CreateCommand, now time.Time) (CheckoutResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if previous, ok := m.checkouts[command.Identity.IdempotencyKey]; ok {
		return previous, nil
	}
	if _, err := m.regions.RequireAvailable(ctx, command.RegionID); err != nil {
		return CheckoutResult{}, err
	}
	plan, err := m.commerce.Plan(ctx, command.PlanVersionID)
	if err != nil {
		return CheckoutResult{}, err
	}
	if !containsRegion(plan.RegionIDs, command.RegionID) {
		return CheckoutResult{}, ErrPlanUnavailableInRegion
	}
	preparedInstance, err := m.instances.PrepareCreate(instancecontrol.CreateCommand{
		WorkspaceID: command.WorkspaceID, Name: command.Name, GameKey: command.GameKey,
		GameVersion: command.GameVersion, Configuration: command.Configuration, RegionID: command.RegionID,
	}, now)
	if err != nil {
		return CheckoutResult{}, err
	}
	preparedOrder, err := m.commerce.PrepareOrder(command.WorkspaceID, preparedInstance.Instance.ID, command.PlanVersionID, now)
	if err != nil {
		return CheckoutResult{}, err
	}
	m.instances.CommitCreate(preparedInstance)
	m.commerce.CommitOrder(preparedOrder)
	result := CheckoutResult{Instance: preparedInstance.Instance, Order: preparedOrder.Order}
	m.checkouts[command.Identity.IdempotencyKey] = result
	return result, nil
}

func (m *Module) ActivateVerifiedPayment(_ context.Context, orderID contract.OrderID, notificationID string, verified bool, now time.Time) (ActivationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if previous, ok := m.activations[notificationID]; ok {
		return previous, nil
	}
	payment, entitlement, err := m.commerce.VerifyPayment(orderID, notificationID, verified, now)
	if err != nil {
		return ActivationResult{}, err
	}
	if err := m.instances.Activate(entitlement.LogicalInstanceID); err != nil {
		return ActivationResult{}, err
	}
	key := contract.IdempotencyKey(notificationID)
	m.messaging.Publish("entitlement.changed.v1", key, map[string]any{
		"workspaceId":       entitlement.WorkspaceID,
		"logicalInstanceId": entitlement.LogicalInstanceID,
		"entitlementId":     entitlement.ID,
		"status":            "active",
		"effectiveAt":       entitlement.EffectiveAt,
		"expiresAt":         entitlement.ExpiresAt,
	}, now)
	m.publishDeployment(entitlement.LogicalInstanceID, key, now)
	result := ActivationResult{Payment: payment, Entitlement: entitlement}
	m.activations[notificationID] = result
	return result, nil
}

func (m *Module) RequestDeployment(_ context.Context, instanceID contract.LogicalInstanceID, key contract.IdempotencyKey, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, active := m.commerce.ActiveEntitlement(instanceID, now); !active {
		return ErrActiveEntitlementNeeded
	}
	m.publishDeployment(instanceID, key, now)
	return nil
}

func (m *Module) Regions(ctx context.Context) ([]regiondirectory.Region, error) {
	return m.regions.List(ctx), nil
}

func (m *Module) Plans(ctx context.Context) ([]commerce.PlanVersion, error) {
	return m.commerce.Plans(ctx), nil
}

func (m *Module) Instances(ctx context.Context, workspaceID contract.WorkspaceID) ([]instancecontrol.LogicalInstance, error) {
	return m.instances.List(ctx, workspaceID), nil
}

func (m *Module) Instance(ctx context.Context, workspaceID contract.WorkspaceID, instanceID contract.LogicalInstanceID) (instancecontrol.Detail, error) {
	return m.instances.Get(ctx, workspaceID, instanceID)
}

func (m *Module) Orders(ctx context.Context, workspaceID contract.WorkspaceID) ([]commerce.Order, error) {
	return m.commerce.OrdersForWorkspace(ctx, workspaceID), nil
}

func (m *Module) AllInstances(ctx context.Context) ([]instancecontrol.LogicalInstance, error) {
	return m.instances.All(ctx), nil
}

func (m *Module) AllOrders(ctx context.Context) ([]commerce.Order, error) {
	return m.commerce.AllOrders(ctx), nil
}

func (m *Module) publishDeployment(instanceID contract.LogicalInstanceID, key contract.IdempotencyKey, now time.Time) {
	detail, err := m.instances.GetByID(context.Background(), instanceID)
	if err != nil {
		// Checkout invariants ensure an Entitlement always names an existing instance.
		return
	}
	m.messaging.Publish("deployment.desired.v1", key, map[string]any{
		"workspaceId":        detail.Instance.WorkspaceID,
		"logicalInstanceId":  instanceID,
		"regionId":           detail.Placement.RegionID,
		"placementVersion":   detail.Placement.Version,
		"instanceRevisionId": detail.Revision.ID,
		"desiredState":       detail.Instance.DesiredState,
	}, now)
}

func containsRegion(regionIDs []contract.RegionID, regionID contract.RegionID) bool {
	for _, candidate := range regionIDs {
		if candidate == regionID {
			return true
		}
	}
	return false
}
