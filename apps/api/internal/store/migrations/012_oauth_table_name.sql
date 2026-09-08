-- Match GORM's default table for OAuthIdentity and SQLite's existing table name.
-- Rename rather than recreate so linked accounts and uniqueness survive upgrade.
ALTER TABLE oauth_identities RENAME TO o_auth_identities;
