package authentication

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidOAuthState  = errors.New("invalid or expired oauth state")
	ErrInvalidSession     = errors.New("invalid session")
	ErrReauthentication   = errors.New("recent reauthentication required")
	ErrInvitationInvalid  = errors.New("invalid or expired invitation")
	ErrTOTPInvalid        = errors.New("invalid or replayed totp code")
)

type User struct {
	ID          string
	Username    string
	DisplayName string
	Email       string
	CreatedAt   time.Time
}

type OAuthIdentity struct {
	Provider string
	Subject  string
	UserID   string
}

type OAuthProfile struct {
	Provider    string
	Subject     string
	Username    string
	Email       string
	DisplayName string
}

type OAuthState struct {
	Hash       string
	ReturnPath string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt *time.Time
}

type Credential struct {
	UserID       string
	Login        string
	PasswordHash string
	MustChange   bool
	UpdatedAt    time.Time
}

type Session struct {
	ID                 string
	UserID             string
	TokenHash          string
	CreatedAt          time.Time
	LastSeenAt         time.Time
	ExpiresAt          time.Time
	ReauthenticatedAt  *time.Time
	OperatorVerifiedAt *time.Time
	RevokedAt          *time.Time
}

type TOTPRecord struct {
	UserID              string
	EncryptedSecret     []byte
	LastAcceptedCounter int64
	CreatedAt           time.Time
	VerifiedAt          *time.Time
}

type Invitation struct {
	ID          string
	WorkspaceID string
	Role        string
	TokenHash   string
	InvitedBy   string
	ExpiresAt   time.Time
	AcceptedBy  string
	AcceptedAt  *time.Time
	CreatedAt   time.Time
}

type OAuthClient interface {
	AuthorizationURL(state string) string
	Exchange(context.Context, string) (OAuthProfile, error)
}

type Store interface {
	SaveOAuthState(context.Context, OAuthState) error
	ConsumeOAuthState(context.Context, string, time.Time) (OAuthState, error)
	UpsertOAuthUser(context.Context, OAuthProfile, time.Time) (User, error)
	CreateLocalAccount(context.Context, User, Credential) error
	HasIdentity(context.Context, string, string) (bool, error)
	UserByID(context.Context, string) (User, error)
	PutCredential(context.Context, Credential) error
	CredentialByUser(context.Context, string) (Credential, error)
	CredentialByLogin(context.Context, string) (Credential, error)
	PutSession(context.Context, Session) error
	SessionByTokenHash(context.Context, string) (Session, error)
	TouchSession(context.Context, string, time.Time) error
	RevokeSession(context.Context, string, time.Time) error
	PutTOTP(context.Context, TOTPRecord) error
	TOTPByUser(context.Context, string) (TOTPRecord, error)
	AcceptTOTPCounter(context.Context, string, int64, time.Time) (bool, error)
	PutInvitation(context.Context, Invitation) error
	ConsumeInvitation(context.Context, string, string, time.Time) (Invitation, error)
}
