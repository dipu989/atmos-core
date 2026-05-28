-- 013_create_email_ingestion_logs.sql
-- Tracks every Gmail message we have processed (or attempted).
-- Runs after 012 so sender_code can FK to provider_email_types.
--
-- Deduplication guarantee: UNIQUE (user_id, message_id) ensures we never
-- process the same Gmail message twice, even if sync is triggered multiple times.

CREATE TABLE IF NOT EXISTS email_ingestion_logs (
    id           UUID         PRIMARY KEY,
    user_id      UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id   TEXT         NOT NULL,       -- Gmail message ID
    sender_code  TEXT         REFERENCES provider_email_types(code),
    -- NULL when message was skipped before a parser could be determined
    -- (e.g. cancellation email detected from snippet)
    subject      TEXT         NOT NULL DEFAULT '',
    snippet      TEXT         NOT NULL DEFAULT '',  -- first ~100 chars, aids debugging
    status       TEXT         NOT NULL DEFAULT 'pending',
    -- "parsed"  → activity created successfully
    -- "skipped" → recognised but intentionally ignored (cancellation, duplicate)
    -- "failed"  → parsing attempted but failed
    activity_id  UUID         REFERENCES activities(id) ON DELETE SET NULL,
    error_reason TEXT,
    parsed_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE (user_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_email_logs_user   ON email_ingestion_logs(user_id, parsed_at DESC);
CREATE INDEX IF NOT EXISTS idx_email_logs_failed ON email_ingestion_logs(status) WHERE status = 'failed';
