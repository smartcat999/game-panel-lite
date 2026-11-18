package request

import (
	"context"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
)

// Caller identity is defined in pkg/apis/iam/v1 (Layer 1) so pkg/models never imports the HTTP layer.
// These aliases keep the apiserver filter chain and kapis handlers source-compatible.
const (
	AnonymousUser        = iamv1.AnonymousUser
	GroupAuthenticated   = iamv1.GroupAuthenticated
	GroupUnauthenticated = iamv1.GroupUnauthenticated

	KindUser   = iamv1.SubjectKindUser
	KindNode   = iamv1.SubjectKindNode
	KindGroup  = iamv1.SubjectKindGroup
	KindSystem = iamv1.SubjectKindSystem
)

// UserInfo describes an authenticated or anonymous caller on the request context.
type UserInfo = iamv1.UserInfo

// WithUser returns a copy of parent context in which the user value is set.
func WithUser(parent context.Context, user UserInfo) context.Context {
	return iamv1.WithUser(parent, user)
}

// UserFrom returns the value of the user key on the ctx.
func UserFrom(ctx context.Context) (UserInfo, bool) {
	return iamv1.UserFrom(ctx)
}
