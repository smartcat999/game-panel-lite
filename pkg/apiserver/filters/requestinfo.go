package filters

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
)

const HeaderRequestID = "X-Request-ID"

// WithRequestInfo attaches a parsed RequestInfo and X-Request-ID to the request context and response headers
// (modeled after k8s.io/apiserver/pkg/endpoints/filters.WithRequestInfo & KubeSphere pkg/apiserver/filters/requestinfo.go).
func WithRequestInfo(handler http.Handler, resolver request.RequestInfoResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reqID := strings.TrimSpace(req.Header.Get(HeaderRequestID))
		if reqID == "" {
			reqID = uuid.NewString()
		}
		w.Header().Set(HeaderRequestID, reqID)

		info, err := resolver.NewRequestInfo(req)
		if err != nil {
			apiruntime.WriteError(w, http.StatusBadRequest, "failed to resolve RequestInfo: "+err.Error())
			return
		}
		info.RequestID = reqID

		ctx := request.WithRequestID(req.Context(), reqID)
		ctx = request.WithRequestInfo(ctx, info)
		handler.ServeHTTP(w, req.WithContext(ctx))
	})
}
