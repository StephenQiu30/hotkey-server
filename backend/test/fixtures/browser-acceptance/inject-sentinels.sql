\set ON_ERROR_STOP on

UPDATE reports
SET body = :'fixture_body', version = version + 1, updated_at = now()
WHERE id = :report_id
  AND monitor_id = 910001
  AND status = 'draft';

-- Simulate a legacy/imported row that bypassed today's report write boundary.
-- The real browser must render every marker as inert text even when storage is
-- already compromised. This row is never submitted or approved.
INSERT INTO reports (
    id,version,report_type,monitor_id,period_start,period_end,timezone,title,summary,body,
    input_snapshot_hash,status,version_no,created_by,updated_by
) VALUES (
    919007,1,'daily',910001,
    '2026-08-27T16:00:00Z','2026-08-28T16:00:00Z','Asia/Shanghai',
    'BrowserXSSFixture','必须作为纯文本展示',
    E'<script>globalThis.__HOTKEY_XSS__=true</script>\n<svg onload="globalThis.__HOTKEY_XSS__=true">x</svg>\n[open](javascript:globalThis.__HOTKEY_XSS__=true)',
    repeat('9',64),'draft',99,910001,910001
);

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

    IF (
        SELECT count(*)
        FROM reports
        WHERE id = 919007
          AND status = 'draft'
          AND body LIKE '%globalThis.__HOTKEY_XSS__=true%'
    ) <> 1 THEN
        RAISE EXCEPTION 'browser acceptance malicious content fixture mismatch';
    END IF;
END;
$$;
