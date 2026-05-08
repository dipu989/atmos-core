-- 001_create_users.sql
CREATE TABLE IF NOT EXISTS users (
    id            UUID        PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NULL,
    display_name  TEXT        NOT NULL DEFAULT '',
    avatar_url    TEXT        NULL,
    timezone      TEXT        NOT NULL DEFAULT 'UTC',
    locale        TEXT        NOT NULL DEFAULT 'en',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at) WHERE deleted_at IS NOT NULL;
