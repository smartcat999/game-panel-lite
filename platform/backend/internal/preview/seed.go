package preview

import (
	"context"
	"fmt"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/commerce"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/globalproduct"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/identity"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instancecontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regiondirectory"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/workspace"
)

const SessionToken = "local-preview"

type Environment struct {
	Identity  *identity.Module
	Workspace *workspace.Module
	Product   *globalproduct.Module
	Region    *regionexecution.Module
}

func Modules() Environment {
	userID := contract.UserID("usr_local")
	identityModule := identity.New(identity.Seed{
		Users:             []identity.User{{ID: userID, DisplayName: "Local Operator", Email: "operator@localhost"}},
		Identities:        []identity.Identity{{ID: contract.IdentityID("idn_local"), UserID: userID, Provider: "local", Subject: "operator@localhost"}},
		Preferences:       map[contract.UserID]identity.Preferences{userID: {Locale: "en", Theme: "system", TimeZone: "Asia/Shanghai"}},
		Sessions:          map[string]contract.UserID{SessionToken: userID},
		PlatformOperators: []contract.UserID{userID},
		RegionOperators: map[contract.UserID][]contract.RegionID{
			userID: {"reg_asia_east", "reg_europe_west"},
		},
	})
	workspaceModule := workspace.New(workspace.Seed{
		Workspaces: []workspace.Workspace{
			{ID: contract.WorkspaceID("ws_northstar"), Slug: "northstar", Name: "Northstar Games"},
			{ID: contract.WorkspaceID("ws_ember"), Slug: "ember", Name: "Ember Realms"},
		},
		Memberships: []workspace.Membership{
			{ID: contract.MembershipID("mem_northstar_owner"), WorkspaceID: contract.WorkspaceID("ws_northstar"), UserID: userID, Role: workspace.RoleOwner},
			{ID: contract.MembershipID("mem_ember_operator"), WorkspaceID: contract.WorkspaceID("ws_ember"), UserID: userID, Role: workspace.RoleOperator},
		},
		Selections: map[contract.UserID]contract.WorkspaceID{userID: contract.WorkspaceID("ws_northstar")},
	})
	regionID := contract.RegionID("reg_asia_east")
	regions := regiondirectory.New([]regiondirectory.Region{
		{ID: regionID, Code: "asia-east", Name: "Asia East", Available: true},
		{ID: "reg_europe_west", Code: "europe-west", Name: "Europe West", Available: true},
	})
	commerceModule := commerce.New([]commerce.PlanVersion{{
		ID: "plv_standard_1", PlanID: "pln_standard", Version: 1, Name: "Standard",
		PriceMinor: 1200, Currency: "USD", BillingPeriod: "month", RegionIDs: []contract.RegionID{regionID, "reg_europe_west"},
		MemoryMegabytes: 2048, CPUUnits: 1000,
	}})
	instances := instancecontrol.New()
	messages := messaging.New()
	product := globalproduct.New(regions, commerceModule, instances, messages)
	seedProduct(product, instances, contract.WorkspaceID("ws_northstar"), regionID)
	region := seedRegion(regionID, nowForRegionSeed())
	return Environment{Identity: identityModule, Workspace: workspaceModule, Product: product, Region: region}
}

func seedRegion(regionID contract.RegionID, now time.Time) *regionexecution.Module {
	region := regionexecution.New(regionID, []regionexecution.Node{
		{ID: "nod_jade", RegionID: regionID, Name: "Jade", State: regionexecution.NodeReady, Games: []string{"terraria"}, CPUCapacity: 8000, MemoryCapacityMB: 16384, LeaseUntil: now.Add(time.Hour), LastHeartbeatAt: now},
		{ID: "nod_cedar", RegionID: regionID, Name: "Cedar", State: regionexecution.NodeDraining, Games: []string{"terraria"}, CPUCapacity: 4000, MemoryCapacityMB: 8192, LeaseUntil: now.Add(time.Hour), LastHeartbeatAt: now},
		{ID: "nod_echo", RegionID: regionID, Name: "Echo", State: regionexecution.NodeStale, Games: []string{"terraria"}, CPUCapacity: 4000, MemoryCapacityMB: 8192, LeaseUntil: now.Add(-time.Minute), LastHeartbeatAt: now.Add(-time.Hour)},
	})
	items := []struct {
		instanceID  contract.LogicalInstanceID
		workspaceID contract.WorkspaceID
		game        string
		state       regionexecution.ObservedState
	}{
		{instanceID: "lin_000003", workspaceID: "ws_northstar", game: "terraria", state: regionexecution.ObservedRunning},
		{instanceID: "lin_000005", workspaceID: "ws_northstar", game: "terraria", state: regionexecution.ObservedFailed},
		{instanceID: "lin_remote", workspaceID: "ws_ember", game: "unsupported-preview", state: regionexecution.ObservedPending},
	}
	for index, item := range items {
		deployment, _, err := region.ReceiveDesired(context.Background(), regionexecution.DesiredDeployment{MessageID: contract.EventID(fmt.Sprintf("evt_region_seed_%d", index)), WorkspaceID: item.workspaceID, LogicalInstanceID: item.instanceID, RegionID: regionID, PlacementVersion: 1, InstanceRevisionID: contract.InstanceRevisionID(fmt.Sprintf("rev_region_%d", index)), DesiredState: "running", GameKey: item.game, CPUUnits: 1000, MemoryMegabytes: 2048}, now)
		if err != nil {
			panic(err)
		}
		if _, err := region.Schedule(context.Background(), deployment.ID, now); err == nil {
			_, _ = region.ApplyObservation(context.Background(), regionexecution.Observation{MessageID: contract.EventID(fmt.Sprintf("evt_region_observed_%d", index)), RegionalDeploymentID: deployment.ID, Sequence: 1, State: item.state, ObservedAt: now})
		}
	}
	return region
}

func nowForRegionSeed() time.Time { return time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC) }

func seedProduct(product *globalproduct.Module, instances *instancecontrol.Module, workspaceID contract.WorkspaceID, regionID contract.RegionID) {
	now := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	states := []struct {
		name  string
		state instancecontrol.DeploymentState
		stale bool
	}{
		{name: "Pending Realm", state: instancecontrol.DeploymentPendingPayment},
		{name: "Queue Watch", state: instancecontrol.DeploymentWaitingRegion},
		{name: "Green Valley", state: instancecontrol.DeploymentRunning},
		{name: "Quiet Forge", state: instancecontrol.DeploymentStopped},
		{name: "Broken Compass", state: instancecontrol.DeploymentFailed},
		{name: "Old Signal", state: instancecontrol.DeploymentRunning, stale: true},
	}
	for index, item := range states {
		checkout, err := product.CreateCheckout(context.Background(), globalproduct.CreateCommand{
			Identity:    contract.CommandIdentity{CommandID: contract.CommandID(fmt.Sprintf("cmd_seed_%d", index)), IdempotencyKey: contract.IdempotencyKey(fmt.Sprintf("idem_seed_%d", index))},
			WorkspaceID: workspaceID, PlanVersionID: "plv_standard_1", RegionID: regionID,
			Name: item.name, GameKey: "terraria", GameVersion: "1.4.4.9", Configuration: map[string]any{"maxPlayers": 8},
		}, now.Add(time.Duration(index)*time.Minute))
		if err != nil {
			panic(err)
		}
		if item.state != instancecontrol.DeploymentPendingPayment {
			if _, err := product.ActivateVerifiedPayment(context.Background(), checkout.Order.ID, fmt.Sprintf("seed_payment_%d", index), true, now); err != nil {
				panic(err)
			}
			if err := instances.SetPreviewState(checkout.Instance.ID, item.state, item.stale); err != nil {
				panic(err)
			}
		}
	}
}
