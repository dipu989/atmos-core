-- 024_add_canonical_bus_metro_factors.sql
-- Canonical (no fuel type) factors for bus and metro, mirroring 023's approach.
-- Both modes have exactly one fuel-specific row in India (diesel bus, electric
-- metro — there is no realistic alternate fuel variant in the existing seed),
-- so the canonical row simply equals that single value. Without this, any
-- lookup that omits fuel type (e.g. the trip-impact "greener alternative"
-- comparison, which never knows the alternative mode's fuel type) finds no
-- factor at all for bus or metro and silently omits the comparison.
INSERT INTO emission_factors
    (id, activity_type, transport_mode, region, kg_co2e_per_km, source_name, effective_from)
VALUES
    (gen_random_uuid(), 'transport', 'bus',   'IN', 0.08900, 'Atmos_2024', '2024-01-01'),
    (gen_random_uuid(), 'transport', 'metro', 'IN', 0.03100, 'Atmos_2024', '2024-01-01')
ON CONFLICT DO NOTHING;
