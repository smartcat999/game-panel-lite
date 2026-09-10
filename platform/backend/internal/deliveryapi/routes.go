package deliveryapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/httpfilter"
)

type Services struct {
	Control interface {
		Operation(context.Context, string, string) (deliverycontrol.Operation, error)
	}
	Sessions   httpfilter.Authenticator
	Authorizer httpfilter.Authorizer
	Resolver   httpfilter.ScopeResolver
}

func Routes(services Services) http.Handler {
	mux := http.NewServeMux()
	policy := httpfilter.RoutePolicy{Action: authorization.ActionInstanceRead, ResourceType: httpfilter.ResourceOperation, ResourceIDs: httpfilter.PathID("operationId")}
	mux.Handle("GET /v1/operations/{operationId}", httpfilter.Authenticate(services.Sessions, httpfilter.Authorize(services.Resolver, services.Authorizer, policy, http.HandlerFunc(services.getOperation))))
	return mux
}

func WithFallback(routes, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/operations/") {
			routes.ServeHTTP(response, request)
			return
		}
		fallback.ServeHTTP(response, request)
	})
}

func (s Services) getOperation(response http.ResponseWriter, request *http.Request) {
	resources := httpfilter.AuthorizedResources(request.Context())
	if len(resources) != 1 {
		writeError(response, http.StatusForbidden, "forbidden")
		return
	}
	operation, err := s.Control.Operation(request.Context(), resources[0].Scope.ID, request.PathValue("operationId"))
	if err != nil {
		writeError(response, http.StatusNotFound, "operation_not_found")
		return
	}
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(operation)
}

func writeError(response http.ResponseWriter, status int, code string) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]string{"error": code})
}
