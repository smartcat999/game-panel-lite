package authentication

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type PostgresStore struct {
	database *sql.DB
}

func NewPostgresStore(database *sql.DB) *PostgresStore {
	return &PostgresStore{database: database}
}

func (s *PostgresStore) SaveOAuthState(ctx context.Context, state OAuthState) error {
	_, err := s.database.ExecContext(ctx, `INSERT INTO oauth_states (state_hash, return_path, created_at, expires_at) VALUES ($1, $2, $3, $4)`, state.Hash, state.ReturnPath, state.CreatedAt, state.ExpiresAt)
	return err
}

func (s *PostgresStore) ConsumeOAuthState(ctx context.Context, hash string, now time.Time) (OAuthState, error) {
	var state OAuthState
	err := s.database.QueryRowContext(ctx, `
		UPDATE oauth_states
		SET consumed_at = $2
		WHERE state_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING state_hash, return_path, created_at, expires_at, consumed_at`, hash, now).
		Scan(&state.Hash, &state.ReturnPath, &state.CreatedAt, &state.ExpiresAt, &state.ConsumedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthState{}, ErrInvalidOAuthState
	}
	return state, err
}

func (s *PostgresStore) UpsertOAuthUser(ctx context.Context, profile OAuthProfile, now time.Time) (User, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	var userID string
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM identities WHERE provider = $1 AND subject = $2`, profile.Provider, profile.Subject).Scan(&userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return User{}, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		email := strings.ToLower(strings.TrimSpace(profile.Email))
		err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			userID, err = randomID("usr")
			if err != nil {
				return User{}, err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO users (id, username, display_name, email, created_at) VALUES ($1, $2, $3, $4, $5)`, userID, normalizeLogin(profile.Username), profile.DisplayName, email, now)
		}
		if err != nil {
			return User{}, err
		}
		identityID, idErr := randomID("idn")
		if idErr != nil {
			return User{}, idErr
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO identities (id, user_id, provider, subject, created_at) VALUES ($1, $2, $3, $4, $5)`, identityID, userID, profile.Provider, profile.Subject, now); err != nil {
			return User{}, err
		}
	}

	var user User
	if err = tx.QueryRowContext(ctx, `SELECT id, COALESCE(username, ''), display_name, COALESCE(email, ''), created_at FROM users WHERE id = $1`, userID).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Email, &user.CreatedAt); err != nil {
		return User{}, err
	}
	if err = tx.Commit(); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *PostgresStore) CreateLocalAccount(ctx context.Context, user User, credential Credential) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO users (id, username, display_name, email, created_at) VALUES ($1, $2, $3, NULL, $4)`, user.ID, user.Username, user.DisplayName, user.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_credentials (user_id, login, password_hash, must_change, updated_at) VALUES ($1, $2, $3, $4, $5)`, credential.UserID, credential.Login, credential.PasswordHash, credential.MustChange, credential.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) UserByID(ctx context.Context, userID string) (User, error) {
	var user User
	err := s.database.QueryRowContext(ctx, `SELECT id, COALESCE(username, ''), display_name, COALESCE(email, ''), created_at FROM users WHERE id = $1`, userID).
		Scan(&user.ID, &user.Username, &user.DisplayName, &user.Email, &user.CreatedAt)
	return user, err
}

func (s *PostgresStore) HasIdentity(ctx context.Context, userID, provider string) (bool, error) {
	var exists bool
	err := s.database.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM identities WHERE user_id = $1 AND provider = $2)`, userID, provider).Scan(&exists)
	return exists, err
}

func (s *PostgresStore) PutCredential(ctx context.Context, credential Credential) error {
	_, err := s.database.ExecContext(ctx, `
		INSERT INTO local_credentials (user_id, login, password_hash, must_change, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET login = EXCLUDED.login, password_hash = EXCLUDED.password_hash, must_change = EXCLUDED.must_change, updated_at = EXCLUDED.updated_at`,
		credential.UserID, credential.Login, credential.PasswordHash, credential.MustChange, credential.UpdatedAt)
	return err
}

func (s *PostgresStore) CredentialByUser(ctx context.Context, userID string) (Credential, error) {
	return scanCredential(s.database.QueryRowContext(ctx, `SELECT user_id, login, password_hash, must_change, updated_at FROM local_credentials WHERE user_id = $1`, userID))
}

func (s *PostgresStore) CredentialByLogin(ctx context.Context, login string) (Credential, error) {
	return scanCredential(s.database.QueryRowContext(ctx, `SELECT user_id, login, password_hash, must_change, updated_at FROM local_credentials WHERE login = $1`, login))
}

type rowScanner interface {
	Scan(...any) error
}

func scanCredential(row rowScanner) (Credential, error) {
	var credential Credential
	err := row.Scan(&credential.UserID, &credential.Login, &credential.PasswordHash, &credential.MustChange, &credential.UpdatedAt)
	return credential, err
}

func (s *PostgresStore) PutSession(ctx context.Context, session Session) error {
	_, err := s.database.ExecContext(ctx, `
		INSERT INTO auth_sessions (id, user_id, token_hash, created_at, last_seen_at, expires_at, reauthenticated_at, operator_verified_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		session.ID, session.UserID, session.TokenHash, session.CreatedAt, session.LastSeenAt, session.ExpiresAt, session.ReauthenticatedAt, session.OperatorVerifiedAt, session.RevokedAt)
	return err
}

func (s *PostgresStore) SessionByTokenHash(ctx context.Context, hash string) (Session, error) {
	var session Session
	err := s.database.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, created_at, last_seen_at, expires_at, reauthenticated_at, operator_verified_at, revoked_at
		FROM auth_sessions WHERE token_hash = $1`, hash).
		Scan(&session.ID, &session.UserID, &session.TokenHash, &session.CreatedAt, &session.LastSeenAt, &session.ExpiresAt, &session.ReauthenticatedAt, &session.OperatorVerifiedAt, &session.RevokedAt)
	return session, err
}

func (s *PostgresStore) TouchSession(ctx context.Context, hash string, now time.Time) error {
	result, err := s.database.ExecContext(ctx, `UPDATE auth_sessions SET last_seen_at = $2 WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > $2`, hash, now)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return ErrInvalidSession
	}
	return nil
}

func (s *PostgresStore) RevokeSession(ctx context.Context, hash string, now time.Time) error {
	result, err := s.database.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at = $2 WHERE token_hash = $1 AND revoked_at IS NULL`, hash, now)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return ErrInvalidSession
	}
	return nil
}

func (s *PostgresStore) PutTOTP(ctx context.Context, record TOTPRecord) error {
	_, err := s.database.ExecContext(ctx, `
		INSERT INTO operator_totp_credentials (user_id, encrypted_secret, last_accepted_counter, created_at, verified_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET encrypted_secret = EXCLUDED.encrypted_secret, last_accepted_counter = -1, created_at = EXCLUDED.created_at, verified_at = NULL`,
		record.UserID, record.EncryptedSecret, record.LastAcceptedCounter, record.CreatedAt, record.VerifiedAt)
	return err
}

func (s *PostgresStore) TOTPByUser(ctx context.Context, userID string) (TOTPRecord, error) {
	var record TOTPRecord
	err := s.database.QueryRowContext(ctx, `SELECT user_id, encrypted_secret, last_accepted_counter, created_at, verified_at FROM operator_totp_credentials WHERE user_id = $1`, userID).
		Scan(&record.UserID, &record.EncryptedSecret, &record.LastAcceptedCounter, &record.CreatedAt, &record.VerifiedAt)
	return record, err
}

func (s *PostgresStore) AcceptTOTPCounter(ctx context.Context, userID string, counter int64, now time.Time) (bool, error) {
	result, err := s.database.ExecContext(ctx, `
		UPDATE operator_totp_credentials SET last_accepted_counter = $2, verified_at = $3
		WHERE user_id = $1 AND last_accepted_counter < $2`, userID, counter, now)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (s *PostgresStore) PutInvitation(ctx context.Context, invitation Invitation) error {
	_, err := s.database.ExecContext(ctx, `
		INSERT INTO workspace_invitations (id, workspace_id, role, token_hash, invited_by, expires_at, accepted_by, accepted_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		invitation.ID, invitation.WorkspaceID, invitation.Role, invitation.TokenHash, invitation.InvitedBy, invitation.ExpiresAt, invitation.AcceptedBy, invitation.AcceptedAt, invitation.CreatedAt)
	return err
}

func (s *PostgresStore) ConsumeInvitation(ctx context.Context, hash, userID string, now time.Time) (Invitation, error) {
	var invitation Invitation
	err := s.database.QueryRowContext(ctx, `
		UPDATE workspace_invitations SET accepted_by = $2, accepted_at = $3
		WHERE token_hash = $1 AND accepted_at IS NULL AND expires_at > $3
		RETURNING id, workspace_id, role, token_hash, invited_by, expires_at, accepted_by, accepted_at, created_at`, hash, userID, now).
		Scan(&invitation.ID, &invitation.WorkspaceID, &invitation.Role, &invitation.TokenHash, &invitation.InvitedBy, &invitation.ExpiresAt, &invitation.AcceptedBy, &invitation.AcceptedAt, &invitation.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.database.QueryRowContext(ctx, `
			SELECT id, workspace_id, role, token_hash, invited_by, expires_at, accepted_by, accepted_at, created_at
			FROM workspace_invitations
			WHERE token_hash = $1 AND accepted_by = $2`, hash, userID).
			Scan(&invitation.ID, &invitation.WorkspaceID, &invitation.Role, &invitation.TokenHash, &invitation.InvitedBy, &invitation.ExpiresAt, &invitation.AcceptedBy, &invitation.AcceptedAt, &invitation.CreatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return Invitation{}, ErrInvitationInvalid
		}
	}
	return invitation, err
}
