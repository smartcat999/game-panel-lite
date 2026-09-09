CREATE TABLE users (
    id text PRIMARY KEY,
    display_name text NOT NULL,
    email text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL
);

CREATE TABLE identities (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    provider text NOT NULL,
    subject text NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (provider, subject)
);
CREATE INDEX identities_user_id_idx ON identities (user_id);

CREATE TABLE user_preferences (
    user_id text PRIMARY KEY,
    locale text NOT NULL,
    theme text NOT NULL,
    time_zone text NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE platform_operators (
    user_id text PRIMARY KEY,
    created_at timestamptz NOT NULL
);

CREATE TABLE workspaces (
    id text PRIMARY KEY,
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE TABLE memberships (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    user_id text NOT NULL,
    role text NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (workspace_id, user_id)
);
CREATE INDEX memberships_user_id_idx ON memberships (user_id);
CREATE INDEX memberships_workspace_id_idx ON memberships (workspace_id);

CREATE TABLE workspace_selections (
    user_id text PRIMARY KEY,
    workspace_id text NOT NULL,
    updated_at timestamptz NOT NULL
);
