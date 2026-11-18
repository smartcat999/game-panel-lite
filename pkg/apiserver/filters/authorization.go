package filters

import (
	"context"
	"net/http"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/authorization"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
)

// WithAuthorization evaluates the UnionAuthorizer (PathAuthorizer -> RBACAuthorizer) against RequestInfo and UserInfo
// (modeled after k8s.io/apiserver/pkg/endpoints/filters.WithAuthorization & KubeSphere pkg/apiserver/filters/authorization.go).
func WithAuthorization(handler http.Handler, a authorization.Authorizer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		info, ok := request.RequestInfoFrom(ctx)
		if !ok || info == nil {
			apiruntime.WriteError(w, http.StatusInternalServerError, "no RequestInfo found in context")
			return
		}
		user, _ := request.UserFrom(ctx)

		attrs := authorization.AttributesRecord{
			User:              user,
			IsResourceRequest: info.IsResourceRequest,
			Path:              info.Path,
			Verb:              info.Verb,
			APIGroup:          info.APIGroup,
			APIVersion:        info.APIVersion,
			Tenant:            info.Tenant,
			Resource:          info.Resource,
			Subresource:       info.Subresource,
			Name:              info.Name,
		}

		decision, reason, err := a.Authorize(ctx, attrs)
		if err != nil {
			apiruntime.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if decision == authorization.DecisionAllow {
			if info.Verb == iamv1.VerbWatch {
				req = req.WithContext(request.WithWatchRevalidator(ctx, func(checkCtx context.Context) error {
					recheckDecision, _, recheckErr := a.Authorize(checkCtx, attrs)
					if recheckErr != nil {
						return recheckErr
					}
					if recheckDecision != authorization.DecisionAllow {
						return request.ErrWatchUnauthorized
					}
					return nil
				}))
			}
			handler.ServeHTTP(w, req)
			return
		}

		if user.IsAnonymous() {
			apiruntime.WriteError(w, http.StatusUnauthorized, "unauthorized: authentication required")
			return
		}
		if reason == "" {
			reason = "forbidden"
		}
		apiruntime.WriteError(w, http.StatusForbidden, reason)
	})
}
