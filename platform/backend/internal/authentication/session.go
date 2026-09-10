package authentication

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const SessionCookieName = "gamepanel_session"

type SessionPolicy struct {
	AbsoluteLifetime time.Duration
	IdleTimeout      time.Duration
	ReauthWindow     time.Duration
	SecureCookies    bool
}

type SessionService struct {
	store  Store
	policy SessionPolicy
	now    func() time.Time
}

func NewSessionService(store Store, policy SessionPolicy) *SessionService {
	return &SessionService{store: store, policy: policy, now: time.Now}
}

func (s *SessionService) Issue(ctx context.Context, userID string, reauthenticated bool) (string, Session, error) {
	return s.issue(ctx, userID, reauthenticated, false)
}

func (s *SessionService) issue(ctx context.Context, userID string, reauthenticated, operatorVerified bool) (string, Session, error) {
	now := s.now().UTC()
	token, err := randomToken(32)
	if err != nil {
		return "", Session{}, err
	}
	id, err := randomID("ses")
	if err != nil {
		return "", Session{}, err
	}
	session := Session{
		ID:         id,
		UserID:     userID,
		TokenHash:  tokenHash(token),
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(s.policy.AbsoluteLifetime),
	}
	if reauthenticated {
		session.ReauthenticatedAt = &now
	}
	if operatorVerified {
		session.OperatorVerifiedAt = &now
	}
	if err := s.store.PutSession(ctx, session); err != nil {
		return "", Session{}, err
	}
	return token, session, nil
}

func (s *SessionService) Authenticate(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrInvalidSession
	}
	hash := tokenHash(token)
	session, err := s.store.SessionByTokenHash(ctx, hash)
	if err != nil {
		return Session{}, ErrInvalidSession
	}
	now := s.now().UTC()
	if session.RevokedAt != nil || !now.Before(session.ExpiresAt) || now.Sub(session.LastSeenAt) > s.policy.IdleTimeout {
		return Session{}, ErrInvalidSession
	}
	if err := s.store.TouchSession(ctx, hash, now); err != nil {
		return Session{}, ErrInvalidSession
	}
	session.LastSeenAt = now
	return session, nil
}

func (s *SessionService) RotateAfterReauthentication(ctx context.Context, currentToken string) (string, Session, error) {
	session, err := s.Authenticate(ctx, currentToken)
	if err != nil {
		return "", Session{}, err
	}
	if err := s.store.RevokeSession(ctx, tokenHash(currentToken), s.now().UTC()); err != nil {
		return "", Session{}, err
	}
	return s.Issue(ctx, session.UserID, true)
}

func (s *SessionService) RotateAfterOperatorVerification(ctx context.Context, currentToken string) (string, Session, error) {
	session, err := s.Authenticate(ctx, currentToken)
	if err != nil {
		return "", Session{}, err
	}
	if err := s.store.RevokeSession(ctx, tokenHash(currentToken), s.now().UTC()); err != nil {
		return "", Session{}, err
	}
	return s.issue(ctx, session.UserID, true, true)
}

func (s *SessionService) RequireRecentReauthentication(session Session) error {
	if session.ReauthenticatedAt == nil || s.now().UTC().Sub(*session.ReauthenticatedAt) > s.policy.ReauthWindow {
		return ErrReauthentication
	}
	return nil
}

func (s *SessionService) RequireRecentOperatorVerification(session Session) error {
	if session.OperatorVerifiedAt == nil || s.now().UTC().Sub(*session.OperatorVerifiedAt) > s.policy.ReauthWindow {
		return ErrReauthentication
	}
	return nil
}

func (s *SessionService) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return ErrInvalidSession
	}
	if err := s.store.RevokeSession(ctx, tokenHash(token), s.now().UTC()); err != nil {
		return ErrInvalidSession
	}
	return nil
}

func (s *SessionService) Cookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.policy.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.policy.AbsoluteLifetime.Seconds()),
	}
}

func SessionToken(request *http.Request) (string, error) {
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", ErrInvalidSession
	}
	return cookie.Value, nil
}

func IsInvalidSession(err error) bool {
	return errors.Is(err, ErrInvalidSession)
}
