-- 015_email_verification.sql
-- Adds email_verified_at to users and creates the verification token table.
-- OAuth users are auto-verified at login (Google already confirmed the address).
-- Email+password users receive a link after registration.

-- ── 1. Mark existing users as verified ───────────────────────────────────────
-- Pre-existing accounts have no verification history; we trust them as-is.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ NULL;
UPDATE users SET email_verified_at = created_at WHERE email_verified_at IS NULL;

-- ── 2. Verification token table ───────────────────────────────────────────────
-- Same pattern as password_reset_tokens: raw token emailed, hash stored.
CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id          UUID        PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_evt_user ON email_verification_tokens(user_id);
