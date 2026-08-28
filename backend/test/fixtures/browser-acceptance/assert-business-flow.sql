\set ON_ERROR_STOP on

DO $$
DECLARE
    report_count bigint;
    projection_count bigint;
    notification_count bigint;
    search_count bigint;
    leak_count bigint;
BEGIN
    SELECT count(*) INTO report_count
    FROM reports
    WHERE monitor_id = 910001 AND status = 'published' AND published_at IS NOT NULL;
    IF report_count <> 1 THEN
        RAISE EXCEPTION 'browser acceptance report count mismatch';
    END IF;

    SELECT count(*) INTO projection_count
    FROM knowledge_documents AS document
    JOIN knowledge_change_proposals AS proposal ON proposal.document_id = document.id
    JOIN knowledge_revisions AS revision ON revision.document_id = document.id AND revision.proposal_id = proposal.id
    JOIN reports AS report ON report.id = document.report_id
    WHERE report.monitor_id = 910001
      AND document.status = 'active' AND document.revision_no = 1
      AND proposal.status = 'applied' AND revision.source = 'proposal';
    IF projection_count <> 1 THEN
        RAISE EXCEPTION 'browser acceptance knowledge projection mismatch';
    END IF;

    SELECT count(*) INTO notification_count
    FROM user_notifications
    WHERE user_id = 910001 AND monitor_id = 910001
      AND event_type IN ('report.approval_requested','report.published');
    IF notification_count <> 2 THEN
        RAISE EXCEPTION 'browser acceptance notification count mismatch';
    END IF;

    SELECT count(*) INTO search_count
    FROM knowledge_documents AS document
    JOIN knowledge_revisions AS revision
      ON revision.document_id = document.id AND revision.revision_no = document.revision_no
    JOIN knowledge_change_proposals AS proposal ON proposal.id = revision.proposal_id
    WHERE document.status = 'active'
      AND to_tsvector('simple', coalesce(proposal.proposed_frontmatter::text, '') || ' ' || proposal.proposed_body)
          @@ plainto_tsquery('simple', 'BrowserAcceptanceTopic2026');
    IF search_count <> 1 THEN
        RAISE EXCEPTION 'browser acceptance lexical projection mismatch';
    END IF;

    SELECT count(*) INTO leak_count
    FROM (
        SELECT coalesce(before_data::text, '') || coalesce(after_data::text, '') AS value FROM audit_logs
        UNION ALL SELECT coalesce(errors::text, '') FROM river_job
        UNION ALL SELECT coalesce(error, '') FROM river_job_attempt
        UNION ALL SELECT coalesce(error, '') FROM vault_sync_runs
        UNION ALL SELECT coalesce(last_error, '') FROM report_deliveries
        UNION ALL SELECT coalesce(error, '') FROM delivery_attempts
    ) AS operational_output
    WHERE operational_output.value LIKE '%HOTKEY_BODY_SENTINEL_7f4b9c2a%'
       OR operational_output.value LIKE '%mail-list-one@example.test%'
       OR operational_output.value LIKE '%mail-list-two@example.test%'
       OR operational_output.value LIKE '%/Users/hotkey-ci/private-vault/%'
       OR operational_output.value LIKE '%BrowserFixture-Only-2026!%';
    IF leak_count <> 0 THEN
        RAISE EXCEPTION 'browser acceptance sentinel leaked to operational output';
    END IF;
END;
$$;

SELECT json_build_object(
    'run_id', monitor.name,
    'report_id', report.id,
    'report_version', report.version,
    'document_id', document.id,
    'document_revision', document.revision_no,
    'proposal_id', proposal.id,
    'proposal_version', proposal.version,
    'notifications', (
        SELECT count(*) FROM user_notifications WHERE user_id = 910001 AND monitor_id = 910001
    )
)::text AS acceptance_summary
FROM monitors AS monitor
JOIN reports AS report ON report.monitor_id = monitor.id
JOIN knowledge_documents AS document ON document.report_id = report.id
JOIN knowledge_change_proposals AS proposal ON proposal.document_id = document.id
WHERE monitor.id = 910001;
