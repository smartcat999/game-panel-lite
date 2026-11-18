package filters

import (
	"fmt"
	"net/http"

	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
)

// WithPanicRecovery catches unhandled panics and returns a standard Kubernetes 500 Status object
// (modeled after k8s.io/apiserver/pkg/endpoints/filters.WithPanicRecovery).
func WithPanicRecovery(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("apiserver panic recovered", "method", req.Method, "path", req.URL.Path, "panic", fmt.Sprint(rec))
				apiruntime.WriteError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		handler.ServeHTTP(w, req)
	})
}
