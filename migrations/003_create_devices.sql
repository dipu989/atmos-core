-- 003_create_devices.sql
CREATE TABLE IF NOT EXISTS devices (
    id               UUID        PRIMARY KEY,
    user_id          UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_token     TEXT        NOT NULL UNIQUE,
    platform         TEXT        NOT NULL,
    push_provider    TEXT        NOT NULL DEFAULT 'none',
    apns_environment TEXT        NULL,
    device_name      TEXT        NULL,
    os_version       TEXT        NULL,
    app_version      TEXT        NULL,
    push_token       TEXT        NULL,
    last_seen_at     TIMESTAMPTZ NULL,
    is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_devices_user_id_active ON devices(user_id) WHERE is_active = TRUE;
CREATE INDEX IF NOT EXISTS idx_devices_device_token ON devices(device_token);
