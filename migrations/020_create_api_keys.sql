CREATE TABLE api_keys (
    id           UUID        NOT NULL PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    key_hash     TEXT        NOT NULL UNIQUE,
    prefix       TEXT        NOT NULL,
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at   TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
