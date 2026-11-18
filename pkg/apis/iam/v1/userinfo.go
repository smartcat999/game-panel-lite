package v1

import "context"

// Well-known caller identities and groups (modeled after k8s.io/apiserver/pkg/authentication/user).
const (
	AnonymousUser        = "system:anonymous"
	GroupAuthenticated   = "system:authenticated"
	GroupUnauthenticated = "system:unauthenticated"
)

// UserInfo describes an authenticated or anonymous caller carried on a context.Context.
// It lives in the API layer so business operators (pkg/models) can read the caller identity
// without importing the HTTP transport layer (pkg/apiserver).
type UserInfo struct {
	SessionID string
	Kind      string // SubjectKindUser, SubjectKindNode, or SubjectKindSystem
	Name      string
	Groups    []string
}

// IsAnonymous reports whether the caller is unauthenticated.
func (u UserInfo) IsAnonymous() bool {
	return u.Name == "" || u.Name == AnonymousUser
}

type userContextKey struct{}

// WithUser returns a copy of parent in which the caller identity is set.
func WithUser(parent context.Context, user UserInfo) context.Context {
	return context.WithValue(parent, userContextKey{}, user)
}

// UserFrom returns the caller identity stored on ctx.
func UserFrom(ctx context.Context) (UserInfo, bool) {
	u, ok := ctx.Value(userContextKey{}).(UserInfo)
	return u, ok
}
