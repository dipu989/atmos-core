-- Add human-readable origin/destination strings to activities.
-- Populated from receipt email text (pickup/drop address lines).
-- Nullable — GPS-only activities have no text address.
ALTER TABLE activities
    ADD COLUMN IF NOT EXISTS origin      TEXT,
    ADD COLUMN IF NOT EXISTS destination TEXT;
