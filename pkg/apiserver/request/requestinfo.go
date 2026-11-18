package request

import (
	"context"
	"net"
	"net/http"
	"strings"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
)

// RequestInfo holds information parsed from the http.Request
// (modeled after k8s.io/apiserver/pkg/endpoints/request.RequestInfo & KubeSphere pkg/apiserver/request).
type RequestInfo struct {
	RequestID         string
	IsResourceRequest bool
	Path              string
	Verb              string
	APIPrefix         string
	APIGroup          string
	APIVersion        string
	Tenant            string
	Resource          string
	Subresource       string
	Name              string
	Parts             []string
}

// QualifiedResource returns "resource" or "resource/subresource" for RBAC matching.
func (r *RequestInfo) QualifiedResource() string {
	if r == nil {
		return ""
	}
	if r.Subresource != "" {
		return r.Resource + "/" + r.Subresource
	}
	return r.Resource
}

// RequestInfoResolver parses an HTTP request into a structured RequestInfo.
type RequestInfoResolver interface {
	NewRequestInfo(req *http.Request) (*RequestInfo, error)
}

// RequestInfoFactory implements RequestInfoResolver for Kubernetes/KubeSphere-style "/apis/{group}/{version}/..." paths.
type RequestInfoFactory struct {
	APIPrefixes map[string]bool
}

func NewRequestInfoResolver() RequestInfoResolver {
	return &RequestInfoFactory{
		APIPrefixes: map[string]bool{"apis": true},
	}
}

func (f *RequestInfoFactory) NewRequestInfo(req *http.Request) (*RequestInfo, error) {
	info := &RequestInfo{
		IsResourceRequest: false,
		Path:              req.URL.Path,
		Verb:              strings.ToLower(req.Method),
	}
	currentParts := splitPath(req.URL.Path)
	if len(currentParts) < 3 || !f.APIPrefixes[currentParts[0]] {
		return info, nil
	}
	info.APIPrefix = currentParts[0]
	info.APIGroup = currentParts[1]
	info.APIVersion = currentParts[2]
	info.IsResourceRequest = true
	info.Parts = currentParts[3:]
	tail := info.Parts
	if len(tail) == 0 {
		info.IsResourceRequest = false
		return info, nil
	}
	isCollection := parseResourceTail(info, tail)
	if info.Subresource == "exec" {
		info.Verb = iamv1.VerbCreate
	} else {
		isWatch := req.Method == http.MethodGet && isCollection && info.Subresource == "" &&
			IsWatchableCollection(info.APIGroup, info.Resource) && hasWatchQueryParam(req)
		info.Verb = resolveResourceVerb(req.Method, isCollection, isWatch)
	}
	return info, nil
}

func hasWatchQueryParam(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	v := req.URL.Query().Get("watch")
	return v == "true" || v == "1"
}

// IsWatchRequest reports whether the HTTP request requests a watch stream on a watchable collection (?watch=true or ?watch=1).
func IsWatchRequest(req *http.Request) bool {
	if req == nil || req.URL == nil || req.Method != http.MethodGet || !hasWatchQueryParam(req) {
		return false
	}
	info, ok := RequestInfoFrom(req.Context())
	if !ok {
		parsed, err := NewRequestInfoResolver().NewRequestInfo(req)
		if err != nil {
			return false
		}
		info = parsed
	}
	return info != nil && info.IsResourceRequest && info.Name == "" && info.Subresource == "" &&
		IsWatchableCollection(info.APIGroup, info.Resource)
}

func parseResourceTail(info *RequestInfo, tail []string) bool {
	if len(tail) >= 3 && tail[0] == "tenants" && tail[2] != "status" {
		info.Tenant = tail[1]
		info.Resource = tail[2]
		switch len(tail) {
		case 3:
			return true
		case 4:
			info.Name = tail[3]
		default:
			info.Name = tail[3]
			info.Subresource = tail[4]
		}
		return false
	}
	info.Resource = tail[0]
	switch len(tail) {
	case 1:
		return true
	case 2:
		info.Name = tail[1]
	default:
		info.Name = tail[1]
		info.Subresource = tail[2]
	}
	return false
}

func resolveResourceVerb(method string, isCollection bool, isWatch bool) string {
	switch method {
	case http.MethodGet:
		if isWatch {
			return iamv1.VerbWatch
		}
		if isCollection {
			return iamv1.VerbList
		}
		return iamv1.VerbGet
	case http.MethodPost:
		return iamv1.VerbCreate
	case http.MethodPut:
		return iamv1.VerbUpdate
	case http.MethodPatch:
		return iamv1.VerbPatch
	case http.MethodDelete:
		return iamv1.VerbDelete
	default:
		return strings.ToLower(method)
	}
}

func splitPath(thePath string) []string {
	thePath = strings.Trim(thePath, "/")
	if thePath == "" {
		return nil
	}
	return strings.Split(thePath, "/")
}

type (
	requestInfoContextKey struct{}
	requestIDContextKey   struct{}
)

// WithRequestInfo returns a copy of parent context in which the RequestInfo value is set.
func WithRequestInfo(parent context.Context, info *RequestInfo) context.Context {
	return context.WithValue(parent, requestInfoContextKey{}, info)
}

// RequestInfoFrom returns the value of the RequestInfo key on the ctx.
func RequestInfoFrom(ctx context.Context) (*RequestInfo, bool) {
	info, ok := ctx.Value(requestInfoContextKey{}).(*RequestInfo)
	return info, ok
}

// WithRequestID returns a copy of parent context in which the RequestID value is set.
func WithRequestID(parent context.Context, requestID string) context.Context {
	return context.WithValue(parent, requestIDContextKey{}, requestID)
}

// RequestIDFrom returns the X-Request-ID stored on the ctx.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

// ClientIP extracts the client IP address from req, trusting X-Forwarded-For / X-Real-IP
// only when the direct peer (req.RemoteAddr) is a loopback or private-network reverse proxy.
func ClientIP(req *http.Request) string {
	if req == nil {
		return "127.0.0.1"
	}
	remoteHost := strings.TrimSpace(req.RemoteAddr)
	if host, _, err := net.SplitHostPort(remoteHost); err == nil && host != "" {
		remoteHost = host
	}
	if isTrustedProxyIP(remoteHost) {
		if xff := strings.TrimSpace(req.Header.Get("X-Forwarded-For")); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			if candidate := strings.TrimSpace(first); net.ParseIP(candidate) != nil {
				return candidate
			}
		}
		if xrip := strings.TrimSpace(req.Header.Get("X-Real-IP")); net.ParseIP(xrip) != nil {
			return xrip
		}
	}
	if remoteHost != "" {
		return remoteHost
	}
	return "127.0.0.1"
}

func isTrustedProxyIP(host string) bool {
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}
