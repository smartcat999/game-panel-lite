ALTER TABLE users ADD COLUMN username text;
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
CREATE UNIQUE INDEX users_username_idx ON users (username) WHERE username IS NOT NULL;

CREATE TABLE local_credentials (
    user_id text PRIMARY KEY,
    login text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    must_change boolean NOT NULL DEFAULT false,
    expires_at timestamptz,
    updated_at timestamptz NOT NULL
);

CREATE TABLE auth_sessions (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    reauthenticated_at timestamptz,
    operator_verified_at timestamptz,
    revoked_at timestamptz
);
CREATE INDEX auth_sessions_user_id_idx ON auth_sessions (user_id);
CREATE INDEX auth_sessions_expires_at_idx ON auth_sessions (expires_at);

CREATE TABLE oauth_states (
    state_hash text PRIMARY KEY,
    return_path text NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz
);
CREATE INDEX oauth_states_expires_at_idx ON oauth_states (expires_at);

CREATE TABLE operator_totp_credentials (
    user_id text PRIMARY KEY,
    encrypted_secret bytea NOT NULL,
    last_accepted_counter bigint NOT NULL DEFAULT -1,
    created_at timestamptz NOT NULL,
    verified_at timestamptz
);

CREATE TABLE workspace_invitations (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    role text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    invited_by text NOT NULL,
    expires_at timestamptz NOT NULL,
    accepted_by text,
    accepted_at timestamptz,
    created_at timestamptz NOT NULL
);
CREATE INDEX workspace_invitations_workspace_id_idx ON workspace_invitations (workspace_id);
CREATE INDEX workspace_invitations_expires_at_idx ON workspace_invitations (expires_at);

CREATE TABLE authorization_role_bindings (
    id text PRIMARY KEY,
    principal_id text NOT NULL,
    role text NOT NULL,
    scope_type text NOT NULL,
    scope_id text NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (principal_id, role, scope_type, scope_id)
);
CREATE INDEX authorization_role_bindings_principal_id_idx ON authorization_role_bindings (principal_id);
CREATE INDEX authorization_role_bindings_scope_idx ON authorization_role_bindings (scope_type, scope_id);
