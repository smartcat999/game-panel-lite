package httpfilter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authentication"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
)

type sessionStub struct {
	session authentication.Session
}

func (s sessionStub) Authenticate(_ context.Context, token string) (authentication.Session, error) {
	if token != "valid-token" {
		return authentication.Session{}, authentication.ErrInvalidSession
	}
	return s.session, nil
}

type bindingStoreStub struct {
	bindings []authorization.RoleBinding
	loads    int
}

func (s *bindingStoreStub) BindingsForPrincipal(_ context.Context, _ authorization.PrincipalID) ([]authorization.RoleBinding, error) {
	s.loads++
	return s.bindings, nil
}

type resolverStub struct {
	scopes map[string]authorization.Scope
	loads  int
}

func (s *resolverStub) Resolve(_ context.Context, _ ResourceType, ids []string) (map[string]authorization.Scope, error) {
	s.loads++
	result := make(map[string]authorization.Scope, len(ids))
	for _, id := range ids {
		if scope, ok := s.scopes[id]; ok {
			result[id] = scope
		}
	}
	return result, nil
}

func protectedRequest(handler http.Handler, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.AddCookie(&http.Cookie{Name: authentication.SessionCookieName, Value: "valid-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestFiltersDenyCrossWorkspaceAndDoNotInvokeHandler(t *testing.T) {
	principalID := authorization.PrincipalID("usr_operator")
	bindings := &bindingStoreStub{bindings: []authorization.RoleBinding{{ID: "rb_one", PrincipalID: principalID, Role: authorization.RoleWorkspaceOperator, Scope: authorization.Scope{Type: authorization.ScopeWorkspace, ID: "ws_one"}}}}
	resolver := &resolverStub{scopes: map[string]authorization.Scope{"lin_other": {Type: authorization.ScopeWorkspace, ID: "ws_other"}}}
	invoked := false
	endpoint := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { invoked = true })
	policy := RoutePolicy{Action: authorization.ActionInstanceRestart, ResourceType: ResourceInstance, ResourceIDs: QueryIDs("id")}
	handler := Authenticate(sessionStub{session: authentication.Session{UserID: string(principalID)}}, Authorize(resolver, authorization.NewEngine(bindings), policy, endpoint))
	response := protectedRequest(handler, "/instances/actions/restart?id=lin_other")
	if response.Code != http.StatusForbidden || invoked {
		t.Fatalf("cross-workspace request status=%d invoked=%v", response.Code, invoked)
	}
	if resolver.loads != 1 || bindings.loads != 1 {
		t.Fatalf("query counts resolver=%d bindings=%d", resolver.loads, bindings.loads)
	}
}

func TestPlatformRoleDoesNotCrossIntoWorkspaceScope(t *testing.T) {
	principalID := authorization.PrincipalID("usr_platform")
	bindings := &bindingStoreStub{bindings: []authorization.RoleBinding{{ID: "rb_platform", PrincipalID: principalID, Role: authorization.RolePlatformAdmin, Scope: authorization.Scope{Type: authorization.ScopePlatform, ID: "platform"}}}}
	resolver := &resolverStub{scopes: map[string]authorization.Scope{"lin_one": {Type: authorization.ScopeWorkspace, ID: "ws_one"}}}
	policy := RoutePolicy{Action: authorization.ActionInstanceRead, ResourceType: ResourceInstance, ResourceIDs: QueryIDs("id")}
	handler := Authenticate(sessionStub{session: authentication.Session{UserID: string(principalID)}}, Authorize(resolver, authorization.NewEngine(bindings), policy, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	if response := protectedRequest(handler, "/instances?id=lin_one"); response.Code != http.StatusForbidden {
		t.Fatalf("platform role crossed scope: %d", response.Code)
	}
}

func TestHundredResourceBatchUsesConstantQueriesAndPassesResolvedResources(t *testing.T) {
	principalID := authorization.PrincipalID("usr_operator")
	scope := authorization.Scope{Type: authorization.ScopeWorkspace, ID: "ws_one"}
	bindings := &bindingStoreStub{bindings: []authorization.RoleBinding{{ID: "rb_one", PrincipalID: principalID, Role: authorization.RoleWorkspaceOperator, Scope: scope}}}
	resolver := &resolverStub{scopes: map[string]authorization.Scope{}}
	ids := make([]string, authorization.MaxBatchSize)
	for index := range ids {
		ids[index] = fmt.Sprintf("lin_%03d", index)
		resolver.scopes[ids[index]] = scope
	}
	seen := 0
	endpoint := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		seen = len(AuthorizedResources(request.Context()))
		if principal, ok := PrincipalFromContext(request.Context()); !ok || principal.ID != string(principalID) {
			t.Fatal("authenticated principal missing from context")
		}
	})
	policy := RoutePolicy{Action: authorization.ActionInstanceRead, ResourceType: ResourceInstance, ResourceIDs: QueryIDs("id")}
	handler := Authenticate(sessionStub{session: authentication.Session{UserID: string(principalID)}}, Authorize(resolver, authorization.NewEngine(bindings), policy, endpoint))
	response := protectedRequest(handler, "/instances?id="+strings.Join(ids, ","))
	if response.Code != http.StatusOK || seen != authorization.MaxBatchSize {
		t.Fatalf("batch status=%d resources=%d", response.Code, seen)
	}
	if resolver.loads != 1 || bindings.loads != 1 {
		t.Fatalf("100 resources caused resolver=%d binding=%d queries", resolver.loads, bindings.loads)
	}
}

func TestMissingResourceAndOversizedBatchFailClosed(t *testing.T) {
	bindings := &bindingStoreStub{}
	resolver := &resolverStub{scopes: map[string]authorization.Scope{}}
	policy := RoutePolicy{Action: authorization.ActionInstanceRead, ResourceType: ResourceInstance, ResourceIDs: QueryIDs("id")}
	handler := Authenticate(sessionStub{session: authentication.Session{UserID: "usr_one"}}, Authorize(resolver, authorization.NewEngine(bindings), policy, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	if response := protectedRequest(handler, "/instances?id=missing"); response.Code != http.StatusForbidden {
		t.Fatalf("missing resource status=%d", response.Code)
	}
	ids := make([]string, authorization.MaxBatchSize+1)
	for index := range ids {
		ids[index] = fmt.Sprintf("lin_%d", index)
	}
	if response := protectedRequest(handler, "/instances?id="+strings.Join(ids, ",")); response.Code != http.StatusBadRequest {
		t.Fatalf("oversized batch status=%d", response.Code)
	}
}

func TestStoredScopeMustMatchNestedPathClaim(t *testing.T) {
	principalID := authorization.PrincipalID("usr_multi")
	bindings := &bindingStoreStub{bindings: []authorization.RoleBinding{
		{ID: "rb_one", PrincipalID: principalID, Role: authorization.RoleWorkspaceViewer, Scope: authorization.Scope{Type: authorization.ScopeWorkspace, ID: "ws_one"}},
		{ID: "rb_two", PrincipalID: principalID, Role: authorization.RoleWorkspaceViewer, Scope: authorization.Scope{Type: authorization.ScopeWorkspace, ID: "ws_two"}},
	}}
	resolver := &resolverStub{scopes: map[string]authorization.Scope{"lin_two": {Type: authorization.ScopeWorkspace, ID: "ws_two"}}}
	invoked := false
	policy := RoutePolicy{Action: authorization.ActionInstanceRead, ResourceType: ResourceInstance, ResourceIDs: PathID("instanceId"), ClaimedScopeID: PathScopeID("workspaceId")}
	handler := Authenticate(sessionStub{session: authentication.Session{UserID: string(principalID)}}, Authorize(resolver, authorization.NewEngine(bindings), policy, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { invoked = true })))
	request := httptest.NewRequest(http.MethodGet, "/v1/workspaces/ws_one/instances/lin_two", nil)
	request.SetPathValue("workspaceId", "ws_one")
	request.SetPathValue("instanceId", "lin_two")
	request.AddCookie(&http.Cookie{Name: authentication.SessionCookieName, Value: "valid-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || invoked {
		t.Fatalf("mismatched nested scope status=%d invoked=%v", response.Code, invoked)
	}
}

func TestResourceTypesMapToOneAuthoritativeTable(t *testing.T) {
	for _, test := range []struct {
		resource ResourceType
		table    string
		scope    authorization.ScopeType
	}{
		{ResourceRegion, "FROM regions", authorization.ScopeRegion},
		{ResourceWorkspace, "FROM workspaces", authorization.ScopeWorkspace},
		{ResourceInstance, "FROM managed_instances", authorization.ScopeWorkspace},
		{ResourceOperation, "FROM operations", authorization.ScopeWorkspace},
		{ResourceBackup, "FROM backup_requests", authorization.ScopeWorkspace},
	} {
		query, scope, err := scopeQuery(test.resource)
		if err != nil || !strings.Contains(query, test.table) || scope != test.scope || strings.Contains(strings.ToUpper(query), " JOIN ") {
			t.Fatalf("scope query for %q = %q scope=%q err=%v", test.resource, query, scope, err)
		}
	}
	if _, _, err := scopeQuery("client-defined"); err == nil {
		t.Fatal("unknown resource type did not fail closed")
	}
}
