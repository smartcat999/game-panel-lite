package httpfilter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authentication"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
)

type contextKey string

const (
	principalKey contextKey = "authenticated_principal"
	sessionKey   contextKey = "authenticated_session"
	resourcesKey contextKey = "authorized_resources"
)

type Principal struct {
	ID string
}

type Authenticator interface {
	Authenticate(context.Context, string) (authentication.Session, error)
}

type ScopeResolver interface {
	Resolve(context.Context, ResourceType, []string) (map[string]authorization.Scope, error)
}

type Authorizer interface {
	CheckBatch(context.Context, authorization.PrincipalID, []authorization.Check) ([]authorization.Decision, error)
}

type ResourceType string

const (
	ResourcePlatform  ResourceType = "platform"
	ResourceRegion    ResourceType = "region"
	ResourceWorkspace ResourceType = "workspace"
	ResourceInstance  ResourceType = "instance"
	ResourceBackup    ResourceType = "backup"
)

type ResourceIDs func(*http.Request) ([]string, error)

type RoutePolicy struct {
	Action         authorization.Action
	ResourceType   ResourceType
	ResourceIDs    ResourceIDs
	ClaimedScopeID func(*http.Request) string
}

type ResolvedResource struct {
	Type  ResourceType
	ID    string
	Scope authorization.Scope
}

func Authenticate(authenticator Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		token, err := authentication.SessionToken(request)
		if err != nil {
			writeError(response, http.StatusUnauthorized, "authentication_required")
			return
		}
		session, err := authenticator.Authenticate(request.Context(), token)
		if err != nil {
			writeError(response, http.StatusUnauthorized, "invalid_session")
			return
		}
		ctx := context.WithValue(request.Context(), principalKey, Principal{ID: session.UserID})
		ctx = context.WithValue(ctx, sessionKey, session)
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

func RequireRecentReauthentication(sessions *authentication.SessionService, next http.Handler) http.Handler {
	return requireSessionCondition(sessions.RequireRecentReauthentication, next)
}

func RequireOperatorVerification(sessions *authentication.SessionService, next http.Handler) http.Handler {
	return requireSessionCondition(sessions.RequireRecentOperatorVerification, next)
}

func requireSessionCondition(check func(authentication.Session) error, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		session, ok := SessionFromContext(request.Context())
		if !ok || check(session) != nil {
			writeError(response, http.StatusForbidden, "reauthentication_required")
			return
		}
		next.ServeHTTP(response, request)
	})
}

func Authorize(resolver ScopeResolver, authorizer Authorizer, policy RoutePolicy, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		principal, ok := PrincipalFromContext(request.Context())
		if !ok {
			writeError(response, http.StatusUnauthorized, "authentication_required")
			return
		}
		ids, err := policy.ResourceIDs(request)
		if err != nil || len(ids) == 0 {
			writeError(response, http.StatusBadRequest, "invalid_resource_ids")
			return
		}
		ids = unique(ids)
		if len(ids) > authorization.MaxBatchSize {
			writeError(response, http.StatusBadRequest, "batch_limit_exceeded")
			return
		}
		scopes, err := resolver.Resolve(request.Context(), policy.ResourceType, ids)
		if err != nil {
			writeError(response, http.StatusInternalServerError, "scope_resolution_failed")
			return
		}
		checks := make([]authorization.Check, 0, len(ids))
		resources := make([]ResolvedResource, 0, len(ids))
		for _, id := range ids {
			scope, exists := scopes[id]
			if !exists {
				writeError(response, http.StatusForbidden, "forbidden")
				return
			}
			if policy.ClaimedScopeID != nil && scope.ID != policy.ClaimedScopeID(request) {
				writeError(response, http.StatusForbidden, "forbidden")
				return
			}
			checks = append(checks, authorization.Check{ResourceType: string(policy.ResourceType), ResourceID: id, Scope: scope, Action: policy.Action})
			resources = append(resources, ResolvedResource{Type: policy.ResourceType, ID: id, Scope: scope})
		}
		decisions, err := authorizer.CheckBatch(request.Context(), authorization.PrincipalID(principal.ID), checks)
		if err != nil {
			writeError(response, http.StatusInternalServerError, "authorization_failed")
			return
		}
		for _, decision := range decisions {
			if !decision.Allowed {
				writeError(response, http.StatusForbidden, "forbidden")
				return
			}
		}
		ctx := context.WithValue(request.Context(), resourcesKey, resources)
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

func PathScopeID(name string) func(*http.Request) string {
	return func(request *http.Request) string {
		return strings.TrimSpace(request.PathValue(name))
	}
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey).(Principal)
	return principal, ok
}

func SessionFromContext(ctx context.Context) (authentication.Session, bool) {
	session, ok := ctx.Value(sessionKey).(authentication.Session)
	return session, ok
}

func AuthorizedResources(ctx context.Context) []ResolvedResource {
	resources, _ := ctx.Value(resourcesKey).([]ResolvedResource)
	return append([]ResolvedResource(nil), resources...)
}

func PathID(name string) ResourceIDs {
	return func(request *http.Request) ([]string, error) {
		value := strings.TrimSpace(request.PathValue(name))
		if value == "" {
			return nil, errors.New("missing path resource id")
		}
		return []string{value}, nil
	}
}

func QueryIDs(name string) ResourceIDs {
	return func(request *http.Request) ([]string, error) {
		values := request.URL.Query()[name]
		if len(values) == 0 {
			return nil, errors.New("missing query resource ids")
		}
		var result []string
		for _, value := range values {
			for _, id := range strings.Split(value, ",") {
				if id = strings.TrimSpace(id); id != "" {
					result = append(result, id)
				}
			}
		}
		return result, nil
	}
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func writeError(response http.ResponseWriter, status int, code string) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]string{"error": code})
}
