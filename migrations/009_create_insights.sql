-- 009_create_insights.sql
CREATE TABLE IF NOT EXISTS insights (
    id            UUID        PRIMARY KEY,
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    insight_type  TEXT        NOT NULL,
    period_type   TEXT        NOT NULL,
    period_start  DATE        NOT NULL,
    period_end    DATE        NOT NULL,
    title         TEXT        NOT NULL,
    body          TEXT        NOT NULL,
    cta_label     TEXT        NULL,
    cta_target    TEXT        NULL,
    metadata      JSONB       NOT NULL DEFAULT '{}',
    is_read       BOOLEAN     NOT NULL DEFAULT FALSE,
    expires_at    TIMESTAMPTZ NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_insights_user_unread
    ON insights(user_id, created_at DESC) WHERE is_read = FALSE;
CREATE INDEX IF NOT EXISTS idx_insights_user_period
    ON insights(user_id, period_start DESC);
