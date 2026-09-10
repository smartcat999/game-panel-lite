package globalproduct

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/commerce"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instancecontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regiondirectory"
)

type Postgres struct {
	db        *sql.DB
	regions   *regiondirectory.Postgres
	commerce  *commerce.Postgres
	instances *instancecontrol.Postgres
	messages  *messaging.Postgres
}

func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db, regions: regiondirectory.NewPostgres(db), commerce: commerce.NewPostgres(db), instances: instancecontrol.NewPostgres(db), messages: messaging.NewPostgres(db)}
}

func (p *Postgres) CreateCheckout(ctx context.Context, command CreateCommand, now time.Time) (CheckoutResult, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return CheckoutResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, command.Identity.IdempotencyKey); err != nil {
		return CheckoutResult{}, err
	}
	var previousJSON []byte
	err = tx.QueryRowContext(ctx, `SELECT result FROM command_results WHERE idempotency_key = $1`, command.Identity.IdempotencyKey).Scan(&previousJSON)
	if err == nil {
		var previous CheckoutResult
		if err := json.Unmarshal(previousJSON, &previous); err != nil {
			return CheckoutResult{}, err
		}
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return CheckoutResult{}, err
	}
	if _, err := p.regions.RequireAvailable(ctx, tx, command.RegionID); err != nil {
		return CheckoutResult{}, err
	}
	plan, err := p.commerce.Plan(ctx, tx, command.PlanVersionID)
	if err != nil {
		return CheckoutResult{}, err
	}
	if !containsRegion(plan.RegionIDs, command.RegionID) {
		return CheckoutResult{}, ErrPlanUnavailableInRegion
	}
	instanceID, revisionID, placementID, orderID, err := checkoutIDs()
	if err != nil {
		return CheckoutResult{}, err
	}
	preparedInstance, err := instancecontrol.PrepareCreateWithIDs(instancecontrol.CreateCommand{
		WorkspaceID: command.WorkspaceID, Name: command.Name, GameKey: command.GameKey, GameVersion: command.GameVersion,
		Configuration: command.Configuration, RegionID: command.RegionID,
	}, instancecontrol.CreateIDs{InstanceID: instanceID, RevisionID: revisionID, PlacementID: placementID}, now)
	if err != nil {
		return CheckoutResult{}, err
	}
	preparedOrder := commerce.PrepareOrderWithID(orderID, command.WorkspaceID, instanceID, command.PlanVersionID, now)
	if err := p.instances.InsertCreate(ctx, tx, preparedInstance); err != nil {
		return CheckoutResult{}, err
	}
	if err := p.commerce.InsertOrder(ctx, tx, preparedOrder.Order, now); err != nil {
		return CheckoutResult{}, err
	}
	result := CheckoutResult{Instance: preparedInstance.Instance, Order: preparedOrder.Order}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CheckoutResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO command_results (idempotency_key, command_id, result, created_at) VALUES ($1, $2, $3, $4)`, command.Identity.IdempotencyKey, command.Identity.CommandID, resultJSON, now); err != nil {
		return CheckoutResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CheckoutResult{}, err
	}
	return result, nil
}

func (p *Postgres) ActivateVerifiedPayment(ctx context.Context, orderID contract.OrderID, notificationID string, verified bool, now time.Time) (ActivationResult, error) {
	if !verified {
		return ActivationResult{}, commerce.ErrPaymentUnverified
	}
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ActivationResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, notificationID); err != nil {
		return ActivationResult{}, err
	}
	if payment, entitlement, found, err := p.commerce.ActivationByNotification(ctx, tx, notificationID); err != nil {
		return ActivationResult{}, err
	} else if found {
		return ActivationResult{Payment: payment, Entitlement: entitlement}, nil
	}
	order, err := p.commerce.Order(ctx, tx, orderID)
	if err != nil {
		return ActivationResult{}, err
	}
	paymentID, err := persistence.NewID("pay")
	if err != nil {
		return ActivationResult{}, err
	}
	entitlementID, err := persistence.NewID("ent")
	if err != nil {
		return ActivationResult{}, err
	}
	payment := commerce.Payment{ID: contract.PaymentID(paymentID), OrderID: order.ID, ProviderNotificationID: notificationID, VerifiedAt: now}
	entitlement := commerce.Entitlement{ID: contract.EntitlementID(entitlementID), WorkspaceID: order.WorkspaceID, LogicalInstanceID: order.LogicalInstanceID, PlanVersionID: order.PlanVersionID, EffectiveAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour), Active: true}
	if err := p.commerce.InsertActivation(ctx, tx, payment, entitlement); err != nil {
		return ActivationResult{}, err
	}
	if err := p.instances.Activate(ctx, tx, order.LogicalInstanceID); err != nil {
		return ActivationResult{}, err
	}
	if err := p.insertEntitlementEvent(ctx, tx, entitlement, notificationID, now); err != nil {
		return ActivationResult{}, err
	}
	if err := p.insertDeploymentEvent(ctx, tx, order.LogicalInstanceID, contract.IdempotencyKey(notificationID), now); err != nil {
		return ActivationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ActivationResult{}, err
	}
	return ActivationResult{Payment: payment, Entitlement: entitlement}, nil
}

func (p *Postgres) RequestDeployment(ctx context.Context, instanceID contract.LogicalInstanceID, key contract.IdempotencyKey, now time.Time) error {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, active, err := p.commerce.ActiveEntitlement(ctx, tx, instanceID, now); err != nil {
		return err
	} else if !active {
		return ErrActiveEntitlementNeeded
	}
	if err := p.insertDeploymentEvent(ctx, tx, instanceID, key, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) Regions(ctx context.Context) ([]regiondirectory.Region, error) {
	return p.regions.List(ctx)
}
func (p *Postgres) Plans(ctx context.Context) ([]commerce.PlanVersion, error) {
	return p.commerce.Plans(ctx)
}
func (p *Postgres) Instances(ctx context.Context, workspaceID contract.WorkspaceID) ([]instancecontrol.LogicalInstance, error) {
	instances, err := p.instances.List(ctx, &workspaceID)
	if err != nil {
		return nil, err
	}
	return p.composeSummaries(ctx, instances)
}
func (p *Postgres) Instance(ctx context.Context, workspaceID contract.WorkspaceID, instanceID contract.LogicalInstanceID) (instancecontrol.Detail, error) {
	detail, err := p.instances.Detail(ctx, p.db, &workspaceID, instanceID)
	if err != nil {
		return instancecontrol.Detail{}, err
	}
	instances, err := p.composeSummaries(ctx, []instancecontrol.LogicalInstance{detail.Instance})
	if err != nil {
		return instancecontrol.Detail{}, err
	}
	detail.Instance = instances[0]
	return detail, nil
}
func (p *Postgres) Orders(ctx context.Context, workspaceID contract.WorkspaceID) ([]commerce.Order, error) {
	return p.commerce.Orders(ctx, &workspaceID)
}
func (p *Postgres) AllInstances(ctx context.Context) ([]instancecontrol.LogicalInstance, error) {
	instances, err := p.instances.List(ctx, nil)
	if err != nil {
		return nil, err
	}
	return p.composeSummaries(ctx, instances)
}

func (p *Postgres) composeSummaries(ctx context.Context, instances []instancecontrol.LogicalInstance) ([]instancecontrol.LogicalInstance, error) {
	ids := make([]contract.LogicalInstanceID, 0, len(instances))
	for _, instance := range instances {
		ids = append(ids, instance.ID)
	}
	summaries, err := p.instances.DeploymentSummaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[contract.LogicalInstanceID]instancecontrol.DeploymentSummary, len(summaries))
	for _, summary := range summaries {
		byID[summary.LogicalInstanceID] = summary
	}
	for index := range instances {
		if summary, ok := byID[instances[index].ID]; ok {
			instances[index] = instancecontrol.ApplySummary(instances[index], summary)
		}
	}
	return instances, nil
}
func (p *Postgres) AllOrders(ctx context.Context) ([]commerce.Order, error) {
	return p.commerce.Orders(ctx, nil)
}

func (p *Postgres) insertEntitlementEvent(ctx context.Context, query persistence.DBTX, entitlement commerce.Entitlement, notificationID string, now time.Time) error {
	eventID, err := persistence.NewID("evt")
	if err != nil {
		return err
	}
	return p.messages.InsertOutbox(ctx, query, messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(eventID), MessageType: "entitlement.changed.v1", IdempotencyKey: contract.IdempotencyKey(notificationID), CreatedAt: now, Payload: map[string]any{"workspaceId": entitlement.WorkspaceID, "logicalInstanceId": entitlement.LogicalInstanceID, "entitlementId": entitlement.ID, "status": "active", "effectiveAt": entitlement.EffectiveAt, "expiresAt": entitlement.ExpiresAt}})
}

func (p *Postgres) insertDeploymentEvent(ctx context.Context, query persistence.DBTX, instanceID contract.LogicalInstanceID, key contract.IdempotencyKey, now time.Time) error {
	detail, err := p.instances.Detail(ctx, query, nil, instanceID)
	if err != nil {
		return err
	}
	entitlement, active, err := p.commerce.ActiveEntitlement(ctx, query, instanceID, now)
	if err != nil {
		return err
	}
	if !active {
		return ErrActiveEntitlementNeeded
	}
	plan, err := p.commerce.Plan(ctx, query, entitlement.PlanVersionID)
	if err != nil {
		return err
	}
	eventID, err := persistence.NewID("evt")
	if err != nil {
		return err
	}
	return p.messages.InsertOutbox(ctx, query, messaging.OutboxMessage{SchemaVersion: 1, ID: contract.EventID(eventID), MessageType: "deployment.desired.v1", IdempotencyKey: key, CreatedAt: now, Payload: map[string]any{"workspaceId": detail.Instance.WorkspaceID, "logicalInstanceId": instanceID, "regionId": detail.Placement.RegionID, "placementVersion": detail.Placement.Version, "instanceRevisionId": detail.Revision.ID, "desiredState": detail.Instance.DesiredState, "gameKey": detail.Instance.GameKey, "gameVersion": detail.Revision.GameVersion, "configuration": detail.Revision.Configuration, "cpuUnits": plan.CPUUnits, "memoryMegabytes": plan.MemoryMegabytes}})
}

func checkoutIDs() (contract.LogicalInstanceID, contract.InstanceRevisionID, contract.PlacementID, contract.OrderID, error) {
	instanceID, err := persistence.NewID("lin")
	if err != nil {
		return "", "", "", "", err
	}
	revisionID, err := persistence.NewID("rev")
	if err != nil {
		return "", "", "", "", err
	}
	placementID, err := persistence.NewID("plc")
	if err != nil {
		return "", "", "", "", err
	}
	orderID, err := persistence.NewID("ord")
	if err != nil {
		return "", "", "", "", err
	}
	return contract.LogicalInstanceID(instanceID), contract.InstanceRevisionID(revisionID), contract.PlacementID(placementID), contract.OrderID(orderID), nil
}
