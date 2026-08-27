package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type Repository struct{ runtime *database.Runtime }

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) FindByPeriod(ctx context.Context, reportType domain.ReportType, monitorID *int64, start, end time.Time) (domain.Report, error) {
	if repository == nil || repository.runtime == nil || start.IsZero() || end.IsZero() {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	queryer := reportQueryerFor(ctx, repository.runtime)
	report, err := scanReport(queryer.QueryRowContext(ctx, reportSelect+`
WHERE report_type = $1 AND monitor_id IS NOT DISTINCT FROM $2
  AND period_start = $3 AND period_end = $4 AND version_no = 1 AND deleted_at IS NULL`, reportType, monitorID, start.UTC(), end.UTC()))
	if err != nil {
		return domain.Report{}, err
	}
	report.Items, err = repository.items(ctx, queryer, report.ID)
	if err != nil {
		return domain.Report{}, err
	}
	return report, nil
}

// Create allocates the database identity for an automatic report and uses the
// period uniqueness key as the idempotency boundary for concurrent schedulers.
func (repository *Repository) Create(ctx context.Context, report domain.Report) (domain.Report, error) {
	if repository == nil || repository.runtime == nil {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	validation := report
	if validation.ID <= 0 {
		validation.ID = 1
	}
	if validation.Version <= 0 {
		validation.Version = 1
	}
	if validation.VersionNo <= 0 {
		validation.VersionNo = 1
	}
	if err := validation.Validate(); err != nil {
		return domain.Report{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	created := false
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		var reportID int64
		err := transaction.SQL.QueryRowContext(transactionCtx, `
INSERT INTO reports (version, report_type, monitor_id, period_start, period_end, timezone, title, summary, body, status, version_no, generated_at, created_by, updated_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now(),$12,$13)
ON CONFLICT (report_type, COALESCE(monitor_id, 0), period_start, period_end, version_no) DO NOTHING
RETURNING id`, report.Version, report.Type, report.MonitorID, report.Period.Start.UTC(), report.Period.End.UTC(), report.Period.Location.String(), report.Title, report.Summary, report.Body, report.Status, report.VersionNo, report.CreatedBy, report.UpdatedBy).Scan(&reportID)
		if err == nil {
			report.ID = reportID
			created = true
			return insertReportItems(transactionCtx, transaction.SQL, report)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return databaserepository.MapError(err)
		}
		return transaction.SQL.QueryRowContext(transactionCtx, `
SELECT id FROM reports
WHERE report_type = $1 AND monitor_id IS NOT DISTINCT FROM $2
  AND period_start = $3 AND period_end = $4 AND version_no = $5 AND deleted_at IS NULL`, report.Type, report.MonitorID, report.Period.Start.UTC(), report.Period.End.UTC(), report.VersionNo).Scan(&report.ID)
	})
	if err != nil {
		return domain.Report{}, err
	}
	if !created {
		return repository.Get(ctx, report.ID)
	}
	return repository.Get(ctx, report.ID)
}

func (repository *Repository) Save(ctx context.Context, report domain.Report) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := report.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	write := func(ctx context.Context, transaction database.Transaction) error {
		var existingStatus string
		err := transaction.SQL.QueryRowContext(ctx, `SELECT status FROM reports WHERE id = $1 FOR UPDATE`, report.ID).Scan(&existingStatus)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return databaserepository.MapError(err)
		}
		if err == nil && existingStatus == string(domain.ReportPublished) {
			return sharedrepository.ErrImmutable
		}
		if _, err := transaction.SQL.ExecContext(ctx, `INSERT INTO reports (id, version, report_type, monitor_id, period_start, period_end, timezone, title, summary, body, status, version_no, generated_at, published_at, created_by, updated_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now(),CASE WHEN $13::text = 'published' THEN now() ELSE NULL END,$14,$15) ON CONFLICT (id) DO UPDATE SET version = EXCLUDED.version, title = EXCLUDED.title, summary = EXCLUDED.summary, body = EXCLUDED.body, status = EXCLUDED.status, generated_at = EXCLUDED.generated_at, published_at = EXCLUDED.published_at, updated_by = EXCLUDED.updated_by, updated_at = now()`, report.ID, report.Version, report.Type, report.MonitorID, report.Period.Start.UTC(), report.Period.End.UTC(), report.Period.Location.String(), report.Title, report.Summary, report.Body, report.Status, report.VersionNo, report.Status, report.CreatedBy, report.UpdatedBy); err != nil {
			return databaserepository.MapError(err)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `DELETE FROM report_items WHERE report_id = $1`, report.ID); err != nil {
			return databaserepository.MapError(err)
		}
		return insertReportItems(ctx, transaction.SQL, report)
	}
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return write(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, write)
}

func insertReportItems(ctx context.Context, transaction *sql.Tx, report domain.Report) error {
	for _, item := range report.Items {
		var itemID int64
		if err := transaction.QueryRowContext(ctx, `INSERT INTO report_items
(report_id,event_id,event_update_id,micro_event_id,micro_event_version,micro_event_update_id,micro_event_summary_id,
 rank,section,inclusion_reason,title_snapshot,summary_snapshot,heat_score_snapshot,evidence_set_hash,reason_codes)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'events',$9,$10,$11,$12,$13,$14) RETURNING id`, report.ID,
			nullablePositiveInt64(item.EventID), nullablePositiveInt64(item.EventUpdateID), nullablePositiveInt64(item.MicroEventID),
			nullablePositiveInt64(item.MicroEventVersion), nullablePositiveInt64(item.MicroEventUpdateID), nullablePositiveInt64(item.MicroEventSummaryID),
			item.Rank, item.InclusionReason, item.Title, item.Summary, item.HeatScore, item.EvidenceSetHash, item.ReasonCodes).Scan(&itemID); err != nil {
			return databaserepository.MapError(err)
		}
		for _, sentence := range item.Sentences {
			var sentenceID int64
			if err := transaction.QueryRowContext(ctx, `INSERT INTO report_item_sentences
(report_item_id,source_summary_sentence_id,ordinal,sentence,editorial_note,decision_origin,model_run_id,actor_user_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, itemID, sentence.SourceSummarySentenceID, sentence.Ordinal,
				sentence.Text, sentence.EditorialNote, sentence.DecisionOrigin, sentence.ModelRunID, sentence.ActorUserID).Scan(&sentenceID); err != nil {
				return databaserepository.MapError(err)
			}
			for ordinal, evidenceID := range sentence.ClaimEvidenceVersionIDs {
				if _, err := transaction.ExecContext(ctx, `INSERT INTO report_item_sentence_evidences
(report_item_sentence_id,claim_evidence_version_id,ordinal) VALUES ($1,$2,$3)`, sentenceID, evidenceID, ordinal); err != nil {
					return databaserepository.MapError(err)
				}
			}
		}
	}
	return nil
}

func nullablePositiveInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

// ValidatePublication fails closed against the persisted draft. Event-level
// Evidence State only freezes the update's candidate set and hash; it never
// substitutes for per-sentence citations. Every factual sentence must retain
// the exact source-summary whitelist and every ClaimEvidence relation must be
// current, in-window, hash-consistent, unsuperseded and still quotable.
func (repository *Repository) ValidatePublication(ctx context.Context, report domain.Report) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := report.ValidatePublicationShape(); err != nil {
		return domain.ErrEvidenceInvalid
	}
	var invalid bool
	err := reportQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
SELECT NOT EXISTS (
    SELECT 1 FROM reports
    WHERE id=$1 AND version=$2 AND status='draft' AND deleted_at IS NULL
) OR EXISTS (
    SELECT 1
    FROM report_items AS item
    JOIN reports AS report ON report.id=item.report_id
    WHERE item.report_id=$1 AND (
        item.event_id IS NOT NULL OR item.event_update_id IS NOT NULL
        OR item.micro_event_id IS NULL OR item.micro_event_version IS NULL
        OR item.micro_event_update_id IS NULL OR item.micro_event_summary_id IS NULL
        OR NOT EXISTS (
            SELECT 1
            FROM micro_events AS event
            JOIN micro_event_updates AS event_update
              ON event_update.id=item.micro_event_update_id
             AND event_update.micro_event_id=event.id
             AND event_update.micro_event_version=item.micro_event_version
            JOIN evidence_state_snapshots AS evidence_snapshot
              ON evidence_snapshot.id=event_update.evidence_state_snapshot_id
             AND evidence_snapshot.micro_event_id=event.id
             AND evidence_snapshot.micro_event_version=item.micro_event_version
            JOIN micro_event_summaries AS event_summary
              ON event_summary.id=item.micro_event_summary_id
             AND event_summary.micro_event_id=event.id
             AND event_summary.micro_event_version=item.micro_event_version
            WHERE event.id=item.micro_event_id AND event.version=item.micro_event_version
              AND event.status IN ('active','review_pending')
              AND event_update.window_ended_at>=report.period_start
              AND event_update.window_ended_at<report.period_end
              AND evidence_snapshot.evidence_set_hash=item.evidence_set_hash
              AND event_update.heat_score=item.heat_score_snapshot
        )
        OR NOT EXISTS (
            SELECT 1 FROM report_item_sentences WHERE report_item_id=item.id
        )
        OR EXISTS (
            SELECT 1
            FROM report_item_sentences AS sentence
            LEFT JOIN micro_event_summary_sentences AS source_sentence
              ON source_sentence.id=sentence.source_summary_sentence_id
             AND source_sentence.micro_event_summary_id=item.micro_event_summary_id
            WHERE sentence.report_item_id=item.id AND (
                source_sentence.id IS NULL
                OR source_sentence.ordinal<>sentence.ordinal
                OR source_sentence.sentence<>sentence.sentence
                OR source_sentence.editorial_note<>sentence.editorial_note
                OR source_sentence.decision_origin<>sentence.decision_origin
                OR source_sentence.model_run_id IS DISTINCT FROM sentence.model_run_id
                OR source_sentence.actor_user_id IS DISTINCT FROM sentence.actor_user_id
                OR sentence.editorial_note AND EXISTS (
                    SELECT 1 FROM report_item_sentence_evidences
                    WHERE report_item_sentence_id=sentence.id
                )
                OR NOT sentence.editorial_note AND NOT EXISTS (
                    SELECT 1 FROM report_item_sentence_evidences
                    WHERE report_item_sentence_id=sentence.id
                )
                OR EXISTS (
                    SELECT 1
                    FROM report_item_sentence_evidences AS citation
                    LEFT JOIN micro_event_summary_sentence_evidences AS source_citation
                      ON source_citation.summary_sentence_id=source_sentence.id
                     AND source_citation.claim_evidence_version_id=citation.claim_evidence_version_id
                     AND source_citation.ordinal=citation.ordinal
                    LEFT JOIN claim_evidence_versions AS evidence ON evidence.id=citation.claim_evidence_version_id
                    LEFT JOIN claims AS claim ON claim.id=evidence.claim_id
                    LEFT JOIN document_text_quote_selectors AS selector ON selector.id=evidence.text_quote_selector_id
                    LEFT JOIN document_versions AS document_version ON document_version.id=evidence.document_version_id
                    LEFT JOIN documents AS document ON document.id=document_version.document_id
                    WHERE citation.report_item_sentence_id=sentence.id AND (
                        source_citation.id IS NULL OR evidence.id IS NULL
                        OR NOT EXISTS (
                            SELECT 1
                            FROM evidence_state_snapshot_items AS snapshot_item
                            JOIN micro_event_updates AS citation_update
                              ON citation_update.evidence_state_snapshot_id=snapshot_item.evidence_state_snapshot_id
                            WHERE citation_update.id=item.micro_event_update_id
                              AND snapshot_item.claim_evidence_version_id=evidence.id
                        )
                        OR claim.micro_event_id<>item.micro_event_id
                        OR claim.micro_event_version<>item.micro_event_version
                        OR evidence.relation IN ('withdraws','unknown')
                        OR selector.document_version_id<>evidence.document_version_id
                        OR selector.quote_sha256<>evidence.quote_sha256
                        OR selector.plaintext_sha256<>evidence.plaintext_sha256
                        OR selector.selector_version<>evidence.selector_version
                        OR selector.retention_until<=CURRENT_TIMESTAMP
                        OR evidence.retention_until<=CURRENT_TIMESTAMP
                        OR evidence.captured_at_snapshot<report.period_start
                        OR evidence.captured_at_snapshot>=report.period_end
                        OR document.document_state<>'active'
                        OR document_version.lifecycle_state IN ('policy_blocked','retention_blocked','quarantined','tombstoned')
                        OR EXISTS (
                            SELECT 1 FROM claim_evidence_feedbacks AS feedback
                            WHERE feedback.original_claim_evidence_version_id=evidence.id
                        )
                        OR NOT COALESCE(current_rights_action_allowed(
                            selector.quote_rights_decision_id,selector.source_connection_id,
                            'document_version',evidence.document_version_id::text,evidence.plaintext_sha256,
                            'quote',CURRENT_TIMESTAMP),false)
                        OR NOT COALESCE(current_rights_action_allowed(
                            selector.retain_rights_decision_id,selector.source_connection_id,
                            'document_version',evidence.document_version_id::text,evidence.plaintext_sha256,
                            'retain',CURRENT_TIMESTAMP),false)
                    )
                )
                OR EXISTS (
                    SELECT 1
                    FROM micro_event_summary_sentence_evidences AS source_citation
                    WHERE source_citation.summary_sentence_id=source_sentence.id
                      AND NOT EXISTS (
                          SELECT 1 FROM report_item_sentence_evidences AS citation
                          WHERE citation.report_item_sentence_id=sentence.id
                            AND citation.claim_evidence_version_id=source_citation.claim_evidence_version_id
                            AND citation.ordinal=source_citation.ordinal
                      )
                )
            )
        )
    )
)`, report.ID, report.Version).Scan(&invalid)
	if err != nil {
		return databaserepository.MapError(err)
	}
	if invalid {
		return domain.ErrEvidenceInvalid
	}
	return nil
}

func (repository *Repository) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if repository == nil || repository.runtime == nil || fn == nil {
		return sharedrepository.ErrUnavailable
	}
	return repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error { return fn(transactionCtx) })
}

func (repository *Repository) Get(ctx context.Context, reportID int64) (domain.Report, error) {
	if repository == nil || repository.runtime == nil || reportID <= 0 {
		return domain.Report{}, sharedrepository.ErrUnavailable
	}
	queryer := reportQueryerFor(ctx, repository.runtime)
	report, err := scanReport(queryer.QueryRowContext(ctx, reportSelect+` WHERE id = $1 AND deleted_at IS NULL`, reportID))
	if err != nil {
		return domain.Report{}, err
	}
	items, err := repository.items(ctx, queryer, report.ID)
	if err != nil {
		return domain.Report{}, err
	}
	report.Items = items
	return report, nil
}

func (repository *Repository) List(ctx context.Context, query domain.ListQuery) (domain.Page, error) {
	if repository == nil || repository.runtime == nil {
		return domain.Page{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return domain.Page{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	reportType, status := "", ""
	if query.Type != nil {
		reportType = string(*query.Type)
	}
	if query.Status != nil {
		status = string(*query.Status)
	}
	rows, err := reportQueryerFor(ctx, repository.runtime).QueryContext(ctx, reportSelect+`
WHERE deleted_at IS NULL
  AND ($1 = '' OR report_type = $1)
  AND ($2 = '' OR status = $2)
  AND ($3 = 0 OR id < $3)
ORDER BY id DESC
LIMIT $4`, reportType, status, query.Cursor, query.Limit+1)
	if err != nil {
		return domain.Page{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	page := domain.Page{Items: make([]domain.Report, 0, query.Limit)}
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return domain.Page{}, err
		}
		page.Items = append(page.Items, report)
	}
	if err := rows.Err(); err != nil {
		return domain.Page{}, databaserepository.MapError(err)
	}
	if len(page.Items) > query.Limit {
		page.NextCursor = page.Items[query.Limit-1].ID
		page.Items = page.Items[:query.Limit]
	}
	if err := rows.Close(); err != nil {
		return domain.Page{}, databaserepository.MapError(err)
	}
	queryer := reportQueryerFor(ctx, repository.runtime)
	for index := range page.Items {
		items, err := repository.items(ctx, queryer, page.Items[index].ID)
		if err != nil {
			return domain.Page{}, err
		}
		page.Items[index].Items = items
	}
	return page, nil
}

const reportSelect = `SELECT id, version, report_type, monitor_id, period_start, period_end, timezone, title, summary, body, status, version_no, generated_at, published_at, created_by, updated_by FROM reports`

type reportRow interface {
	Scan(...any) error
}

type reportQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func reportQueryerFor(ctx context.Context, runtime *database.Runtime) reportQueryer {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return transaction.SQL
	}
	return runtime.SQL
}

func scanReport(row reportRow) (domain.Report, error) {
	var report domain.Report
	var reportType, status string
	var monitorID, createdBy, updatedBy sql.NullInt64
	var generatedAt, publishedAt sql.NullTime
	if err := row.Scan(&report.ID, &report.Version, &reportType, &monitorID, &report.Period.Start, &report.Period.End, &reportTimezone{period: &report.Period}, &report.Title, &report.Summary, &report.Body, &status, &report.VersionNo, &generatedAt, &publishedAt, &createdBy, &updatedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Report{}, sharedrepository.ErrNotFound
		}
		return domain.Report{}, databaserepository.MapError(err)
	}
	report.Type, report.Status = domain.ReportType(reportType), domain.ReportStatus(status)
	report.Frozen = report.Status == domain.ReportPublished
	if monitorID.Valid {
		value := monitorID.Int64
		report.MonitorID = &value
	}
	if generatedAt.Valid {
		value := generatedAt.Time.UTC()
		report.GeneratedAt = &value
	}
	if publishedAt.Valid {
		value := publishedAt.Time.UTC()
		report.PublishedAt = &value
	}
	if createdBy.Valid {
		value := createdBy.Int64
		report.CreatedBy = &value
	}
	if updatedBy.Valid {
		value := updatedBy.Int64
		report.UpdatedBy = &value
	}
	return report, nil
}

// reportTimezone restores the location used when the report period was
// calculated. The database keeps timestamps in UTC while the report contract
// keeps its original calendar timezone for display and future versioning.
type reportTimezone struct{ period *domain.Period }

func (target *reportTimezone) Scan(value any) error {
	var name string
	switch typed := value.(type) {
	case string:
		name = typed
	case []byte:
		name = string(typed)
	default:
		return fmt.Errorf("invalid report timezone")
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return fmt.Errorf("invalid report timezone: %w", err)
	}
	target.period.Location = location
	return nil
}

func (repository *Repository) items(ctx context.Context, queryer reportQueryer, reportID int64) ([]domain.Item, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT id,event_id,event_update_id,micro_event_id,micro_event_version,
micro_event_update_id,micro_event_summary_id,rank,inclusion_reason,title_snapshot,summary_snapshot,
heat_score_snapshot,evidence_set_hash,array_to_json(reason_codes)
FROM report_items WHERE report_id = $1 ORDER BY rank,COALESCE(event_id,micro_event_id)`, reportID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	type itemRecord struct {
		id   int64
		item domain.Item
	}
	records := make([]itemRecord, 0)
	for rows.Next() {
		var item domain.Item
		var itemID int64
		var eventID, eventUpdateID, microEventID, microEventVersion, microEventUpdateID, microEventSummaryID sql.NullInt64
		var reasons []byte
		if err := rows.Scan(&itemID, &eventID, &eventUpdateID, &microEventID, &microEventVersion, &microEventUpdateID,
			&microEventSummaryID, &item.Rank, &item.InclusionReason, &item.Title, &item.Summary, &item.HeatScore,
			&item.EvidenceSetHash, &reasons); err != nil {
			return nil, databaserepository.MapError(err)
		}
		item.EventID, item.EventUpdateID = nullableReportID(eventID), nullableReportID(eventUpdateID)
		item.MicroEventID, item.MicroEventVersion = nullableReportID(microEventID), nullableReportID(microEventVersion)
		item.MicroEventUpdateID, item.MicroEventSummaryID = nullableReportID(microEventUpdateID), nullableReportID(microEventSummaryID)
		if err := json.Unmarshal(reasons, &item.ReasonCodes); err != nil {
			return nil, fmt.Errorf("decode report item reasons: %w", err)
		}
		records = append(records, itemRecord{id: itemID, item: item})
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	if err := rows.Close(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	items := make([]domain.Item, 0, len(records))
	for _, record := range records {
		item := record.item
		item.Sentences, err = reportSentences(ctx, queryer, record.id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func reportSentences(ctx context.Context, queryer reportQueryer, reportItemID int64) ([]domain.Sentence, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT sentence.id,sentence.source_summary_sentence_id,sentence.ordinal,
sentence.sentence,sentence.editorial_note,sentence.decision_origin,sentence.model_run_id,sentence.actor_user_id,
COALESCE(json_agg(citation.claim_evidence_version_id ORDER BY citation.ordinal)
    FILTER (WHERE citation.claim_evidence_version_id IS NOT NULL),'[]'::json)
FROM report_item_sentences AS sentence
LEFT JOIN report_item_sentence_evidences AS citation ON citation.report_item_sentence_id=sentence.id
WHERE sentence.report_item_id=$1
GROUP BY sentence.id ORDER BY sentence.ordinal`, reportItemID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	sentences := make([]domain.Sentence, 0)
	for rows.Next() {
		var sentence domain.Sentence
		var sentenceID int64
		var modelRunID, actorUserID sql.NullInt64
		var evidenceJSON []byte
		if err := rows.Scan(&sentenceID, &sentence.SourceSummarySentenceID, &sentence.Ordinal, &sentence.Text,
			&sentence.EditorialNote, &sentence.DecisionOrigin, &modelRunID, &actorUserID, &evidenceJSON); err != nil {
			return nil, databaserepository.MapError(err)
		}
		sentence.ModelRunID, sentence.ActorUserID = nullableReportIDPointer(modelRunID), nullableReportIDPointer(actorUserID)
		if err := json.Unmarshal(evidenceJSON, &sentence.ClaimEvidenceVersionIDs); err != nil {
			return nil, fmt.Errorf("decode report sentence citations: %w", err)
		}
		sentences = append(sentences, sentence)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return sentences, nil
}

func nullableReportID(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullableReportIDPointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
