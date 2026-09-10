package commerce

import (
	"context"
	"database/sql"
	"errors"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

type Postgres struct{ db *sql.DB }

func NewPostgres(db *sql.DB) *Postgres { return &Postgres{db: db} }

func (p *Postgres) Plans(ctx context.Context) ([]PlanVersion, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, plan_id, version, name, price_minor, currency, billing_period, memory_megabytes, cpu_units FROM plan_versions ORDER BY id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	var plans []PlanVersion
	var ids []string
	for rows.Next() {
		var plan PlanVersion
		if err := rows.Scan(&plan.ID, &plan.PlanID, &plan.Version, &plan.Name, &plan.PriceMinor, &plan.Currency, &plan.BillingPeriod, &plan.MemoryMegabytes, &plan.CPUUnits); err != nil {
			rows.Close()
			return nil, err
		}
		plans = append(plans, plan)
		ids = append(ids, string(plan.ID))
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return plans, nil
	}
	regionRows, err := p.db.QueryContext(ctx, `SELECT plan_version_id, region_id FROM plan_version_regions WHERE plan_version_id = ANY($1) ORDER BY plan_version_id, region_id LIMIT 1000`, ids)
	if err != nil {
		return nil, err
	}
	defer regionRows.Close()
	regions := make(map[contract.PlanVersionID][]contract.RegionID)
	for regionRows.Next() {
		var planID contract.PlanVersionID
		var regionID contract.RegionID
		if err := regionRows.Scan(&planID, &regionID); err != nil {
			return nil, err
		}
		regions[planID] = append(regions[planID], regionID)
	}
	for index := range plans {
		plans[index].RegionIDs = regions[plans[index].ID]
	}
	return plans, regionRows.Err()
}

func (p *Postgres) Plan(ctx context.Context, query persistence.DBTX, planVersionID contract.PlanVersionID) (PlanVersion, error) {
	var plan PlanVersion
	err := query.QueryRowContext(ctx, `SELECT id, plan_id, version, name, price_minor, currency, billing_period, memory_megabytes, cpu_units FROM plan_versions WHERE id = $1`, planVersionID).Scan(
		&plan.ID, &plan.PlanID, &plan.Version, &plan.Name, &plan.PriceMinor, &plan.Currency, &plan.BillingPeriod, &plan.MemoryMegabytes, &plan.CPUUnits,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PlanVersion{}, ErrPlanNotFound
	}
	if err != nil {
		return PlanVersion{}, err
	}
	rows, err := query.QueryContext(ctx, `SELECT region_id FROM plan_version_regions WHERE plan_version_id = $1 ORDER BY region_id LIMIT 100`, planVersionID)
	if err != nil {
		return PlanVersion{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var regionID contract.RegionID
		if err := rows.Scan(&regionID); err != nil {
			return PlanVersion{}, err
		}
		plan.RegionIDs = append(plan.RegionIDs, regionID)
	}
	return plan, rows.Err()
}

func (p *Postgres) InsertOrder(ctx context.Context, query persistence.DBTX, order Order, createdAt time.Time) error {
	_, err := query.ExecContext(ctx, `INSERT INTO orders (id, workspace_id, logical_instance_id, plan_version_id, status, expires_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`, order.ID, order.WorkspaceID, order.LogicalInstanceID, order.PlanVersionID, order.Status, order.ExpiresAt, createdAt)
	return err
}

func (p *Postgres) Orders(ctx context.Context, workspaceID *contract.WorkspaceID) ([]Order, error) {
	query := `SELECT id, workspace_id, logical_instance_id, plan_version_id, status, expires_at FROM orders ORDER BY id LIMIT 100`
	args := []any{}
	if workspaceID != nil {
		query = `SELECT id, workspace_id, logical_instance_id, plan_version_id, status, expires_at FROM orders WHERE workspace_id = $1 ORDER BY id LIMIT 100`
		args = append(args, *workspaceID)
	}
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orders []Order
	for rows.Next() {
		var order Order
		if err := rows.Scan(&order.ID, &order.WorkspaceID, &order.LogicalInstanceID, &order.PlanVersionID, &order.Status, &order.ExpiresAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

func (p *Postgres) Order(ctx context.Context, query persistence.DBTX, orderID contract.OrderID) (Order, error) {
	var order Order
	err := query.QueryRowContext(ctx, `SELECT id, workspace_id, logical_instance_id, plan_version_id, status, expires_at FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&order.ID, &order.WorkspaceID, &order.LogicalInstanceID, &order.PlanVersionID, &order.Status, &order.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	return order, err
}

func (p *Postgres) ActivationByNotification(ctx context.Context, query persistence.DBTX, notificationID string) (Payment, Entitlement, bool, error) {
	var payment Payment
	err := query.QueryRowContext(ctx, `SELECT id, order_id, provider_notification_id, verified_at FROM payments WHERE provider_notification_id = $1`, notificationID).Scan(&payment.ID, &payment.OrderID, &payment.ProviderNotificationID, &payment.VerifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Payment{}, Entitlement{}, false, nil
	}
	if err != nil {
		return Payment{}, Entitlement{}, false, err
	}
	order, err := p.Order(ctx, query, payment.OrderID)
	if err != nil {
		return Payment{}, Entitlement{}, false, err
	}
	var entitlement Entitlement
	err = query.QueryRowContext(ctx, `SELECT id, workspace_id, logical_instance_id, plan_version_id, effective_at, expires_at, active FROM entitlements WHERE logical_instance_id = $1 ORDER BY expires_at DESC LIMIT 1`, order.LogicalInstanceID).Scan(&entitlement.ID, &entitlement.WorkspaceID, &entitlement.LogicalInstanceID, &entitlement.PlanVersionID, &entitlement.EffectiveAt, &entitlement.ExpiresAt, &entitlement.Active)
	return payment, entitlement, true, err
}

func (p *Postgres) InsertActivation(ctx context.Context, query persistence.DBTX, payment Payment, entitlement Entitlement) error {
	if _, err := query.ExecContext(ctx, `INSERT INTO payments (id, order_id, provider_notification_id, verified_at) VALUES ($1, $2, $3, $4)`, payment.ID, payment.OrderID, payment.ProviderNotificationID, payment.VerifiedAt); err != nil {
		return err
	}
	if _, err := query.ExecContext(ctx, `UPDATE orders SET status = $1 WHERE id = $2`, OrderPaid, payment.OrderID); err != nil {
		return err
	}
	_, err := query.ExecContext(ctx, `INSERT INTO entitlements (id, workspace_id, logical_instance_id, plan_version_id, effective_at, expires_at, active) VALUES ($1, $2, $3, $4, $5, $6, $7)`, entitlement.ID, entitlement.WorkspaceID, entitlement.LogicalInstanceID, entitlement.PlanVersionID, entitlement.EffectiveAt, entitlement.ExpiresAt, entitlement.Active)
	return err
}

func (p *Postgres) ActiveEntitlement(ctx context.Context, query persistence.DBTX, instanceID contract.LogicalInstanceID, now time.Time) (Entitlement, bool, error) {
	var entitlement Entitlement
	err := query.QueryRowContext(ctx, `SELECT id, workspace_id, logical_instance_id, plan_version_id, effective_at, expires_at, active FROM entitlements WHERE logical_instance_id = $1 AND active = true AND effective_at <= $2 AND expires_at > $2 ORDER BY expires_at DESC LIMIT 1`, instanceID, now).Scan(&entitlement.ID, &entitlement.WorkspaceID, &entitlement.LogicalInstanceID, &entitlement.PlanVersionID, &entitlement.EffectiveAt, &entitlement.ExpiresAt, &entitlement.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return Entitlement{}, false, nil
	}
	return entitlement, err == nil, err
}
