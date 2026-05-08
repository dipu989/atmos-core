-- 002_create_oauth_providers.sql
CREATE TABLE IF NOT EXISTS oauth_providers (
    id               UUID        PRIMARY KEY,
    user_id          UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT        NOT NULL,
    provider_user_id TEXT        NOT NULL,
    access_token     TEXT        NOT NULL DEFAULT '',
    refresh_token    TEXT        NULL,
    token_expiry     TIMESTAMPTZ NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_user_id)
);

CREATE INDEX IF NOT EXISTS idx_oauth_providers_user_id ON oauth_providers(user_id);
