package authorization

import (
	"context"

	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
)

// Decision represents an authorization decision
// (modeled after k8s.io/apiserver/pkg/authorization/authorizer.Decision).
type Decision int

const (
	DecisionDeny Decision = iota
	DecisionAllow
	DecisionNoOpinion
)

// AttributesRecord carries the request attributes needed for Path and RBAC authorization
// (modeled after k8s.io/apiserver/pkg/authorization/authorizer.AttributesRecord).
type AttributesRecord struct {
	User              request.UserInfo
	IsResourceRequest bool
	Path              string
	Verb              string
	APIGroup          string
	APIVersion        string
	Tenant            string
	Resource          string
	Subresource       string
	Name              string
	Labels            map[string]string
}

func (a AttributesRecord) QualifiedResource() string {
	if a.Subresource != "" {
		return a.Resource + "/" + a.Subresource
	}
	return a.Resource
}

// Authorizer makes an authorization decision based on AttributesRecord.
type Authorizer interface {
	Authorize(ctx context.Context, a AttributesRecord) (Decision, string, error)
}

// UnionAuthorizer chains multiple Authorizers in order (e.g. PathAuthorizer -> RBACAuthorizer)
// (modeled after k8s.io/apiserver/pkg/authorization/union & KubeSphere pkg/apiserver/authorization/union).
type UnionAuthorizer []Authorizer

func NewUnionAuthorizer(authorizers ...Authorizer) Authorizer {
	return UnionAuthorizer(authorizers)
}

func (u UnionAuthorizer) Authorize(ctx context.Context, a AttributesRecord) (Decision, string, error) {
	for _, authz := range u {
		decision, reason, err := authz.Authorize(ctx, a)
		if err != nil {
			return DecisionDeny, reason, err
		}
		switch decision {
		case DecisionAllow, DecisionDeny:
			return decision, reason, nil
		case DecisionNoOpinion:
		}
	}
	return DecisionNoOpinion, "access denied", nil
}
