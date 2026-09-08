package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// RegionalMigrationAudit describes ownership only. It deliberately excludes
// configuration, credentials, host paths and runtime payloads. A clean report
// is a prerequisite for splitting data, not proof that migration is complete.
type RegionalMigrationAudit struct {
	Servers     int                       `json:"servers"`
	Nodes       int                       `json:"nodes"`
	Assignments int                       `json:"assignments"`
	Issues      []RegionalMigrationIssue  `json:"issues"`
	Placements  []LegacyRegionalPlacement `json:"placements"`
}

type RegionalMigrationIssue struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Code string `json:"code"`
}

// NodeID is the observed legacy allocation, not a user-requested strict binding.
type LegacyRegionalPlacement struct {
	ServerID       string `json:"serverId"`
	OrganizationID string `json:"organizationId"`
	RegionID       string `json:"regionId"`
	NodeID         string `json:"nodeId"`
}

// AuditPostgresRegionalMigration never migrates or initializes the source.
// Deployment tooling must use the matching schema and a SELECT-only account.
func AuditPostgresRegionalMigration(ctx context.Context, dsn string) (RegionalMigrationAudit, error) {
	if strings.TrimSpace(dsn) == "" {
		return RegionalMigrationAudit{}, fmt.Errorf("PostgreSQL DSN is required")
	}
	db, err := connectPostgres(dsn, 1)
	if err != nil {
		return RegionalMigrationAudit{}, err
	}
	pool, _ := db.DB()
	defer pool.Close()
	if err := checkPostgresSchema(ctx, db, postgresMigrations()); err != nil {
		return RegionalMigrationAudit{}, err
	}
	return (&Store{db: db}).AuditRegionalMigration(ctx)
}

// AuditRegionalMigration reads one consistent snapshot. In particular, a
// concurrent node reassignment cannot combine old instance data with new node
// ownership. No defaults or repairs are applied to ambiguous legacy records.
func (s *Store) AuditRegionalMigration(ctx context.Context) (RegionalMigrationAudit, error) {
	report := RegionalMigrationAudit{Issues: []RegionalMigrationIssue{}, Placements: []LegacyRegionalPlacement{}}
	options := &sql.TxOptions{ReadOnly: true}
	if s.db.Dialector.Name() == "postgres" {
		options.Isolation = sql.LevelRepeatableRead
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var organizations []struct{ ID string }
		var nodes []struct{ ID, Region string }
		var servers []struct{ ID, OrganizationID, NodeID string }
		var assignments []struct{ ID, ServerID, NodeID string }
		for _, read := range []struct {
			table, columns string
			target         any
		}{
			{"organizations", "id", &organizations},
			{"compute_nodes", "id, region", &nodes},
			{"game_servers", "id, organization_id, node_id", &servers},
			{"workload_assignments", "id, server_id, node_id", &assignments},
		} {
			if err := tx.Table(read.table).Select(read.columns).Order("id").Scan(read.target).Error; err != nil {
				return err
			}
		}
		report.Servers, report.Nodes, report.Assignments = len(servers), len(nodes), len(assignments)
		issue := func(kind, id, code string) {
			report.Issues = append(report.Issues, RegionalMigrationIssue{kind, id, code})
		}
		owners := make(map[string]bool, len(organizations))
		for _, owner := range organizations {
			if strings.TrimSpace(owner.ID) != "" {
				owners[owner.ID] = true
			}
		}
		regions := make(map[string]string, len(nodes))
		for _, node := range nodes {
			regions[node.ID] = node.Region
			if strings.TrimSpace(node.Region) == "" {
				issue("node", node.ID, "region_required")
			} else if node.Region != strings.TrimSpace(node.Region) {
				issue("node", node.ID, "region_not_canonical")
			}
		}
		serverNodes := make(map[string]string, len(servers))
		for _, server := range servers {
			serverNodes[server.ID] = server.NodeID
			valid := true
			if !owners[server.OrganizationID] {
				issue("server", server.ID, "organization_unresolved")
				valid = false
			}
			region, exists := regions[server.NodeID]
			switch {
			case strings.TrimSpace(server.NodeID) == "":
				// Empty historically means local, but can also be pending cloud
				// scheduling. An operator must distinguish these before migration.
				issue("server", server.ID, "node_ambiguous")
				valid = false
			case !exists:
				issue("server", server.ID, "node_missing")
				valid = false
			case strings.TrimSpace(region) == "" || region != strings.TrimSpace(region):
				issue("server", server.ID, "region_unresolved")
				valid = false
			}
			if valid {
				report.Placements = append(report.Placements, LegacyRegionalPlacement{server.ID, server.OrganizationID, region, server.NodeID})
			}
		}
		for _, assignment := range assignments {
			nodeID, exists := serverNodes[assignment.ServerID]
			if !exists {
				issue("assignment", assignment.ID, "server_missing")
			}
			if _, exists := regions[assignment.NodeID]; !exists || strings.TrimSpace(assignment.NodeID) == "" {
				issue("assignment", assignment.ID, "node_missing")
			}
			if exists && assignment.NodeID != nodeID {
				// This may be a retiring source. Do not silently discard it or
				// interpret the newer NodeID as proof the source stopped.
				issue("assignment", assignment.ID, "placement_diverged")
			}
		}
		return nil
	}, options)
	if err != nil {
		return RegionalMigrationAudit{}, err
	}
	return report, nil
}
