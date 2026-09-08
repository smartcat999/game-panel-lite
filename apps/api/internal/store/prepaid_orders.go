package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type prepaidOrderRow struct {
	ID, OrganizationID, ServerID, RevisionID string
	PlacementEpoch                           int64
	Quote, Status                            string
	CreatedAtMS, ExpiresAtMS                 int64
	ActorID, IdempotencyKey, RequestHash     string
}

func (r prepaidOrderRow) order() (commerce.Order, error) {
	var quote commerce.Quote
	if json.Unmarshal([]byte(r.Quote), &quote) != nil {
		return commerce.Order{}, commerce.ErrInvalidOrder
	}
	expected, err := commerce.QuotePrepaid(quote.Plan, quote.Periods)
	if err != nil || expected != quote {
		return commerce.Order{}, commerce.ErrInvalidOrder
	}
	return commerce.Order{ID: r.ID, OrganizationID: r.OrganizationID, ServerID: r.ServerID, RevisionID: r.RevisionID, PlacementEpoch: r.PlacementEpoch, Quote: quote, Status: r.Status, CreatedAtMS: r.CreatedAtMS, ExpiresAtMS: r.ExpiresAtMS}, nil
}

// CreatePrepaidOrder locks workspace, instance and sale eligibility in that order.
// It captures exact configuration identity but does not modify user intent.
func (s *Store) CreatePrepaidOrder(ctx context.Context, actor string, request commerce.OrderRequest, paymentWindow time.Duration) (commerce.Order, error) {
	if actor == "" || request.Validate() != nil || paymentWindow < time.Second || paymentWindow > 24*time.Hour {
		return commerce.Order{}, commerce.ErrInvalidOrder
	}
	encoded, _ := json.Marshal(request)
	sum := sha256.Sum256(encoded)
	hash := hex.EncodeToString(sum[:])
	var result commerce.Order
	err := s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspaceWriter(ctx, request.OrganizationID, actor); err != nil {
			return err
		}
		var existing prepaidOrderRow
		err := tx.db.Table("prepaid_orders").Where("organization_id = ? AND idempotency_key = ?", request.OrganizationID, request.IdempotencyKey).Take(&existing).Error
		if err == nil {
			if existing.RequestHash != hash {
				return commerce.ErrOrderConflict
			}
			result, err = existing.order()
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var server instances.Server
		q := tx.db.Table("logical_servers").Where("id = ? AND organization_id = ?", request.ServerID, request.OrganizationID)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Take(&server).Error; err != nil {
			return err
		}
		if server.DesiredState == "deleted" {
			return commerce.ErrOrderUnavailable
		}
		var placement instances.Placement
		if err := tx.db.Table("server_placements").Where("server_id = ?", server.ID).Take(&placement).Error; err != nil {
			return err
		}
		var revision globalRevisionRow
		if err := tx.db.Table("server_revisions").Where("id = ? AND server_id = ?", server.CurrentRevisionID, server.ID).Take(&revision).Error; err != nil {
			return err
		}
		var spec instances.Specification
		if json.Unmarshal([]byte(revision.Specification), &spec) != nil || spec.Validate() != nil {
			return commerce.ErrOrderUnavailable
		}
		var sale prepaidSaleRow
		q = tx.db.Table("prepaid_plan_sales").Where("plan_id = ? AND plan_version = ? AND enabled = ?", request.PlanID, request.PlanVersion, true)
		if tx.db.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "SHARE"})
		}
		if err := q.Take(&sale).Error; err != nil {
			return err
		}
		var row prepaidPlanRow
		if err := tx.db.Table("prepaid_plan_versions").Where("plan_id = ? AND version = ?", request.PlanID, request.PlanVersion).Take(&row).Error; err != nil {
			return err
		}
		var plan commerce.PlanVersion
		if json.Unmarshal([]byte(row.Terms), &plan) != nil || plan.PlanID != request.PlanID || plan.Version != request.PlanVersion {
			return commerce.ErrInvalidPlan
		}
		if plan.RegionID != placement.RegionID || plan.ProviderKey != spec.ProviderKey || plan.CPU != spec.Resources.CPU || plan.MemoryMB != spec.Resources.MemoryMB {
			return commerce.ErrOrderUnavailable
		}
		quote, err := commerce.QuotePrepaid(plan, request.Periods)
		if err != nil {
			return err
		}
		terms, err := json.Marshal(quote)
		if err != nil {
			return err
		}
		now, err := outboxNow(tx.db)
		if err != nil {
			return err
		}
		order := prepaidOrderRow{ID: uuid.NewString(), OrganizationID: request.OrganizationID, ServerID: server.ID, RevisionID: revision.ID, PlacementEpoch: placement.PlacementEpoch, Quote: string(terms), Status: "pending", CreatedAtMS: now, ExpiresAtMS: now + paymentWindow.Milliseconds(), ActorID: actor, IdempotencyKey: request.IdempotencyKey, RequestHash: hash}
		if err := tx.db.Table("prepaid_orders").Create(&order).Error; err != nil {
			return err
		}
		result, err = order.order()
		return err
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = commerce.ErrOrderUnavailable
		}
		return commerce.Order{}, err
	}
	return result, nil
}

func migrateSQLitePrepaidOrders(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 12").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Exec(prepaidOrdersSQL).Error; err != nil {
			return err
		}
		for _, statement := range []string{
			"CREATE TRIGGER prepaid_order_terms_immutable BEFORE UPDATE OF id,organization_id,server_id,revision_id,placement_epoch,quote,created_at_ms,expires_at_ms,actor_id,idempotency_key,request_hash ON prepaid_orders BEGIN SELECT RAISE(ABORT, 'order terms are immutable'); END",
			"CREATE TRIGGER prepaid_orders_no_delete BEFORE DELETE ON prepaid_orders BEGIN SELECT RAISE(ABORT, 'order history cannot be deleted'); END",
			"INSERT INTO gamepanel_sqlite_migrations(version) VALUES(12)",
		} {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
