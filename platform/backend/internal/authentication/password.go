package authentication

import (
	"context"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type PasswordService struct {
	store    Store
	sessions *SessionService
	now      func() time.Time
	cost     int
}

func NewPasswordService(store Store, sessions *SessionService) *PasswordService {
	return &PasswordService{store: store, sessions: sessions, now: time.Now, cost: bcrypt.DefaultCost}
}

func (s *PasswordService) CreateOneTimeCredential(ctx context.Context, userID, login, password string) error {
	return s.put(ctx, userID, login, password, true)
}

func (s *PasswordService) CreateLocalAccount(ctx context.Context, username, displayName, password string) (User, error) {
	username = normalizeLogin(username)
	if len(username) < 3 || len(username) > 64 || len(displayName) < 1 || len(displayName) > 80 || len(password) < 12 || len(password) > 128 {
		return User{}, ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return User{}, err
	}
	id, err := randomID("usr")
	if err != nil {
		return User{}, err
	}
	now := s.now().UTC()
	user := User{ID: id, Username: username, DisplayName: displayName, CreatedAt: now}
	expiresAt := now.Add(24 * time.Hour)
	credential := Credential{UserID: id, Login: username, PasswordHash: string(hash), MustChange: true, ExpiresAt: &expiresAt, UpdatedAt: now}
	if err := s.store.CreateLocalAccount(ctx, user, credential); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *PasswordService) SetForOAuthUser(ctx context.Context, session Session, password string) error {
	if err := s.sessions.RequireRecentReauthentication(session); err != nil {
		return err
	}
	ok, err := s.store.HasIdentity(ctx, session.UserID, "github")
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCredentials
	}
	user, err := s.store.UserByID(ctx, session.UserID)
	if err != nil || user.Username == "" {
		return ErrInvalidCredentials
	}
	return s.put(ctx, session.UserID, user.Username, password, false)
}

func (s *PasswordService) SetForAuthenticatedUser(ctx context.Context, session Session, password string) error {
	if err := s.sessions.RequireRecentReauthentication(session); err != nil {
		return err
	}
	credential, credentialErr := s.store.CredentialByUser(ctx, session.UserID)
	if credentialErr == nil {
		return s.put(ctx, session.UserID, credential.Login, password, false)
	}
	return s.SetForOAuthUser(ctx, session, password)
}

func (s *PasswordService) ChangeTemporary(ctx context.Context, userID, currentPassword, newPassword string) error {
	credential, err := s.store.CredentialByUser(ctx, userID)
	if err != nil || !credential.MustChange || credential.ExpiresAt == nil || !s.now().UTC().Before(*credential.ExpiresAt) || bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(currentPassword)) != nil {
		return ErrInvalidCredentials
	}
	return s.put(ctx, userID, credential.Login, newPassword, false)
}

func (s *PasswordService) Authenticate(ctx context.Context, login, password string) (string, Session, bool, error) {
	credential, err := s.store.CredentialByLogin(ctx, normalizeLogin(login))
	if err != nil || (credential.MustChange && (credential.ExpiresAt == nil || !s.now().UTC().Before(*credential.ExpiresAt))) || bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(password)) != nil {
		return "", Session{}, false, ErrInvalidCredentials
	}
	token, session, err := s.sessions.Issue(ctx, credential.UserID, true)
	return token, session, credential.MustChange, err
}

func (s *PasswordService) put(ctx context.Context, userID, login, password string, mustChange bool) error {
	login = normalizeLogin(login)
	if len(login) < 3 || len(login) > 64 || len(password) < 12 || len(password) > 128 {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return err
	}
	credential := Credential{UserID: userID, Login: login, PasswordHash: string(hash), MustChange: mustChange, UpdatedAt: s.now().UTC()}
	if mustChange {
		expiresAt := credential.UpdatedAt.Add(24 * time.Hour)
		credential.ExpiresAt = &expiresAt
	}
	return s.store.PutCredential(ctx, credential)
}

func normalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}
