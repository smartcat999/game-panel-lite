package deliveryapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authentication"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/httpfilter"
)

type operationReader struct{ requestedWorkspace string }

func (r *operationReader) Operation(_ context.Context, workspaceID, operationID string) (deliverycontrol.Operation, error) {
	r.requestedWorkspace = workspaceID
	return deliverycontrol.Operation{ID: operationID, WorkspaceID: workspaceID, Kind: "instance.create", ResourceType: "instance", ResourceID: "lin_one", Status: "running", Steps: []deliverycontrol.Step{{Key: "accepted", Label: "Request accepted", Status: "succeeded"}}, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

type routeAuthenticator struct{}

func (routeAuthenticator) Authenticate(context.Context, string) (authentication.Session, error) {
	return authentication.Session{UserID: "usr_one"}, nil
}

type routeResolver struct{}

func (routeResolver) Resolve(_ context.Context, resourceType httpfilter.ResourceType, ids []string) (map[string]authorization.Scope, error) {
	return map[string]authorization.Scope{ids[0]: {Type: authorization.ScopeWorkspace, ID: "ws_ember"}}, nil
}

type routeAuthorizer struct{}

func (routeAuthorizer) CheckBatch(_ context.Context, _ authorization.PrincipalID, checks []authorization.Check) ([]authorization.Decision, error) {
	return []authorization.Decision{{Allowed: checks[0].Action == authorization.ActionInstanceRead}}, nil
}

func TestOperationReadUsesResolvedWorkspaceScope(t *testing.T) {
	reader := &operationReader{}
	handler := Routes(Services{Control: reader, Sessions: routeAuthenticator{}, Resolver: routeResolver{}, Authorizer: routeAuthorizer{}})
	request := httptest.NewRequest(http.MethodGet, "/v1/operations/op_one", nil)
	request.AddCookie(&http.Cookie{Name: authentication.SessionCookieName, Value: strings.Repeat("a", 43)})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || reader.requestedWorkspace != "ws_ember" || !strings.Contains(response.Body.String(), `"id":"op_one"`) {
		t.Fatalf("status=%d workspace=%q body=%s", response.Code, reader.requestedWorkspace, response.Body.String())
	}
}
