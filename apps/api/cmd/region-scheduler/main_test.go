package main

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type entitlementFixture struct {
	record entitlements.Record
	err    error
	calls  int
}

func (f *entitlementFixture) GetRunEntitlement(_ context.Context, event instances.RevisionAvailable, intent int64) (entitlements.Record, error) {
	f.calls++
	if event.ServerID != "server" || intent != 2 {
		return entitlements.Record{}, entitlements.ErrInvalid
	}
	return f.record, f.err
}

type nodeAccessFixture struct {
	calls int
}

func (f *nodeAccessFixture) RegionalNodeAccessPolicy(_ context.Context, organization string) (regional.NodeAccessPolicy, error) {
	f.calls++
	if organization != "tenant" {
		return regional.NodeAccessPolicy{}, regional.ErrNodeAccessDenied
	}
	return regional.NodeAccessPolicy{OrganizationID: organization, NodeIDs: []string{"node-a"}, Enabled: true, Version: 1}, nil
}

func TestSchedulingScopeRequiresCurrentGlobalEntitlement(t *testing.T) {
	game := terraria.NewVanillaProvider()
	registry, err := provider.NewRegistry(game)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := regional.RevisionSnapshot{
		Event:         instances.RevisionAvailable{OrganizationID: "tenant", ServerID: "server"},
		IntentVersion: 2,
		Revision: instances.Revision{Specification: instances.Specification{
			ProviderKey: string(game.Key()),
			Resources:   instances.Resources{CPU: 1, MemoryMB: 128},
		}},
	}
	nodes := &nodeAccessFixture{}
	rights := &entitlementFixture{err: entitlements.ErrUnavailable}
	resolver := schedulingScopes{db: nodes, entitlements: rights, registry: registry, architecture: "amd64", firstPort: 32000, lastPort: 32010}
	if _, err := resolver.SchedulingScope(context.Background(), snapshot); !errors.Is(err, entitlements.ErrUnavailable) || nodes.calls != 0 || rights.calls != 1 {
		t.Fatalf("missing entitlement reached regional node admission: %v", err)
	}
	rights.err = nil
	rights.record = entitlements.Record{Policy: entitlements.Policy{OrganizationID: "tenant", ServerID: "server", CPU: 1, MemoryMB: 128, StartsAtMS: 1, EndsAtMS: 2, Status: "active"}, Version: 1, SourceKind: "operator", SourceID: "grant"}
	scope, err := resolver.SchedulingScope(context.Background(), snapshot)
	if err != nil || len(scope.AllowedNodeIDs) != 1 || scope.AllowedNodeIDs[0] != "node-a" || scope.Architecture != "amd64" || scope.HostPort != 32000 || scope.MaxHostPort != 32010 || nodes.calls != 1 || rights.calls != 2 {
		t.Fatalf("authorized scheduling scope: %+v %v", scope, err)
	}
	underProvisioned := rights.record
	underProvisioned.MemoryMB = 64
	rights.record = underProvisioned
	if _, err := resolver.SchedulingScope(context.Background(), snapshot); !errors.Is(err, entitlements.ErrUnavailable) || nodes.calls != 1 {
		t.Fatalf("insufficient entitlement reached node admission: %v", err)
	}
}
