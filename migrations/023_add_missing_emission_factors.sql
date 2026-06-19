-- 023_add_missing_emission_factors.sql
-- Canonical (no fuel/vehicle type) factors that were absent from the original seed.
-- These serve as the fallback when fuel type is unknown.
-- auto_rickshaw and two_wheeler values are derived from the weighted average of
-- their fuel-specific 2023 rows (0.072 CNG for rickshaw; 0.113 petrol / 0.042 EV
-- for two-wheeler, weighted toward petrol given India fleet mix).
INSERT INTO emission_factors
    (id, activity_type, transport_mode, region, kg_co2e_per_km, source_name, effective_from)
VALUES
    (gen_random_uuid(), 'transport', 'car',           'IN',     0.19000, 'Atmos_2024', '2024-01-01'),
    (gen_random_uuid(), 'transport', 'walking',        'global', 0.00000, 'Atmos_2024', '2024-01-01'),
    (gen_random_uuid(), 'transport', 'cycling',        'global', 0.00000, 'Atmos_2024', '2024-01-01'),
    (gen_random_uuid(), 'transport', 'auto_rickshaw',  'IN',     0.07800, 'Atmos_2024', '2024-01-01'),
    (gen_random_uuid(), 'transport', 'two_wheeler',    'IN',     0.10000, 'Atmos_2024', '2024-01-01')
ON CONFLICT DO NOTHING;
