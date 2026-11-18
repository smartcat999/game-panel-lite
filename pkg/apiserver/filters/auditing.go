package filters

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not support http.Hijacker")
	}
	return hijacker.Hijack()
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// WithRequestLog records structured access/audit logs and Prometheus HTTP metrics for every HTTP request.
func WithRequestLog(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		handler.ServeHTTP(rec, req)
		elapsed := time.Since(start)
		info, _ := request.RequestInfoFrom(req.Context())
		user, _ := request.UserFrom(req.Context())
		reqID := request.RequestIDFrom(req.Context())
		resourceLabel := req.URL.Path
		if info != nil && info.IsResourceRequest {
			resourceLabel = info.QualifiedResource()
		}
		metrics.HTTPRequestsTotal.WithLabelValues(req.Method, resourceLabel, strconv.Itoa(rec.status)).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(req.Method, resourceLabel).Observe(elapsed.Seconds())
		logHTTPRequestOutcome(req, info, user, reqID, rec.status, elapsed)
	})
}

func logHTTPRequestOutcome(req *http.Request, info *request.RequestInfo, user request.UserInfo, reqID string, status int, elapsed time.Duration) {
	if info == nil || !info.IsResourceRequest {
		log.Debug("apiserver non-resource request", "requestId", reqID, "method", req.Method, "path", req.URL.Path, "status", status, "duration", elapsed)
		return
	}
	isMutating := info.Verb == iamv1.VerbCreate || info.Verb == iamv1.VerbUpdate || info.Verb == iamv1.VerbPatch || info.Verb == iamv1.VerbDelete
	isNodeHeartbeat := (user.Kind == request.KindNode || strings.HasPrefix(user.Name, "system:node")) && info.Subresource == "status" && status < 400
	safeURI := redactSensitiveAuditURI(req.URL.Path)
	safeName := redactSensitiveAuditName(info.Resource, info.Name)
	if isMutating && !isNodeHeartbeat {
		log.Info("audit.event",
			"stage", "ResponseComplete", "requestId", reqID, "method", req.Method, "requestURI", safeURI,
			"apiGroup", info.APIGroup, "apiVersion", info.APIVersion, "resource", info.QualifiedResource(),
			"tenant", info.Tenant, "name", safeName, "verb", info.Verb,
			"userKind", user.Kind, "user", user.Name, "sourceIP", req.RemoteAddr, "status", status, "duration", elapsed,
		)
		return
	}
	log.Debug("apiserver request",
		"requestId", reqID, "method", req.Method, "path", safeURI, "apiGroup", info.APIGroup,
		"resource", info.QualifiedResource(), "tenant", info.Tenant, "name", safeName,
		"verb", info.Verb, "user", user.Name, "status", status, "duration", elapsed,
	)
}

func redactSensitiveAuditName(resource, name string) string {
	if resource == "invites" {
		return ""
	}
	return name
}

func redactSensitiveAuditURI(rawPath string) string {
	const marker = "/invites/"
	idx := strings.Index(rawPath, marker)
	if idx < 0 {
		return rawPath
	}
	prefix := rawPath[:idx+len(marker)]
	rest := rawPath[idx+len(marker):]
	if rest == "" {
		return rawPath
	}
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		return prefix + "[REDACTED]" + rest[slash:]
	}
	return prefix + "[REDACTED]"
}
