-- 012_create_email_providers.sql
-- Reference / config tables for the email ingestion pipeline.
-- Must run BEFORE 013 (email_ingestion_logs) because that table FKs here.
--
-- Three tables, each at a distinct level of abstraction:
--   emission_categories  → taxonomy of what we track (transport_cab, food_delivery, …)
--   providers            → the companies whose emails we parse (Uber, Rapido, …)
--   provider_email_types → one row per distinct parseable email format
--
-- Adding a new platform = INSERT rows here + add a parser Go file.
-- No schema changes ever needed.

-- ── 1. Emission category hierarchy ──────────────────────────────────────────
-- Self-referencing so the tree can grow arbitrarily deep.
-- activity_type maps directly to activities.activity_type enum.
CREATE TABLE IF NOT EXISTS emission_categories (
    code          TEXT        PRIMARY KEY,
    parent_code   TEXT        REFERENCES emission_categories(code),
    display_name  TEXT        NOT NULL,
    activity_type TEXT        NOT NULL, -- "transport" | "flight" | "food" | "energy"
    icon          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── 2. Provider companies ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS providers (
    code         TEXT        PRIMARY KEY,
    display_name TEXT        NOT NULL,
    logo_url     TEXT,
    country      TEXT        NOT NULL DEFAULT 'IN',
    is_active    BOOLEAN     NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── 3. Parseable email types ─────────────────────────────────────────────────
-- One row per distinct email format from a provider.
-- sender_email is queried at sync-time to build the Gmail search filter.
-- subject_pattern (optional regex) disambiguates when one sender sends
-- multiple email types (e.g. Rapido sends both bike and auto invoices).
CREATE TABLE IF NOT EXISTS provider_email_types (
    code             TEXT        PRIMARY KEY,
    provider_code    TEXT        NOT NULL REFERENCES providers(code),
    display_name     TEXT        NOT NULL,
    category_code    TEXT        NOT NULL REFERENCES emission_categories(code),
    sender_email     TEXT        NOT NULL,
    subject_pattern  TEXT,
    is_active        BOOLEAN     NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Routing lookup: sender_email → active parsers
CREATE INDEX IF NOT EXISTS idx_pet_sender_active
    ON provider_email_types(sender_email)
    WHERE is_active = true;

-- ── Seed: emission_categories ────────────────────────────────────────────────
INSERT INTO emission_categories (code, parent_code, display_name, activity_type, icon) VALUES
    ('transport',            NULL,         'Transport',             'transport', '🚗'),
    ('transport_cab',        'transport',  'Cab / Taxi',            'transport', '🚕'),
    ('transport_bike',       'transport',  'Bike Taxi',             'transport', '🏍️'),
    ('transport_auto',       'transport',  'Auto Rickshaw',         'transport', '🛺'),
    ('transport_train',      'transport',  'Train',                 'transport', '🚆'),
    ('transport_bus',        'transport',  'Bus',                   'transport', '🚌'),
    ('flight',               NULL,         'Flight',                'flight',    '✈️'),
    ('flight_domestic',      'flight',     'Domestic Flight',       'flight',    '✈️'),
    ('flight_international', 'flight',     'International Flight',  'flight',    '🌍'),
    ('food_delivery',        NULL,         'Food Delivery',         'food',      '🍔'),
    ('grocery_delivery',     NULL,         'Grocery Delivery',      'food',      '🛒')
ON CONFLICT (code) DO NOTHING;

-- ── Seed: providers ──────────────────────────────────────────────────────────
INSERT INTO providers (code, display_name, country, is_active) VALUES
    ('uber',   'Uber',   'IN', true),
    ('rapido', 'Rapido', 'IN', true),
    ('ola',    'Ola',    'IN', false),
    ('indigo', 'IndiGo', 'IN', false),
    ('irctc',  'IRCTC',  'IN', false),
    ('swiggy', 'Swiggy', 'IN', false)
ON CONFLICT (code) DO NOTHING;

-- ── Seed: provider_email_types ───────────────────────────────────────────────
INSERT INTO provider_email_types
    (code, provider_code, display_name, category_code, sender_email, subject_pattern, is_active)
VALUES
    -- Uber: single sender, subject varies ("Your Monday morning trip with Uber")
    ('uber_ride',     'uber',   'Uber Ride',    'transport_cab',  'noreply@uber.com',      NULL,             true),

    -- Rapido: same sender + subject for all vehicle types; parser resolves via body
    ('rapido_bike',   'rapido', 'Rapido Bike',  'transport_bike', 'shoutout@rapido.bike',  'Rapido Invoice', true),
    ('rapido_auto',   'rapido', 'Rapido Auto',  'transport_auto', 'shoutout@rapido.bike',  'Rapido Invoice', true),
    ('rapido_cab',    'rapido', 'Rapido Cab',   'transport_cab',  'shoutout@rapido.bike',  'Rapido Invoice', false),

    -- Future (parsers not yet written — is_active = false)
    ('ola_ride',      'ola',    'Ola Ride',     'transport_cab',  'noreply@olacabs.com',   NULL,             false),
    ('indigo_flight', 'indigo', 'IndiGo Flight','flight_domestic','noreply@goindigo.in',   NULL,             false)
ON CONFLICT (code) DO NOTHING;
