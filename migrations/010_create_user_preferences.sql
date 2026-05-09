CREATE TABLE user_preferences (
    id                         UUID        NOT NULL PRIMARY KEY,
    user_id                    UUID        NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    distance_unit              VARCHAR(10) NOT NULL DEFAULT 'km',
    push_notifications_enabled BOOLEAN     NOT NULL DEFAULT TRUE,
    weekly_report_enabled      BOOLEAN     NOT NULL DEFAULT TRUE,
    daily_goal_kg_co2e         NUMERIC(8,3),
    data_sharing_enabled       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
