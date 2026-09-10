package authentication

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type OAuthService struct {
	store    Store
	client   OAuthClient
	sessions *SessionService
	now      func() time.Time
	stateTTL time.Duration
}

func NewOAuthService(store Store, client OAuthClient, sessions *SessionService, stateTTL time.Duration) *OAuthService {
	return &OAuthService{store: store, client: client, sessions: sessions, now: time.Now, stateTTL: stateTTL}
}

func (s *OAuthService) Begin(ctx context.Context, returnPath string) (string, error) {
	if !validReturnPath(returnPath) {
		return "", fmt.Errorf("invalid return path")
	}
	state, err := randomToken(32)
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	if err := s.store.SaveOAuthState(ctx, OAuthState{Hash: tokenHash(state), ReturnPath: returnPath, CreatedAt: now, ExpiresAt: now.Add(s.stateTTL)}); err != nil {
		return "", err
	}
	return s.client.AuthorizationURL(state), nil
}

func (s *OAuthService) Complete(ctx context.Context, state, code string) (string, User, string, error) {
	stored, err := s.store.ConsumeOAuthState(ctx, tokenHash(state), s.now().UTC())
	if err != nil {
		return "", User{}, "", ErrInvalidOAuthState
	}
	profile, err := s.client.Exchange(ctx, code)
	if err != nil || profile.Provider != "github" || profile.Subject == "" || profile.Username == "" || profile.Email == "" {
		return "", User{}, "", ErrInvalidCredentials
	}
	user, err := s.store.UpsertOAuthUser(ctx, profile, s.now().UTC())
	if err != nil {
		return "", User{}, "", err
	}
	token, _, err := s.sessions.Issue(ctx, user.ID, true)
	return token, user, stored.ReturnPath, err
}

func validReturnPath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.ContainsAny(path, "\r\n")
}
