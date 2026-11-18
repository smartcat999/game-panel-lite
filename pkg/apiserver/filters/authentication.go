package filters

import (
	"net/http"

	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/authentication"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
)

// WithAuthentication authenticates the request using the UnionAuthenticator (BearerToken -> Anonymous)
// and attaches the resolved UserInfo to the request context
// (modeled after k8s.io/apiserver/pkg/endpoints/filters.WithAuthentication & KubeSphere pkg/apiserver/filters/authentication.go).
func WithAuthentication(handler http.Handler, auth authentication.Request) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		user, ok, err := auth.AuthenticateRequest(req)
		if err != nil || !ok {
			msg := "unauthorized"
			if err != nil {
				msg = err.Error()
			}
			apiruntime.WriteError(w, http.StatusUnauthorized, msg)
			return
		}
		ctx := request.WithUser(req.Context(), user)
		if request.IsWatchRequest(req) && !user.IsAnonymous() {
			expectedName := user.Name
			expectedKind := user.Kind
			ctx = request.WithWatchRequestRevalidator(ctx, req, func(checkReq *http.Request) error {
				rechecked, ok, err := auth.AuthenticateRequest(checkReq)
				if err != nil {
					return err
				}
				if !ok || rechecked.IsAnonymous() || rechecked.Name != expectedName || rechecked.Kind != expectedKind {
					return request.ErrWatchUnauthorized
				}
				return nil
			})
		}
		req = req.WithContext(ctx)
		handler.ServeHTTP(w, req)
	})
}
