ALTER TABLE admin_accounts ADD COLUMN platform_role text;
UPDATE admin_accounts
SET platform_role = CASE WHEN role = 'admin' THEN 'platform_admin' ELSE 'user' END;
ALTER TABLE admin_accounts ALTER COLUMN platform_role SET DEFAULT 'user';
ALTER TABLE admin_accounts ALTER COLUMN platform_role SET NOT NULL;
ALTER TABLE admin_accounts ADD CONSTRAINT admin_accounts_platform_role_check
    CHECK (platform_role IN ('platform_admin', 'user'));
