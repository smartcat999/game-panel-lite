package authorization

import (
	"context"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
)

// PathAuthorizer authorizes non-resource URLs (/healthz, /readyz) and session lifecycle endpoints
// (modeled after KubeSphere pkg/apiserver/authorization/path). All resource/subresource permissions
// (including self-access on /apis/iam.gamepanel.io/v1/users/{name}) are evaluated by RBACAuthorizer.
type PathAuthorizer struct {
	nonResourcePublicPaths map[string]bool
}

func NewPathAuthorizer() *PathAuthorizer {
	return &PathAuthorizer{
		nonResourcePublicPaths: map[string]bool{
			"/healthz":    true,
			"/livez":      true,
			"/readyz":     true,
			"/metrics":    true,
			"/openapi/v3": true,
		},
	}
}

func (p *PathAuthorizer) Authorize(_ context.Context, a AttributesRecord) (Decision, string, error) {
	if !a.IsResourceRequest {
		if p.nonResourcePublicPaths[a.Path] {
			return DecisionAllow, "public non-resource path", nil
		}
		return DecisionNoOpinion, "", nil
	}

	// Token issuance (POST /apis/iam.gamepanel.io/v1/sessions) is open to unauthenticated callers.
	if a.APIGroup == iamv1.GroupName && a.Resource == "sessions" && a.Verb == iamv1.VerbCreate {
		return DecisionAllow, "public session creation", nil
	}

	// Public read-only access to the application marketplace catalog (handler filters to published apps for anonymous callers).
	if a.User.IsAnonymous() && a.APIGroup == corev1.GroupName && a.Resource == "apps" && a.Subresource == "" &&
		(a.Verb == iamv1.VerbList || a.Verb == iamv1.VerbGet) {
		return DecisionAllow, "public app marketplace read", nil
	}

	// HMAC-SHA256 signed registry package icon proxy (GET /apis/registry.gamepanel.io/v1/packages/{name}/icon?u=...&sig=...).
	if a.APIGroup == registryv1.GroupName && a.Resource == "packages" && a.Subresource == "icon" &&
		a.Verb == iamv1.VerbGet {
		return DecisionAllow, "signed registry package icon read", nil
	}

	// Public workspace invitation inspection and redemption (GET/POST /apis/core.gamepanel.io/v1/tenants/{tenant}/invites/{token});
	// the HMAC-SHA256 signed token in {name} is verified by the handler.
	if a.APIGroup == corev1.GroupName && a.Resource == "invites" && a.Name != "" &&
		(a.Verb == iamv1.VerbGet || a.Verb == iamv1.VerbCreate) {
		return DecisionAllow, "public workspace invite token verification", nil
	}

	// Authenticated users can always list, inspect, or revoke their own active sessions.
	if !a.User.IsAnonymous() && a.User.Kind != iamv1.SubjectKindNode &&
		a.APIGroup == iamv1.GroupName && a.Resource == "sessions" && a.Subresource == "" {
		if a.Name == "" && (a.Verb == iamv1.VerbList || a.Verb == iamv1.VerbDelete) {
			return DecisionAllow, "authenticated self-session collection access", nil
		}
		if a.Name != "" && (a.Verb == iamv1.VerbGet || a.Verb == iamv1.VerbDelete) && isOwnSessionName(a.User, a.Name) {
			return DecisionAllow, "authenticated self-session access", nil
		}
	}

	return DecisionNoOpinion, "", nil
}

func isOwnSessionName(u request.UserInfo, name string) bool {
	if u.SessionID != "" && name == u.SessionID {
		return true
	}
	return u.Name != "" && hasUserSessionIDFormat(u.Name, name)
}

func hasUserSessionIDFormat(username, name string) bool {
	prefix := username + "-"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	hexPart := strings.TrimPrefix(name, prefix)
	if len(hexPart) != 16 {
		return false
	}
	for i := range len(hexPart) {
		c := hexPart[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
