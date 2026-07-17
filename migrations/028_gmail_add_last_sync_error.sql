-- 028_gmail_add_last_sync_error.sql
-- Stores the last sync error message so the status endpoint can surface
-- a "reconnect" prompt when the OAuth token has been revoked.
ALTER TABLE gmail_connections
    ADD COLUMN IF NOT EXISTS last_sync_error TEXT NULL;
