-- Remove 'skipped' from email_deliveries CHECK constraint
ALTER TABLE email_deliveries
    DROP CONSTRAINT IF EXISTS email_deliveries_status_check;

ALTER TABLE email_deliveries
    ADD CONSTRAINT email_deliveries_status_check
    CHECK (status IN ('pending', 'sent', 'failed', 'failed_config'));

-- Remove report_type and daily_report_ids_json from daily_reports
ALTER TABLE daily_reports
    DROP COLUMN IF EXISTS daily_report_ids_json;

ALTER TABLE daily_reports
    DROP COLUMN IF EXISTS report_type;
