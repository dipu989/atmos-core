ALTER TABLE user_preferences
    ADD COLUMN IF NOT EXISTS home_address      TEXT,
    ADD COLUMN IF NOT EXISTS work_address      TEXT,
    ADD COLUMN IF NOT EXISTS default_transport VARCHAR(50);
