package controlplane

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/identity"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/workspace"
)

func TestAuthorizationSeparatesWorkspaceAndPlatformAuthority(t *testing.T) {
	memberID := contract.UserID("usr_member")
	operatorID := contract.UserID("usr_operator")
	workspaceID := contract.WorkspaceID("ws_customer")
	otherWorkspaceID := contract.WorkspaceID("ws_other")
	identityModule := identity.New(identity.Seed{
		Users:             []identity.User{{ID: memberID}, {ID: operatorID}},
		Sessions:          map[string]contract.UserID{"member-token": memberID, "operator-token": operatorID},
		PlatformOperators: []contract.UserID{operatorID},
	})
	workspaceModule := workspace.New(workspace.Seed{
		Workspaces:  []workspace.Workspace{{ID: workspaceID}, {ID: otherWorkspaceID}},
		Memberships: []workspace.Membership{{ID: contract.MembershipID("mem_customer"), UserID: memberID, WorkspaceID: workspaceID, Role: workspace.RoleOwner}},
	})
	handler := NewHandler(identityModule, workspaceModule)

	assertStatus(t, handler, "GET", "/v1/platform/regions", "member-token", nil, http.StatusForbidden)
	assertStatus(t, handler, "GET", "/v1/platform/regions", "operator-token", nil, http.StatusOK)
	assertStatus(t, handler, "GET", "/v1/workspaces/ws_customer/members", "operator-token", nil, http.StatusForbidden)
	assertStatus(t, handler, "GET", "/v1/workspaces/ws_other/members", "member-token", nil, http.StatusForbidden)
	assertStatus(t, handler, "GET", "/v1/workspaces/ws_customer/members", "member-token", nil, http.StatusOK)
}

func TestPreferenceUpdateIsVisibleToRestoredSession(t *testing.T) {
	userID := contract.UserID("usr_one")
	identityModule := identity.New(identity.Seed{
		Users:       []identity.User{{ID: userID}},
		Preferences: map[contract.UserID]identity.Preferences{userID: {Locale: "en", Theme: "system", TimeZone: "UTC"}},
		Sessions:    map[string]contract.UserID{"session-token": userID},
	})
	handler := NewHandler(identityModule, workspace.New(workspace.Seed{}))

	assertStatus(t, handler, "PATCH", "/v1/user-preferences", "session-token", []byte(`{"locale":"zh-CN","theme":"dark","timeZone":"Asia/Shanghai"}`), http.StatusOK)
	got, err := identityModule.UserPreferences(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Locale != "zh-CN" || got.Theme != "dark" {
		t.Fatalf("updated preferences were not restored: %#v", got)
	}
}

func assertStatus(t *testing.T, handler http.Handler, method, path, token string, body []byte, want int) {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("%s %s returned %d, want %d; body=%s", method, path, recorder.Code, want, recorder.Body.String())
	}
}
