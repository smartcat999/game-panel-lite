package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
)

type globalRevisionRow struct {
	ID, ServerID   string
	SpecGeneration int64
	Specification  string
	CPU            float64
	MemoryMB       int64
	CreatedAt      time.Time
}

type globalOperationRow struct {
	ID, OrganizationID, ServerID, RevisionID  string
	Kind, Status, IdempotencyKey, RequestHash string
	CreatedAt                                 time.Time
}

// During transition, logical reservations and legacy instances both consume
// tenant quota. Missing revision pointers fail closed instead of disappearing
// from accounting and making capacity available for another purchase.
func (s *Store) globalReservedResources(ctx context.Context, organizationID string) ([]domain.GameServer, error) {
	var servers []struct {
		ID, CurrentRevisionID string
	}
	if err := s.db.WithContext(ctx).Table("logical_servers").Select("id,current_revision_id").
		Where("organization_id = ? AND desired_state <> ?", organizationID, "deleted").Scan(&servers).Error; err != nil {
		return nil, err
	}
	result := make([]domain.GameServer, 0, len(servers))
	for start := 0; start < len(servers); start += idLookupBatchSize {
		batch := servers[start:min(start+idLookupBatchSize, len(servers))]
		ids := make([]string, 0, len(batch))
		for _, server := range batch {
			ids = append(ids, server.CurrentRevisionID)
		}
		var revisions []globalRevisionRow
		if err := s.db.WithContext(ctx).Table("server_revisions").Select("id,server_id,cpu,memory_mb").Where("id IN ?", ids).Find(&revisions).Error; err != nil {
			return nil, err
		}
		byID := make(map[string]globalRevisionRow, len(revisions))
		for _, revision := range revisions {
			byID[revision.ID] = revision
		}
		for _, server := range batch {
			revision, exists := byID[server.CurrentRevisionID]
			resources := domain.ServerResources{CPULimitCores: revision.CPU, MemoryLimitMB: int(revision.MemoryMB)}
			if !exists || revision.ServerID != server.ID || int64(resources.MemoryLimitMB) != revision.MemoryMB || !finiteResources(resources) {
				return nil, ErrFiniteResourcesRequired
			}
			result = append(result, domain.GameServer{ID: server.ID, Spec: domain.ServerSpec{Resources: resources}})
		}
	}
	return result, nil
}

// CreateGlobalServer persists logical intent only; it does not assign a Node or
// launch a runtime. The future API use case must validate the selected Region,
// provider, protected configuration and asset authorization before calling it.
// Membership and quota are rechecked inside the transaction, never delegated to
// an earlier HTTP check. Empty actor IDs cannot bypass authorization here.
func (s *Store) CreateGlobalServer(ctx context.Context, actor string, request instances.CreateRequest) (instances.IntentResult, error) {
	if actor == "" {
		return instances.IntentResult{}, ErrWorkspaceWriteDenied
	}
	if err := request.Validate(); err != nil {
		return instances.IntentResult{}, err
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return instances.IntentResult{}, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(encoded))
	return s.createGlobalServer(ctx, actor, request, hash, func(existing string) (bool, error) { return existing == hash, nil }, nil, false)
}

func (s *Store) createGlobalServer(ctx context.Context, actor string, request instances.CreateRequest, hash string, matches func(string) (bool, error), seal func(instances.Server) (instances.ProtectedConfiguration, error), registeredRegion bool) (instances.IntentResult, error) {
	var result instances.IntentResult
	err := s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockWorkspaceWriter(ctx, request.OrganizationID, actor); err != nil {
			return err
		}
		var existing globalOperationRow
		err := tx.db.WithContext(ctx).Table("server_operations").Where("organization_id = ? AND kind = ? AND idempotency_key = ?", request.OrganizationID, "create", request.IdempotencyKey).Take(&existing).Error
		if err == nil {
			matched, matchErr := matches(existing.RequestHash)
			if matchErr != nil {
				return matchErr
			}
			if !matched {
				return instances.ErrIdempotencyConflict
			}
			result, err = tx.readGlobalIntent(ctx, existing)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if registeredRegion {
			if err := tx.checkRegionCreate(ctx, request.RegionID); err != nil {
				return err
			}
			if err := tx.checkGlobalAssets(ctx, request.OrganizationID, request.Specification.Assets); err != nil {
				return err
			}
		}
		quota, err := tx.GetTenantQuota(ctx, request.OrganizationID)
		if err != nil {
			return err
		}
		// Existing legacy instances and new logical instances share the same
		// tenant admission lock and accounting until legacy data is migrated.
		allocation := domain.GameServer{ID: uuid.NewString(), Spec: domain.ServerSpec{Resources: domain.ServerResources{CPULimitCores: request.Specification.Resources.CPU, MemoryLimitMB: int(request.Specification.Resources.MemoryMB)}}}
		if int64(allocation.Spec.Resources.MemoryLimitMB) != request.Specification.Resources.MemoryMB {
			return instances.ErrInvalidIntent
		}
		if err := tx.checkAllocation(ctx, quota, &allocation); err != nil {
			return err
		}
		now := time.Now().UTC()
		server := instances.Server{ID: allocation.ID, OrganizationID: request.OrganizationID, Name: request.Name, CurrentRevisionID: uuid.NewString(), SpecGeneration: 1, DesiredState: "running", IntentVersion: 1, CreatedAt: now}
		if seal != nil {
			configuration, err := seal(server)
			if err != nil {
				return err
			}
			request.Specification.Configuration = configuration
		}
		specification, err := instances.EncodeSpecification(request.Specification)
		if err != nil {
			return err
		}
		revision := globalRevisionRow{ID: server.CurrentRevisionID, ServerID: server.ID, SpecGeneration: 1, Specification: string(specification), CPU: request.Specification.Resources.CPU, MemoryMB: request.Specification.Resources.MemoryMB, CreatedAt: now}
		placement := instances.Placement{ServerID: server.ID, RegionID: request.RegionID, PlacementEpoch: 1}
		operation := globalOperationRow{ID: uuid.NewString(), OrganizationID: request.OrganizationID, ServerID: server.ID, RevisionID: revision.ID, Kind: "create", Status: "pending", IdempotencyKey: request.IdempotencyKey, RequestHash: hash, CreatedAt: now}
		event := instances.RevisionAvailable{SchemaVersion: 1, EventID: uuid.NewString(), OperationID: operation.ID, OrganizationID: server.OrganizationID, ServerID: server.ID, RevisionID: revision.ID, RegionID: placement.RegionID, PlacementEpoch: placement.PlacementEpoch, SpecGeneration: revision.SpecGeneration}
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		outbox := struct {
			ID, OperationID, RegionID, Payload string
			CreatedAt                          time.Time
		}{event.EventID, operation.ID, placement.RegionID, string(payload), now}
		for _, write := range []struct {
			table string
			value any
		}{
			{"logical_servers", &server}, {"server_revisions", &revision},
			{"server_placements", &placement}, {"server_operations", &operation}, {"server_outbox", &outbox},
		} {
			if err := tx.db.WithContext(ctx).Table(write.table).Create(write.value).Error; err != nil {
				return err
			}
		}
		result, err = tx.readGlobalIntent(ctx, operation)
		return err
	})
	if err != nil {
		return instances.IntentResult{}, err
	}
	return result, nil
}

func (s *Store) readGlobalIntent(ctx context.Context, operation globalOperationRow) (instances.IntentResult, error) {
	var result instances.IntentResult
	var revision globalRevisionRow
	if err := s.db.WithContext(ctx).Table("logical_servers").Where("id = ? AND organization_id = ?", operation.ServerID, operation.OrganizationID).Take(&result.Server).Error; err != nil {
		return result, err
	}
	if err := s.db.WithContext(ctx).Table("server_revisions").Where("id = ? AND server_id = ?", operation.RevisionID, operation.ServerID).Take(&revision).Error; err != nil {
		return result, err
	}
	if err := s.db.WithContext(ctx).Table("server_placements").Where("server_id = ?", operation.ServerID).Take(&result.Placement).Error; err != nil {
		return result, err
	}
	result.Revision = instances.Revision{ID: revision.ID, ServerID: revision.ServerID, SpecGeneration: revision.SpecGeneration, CreatedAt: revision.CreatedAt}
	if err := json.Unmarshal([]byte(revision.Specification), &result.Revision.Specification); err != nil {
		return result, err
	}
	result.Operation = instances.Operation{ID: operation.ID, OrganizationID: operation.OrganizationID, ServerID: operation.ServerID, Kind: operation.Kind, Status: operation.Status, CreatedAt: operation.CreatedAt}
	return result, nil
}

// GetServerOperation retrieves an operation by ID within a tenant organization.
func (s *Store) GetServerOperation(ctx context.Context, organizationID, operationID string) (instances.Operation, error) {
	if organizationID == "" || operationID == "" {
		return instances.Operation{}, ErrNotFound
	}
	var op instances.Operation
	err := s.readSnapshot(ctx, func(tx *Store) error {
		return tx.db.Table("server_operations").Where("id = ? AND organization_id = ?", operationID, organizationID).Take(&op).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return instances.Operation{}, ErrNotFound
		}
		return instances.Operation{}, err
	}
	return op, nil
}
