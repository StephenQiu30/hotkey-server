\set ON_ERROR_STOP on

UPDATE reports
SET body = :'fixture_body', version = version + 1, updated_at = now()
WHERE id = :report_id
  AND monitor_id = 910001
  AND status = 'draft';

DO $$
BEGIN
    IF (
        SELECT count(*)
        FROM reports
        WHERE monitor_id = 910001
          AND status = 'draft'
          AND body LIKE '%HOTKEY_BODY_SENTINEL_7f4b9c2a%'
    ) <> 1 THEN
        RAISE EXCEPTION 'browser acceptance sentinel injection mismatch';
    END IF;
END;
$$;
