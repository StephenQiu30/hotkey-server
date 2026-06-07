-- Add report_type column to daily_reports for daily/weekly distinction
ALTER TABLE daily_reports
    ADD COLUMN IF NOT EXISTS report_type text NOT NULL DEFAULT 'daily';

-- Add daily_report_ids_json to track which daily reports were aggregated into a weekly report
ALTER TABLE daily_reports
    ADD COLUMN IF NOT EXISTS daily_report_ids_json jsonb NOT NULL DEFAULT '[]'::jsonb;

-- Add 'skipped' status to email_deliveries CHECK constraint
-- PostgreSQL does not support ALTER CHECK, so we drop and recreate
ALTER TABLE email_deliveries
    DROP CONSTRAINT IF EXISTS email_deliveries_status_check;

ALTER TABLE email_deliveries
    ADD CONSTRAINT email_deliveries_status_check
    CHECK (status IN ('pending', 'sent', 'failed', 'failed_config', 'skipped'));
