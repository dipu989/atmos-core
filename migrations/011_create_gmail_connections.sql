-- 011_create_gmail_connections.sql
-- Stores per-user Gmail OAuth tokens so we can fetch ride receipts.
CREATE TABLE IF NOT EXISTS gmail_connections (
    id                UUID         PRIMARY KEY,
    user_id           UUID         NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    email             TEXT         NOT NULL,                      -- Gmail address the user connected
    access_token      TEXT         NOT NULL,                      -- short-lived, refreshed automatically
    refresh_token     TEXT         NOT NULL,                      -- long-lived, used to refresh
    token_expiry      TIMESTAMPTZ  NOT NULL,
    history_id        TEXT         NULL,                          -- Gmail historyId for incremental sync
    last_sync_at      TIMESTAMPTZ  NULL,
    last_sync_summary JSONB        NULL,                          -- {messages_checked, parsed, skipped, failed}
    connected_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gmail_connections_user ON gmail_connections(user_id);
