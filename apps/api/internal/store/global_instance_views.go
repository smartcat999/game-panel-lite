package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instanceview"
	"gorm.io/gorm"
)

const maxGlobalInstancePageSize = 100

type globalInstanceComposite struct {
	record instanceview.Record
	nodeID string
	taskID string
}

func (s *Store) ListTenantInstanceViews(ctx context.Context, actor, organizationID, after string, limit int) (instanceview.Page[instanceview.Record], error) {
	if organizationID == "" || !validInstanceViewQuery(actor, organizationID, after, limit) {
		return instanceview.Page[instanceview.Record]{}, instanceview.ErrInvalidQuery
	}
	var result instanceview.Page[instanceview.Record]
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var member domain.OrganizationMember
		err := tx.db.Table("organization_members").Select("id").Where("organization_id = ? AND user_id = ?", organizationID, actor).Take(&member).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		composites, next, err := tx.listGlobalInstanceComposites(ctx, organizationID, after, limit)
		if err != nil {
			return err
		}
		result.Items = make([]instanceview.Record, len(composites))
		for i := range composites {
			result.Items[i] = composites[i].record
		}
		result.NextCursor = next
		return nil
	})
	return result, err
}

func (s *Store) GetTenantInstanceView(ctx context.Context, actor, organizationID, serverID string) (instanceview.Record, error) {
	if !validInstanceViewIdentifier(actor, true) || !validInstanceViewIdentifier(organizationID, true) || !validInstanceViewIdentifier(serverID, true) {
		return instanceview.Record{}, instanceview.ErrInvalidQuery
	}
	var result instanceview.Record
	err := s.readSnapshot(ctx, func(tx *Store) error {
		var member domain.OrganizationMember
		err := tx.db.Table("organization_members").Select("id").Where("organization_id = ? AND user_id = ?", organizationID, actor).Take(&member).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		composite, err := tx.getGlobalInstanceComposite(ctx, organizationID, serverID)
		if err == nil {
			result = composite.record
		}
		return err
	})
	return result, err
}

func (s *Store) ListPlatformInstanceViews(ctx context.Context, actor, organizationID, after string, limit int) (instanceview.Page[instanceview.PlatformRecord], error) {
	if !validInstanceViewQuery(actor, strings.TrimSpace(organizationID), after, limit) || organizationID != strings.TrimSpace(organizationID) {
		return instanceview.Page[instanceview.PlatformRecord]{}, instanceview.ErrInvalidQuery
	}
	var result instanceview.Page[instanceview.PlatformRecord]
	err := s.readSnapshot(ctx, func(tx *Store) error {
		if err := requireInstanceViewOperator(tx.db, actor); err != nil {
			return err
		}
		composites, next, err := tx.listGlobalInstanceComposites(ctx, organizationID, after, limit)
		if err != nil {
			return err
		}
		result.Items = make([]instanceview.PlatformRecord, len(composites))
		for i := range composites {
			result.Items[i] = instanceview.PlatformRecord{Record: composites[i].record, NodeID: composites[i].nodeID, TaskID: composites[i].taskID}
		}
		result.NextCursor = next
		return nil
	})
	return result, err
}

func (s *Store) GetPlatformInstanceView(ctx context.Context, actor, serverID string) (instanceview.PlatformRecord, error) {
	if !validInstanceViewIdentifier(actor, true) || !validInstanceViewIdentifier(serverID, true) {
		return instanceview.PlatformRecord{}, instanceview.ErrInvalidQuery
	}
	var result instanceview.PlatformRecord
	err := s.readSnapshot(ctx, func(tx *Store) error {
		if err := requireInstanceViewOperator(tx.db, actor); err != nil {
			return err
		}
		composite, err := tx.getGlobalInstanceComposite(ctx, "", serverID)
		if err == nil {
			result = instanceview.PlatformRecord{Record: composite.record, NodeID: composite.nodeID, TaskID: composite.taskID}
		}
		return err
	})
	return result, err
}

func validInstanceViewQuery(actor, organizationID, after string, limit int) bool {
	return validInstanceViewIdentifier(actor, true) && validInstanceViewIdentifier(organizationID, false) && validInstanceViewIdentifier(after, false) && limit > 0 && limit <= maxGlobalInstancePageSize
}

func validInstanceViewIdentifier(value string, required bool) bool {
	return (!required || value != "") && value == strings.TrimSpace(value) && len(value) <= 128
}

func requireInstanceViewOperator(db *gorm.DB, actor string) error {
	var account struct {
		Role         domain.Role
		PlatformRole domain.PlatformRole
	}
	err := db.Table("admin_accounts").Select("role", "platform_role").Where("id = ?", actor).Take(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && domain.NormalizePlatformRole(account.PlatformRole, account.Role) != domain.PlatformRoleAdmin) {
		return instanceview.ErrOperatorRequired
	}
	return err
}

func (s *Store) listGlobalInstanceComposites(ctx context.Context, organizationID, after string, limit int) ([]globalInstanceComposite, string, error) {
	query := s.db.WithContext(ctx).Table("logical_servers").Where("id > ?", after)
	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	}
	var servers []instances.Server
	if err := query.Order("id ASC").Limit(limit + 1).Find(&servers).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(servers) > limit {
		servers = servers[:limit]
		next = servers[len(servers)-1].ID
	}
	composites, err := s.composeGlobalInstanceViews(ctx, servers)
	return composites, next, err
}

func (s *Store) getGlobalInstanceComposite(ctx context.Context, organizationID, serverID string) (globalInstanceComposite, error) {
	query := s.db.WithContext(ctx).Table("logical_servers").Where("id = ?", serverID)
	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	}
	var server instances.Server
	if err := query.Take(&server).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return globalInstanceComposite{}, ErrNotFound
		}
		return globalInstanceComposite{}, err
	}
	items, err := s.composeGlobalInstanceViews(ctx, []instances.Server{server})
	if err != nil {
		return globalInstanceComposite{}, err
	}
	return items[0], nil
}

func (s *Store) composeGlobalInstanceViews(ctx context.Context, servers []instances.Server) ([]globalInstanceComposite, error) {
	if len(servers) == 0 {
		return []globalInstanceComposite{}, nil
	}
	serverIDs := make([]string, len(servers))
	revisionIDs := make([]string, len(servers))
	for i, server := range servers {
		serverIDs[i], revisionIDs[i] = server.ID, server.CurrentRevisionID
	}
	var revisions []globalRevisionRow
	if err := s.db.WithContext(ctx).Table("server_revisions").Where("id IN ?", revisionIDs).Find(&revisions).Error; err != nil {
		return nil, err
	}
	var placements []instances.Placement
	if err := s.db.WithContext(ctx).Table("server_placements").Where("server_id IN ?", serverIDs).Find(&placements).Error; err != nil {
		return nil, err
	}
	var statuses []globalDeploymentStatusRow
	if err := s.db.WithContext(ctx).Table("global_deployment_statuses").Where("server_id IN ?", serverIDs).Find(&statuses).Error; err != nil {
		return nil, err
	}
	var operations []globalOperationRow
	if err := s.db.WithContext(ctx).Table("server_operations").Where("revision_id IN ? AND kind IN ?", revisionIDs, []string{"create", "revise"}).Find(&operations).Error; err != nil {
		return nil, err
	}
	revisionByID := make(map[string]globalRevisionRow, len(revisions))
	for _, row := range revisions {
		revisionByID[row.ID] = row
	}
	placementByServer := make(map[string]instances.Placement, len(placements))
	for _, row := range placements {
		placementByServer[row.ServerID] = row
	}
	statusByServer := make(map[string]globalDeploymentStatusRow, len(statuses))
	for _, row := range statuses {
		statusByServer[row.ServerID] = row
	}
	operationByRevision := make(map[string]globalOperationRow, len(operations))
	for _, row := range operations {
		if _, exists := operationByRevision[row.RevisionID]; exists {
			return nil, instanceview.ErrCorruptRecord
		}
		operationByRevision[row.RevisionID] = row
	}
	result := make([]globalInstanceComposite, 0, len(servers))
	for _, server := range servers {
		revision, hasRevision := revisionByID[server.CurrentRevisionID]
		placement, hasPlacement := placementByServer[server.ID]
		operation, hasOperation := operationByRevision[server.CurrentRevisionID]
		if !hasRevision || revision.ServerID != server.ID || !hasPlacement || placement.ServerID != server.ID || !hasOperation ||
			operation.OrganizationID != server.OrganizationID || operation.ServerID != server.ID || operation.RevisionID != revision.ID {
			return nil, instanceview.ErrCorruptRecord
		}
		var specification instances.Specification
		if json.Unmarshal([]byte(revision.Specification), &specification) != nil || specification.Validate() != nil ||
			revision.SpecGeneration != server.SpecGeneration || revision.CPU != specification.Resources.CPU || revision.MemoryMB != specification.Resources.MemoryMB {
			return nil, instanceview.ErrCorruptRecord
		}
		record := instanceview.Record{
			ID: server.ID, OrganizationID: server.OrganizationID, Name: server.Name,
			ProviderKey: specification.ProviderKey, GameVersion: specification.GameVersion, ConfigSchemaVersion: specification.ConfigSchemaVersion,
			Resources: instanceview.Resources{CPU: revision.CPU, MemoryMB: revision.MemoryMB}, DesiredState: server.DesiredState,
			RegionID: placement.RegionID, RevisionID: revision.ID, SpecGeneration: server.SpecGeneration,
			IntentVersion: server.IntentVersion, PlacementEpoch: placement.PlacementEpoch, CreatedAt: server.CreatedAt,
		}
		record.LatestOperation = &instanceview.Operation{ID: operation.ID, Kind: operation.Kind, Status: operation.Status, CreatedAt: operation.CreatedAt}
		composite := globalInstanceComposite{record: record}
		if status, exists := statusByServer[server.ID]; exists {
			if status.OrganizationID != server.OrganizationID || status.RegionID != placement.RegionID || status.OperationID != operation.ID || status.RevisionID != revision.ID ||
				status.PlacementEpoch != placement.PlacementEpoch || status.SpecGeneration != server.SpecGeneration || status.IntentVersion != server.IntentVersion {
				return nil, instanceview.ErrCorruptRecord
			}
			record.Deployment = &instanceview.Deployment{OperationID: status.OperationID, ActualState: status.ActualState, Outcome: status.Outcome, ObservedAt: time.UnixMilli(status.ObservedAtMS).UTC()}
			composite.record = record
			composite.nodeID, composite.taskID = status.NodeID, status.TaskID
		}
		result = append(result, composite)
	}
	return result, nil
}
