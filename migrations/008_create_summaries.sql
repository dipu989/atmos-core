-- 008_create_summaries.sql

CREATE TABLE IF NOT EXISTS daily_summaries (
    id                UUID          PRIMARY KEY,
    user_id           UUID          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date_local        DATE          NOT NULL,
    total_kg_co2e     NUMERIC(12,4) NOT NULL DEFAULT 0,
    total_distance_km NUMERIC(10,3) NOT NULL DEFAULT 0,
    activity_count    INTEGER       NOT NULL DEFAULT 0,
    breakdown         JSONB         NOT NULL DEFAULT '{}',
    computed_at       TIMESTAMPTZ   NOT NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, date_local)
);

CREATE INDEX IF NOT EXISTS idx_daily_summaries_user_date
    ON daily_summaries(user_id, date_local DESC);

CREATE TABLE IF NOT EXISTS weekly_summaries (
    id                UUID          PRIMARY KEY,
    user_id           UUID          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    week_start        DATE          NOT NULL,
    week_end          DATE          NOT NULL,
    total_kg_co2e     NUMERIC(12,4) NOT NULL DEFAULT 0,
    total_distance_km NUMERIC(10,3) NOT NULL DEFAULT 0,
    activity_count    INTEGER       NOT NULL DEFAULT 0,
    breakdown         JSONB         NOT NULL DEFAULT '{}',
    computed_at       TIMESTAMPTZ   NOT NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, week_start)
);

CREATE INDEX IF NOT EXISTS idx_weekly_summaries_user_week
    ON weekly_summaries(user_id, week_start DESC);

CREATE TABLE IF NOT EXISTS monthly_summaries (
    id                UUID          PRIMARY KEY,
    user_id           UUID          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    year              INTEGER       NOT NULL,
    month             INTEGER       NOT NULL,
    total_kg_co2e     NUMERIC(12,4) NOT NULL DEFAULT 0,
    total_distance_km NUMERIC(10,3) NOT NULL DEFAULT 0,
    activity_count    INTEGER       NOT NULL DEFAULT 0,
    breakdown         JSONB         NOT NULL DEFAULT '{}',
    computed_at       TIMESTAMPTZ   NOT NULL,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, year, month)
);

CREATE INDEX IF NOT EXISTS idx_monthly_summaries_user_ym
    ON monthly_summaries(user_id, year DESC, month DESC);
