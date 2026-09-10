package authentication

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

var errNotFound = errors.New("not found")

type MemoryStore struct {
	mu          sync.Mutex
	states      map[string]OAuthState
	users       map[string]User
	identities  map[string]OAuthIdentity
	credentials map[string]Credential
	sessions    map[string]Session
	totp        map[string]TOTPRecord
	invitations map[string]Invitation
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		states:      map[string]OAuthState{},
		users:       map[string]User{},
		identities:  map[string]OAuthIdentity{},
		credentials: map[string]Credential{},
		sessions:    map[string]Session{},
		totp:        map[string]TOTPRecord{},
		invitations: map[string]Invitation{},
	}
}

func (s *MemoryStore) AddUser(user User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[user.ID] = user
}

func (s *MemoryStore) SaveOAuthState(_ context.Context, state OAuthState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state.Hash] = state
	return nil
}

func (s *MemoryStore) ConsumeOAuthState(_ context.Context, hash string, now time.Time) (OAuthState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[hash]
	if !ok || state.ConsumedAt != nil || !now.Before(state.ExpiresAt) {
		return OAuthState{}, ErrInvalidOAuthState
	}
	state.ConsumedAt = &now
	s.states[hash] = state
	return state, nil
}

func (s *MemoryStore) UpsertOAuthUser(_ context.Context, profile OAuthProfile, now time.Time) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := profile.Provider + ":" + profile.Subject
	if identity, ok := s.identities[key]; ok {
		return s.users[identity.UserID], nil
	}
	id, err := randomID("usr")
	if err != nil {
		return User{}, err
	}
	user := User{ID: id, Username: normalizeLogin(profile.Username), DisplayName: profile.DisplayName, Email: strings.ToLower(profile.Email), CreatedAt: now}
	s.users[id] = user
	s.identities[key] = OAuthIdentity{Provider: profile.Provider, Subject: profile.Subject, UserID: id}
	return user, nil
}

func (s *MemoryStore) CreateLocalAccount(_ context.Context, user User, credential Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, current := range s.users {
		if current.Username == user.Username {
			return ErrInvalidCredentials
		}
	}
	s.users[user.ID] = user
	s.credentials[user.ID] = credential
	return nil
}

func (s *MemoryStore) UserByID(_ context.Context, userID string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return User{}, errNotFound
	}
	return user, nil
}

func (s *MemoryStore) HasIdentity(_ context.Context, userID, provider string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, identity := range s.identities {
		if identity.UserID == userID && identity.Provider == provider {
			return true, nil
		}
	}
	return false, nil
}

func (s *MemoryStore) PutCredential(_ context.Context, credential Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for userID, current := range s.credentials {
		if userID != credential.UserID && current.Login == credential.Login {
			return ErrInvalidCredentials
		}
	}
	s.credentials[credential.UserID] = credential
	return nil
}

func (s *MemoryStore) CredentialByUser(_ context.Context, userID string) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	credential, ok := s.credentials[userID]
	if !ok {
		return Credential{}, errNotFound
	}
	return credential, nil
}

func (s *MemoryStore) CredentialByLogin(_ context.Context, login string) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	login = strings.ToLower(strings.TrimSpace(login))
	for _, credential := range s.credentials {
		if credential.Login == login {
			return credential, nil
		}
	}
	return Credential{}, errNotFound
}

func (s *MemoryStore) PutSession(_ context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.TokenHash] = session
	return nil
}

func (s *MemoryStore) SessionByTokenHash(_ context.Context, hash string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[hash]
	if !ok {
		return Session{}, errNotFound
	}
	return session, nil
}

func (s *MemoryStore) TouchSession(_ context.Context, hash string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[hash]
	if !ok {
		return errNotFound
	}
	session.LastSeenAt = now
	s.sessions[hash] = session
	return nil
}

func (s *MemoryStore) RevokeSession(_ context.Context, hash string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[hash]
	if !ok {
		return errNotFound
	}
	session.RevokedAt = &now
	s.sessions[hash] = session
	return nil
}

func (s *MemoryStore) PutTOTP(_ context.Context, record TOTPRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totp[record.UserID] = record
	return nil
}

func (s *MemoryStore) TOTPByUser(_ context.Context, userID string) (TOTPRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.totp[userID]
	if !ok {
		return TOTPRecord{}, errNotFound
	}
	return record, nil
}

func (s *MemoryStore) AcceptTOTPCounter(_ context.Context, userID string, counter int64, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.totp[userID]
	if !ok {
		return false, errNotFound
	}
	if counter <= record.LastAcceptedCounter {
		return false, nil
	}
	record.LastAcceptedCounter = counter
	record.VerifiedAt = &now
	s.totp[userID] = record
	return true, nil
}

func (s *MemoryStore) PutInvitation(_ context.Context, invitation Invitation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invitations[invitation.TokenHash] = invitation
	return nil
}

func (s *MemoryStore) ConsumeInvitation(_ context.Context, hash, userID string, now time.Time) (Invitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invitation, ok := s.invitations[hash]
	if !ok {
		return Invitation{}, ErrInvitationInvalid
	}
	if invitation.AcceptedAt != nil {
		if invitation.AcceptedBy == userID {
			return invitation, nil
		}
		return Invitation{}, ErrInvitationInvalid
	}
	if !now.Before(invitation.ExpiresAt) {
		return Invitation{}, ErrInvitationInvalid
	}
	invitation.AcceptedBy = userID
	invitation.AcceptedAt = &now
	s.invitations[hash] = invitation
	return invitation, nil
}
