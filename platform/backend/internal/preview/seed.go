package preview

import (
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/identity"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/workspace"
)

const SessionToken = "local-preview"

func Modules() (*identity.Module, *workspace.Module) {
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
	return identityModule, workspaceModule
}
