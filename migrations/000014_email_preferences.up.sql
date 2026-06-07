ALTER TABLE users
    ADD COLUMN IF NOT EXISTS weekly_enabled boolean NOT NULL DEFAULT false;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS weekly_send_at text NOT NULL DEFAULT '09:00';
