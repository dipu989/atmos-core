-- 006_create_emission_factors.sql
CREATE TABLE IF NOT EXISTS emission_factors (
    id                UUID           PRIMARY KEY,
    activity_type     TEXT           NOT NULL,
    transport_mode    TEXT           NULL,
    vehicle_type      TEXT           NULL,
    fuel_type         TEXT           NULL,
    region            TEXT           NOT NULL DEFAULT 'global',
    kg_co2e_per_km    NUMERIC(12,6)  NULL,
    kg_co2e_per_kwh   NUMERIC(12,6)  NULL,
    kg_co2e_flat      NUMERIC(12,6)  NULL,
    unit_label        TEXT           NULL,
    source_name       TEXT           NOT NULL,
    source_url        TEXT           NULL,
    effective_from    DATE           NOT NULL,
    effective_until   DATE           NULL,
    created_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

-- Most-specific-wins lookup: ordered by specificity columns
-- No partial predicate: CURRENT_DATE is not immutable and cannot be used in index predicates.
-- The application WHERE clause (effective_until IS NULL OR effective_until >= ?) handles filtering.
CREATE INDEX IF NOT EXISTS idx_emission_factors_lookup
    ON emission_factors(activity_type, transport_mode, region, effective_from);

-- Seed: common transport factors (India defaults)
INSERT INTO emission_factors
    (id, activity_type, transport_mode, vehicle_type, fuel_type, region, kg_co2e_per_km, source_name, effective_from)
VALUES
    (gen_random_uuid(), 'transport', 'cab',          'sedan',   'petrol', 'IN', 0.17100, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'transport', 'cab',          'sedan',   'cng',    'IN', 0.10200, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'transport', 'cab',          'sedan',   'electric','IN',0.05500, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'transport', 'auto_rickshaw', NULL,     'cng',    'IN', 0.07200, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'transport', 'bus',           NULL,     'diesel', 'IN', 0.08900, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'transport', 'metro',         NULL,     'electric','IN',0.03100, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'transport', 'train',         NULL,     'electric','IN',0.04100, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'transport', 'two_wheeler',   NULL,     'petrol', 'IN', 0.11300, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'transport', 'two_wheeler',   NULL,     'electric','IN',0.04200, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'transport', 'walk',          NULL,      NULL,    'global', 0.00000, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'transport', 'bicycle',       NULL,      NULL,    'global', 0.00000, 'DEFRA_2023', '2023-01-01'),
    (gen_random_uuid(), 'flight',    'flight',        NULL,      NULL,    'global', 0.25500, 'IPCC_2023', '2023-01-01')
ON CONFLICT DO NOTHING;
