package filters

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	auditdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/audit"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

const (
	maxAuditCaptureBytes = 65536
	maxAuditInlineBytes  = 16384
	auditRedactedValue   = "***REDACTED***"
)

type auditCaptureMeta struct {
	level    string
	reqBody  string
	respBody string
	elapsed  time.Duration
}

type readCloserWrapper struct {
	io.Reader
	io.Closer
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	respBuf     bytes.Buffer
	captureResp bool
	maxCapture  int
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.captureResp && r.maxCapture > 0 && r.respBuf.Len() < r.maxCapture {
		remain := r.maxCapture - r.respBuf.Len()
		if len(b) < remain {
			remain = len(b)
		}
		_, _ = r.respBuf.Write(b[:remain])
	}
	return r.ResponseWriter.Write(b)
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

// WithRequestLog records structured access/audit logs and Prometheus HTTP metrics for every HTTP request
// (modeled after k8s.io/apiserver/pkg/endpoints/filters & KubeSphere pkg/apiserver/filters/auditing.go).
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

// WithAuditLog publishes state-mutating user operations to mq.TopicAudit ("core.v1.events")
// using the structured request.AuditInfo context and configured observability.auditLevel.
func WithAuditLog(handler http.Handler, bus mq.Interface, reg *repo.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		auditInfo := &request.AuditInfo{}
		req = req.WithContext(request.WithAuditInfo(req.Context(), auditInfo))
		seedRequestAuditTenantUID(req, reg)
		level := resolveAuditLevel()
		meta := auditCaptureMeta{level: level}
		capturePayloads := shouldCaptureAuditPayloads(req, level)
		if capturePayloads {
			meta.reqBody = captureRequestBody(req)
		}
		rec := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
			captureResp:    capturePayloads && level == corev1.AuditLevelRequestResponse,
			maxCapture:     maxAuditCaptureBytes + 1,
		}
		handler.ServeHTTP(rec, req)
		if !shouldEmitAuditEvent(req, auditInfo) {
			return
		}
		meta.elapsed = time.Since(start)
		if rec.captureResp {
			meta.respBody = captureResponseBody(rec)
		}
		ensureFallbackAuditPayloads(req, auditInfo, rec.status, request.RequestIDFrom(req.Context()), &meta)
		emitAndPersistAuditEvent(req, auditInfo, rec.status, meta, bus, reg)
	})
}

func seedRequestAuditTenantUID(req *http.Request, reg *repo.Registry) {
	if reg == nil || reg.Pool() == nil {
		return
	}
	initialInfo, _ := request.RequestInfoFrom(req.Context())
	tenantDB := tenantdb.NewDB(reg.Pool())
	if initialInfo == nil || !initialInfo.IsResourceRequest || tenantDB == nil {
		return
	}
	mutating := initialInfo.Verb == iamv1.VerbCreate || initialInfo.Verb == iamv1.VerbUpdate || initialInfo.Verb == iamv1.VerbPatch || initialInfo.Verb == iamv1.VerbDelete
	tenantName := initialInfo.Tenant
	if tenantName == "" && initialInfo.Resource == "tenants" {
		tenantName = initialInfo.Name
	}
	if mutating && tenantName != "" {
		if tenant, err := tenantDB.Get(req.Context(), tenantName); err == nil && tenant != nil {
			request.SetAuditTenantUID(req.Context(), tenant.Metadata.UID)
		}
	}
}

func shouldEmitAuditEvent(req *http.Request, auditInfo *request.AuditInfo) bool {
	info, _ := request.RequestInfoFrom(req.Context())
	if info == nil || !info.IsResourceRequest || auditInfo.Skip {
		return false
	}
	isMutating := info.Verb == iamv1.VerbCreate || info.Verb == iamv1.VerbUpdate || info.Verb == iamv1.VerbPatch || info.Verb == iamv1.VerbDelete
	if !isMutating {
		return false
	}
	user, _ := request.UserFrom(req.Context())
	if user.Kind == request.KindNode || strings.HasPrefix(user.Name, "system:node") {
		return false
	}
	return info.Subresource != "status" && info.Subresource != "messages" && info.Resource != "sessions"
}

func emitAndPersistAuditEvent(req *http.Request, auditInfo *request.AuditInfo, status int, meta auditCaptureMeta, bus mq.Interface, reg *repo.Registry) {
	info, _ := request.RequestInfoFrom(req.Context())
	user, _ := request.UserFrom(req.Context())
	actor, tenant, resName, targetName := resolveAuditIdentityAndTarget(info, user, auditInfo)
	clientIP := resolveAuditClientIP(req)
	reqID := request.RequestIDFrom(req.Context())
	mergedAttrs := buildAuditMergedAttributes(req, auditInfo, meta)
	ev := mq.NewEvent(mq.EventCreated, corev1.GroupVersion, "Event", tenant, resName, 0, "")
	ev.Topic = mq.TopicAudit
	ev.Attributes = map[string]string{
		"actor": actor, "verb": info.Verb, "apiGroup": info.APIGroup, "resource": info.Resource,
		"subresource": info.Subresource, "resourceName": resName, "action": auditInfo.Action,
		"targetName": targetName, "statusCode": strconv.Itoa(status), "clientIP": clientIP, "requestID": reqID,
	}
	for k, v := range mergedAttrs {
		if _, exists := ev.Attributes[k]; !exists {
			ev.Attributes[k] = v
		}
	}
	if bus != nil {
		_ = bus.Publish(req.Context(), mq.TopicAudit, ev)
	}
	if reg != nil && reg.Pool() != nil {
		if auditDB := auditdb.NewDB(reg.Pool()); auditDB != nil {
			_ = auditDB.Record(req.Context(), &corev1.Event{
				TypeMeta:     metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "Event"},
				Metadata:     metav1.ObjectMeta{Name: ev.ID, UID: ev.ID, Tenant: tenant, CreationTimestamp: ev.Timestamp},
				Actor:        actor,
				Verb:         info.Verb,
				APIGroup:     info.APIGroup,
				Resource:     info.Resource,
				Subresource:  info.Subresource,
				ResourceName: resName,
				Action:       auditInfo.Action,
				TargetName:   targetName,
				Attributes:   mergedAttrs,
				StatusCode:   status,
				ClientIP:     clientIP,
				RequestID:    reqID,
				Timestamp:    ev.Timestamp,
			})
		}
	}
}

func resolveAuditIdentityAndTarget(info *request.RequestInfo, user request.UserInfo, auditInfo *request.AuditInfo) (string, string, string, string) {
	resName := strings.TrimSpace(auditInfo.ResourceName)
	if resName == "" {
		resName = redactSensitiveAuditName(info.Resource, info.Name)
	}
	targetName := strings.TrimSpace(auditInfo.TargetName)
	if targetName == "" {
		targetName = resName
	}
	tenant := info.Tenant
	if tenant == "" && info.Resource == "tenants" && resName != "" {
		tenant = resName
	}
	if resName == "" && tenant != "" {
		resName = tenant
	}
	if targetName == "" && tenant != "" {
		targetName = tenant
	}
	actor := strings.TrimSpace(auditInfo.Actor)
	if actor == "" {
		actor = strings.TrimSpace(user.Name)
	}
	if actor == "" {
		actor = "system:anonymous"
	}
	return actor, tenant, resName, targetName
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

func resolveAuditClientIP(req *http.Request) string {
	return request.ClientIP(req)
}

func buildAuditMergedAttributes(req *http.Request, auditInfo *request.AuditInfo, meta auditCaptureMeta) map[string]string {
	mergedAttrs := map[string]string{
		"method":     req.Method,
		"requestURI": redactSensitiveAuditURI(req.URL.Path),
		"auditLevel": meta.level,
		"duration":   meta.elapsed.Round(time.Millisecond).String(),
	}
	if ua := strings.TrimSpace(req.UserAgent()); ua != "" {
		mergedAttrs["userAgent"] = ua
	}
	if meta.reqBody != "" {
		mergedAttrs["requestBody"] = meta.reqBody
	}
	if meta.respBody != "" {
		mergedAttrs["responseBody"] = meta.respBody
	}
	for k, v := range auditInfo.Attributes {
		if v != "" {
			mergedAttrs[k] = v
		}
	}
	delete(mergedAttrs, "tenantUID")
	if tenantUID := request.AuditTenantUID(req.Context()); tenantUID != "" {
		mergedAttrs["tenantUID"] = tenantUID
	}
	return mergedAttrs
}

func resolveAuditLevel() string {
	return corev1.NormalizeAuditLevel(config.Get().Observability.AuditLevel)
}

func shouldCaptureAuditPayloads(req *http.Request, level string) bool {
	if level == corev1.AuditLevelMetadata || strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
		return false
	}
	info, _ := request.RequestInfoFrom(req.Context())
	if info == nil || !info.IsResourceRequest {
		return false
	}
	mutating := info.Verb == iamv1.VerbCreate || info.Verb == iamv1.VerbUpdate || info.Verb == iamv1.VerbPatch || info.Verb == iamv1.VerbDelete
	if !mutating || info.Subresource == "status" || info.Subresource == "messages" || info.Subresource == "log" || info.Resource == "sessions" {
		return false
	}
	if info.Subresource == "exec" && req.Method == http.MethodGet {
		return false
	}
	user, _ := request.UserFrom(req.Context())
	if user.Kind == request.KindNode || strings.HasPrefix(user.Name, "system:node") {
		return false
	}
	ct := strings.ToLower(req.Header.Get("Content-Type"))
	return !strings.HasPrefix(ct, "multipart/") && !strings.HasPrefix(ct, "application/octet-stream")
}

func captureRequestBody(req *http.Request) string {
	if req.Body == nil || req.Body == http.NoBody || req.Method == http.MethodGet || req.Method == http.MethodHead {
		return ""
	}
	origBody := req.Body
	buf, err := io.ReadAll(io.LimitReader(origBody, maxAuditCaptureBytes+1))
	req.Body = readCloserWrapper{
		Reader: io.MultiReader(bytes.NewReader(buf), origBody),
		Closer: origBody,
	}
	if err != nil || len(buf) == 0 {
		return ""
	}
	if len(buf) > maxAuditCaptureBytes {
		return fmt.Sprintf("[omitted: payload exceeds %d bytes]", maxAuditCaptureBytes)
	}
	return sanitizeAuditPayload(buf)
}

func captureResponseBody(rec *statusRecorder) string {
	if rec.status == http.StatusSwitchingProtocols || rec.respBuf.Len() == 0 {
		return ""
	}
	ct := strings.ToLower(rec.Header().Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "json") && !strings.HasPrefix(ct, "text/") {
		return ""
	}
	if rec.respBuf.Len() > maxAuditCaptureBytes {
		return fmt.Sprintf("[omitted: response exceeds %d bytes]", maxAuditCaptureBytes)
	}
	return sanitizeAuditPayload(rec.respBuf.Bytes())
}

func ensureFallbackAuditPayloads(req *http.Request, auditInfo *request.AuditInfo, status int, reqID string, meta *auditCaptureMeta) {
	if meta == nil || meta.level == corev1.AuditLevelMetadata {
		return
	}
	info, _ := request.RequestInfoFrom(req.Context())
	if meta.reqBody == "" {
		meta.reqBody = buildFallbackAuditRequestPayload(req, info, auditInfo)
	}
	if meta.level == corev1.AuditLevelRequestResponse && meta.respBody == "" {
		meta.respBody = buildFallbackAuditResponsePayload(status, reqID, meta.elapsed)
	}
}

func buildFallbackAuditRequestPayload(req *http.Request, info *request.RequestInfo, auditInfo *request.AuditInfo) string {
	payload := map[string]any{
		"method":     req.Method,
		"requestURI": redactSensitiveAuditURI(req.URL.Path),
	}
	if q := redactAuditQuery(req.URL.Query()); len(q) > 0 {
		payload["query"] = q
	}
	if info != nil {
		if info.APIGroup != "" {
			payload["apiGroup"] = info.APIGroup
		}
		if info.Tenant != "" {
			payload["tenant"] = info.Tenant
		}
		if info.Resource != "" {
			payload["resource"] = info.Resource
		}
		if info.Subresource != "" {
			payload["subresource"] = info.Subresource
		}
		if name := redactSensitiveAuditName(info.Resource, info.Name); name != "" {
			payload["name"] = name
		}
	}
	if attrs := buildFallbackRequestAttributes(auditInfo, payload); len(attrs) > 0 {
		payload["attributes"] = attrs
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(raw)
}

func buildFallbackRequestAttributes(auditInfo *request.AuditInfo, payload map[string]any) map[string]string {
	if auditInfo == nil {
		return nil
	}
	if auditInfo.Action != "" {
		payload["action"] = auditInfo.Action
	}
	out := make(map[string]string)
	for k, v := range auditInfo.Attributes {
		if v == "" || k == "tenantUID" {
			continue
		}
		if isSensitiveAuditKey(k) {
			out[k] = auditRedactedValue
			continue
		}
		out[k] = v
	}
	return out
}

func buildFallbackAuditResponsePayload(status int, reqID string, elapsed time.Duration) string {
	payload := map[string]any{
		"statusCode": status,
		"status":     http.StatusText(status),
		"duration":   elapsed.Round(time.Millisecond).String(),
	}
	if reqID != "" {
		payload["requestId"] = reqID
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(raw)
}

func redactAuditQuery(values map[string][]string) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for k, vals := range values {
		if isSensitiveAuditKey(k) {
			out[k] = auditRedactedValue
			continue
		}
		if len(vals) == 1 {
			out[k] = vals[0]
		} else {
			out[k] = vals
		}
	}
	return out
}

func sanitizeAuditPayload(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err == nil {
		clean, err := json.Marshal(redactAuditJSON(decoded))
		if err != nil {
			return ""
		}
		if len(clean) > maxAuditInlineBytes {
			return fmt.Sprintf("[omitted: sanitized JSON exceeds %d bytes]", maxAuditInlineBytes)
		}
		return string(clean)
	}
	if !utf8.Valid(trimmed) {
		return "[binary payload omitted]"
	}
	if len(trimmed) > 512 {
		return string(trimmed[:512]) + "...(truncated)"
	}
	return string(trimmed)
}

func redactAuditJSON(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			if isSensitiveAuditKey(k) {
				out[k] = auditRedactedValue
				continue
			}
			out[k] = redactAuditJSON(val)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactAuditJSON(item)
		}
		return out
	default:
		return v
	}
}

func isSensitiveAuditKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "password", "adminpassword", "oldpassword", "newpassword", "currentpassword",
		"confirmpassword", "passwordhash", "secret", "secretkey", "accesskey",
		"jwtsecret", "apikey", "steamwebapikey", "token", "accesstoken",
		"refreshtoken", "idtoken", "sessiontoken", "authorization", "webhookurl",
		"credentials", "credential", "privatekey", "kubeconfig",
		"ticket", "uploadurl", "downloadurl", "directurl":
		return true
	}
	if strings.HasSuffix(k, "password") || strings.HasSuffix(k, "secret") ||
		strings.HasSuffix(k, "secretkey") || strings.HasSuffix(k, "accesskey") {
		return true
	}
	return strings.HasSuffix(k, "token") && k != "continuetoken" && k != "pagetoken"
}
