DROP TABLE IF EXISTS email_deliveries;

ALTER TABLE users
    DROP COLUMN IF EXISTS email_enabled;
