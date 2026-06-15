-- 017_trip_dedup_fields.sql
-- Adds coordinate, receipt, fare, and match-confidence columns to activities
-- to support GPS ↔ receipt trip deduplication.

ALTER TABLE activities
    ADD COLUMN IF NOT EXISTS origin_lat       DOUBLE PRECISION NULL,
    ADD COLUMN IF NOT EXISTS origin_lng       DOUBLE PRECISION NULL,
    ADD COLUMN IF NOT EXISTS dest_lat         DOUBLE PRECISION NULL,
    ADD COLUMN IF NOT EXISTS dest_lng         DOUBLE PRECISION NULL,
    ADD COLUMN IF NOT EXISTS receipt_id       TEXT             NULL,
    ADD COLUMN IF NOT EXISTS fare_amount      NUMERIC(10,2)    NULL,
    ADD COLUMN IF NOT EXISTS fare_currency    TEXT             NULL,
    ADD COLUMN IF NOT EXISTS match_confidence NUMERIC(4,3)     NULL
        CHECK (match_confidence IS NULL OR (match_confidence >= 0 AND match_confidence <= 1));

-- Fast lookup: "does a receipt activity already exist for this receipt?"
CREATE UNIQUE INDEX IF NOT EXISTS idx_activities_receipt_id
    ON activities(receipt_id)
    WHERE receipt_id IS NOT NULL;

-- Spatial proximity queries: find candidate activities near a given destination point.
-- Filtered to exclude nulls so the index stays small.
CREATE INDEX IF NOT EXISTS idx_activities_dest_coords
    ON activities(user_id, dest_lat, dest_lng)
    WHERE dest_lat IS NOT NULL AND dest_lng IS NOT NULL;

-- Time-window queries for the dedup matcher: "find activities that overlap this time range."
CREATE INDEX IF NOT EXISTS idx_activities_user_time
    ON activities(user_id, started_at, ended_at);
