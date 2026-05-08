-- 005_create_activities.sql
CREATE TABLE IF NOT EXISTS activities (
    id               UUID           PRIMARY KEY,
    user_id          UUID           NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id        UUID           NULL REFERENCES devices(id) ON DELETE SET NULL,
    activity_type    TEXT           NOT NULL,
    transport_mode   TEXT           NULL,
    distance_km      NUMERIC(10,3)  NULL,
    duration_minutes INTEGER        NULL,
    source           TEXT           NOT NULL,
    provider         TEXT           NULL,
    raw_metadata     JSONB          NOT NULL DEFAULT '{}',
    started_at       TIMESTAMPTZ    NOT NULL,
    ended_at         TIMESTAMPTZ    NULL,
    date_local       DATE           NOT NULL,
    idempotency_key  TEXT           NOT NULL UNIQUE,
    status           TEXT           NOT NULL DEFAULT 'pending',
    failure_reason   TEXT           NULL,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_activities_user_date    ON activities(user_id, date_local DESC);
CREATE INDEX IF NOT EXISTS idx_activities_user_source  ON activities(user_id, source);
CREATE INDEX IF NOT EXISTS idx_activities_status_pending ON activities(status) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_activities_idempotency  ON activities(idempotency_key);
