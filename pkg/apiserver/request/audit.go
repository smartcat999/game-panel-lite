package request

import "context"

type auditContextKey struct{}

// AuditInfo holds structured domain-level audit metadata populated by resource handlers
// (modeled after k8s.io/apiserver/pkg/audit.AddAuditAnnotation).
type AuditInfo struct {
	Skip         bool
	Actor        string
	ResourceName string
	Action       string
	TargetName   string
	Attributes   map[string]string
	tenantUID    string
}

// WithAuditInfo attaches a mutable AuditInfo holder to the request context.
func WithAuditInfo(ctx context.Context, info *AuditInfo) context.Context {
	return context.WithValue(ctx, auditContextKey{}, info)
}

// AuditInfoFrom retrieves the mutable AuditInfo holder from the request context.
func AuditInfoFrom(ctx context.Context) *AuditInfo {
	if v, ok := ctx.Value(auditContextKey{}).(*AuditInfo); ok {
		return v
	}
	return nil
}

// SkipAudit marks the current request as non-auditable (e.g. read-only subresource POSTs like files/list).
func SkipAudit(ctx context.Context) {
	if info := AuditInfoFrom(ctx); info != nil {
		info.Skip = true
	}
}

// SetAuditTarget records structured domain semantics for the current request's audit event.
func SetAuditTarget(ctx context.Context, action, targetName string, attrs map[string]string) {
	info := AuditInfoFrom(ctx)
	if info == nil {
		return
	}
	if action != "" {
		info.Action = action
	}
	if targetName != "" {
		info.TargetName = targetName
	}
	if len(attrs) > 0 {
		if info.Attributes == nil {
			info.Attributes = make(map[string]string, len(attrs))
		}
		for k, v := range attrs {
			info.Attributes[k] = v
		}
	}
}

// SetAuditTenantUID freezes a server-resolved Tenant UID. Handlers may supply
// the UID from a successful create result; an existing request snapshot cannot
// be replaced with the identity of a later same-name resource.
func SetAuditTenantUID(ctx context.Context, tenantUID string) {
	if info := AuditInfoFrom(ctx); info != nil && info.tenantUID == "" && tenantUID != "" {
		info.tenantUID = tenantUID
	}
}

// AuditTenantUID returns the trusted identity snapshot, independently of any
// domain attributes that a handler may attach to the audit event.
func AuditTenantUID(ctx context.Context) string {
	if info := AuditInfoFrom(ctx); info != nil {
		return info.tenantUID
	}
	return ""
}

// SetAuditResourceName records the created resource's metadata.name on POST requests.
func SetAuditResourceName(ctx context.Context, resourceName string) {
	if info := AuditInfoFrom(ctx); info != nil && resourceName != "" {
		info.ResourceName = resourceName
		if info.TargetName == "" {
			info.TargetName = resourceName
		}
	}
}

// SetAuditActor overrides the authenticated principal recorded on the audit event
// (used when a public flow such as accepting a workspace invite authenticates or provisions the user mid-request).
func SetAuditActor(ctx context.Context, actor string) {
	if info := AuditInfoFrom(ctx); info != nil && actor != "" {
		info.Actor = actor
	}
}
