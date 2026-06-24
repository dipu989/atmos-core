-- Add a short, human-friendly display address for activity origin/destination,
-- backed by Google's Places API (New) shortFormattedAddress (e.g. "Kaggadasapura,
-- Bengaluru" instead of the full street+pincode+country text in origin/destination).
-- Nullable — populated lazily; falls back to origin/destination when absent.
ALTER TABLE activities
    ADD COLUMN IF NOT EXISTS display_origin      TEXT,
    ADD COLUMN IF NOT EXISTS display_destination TEXT;

-- Caches reverse-geocode lookups (lat/lng -> short address) so repeat coordinates
-- (e.g. home/work commute trips) don't re-pay for two Google API calls every time.
CREATE TABLE IF NOT EXISTS geocode_cache (
    lat_rounded     DOUBLE PRECISION NOT NULL,
    lng_rounded     DOUBLE PRECISION NOT NULL,
    display_address TEXT NOT NULL,
    place_id        TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (lat_rounded, lng_rounded)
);
