ALTER TABLE compute_nodes ADD COLUMN IF NOT EXISTS public_domain text NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS organization_invitations (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    inviter_user_id text NOT NULL,
    token text NOT NULL UNIQUE,
    role text NOT NULL,
    max_uses integer NOT NULL DEFAULT 0,
    used_count integer NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL,
    revoked boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_org_invitations ON organization_invitations(organization_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_org_invitation_token ON organization_invitations(token);
