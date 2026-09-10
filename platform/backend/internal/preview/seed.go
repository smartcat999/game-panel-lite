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
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/workspace"
)

const SessionToken = "local-preview"

type Environment struct {
	Identity  *identity.Module
	Workspace *workspace.Module
	Product   *globalproduct.Module
}

func Modules() Environment {
	userID := contract.UserID("usr_local")
	identityModule := identity.New(identity.Seed{
		Users:             []identity.User{{ID: userID, DisplayName: "Local Operator", Email: "operator@localhost"}},
		Identities:        []identity.Identity{{ID: contract.IdentityID("idn_local"), UserID: userID, Provider: "local", Subject: "operator@localhost"}},
		Preferences:       map[contract.UserID]identity.Preferences{userID: {Locale: "en", Theme: "system", TimeZone: "Asia/Shanghai"}},
		Sessions:          map[string]contract.UserID{SessionToken: userID},
		PlatformOperators: []contract.UserID{userID},
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
	return Environment{Identity: identityModule, Workspace: workspaceModule, Product: product}
}

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
