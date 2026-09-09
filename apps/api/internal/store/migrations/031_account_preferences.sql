CREATE TABLE account_preferences (
    account_id text PRIMARY KEY REFERENCES admin_accounts(id) ON DELETE CASCADE,
    locale text NOT NULL DEFAULT 'zh' CHECK (locale IN ('zh', 'en')),
    theme text NOT NULL DEFAULT 'system' CHECK (theme IN ('light', 'dark', 'system')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
