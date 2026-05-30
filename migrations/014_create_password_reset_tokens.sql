-- 014_create_password_reset_tokens.sql
-- Stores hashed password reset tokens.
-- The raw token is emailed to the user; only the SHA-256 hash is stored here.
-- One active token per user at a time — previous tokens are invalidated on new request.

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id          UUID        PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL UNIQUE,   -- SHA-256(raw_token) hex-encoded
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ NULL,              -- set when the token is consumed
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_prt_user ON password_reset_tokens(user_id);
