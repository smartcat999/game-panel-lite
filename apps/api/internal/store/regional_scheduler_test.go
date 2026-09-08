package store

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/configprotection"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/gameconfig"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

func testRegionalScheduler(t *testing.T, db *RegionalStore, dsn string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ConfigureRegionalNodeAccess(ctx, regional.NodeAccessPolicy{OrganizationID: "tenant", Enabled: true, NodeIDs: []string{"schedule-a", "schedule-b"}}, 0); err != nil {
		t.Fatal(err)
	}
	p := terraria.NewVanillaProvider()
	registry, err := provider.NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	protector, err := configprotection.New("scheduler", map[string][]byte{"scheduler": bytes.Repeat([]byte{7}, 32)}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := regional.Scheduler{Resources: db, Networks: gameconfig.RegionalNetworkRenderer{Normalizer: gameconfig.LogicalNormalizer{Providers: registry, MaxBytes: 4096}, Configurations: protector}, MaxHeartbeatAge: time.Minute}
	for _, id := range []string{"schedule-a", "schedule-b"} {
		cpu, memory := float64(1), int64(128)
		if id == "schedule-b" {
			cpu, memory = 2, 256
		}
		_, err := db.ConfigureRegionalNode(ctx, regional.NodeConfiguration{ID: id, Name: id, Architecture: "amd64", CPU: cpu, MemoryMB: memory, Schedulable: true}, 0)
		if err != nil {
			t.Fatal(err)
		}
		session, err := db.StartRegionalNodeSession(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.RecordRegionalNodeHeartbeat(ctx, id, regional.NodeHeartbeat{SessionEpoch: session.Epoch, Sequence: 1, Architecture: "amd64", RuntimeReady: true}); err != nil {
			t.Fatal(err)
		}
	}
	create := func(id string) (regional.Deployment, regional.RevisionSnapshot) {
		t.Helper()
		event := instances.RevisionAvailable{SchemaVersion: 1, EventID: id, OperationID: id, OrganizationID: "tenant", ServerID: id, RegionID: db.regionID, RevisionID: id, SpecGeneration: 1, PlacementEpoch: 1}
		binding := instances.ConfigurationBinding{OrganizationID: event.OrganizationID, ServerID: id, RevisionID: id, SpecGeneration: 1, ProviderKey: string(p.Key()), ConfigSchemaVersion: p.CatalogMetadata().ConfigVersion}
		protected, err := protector.Seal(ctx, binding, []byte(`{"password":"private"}`))
		if err != nil {
			t.Fatal(err)
		}
		snapshot := regional.RevisionSnapshot{Event: event, CurrentSpecGeneration: 1, IntentVersion: 1, DesiredState: "running", Revision: instances.Revision{ID: id, ServerID: id, SpecGeneration: 1, Specification: instances.Specification{ProviderKey: string(p.Key()), GameVersion: p.Versions()[0], ConfigSchemaVersion: binding.ConfigSchemaVersion, Resources: instances.Resources{CPU: 1, MemoryMB: 128}, Configuration: protected}}}
		if err := db.RecordRevisionNotification(ctx, event); err != nil {
			t.Fatal(err)
		}
		fetch, err := db.ClaimRevision(ctx, time.Minute)
		if err != nil || fetch == nil {
			t.Fatal("claim", err)
		}
		if err := db.SaveRevision(ctx, *fetch, snapshot); err != nil {
			t.Fatal(err)
		}
		assets, err := db.ClaimAssets(ctx, time.Minute)
		if err != nil || assets == nil {
			t.Fatal("assets", err)
		}
		if err := db.CompleteAssets(ctx, *assets); err != nil {
			t.Fatal(err)
		}
		var deployment regional.Deployment
		if err := db.db.Table("regional_deployments").Where("server_id = ?", id).Take(&deployment).Error; err != nil {
			t.Fatal(err)
		}
		return deployment, snapshot
	}
	scope := regional.SchedulingScope{AllowedNodeIDs: []string{"schedule-a", "schedule-b"}, Architecture: "amd64", HostPort: 32000}
	deployment, snapshot := create("schedule-server-a")
	allocation, err := scheduler.Schedule(ctx, deployment, snapshot, scope)
	if err != nil || allocation.NodeID != "schedule-a" || len(allocation.Ports) != 1 || allocation.Ports[0].HostPort != 32000 {
		t.Fatalf("schedule: %+v %v", allocation, err)
	}
	// Its node is now full and subsequently offline. Retrying only recovers the
	// same receipt; no new allocation or renewed execution authority is created.
	if _, err := db.StartRegionalNodeSession(ctx, "schedule-a"); err != nil {
		t.Fatal(err)
	}
	replay, err := scheduler.Schedule(ctx, deployment, snapshot, scope)
	if err != nil || !reflect.DeepEqual(replay, allocation) {
		t.Fatalf("lost reply recovery: %+v %v", replay, err)
	}
	changed := scope
	changed.RequiredNodeID = "schedule-b"
	if _, err := scheduler.Schedule(ctx, deployment, snapshot, changed); !errors.Is(err, regional.ErrAllocationConflict) {
		t.Fatal("existing allocation moved", err)
	}
	changed = scope
	changed.HostPort++
	if _, err := scheduler.Schedule(ctx, deployment, snapshot, changed); !errors.Is(err, regional.ErrAllocationConflict) {
		t.Fatal("receipt ports changed", err)
	}
	second, secondSnapshot := create("schedule-server-b")
	pinned := scope
	pinned.RequiredNodeID = "schedule-a"
	if _, err := scheduler.Schedule(ctx, second, secondSnapshot, pinned); !errors.Is(err, regional.ErrNodeUnavailable) {
		t.Fatal("pin silently fell back", err)
	}
	next, err := scheduler.Schedule(ctx, second, secondSnapshot, scope)
	if err != nil || next.NodeID != "schedule-b" {
		t.Fatal("automatic scheduling", err)
	}
	var count int64
	if err := db.db.Table("regional_allocations").Where("deployment_id = ?", deployment.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("duplicate reservation", err)
	}
	wrong := snapshot
	wrong.Event.OrganizationID = "other"
	if _, err := scheduler.Schedule(ctx, deployment, wrong, scope); !errors.Is(err, regional.ErrDeploymentConflict) {
		t.Fatal("tenant mismatch accepted", err)
	}
	testSchedulingClaims(t, db, dsn, scheduler, scope, allocation, next)
	third, thirdSnapshot := create("schedule-server-c")
	ranged := scope
	ranged.MaxHostPort = scope.HostPort + 2
	ranged.RequiredNodeID = "schedule-b"
	selected, err := scheduler.Schedule(ctx, third, thirdSnapshot, ranged)
	if err != nil || selected.NodeID != "schedule-b" || len(selected.Ports) != 1 || selected.Ports[0].HostPort != 32001 {
		t.Fatalf("automatic port selection: %+v %v", selected, err)
	}
	recovered, err := scheduler.Schedule(ctx, third, thirdSnapshot, ranged)
	if err != nil || !reflect.DeepEqual(recovered, selected) {
		t.Fatal("range replay changed selected port", err)
	}
	excluded := ranged
	excluded.HostPort = 32002
	if _, err := scheduler.Schedule(ctx, third, thirdSnapshot, excluded); !errors.Is(err, regional.ErrAllocationConflict) {
		t.Fatal("receipt escaped requested range", err)
	}
	invalidRange := ranged
	invalidRange.MaxHostPort = invalidRange.HostPort + 64
	if _, err := scheduler.Schedule(ctx, third, thirdSnapshot, invalidRange); err == nil {
		t.Fatal("unbounded port range accepted")
	}

	t.Log("real Provider/protected revision -> regional scheduler -> PostgreSQL compute and port reservation/replay verified")
}
