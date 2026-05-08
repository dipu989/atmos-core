-- 007_create_emissions.sql
CREATE TABLE IF NOT EXISTS emissions (
    id                   UUID          PRIMARY KEY,
    activity_id          UUID          NOT NULL UNIQUE REFERENCES activities(id) ON DELETE CASCADE,
    user_id              UUID          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emission_factor_id   UUID          NOT NULL REFERENCES emission_factors(id),
    kg_co2e              NUMERIC(12,6) NOT NULL,
    calculation_version  INTEGER       NOT NULL DEFAULT 1,
    calculated_at        TIMESTAMPTZ   NOT NULL,
    created_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_emissions_user_id     ON emissions(user_id);
CREATE INDEX IF NOT EXISTS idx_emissions_activity_id ON emissions(activity_id);
