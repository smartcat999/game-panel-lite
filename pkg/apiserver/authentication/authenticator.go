package authentication

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
)

// Request authenticates an incoming HTTP request
// (modeled after k8s.io/apiserver/pkg/authentication/request.Authenticator).
type Request interface {
	AuthenticateRequest(req *http.Request) (request.UserInfo, bool, error)
}

// TokenVerifier verifies a raw Bearer token against the session operator/cache and returns the authenticated UserInfo.
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (request.UserInfo, error)
}

// TokenVerifierFunc adapts a function to the TokenVerifier interface.
type TokenVerifierFunc func(ctx context.Context, token string) (request.UserInfo, error)

func (f TokenVerifierFunc) Verify(ctx context.Context, token string) (request.UserInfo, error) {
	return f(ctx, token)
}

// BearerTokenAuthenticator authenticates requests carrying an "Authorization: Bearer <token>" header.
type BearerTokenAuthenticator struct {
	Verifier TokenVerifier
}

func NewBearerTokenAuthenticator(verifier TokenVerifier) Request {
	return &BearerTokenAuthenticator{Verifier: verifier}
}

func (a *BearerTokenAuthenticator) AuthenticateRequest(req *http.Request) (request.UserInfo, bool, error) {
	var rawToken string
	hdr := strings.TrimSpace(req.Header.Get("Authorization"))
	if hdr != "" {
		if !strings.HasPrefix(hdr, "Bearer ") {
			return request.UserInfo{}, false, errors.New("invalid Authorization header format, expected Bearer token")
		}
		rawToken = strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer "))
		if rawToken == "" {
			return request.UserInfo{}, false, errors.New("empty Bearer token")
		}
	} else {
		// Browser WebSocket upgrades cannot attach a custom Authorization header, so allow
		// ?access_token=<jwt> as a fallback (matching standard OAuth2 / K8s WebSocket bearer auth).
		rawToken = strings.TrimSpace(req.URL.Query().Get("access_token"))
		if rawToken == "" {
			return request.UserInfo{}, false, nil
		}
	}
	user, err := a.Verifier.Verify(req.Context(), rawToken)
	if err != nil {
		return request.UserInfo{}, false, err
	}
	if len(user.Groups) == 0 {
		user.Groups = []string{request.GroupAuthenticated}
	}
	return user, true, nil
}

// AnonymousAuthenticator assigns system:anonymous to unauthenticated requests so downstream
// PathAuthorizer / RBACAuthorizer can decide whether anonymous access is permitted.
type AnonymousAuthenticator struct{}

func NewAnonymousAuthenticator() Request {
	return &AnonymousAuthenticator{}
}

func (a *AnonymousAuthenticator) AuthenticateRequest(_ *http.Request) (request.UserInfo, bool, error) {
	return request.UserInfo{
		Kind:   request.KindSystem,
		Name:   request.AnonymousUser,
		Groups: []string{request.GroupUnauthenticated},
	}, true, nil
}

// UnionAuthenticator chains multiple request authenticators in order
// (modeled after k8s.io/apiserver/pkg/authentication/request/union).
type UnionAuthenticator struct {
	Handlers    []Request
	FailOnError bool
}

func NewUnionAuthenticator(handlers ...Request) Request {
	return &UnionAuthenticator{
		Handlers:    handlers,
		FailOnError: true,
	}
}

func (u *UnionAuthenticator) AuthenticateRequest(req *http.Request) (request.UserInfo, bool, error) {
	var errlist []error
	for _, h := range u.Handlers {
		info, ok, err := h.AuthenticateRequest(req)
		if err != nil {
			if u.FailOnError {
				return request.UserInfo{}, false, err
			}
			errlist = append(errlist, err)
			continue
		}
		if ok {
			return info, true, nil
		}
	}
	return request.UserInfo{}, false, errors.Join(errlist...)
}
