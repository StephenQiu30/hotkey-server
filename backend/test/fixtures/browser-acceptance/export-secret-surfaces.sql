\set ON_ERROR_STOP on
\pset tuples_only on
\pset format unaligned

SELECT json_build_object('surface', surface, 'value', value)::text
FROM (
    SELECT 'audit_logs' AS surface,
           coalesce(before_data::text, '') || coalesce(after_data::text, '') ||
           coalesce(request_id, '') || coalesce(trace_id, '') AS value
    FROM audit_logs
    UNION ALL
    SELECT 'river_job', args::text || metadata::text || errors::text
    FROM river_job
    UNION ALL
    SELECT 'river_job_attempt', coalesce(error, '')
    FROM river_job_attempt
    UNION ALL
    SELECT 'vault_sync_runs', coalesce(error, '')
    FROM vault_sync_runs
    UNION ALL
    SELECT 'report_deliveries', coalesce(last_error, '')
    FROM report_deliveries
    UNION ALL
    SELECT 'delivery_attempts', coalesce(provider_message_id, '') || coalesce(error, '')
    FROM delivery_attempts
    UNION ALL
    SELECT 'notification_delivery_attempts', coalesce(provider_message_id, '') || coalesce(error_code, '')
    FROM notification_delivery_attempts
    UNION ALL
    SELECT 'source_credentials', encode(nonce, 'hex') || encode(ciphertext, 'hex')
    FROM source_credentials
) AS operational_surface
ORDER BY surface, value;
