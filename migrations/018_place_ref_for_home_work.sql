-- Add lat/lng coordinates to home and work commute locations in user_preferences.
-- home_address and work_address columns remain as the display name / formatted address.

ALTER TABLE user_preferences
    ADD COLUMN IF NOT EXISTS home_lat DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS home_lng DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS work_lat DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS work_lng DOUBLE PRECISION;
