ALTER TABLE users
    ADD COLUMN IF NOT EXISTS email_enabled boolean NOT NULL DEFAULT true;

CREATE TABLE IF NOT EXISTS email_deliveries (
    id text PRIMARY KEY,
    recipient_user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_email text NOT NULL,
    report_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'failed_config')),
    attempt integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    last_error text,
    sent_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_deliveries_recipient_user_id
    ON email_deliveries(recipient_user_id);

CREATE INDEX IF NOT EXISTS idx_email_deliveries_report_id
    ON email_deliveries(report_id);

CREATE INDEX IF NOT EXISTS idx_email_deliveries_status
    ON email_deliveries(status);
