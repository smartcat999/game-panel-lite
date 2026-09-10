package httpfilter

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
)

type PostgresScopeResolver struct {
	database *sql.DB
}

func NewPostgresScopeResolver(database *sql.DB) *PostgresScopeResolver {
	return &PostgresScopeResolver{database: database}
}

func (r *PostgresScopeResolver) Resolve(ctx context.Context, resourceType ResourceType, ids []string) (map[string]authorization.Scope, error) {
	if resourceType == ResourcePlatform {
		result := make(map[string]authorization.Scope, 1)
		if len(ids) == 1 && ids[0] == "platform" {
			result["platform"] = authorization.Scope{Type: authorization.ScopePlatform, ID: "platform"}
		}
		return result, nil
	}
	query, scopeType, err := scopeQuery(resourceType)
	if err != nil {
		return nil, err
	}
	rows, err := r.database.QueryContext(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]authorization.Scope, len(ids))
	for rows.Next() {
		var resourceID, scopeID string
		if err := rows.Scan(&resourceID, &scopeID); err != nil {
			return nil, err
		}
		result[resourceID] = authorization.Scope{Type: scopeType, ID: scopeID}
	}
	return result, rows.Err()
}

func scopeQuery(resourceType ResourceType) (string, authorization.ScopeType, error) {
	switch resourceType {
	case ResourceRegion:
		return `SELECT id, id FROM regions WHERE id = ANY($1)`, authorization.ScopeRegion, nil
	case ResourceWorkspace:
		return `SELECT id, id FROM workspaces WHERE id = ANY($1)`, authorization.ScopeWorkspace, nil
	case ResourceInstance:
		return `SELECT id, workspace_id FROM managed_instances WHERE id = ANY($1)`, authorization.ScopeWorkspace, nil
	case ResourceOperation:
		return `SELECT id, workspace_id FROM operations WHERE id = ANY($1)`, authorization.ScopeWorkspace, nil
	case ResourceBackup:
		return `SELECT id, workspace_id FROM backup_requests WHERE id = ANY($1)`, authorization.ScopeWorkspace, nil
	default:
		return "", "", fmt.Errorf("unsupported resource type %q", resourceType)
	}
}
