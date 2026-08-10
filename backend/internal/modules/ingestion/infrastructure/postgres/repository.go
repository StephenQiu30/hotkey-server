package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const (
	contentListDefaultLimit = 50
	contentListMaximumLimit = 200
)

// ContentRepository owns ingestion's Content, source-author, asset and
// metric-snapshot facts. Source collection state is deliberately outside this
// adapter's table boundary.
type ContentRepository struct{ runtime *database.Runtime }

var _ ingestiondomain.ContentRepository = (*ContentRepository)(nil)

func NewContentRepository(runtime *database.Runtime) *ContentRepository {
	return &ContentRepository{runtime: runtime}
}

// Upsert creates one source fact or refreshes its collection observation. A
// retry must not reconsider the original duplicate decision or replace the
// normalized presentation facts; it only advances fetched metrics and its
// matching point-in-time snapshot.
func (repository *ContentRepository) Upsert(ctx context.Context, content ingestiondomain.NormalizedContent, decision ingestiondomain.DedupeDecision) (ingestiondomain.Content, bool, error) {
	if !repository.available() {
		return ingestiondomain.Content{}, false, sharedrepository.ErrUnavailable
	}
	content.ExternalID = ingestiondomain.NormalizeExternalID(content.ExternalID)
	if err := content.Validate(); err != nil {
		return ingestiondomain.Content{}, false, fmt.Errorf("%w: normalized content: %v", sharedrepository.ErrInvalidInput, err)
	}
	if err := content.Metrics.Validate(); err != nil {
		return ingestiondomain.Content{}, false, fmt.Errorf("%w: normalized metrics: %v", sharedrepository.ErrInvalidInput, err)
	}
	if err := decision.Validate(); err != nil {
		return ingestiondomain.Content{}, false, fmt.Errorf("%w: dedupe decision: %v", sharedrepository.ErrInvalidInput, err)
	}

	var stored ingestiondomain.Content
	created := false
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		var contentID int64
		arguments := []any{
			content.SourceConnectionID, content.ExternalID, content.ContentType, content.Title, content.Excerpt,
			content.CanonicalURL, content.Language, content.PublishedAt.UTC(), content.FetchedAt.UTC(), content.ContentHash,
			decision.DuplicateOfID, nullableString(decision.Reason), nullableString(decision.Version),
		}
		arguments = append(arguments, metricArguments(content.Metrics)...)
		arguments = append(arguments, string(decision.Status))
		if err := transaction.SQL.QueryRowContext(ctx, `
INSERT INTO contents (
    source_connection_id, external_id, content_type, title, excerpt,
    canonical_url, language, published_at, fetched_at, dedupe_key,
    duplicate_of_id, dedupe_reason, dedupe_version, view_count, like_count,
    comment_count, share_count, content_status
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18
)
ON CONFLICT (source_connection_id, external_id) DO UPDATE
SET fetched_at = EXCLUDED.fetched_at,
    view_count = EXCLUDED.view_count,
    like_count = EXCLUDED.like_count,
    comment_count = EXCLUDED.comment_count,
    share_count = EXCLUDED.share_count,
    version = contents.version + 1,
    updated_at = now()
RETURNING id, (xmax = 0)`,
			arguments...).Scan(&contentID, &created); err != nil {
			return databaserepository.MapError(err)
		}
		if created {
			authorID, err := upsertAuthor(ctx, transaction.SQL, content)
			if err != nil {
				return err
			}
			if authorID != nil {
				if _, err := transaction.SQL.ExecContext(ctx, `UPDATE contents SET author_id = $1 WHERE id = $2`, authorID, contentID); err != nil {
					return databaserepository.MapError(err)
				}
			}
		}
		if err := appendMetricSnapshot(ctx, transaction.SQL, contentID, content.FetchedAt, content.Metrics); err != nil {
			return err
		}
		selected, err := selectContentByID(ctx, transaction.SQL, contentID)
		if err != nil {
			return err
		}
		stored = selected
		return nil
	})
	if err != nil {
		return ingestiondomain.Content{}, false, err
	}
	return stored, created, nil
}

func (repository *ContentRepository) AppendMetricSnapshot(ctx context.Context, contentID int64, capturedAt time.Time, metrics sourcedomain.SourceMetrics) error {
	if !repository.available() {
		return sharedrepository.ErrUnavailable
	}
	if contentID <= 0 || capturedAt.IsZero() {
		return fmt.Errorf("%w: positive content id and captured time are required", sharedrepository.ErrInvalidInput)
	}
	if err := metrics.Validate(); err != nil {
		return fmt.Errorf("%w: source metrics: %v", sharedrepository.ErrInvalidInput, err)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		return appendMetricSnapshot(ctx, transaction.SQL, contentID, capturedAt, metrics)
	})
}

func (repository *ContentRepository) CreateAsset(ctx context.Context, asset ingestiondomain.ContentAsset) error {
	if !repository.available() {
		return sharedrepository.ErrUnavailable
	}
	if err := validateAsset(asset); err != nil {
		return fmt.Errorf("%w: content asset: %v", sharedrepository.ErrInvalidInput, err)
	}
	originalURL, err := safeOriginalURL(asset.OriginalURL)
	if err != nil {
		return fmt.Errorf("%w: content asset original URL: %v", sharedrepository.ErrInvalidInput, err)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if _, err := transaction.SQL.ExecContext(ctx, `
INSERT INTO content_assets (
    content_id, asset_type, object_key, original_url, mime_type, sha256,
    size_bytes, captured_at, object_status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			asset.ContentID, asset.AssetType, asset.ObjectKey, originalURL, asset.MIMEType,
			asset.SHA256, asset.SizeBytes, asset.CapturedAt.UTC(), string(asset.Status)); err != nil {
			return databaserepository.MapError(err)
		}
		return nil
	})
}

// MarkAssetStatus uses an internal read/version/write conditional update. The
// public lifecycle command remains idempotent for a repeated desired status,
// while competing state transitions receive a stable conflict instead of a
// silent last-writer-wins update.
func (repository *ContentRepository) MarkAssetStatus(ctx context.Context, objectKey string, status ingestiondomain.AssetStatus) error {
	if !repository.available() {
		return sharedrepository.ErrUnavailable
	}
	if strings.TrimSpace(objectKey) == "" || !validAssetStatus(status) {
		return fmt.Errorf("%w: object key and valid asset status are required", sharedrepository.ErrInvalidInput)
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		var record assetStatusRecord
		if err := transaction.SQL.QueryRowContext(ctx, `
SELECT version, object_status
FROM content_assets
WHERE object_key = $1`, objectKey).Scan(&record.version, &record.status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: asset %q", sharedrepository.ErrNotFound, objectKey)
			}
			return databaserepository.MapError(err)
		}
		if record.status == string(status) {
			return nil
		}
		result, err := transaction.SQL.ExecContext(ctx, `
UPDATE content_assets
SET object_status = $1, version = version + 1, updated_at = now()
WHERE object_key = $2 AND version = $3 AND object_status = $4`, status, objectKey, record.version, record.status)
		if err != nil {
			return databaserepository.MapError(err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return databaserepository.MapError(err)
		}
		if updated == 0 {
			return fmt.Errorf("%w: asset %q changed concurrently", sharedrepository.ErrConflict, objectKey)
		}
		return nil
	})
}

// ListEvidenceAssets returns the undeleted, source-scoped assets that can be
// operated through ingestion's EvidenceStore. Other content asset types never
// cross this storage boundary.
func (repository *ContentRepository) ListEvidenceAssets(ctx context.Context, sourceConnectionID, contentID int64) ([]ingestiondomain.ContentAsset, error) {
	if !repository.available() {
		return nil, sharedrepository.ErrUnavailable
	}
	if sourceConnectionID <= 0 || contentID <= 0 {
		return nil, fmt.Errorf("%w: source connection id and content id are required", sharedrepository.ErrInvalidInput)
	}
	rows, err := repository.queryRows(ctx, `
SELECT asset.id, asset.version, asset.content_id, asset.asset_type,
       asset.object_key, asset.original_url, asset.mime_type, asset.sha256,
       asset.size_bytes, asset.captured_at, asset.object_status
FROM content_assets AS asset
JOIN contents AS content ON content.id = asset.content_id
WHERE content.source_connection_id = $1
  AND content.id = $2
  AND asset.object_status <> 'deleted'
  AND asset.object_key LIKE $3
ORDER BY asset.object_key ASC`, sourceConnectionID, contentID, evidencePrefixPattern(sourceConnectionID))
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	assets := make([]ingestiondomain.ContentAsset, 0)
	for rows.Next() {
		asset, err := scanContentAsset(rows)
		if err != nil {
			return nil, databaserepository.MapError(err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return assets, nil
}

// ListAssetObjectKeys returns the ingestion evidence references that must be
// preserved during source-scoped object reconciliation. A lifecycle-deleted
// asset deliberately stops protecting its object; all other durable asset
// states remain references until the lifecycle command resolves them.
func (repository *ContentRepository) ListAssetObjectKeys(ctx context.Context, sourceConnectionID int64) ([]string, error) {
	if !repository.available() {
		return nil, sharedrepository.ErrUnavailable
	}
	if sourceConnectionID <= 0 {
		return nil, fmt.Errorf("%w: source connection id is required", sharedrepository.ErrInvalidInput)
	}
	rows, err := repository.queryRows(ctx, `
SELECT asset.object_key
FROM content_assets AS asset
JOIN contents AS content ON content.id = asset.content_id
WHERE content.source_connection_id = $1
  AND asset.object_status <> 'deleted'
  AND asset.object_key LIKE $2
ORDER BY asset.object_key ASC`, sourceConnectionID, evidencePrefixPattern(sourceConnectionID))
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	keys := make([]string, 0)
	for rows.Next() {
		var objectKey string
		if err := rows.Scan(&objectKey); err != nil {
			return nil, databaserepository.MapError(err)
		}
		keys = append(keys, objectKey)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return keys, nil
}

func (repository *ContentRepository) ListActive(ctx context.Context, query ingestiondomain.ContentListQuery) (ingestiondomain.ContentPage, error) {
	if !repository.available() {
		return ingestiondomain.ContentPage{}, sharedrepository.ErrUnavailable
	}
	query, cursorID, fingerprint, err := contentListParameters(query)
	if err != nil {
		return ingestiondomain.ContentPage{}, err
	}
	statement, arguments := contentListStatement(query, cursorID)
	rows, err := repository.queryRows(ctx, statement, arguments...)
	if err != nil {
		return ingestiondomain.ContentPage{}, databaserepository.MapError(err)
	}
	defer rows.Close()

	page := ingestiondomain.ContentPage{Items: make([]ingestiondomain.Content, 0, query.Limit+1)}
	for rows.Next() {
		content, err := scanContentSearch(rows)
		if err != nil {
			return ingestiondomain.ContentPage{}, databaserepository.MapError(err)
		}
		page.Items = append(page.Items, content)
	}
	if err := rows.Err(); err != nil {
		return ingestiondomain.ContentPage{}, databaserepository.MapError(err)
	}
	if len(page.Items) <= query.Limit {
		return page, nil
	}
	page.Items = page.Items[:query.Limit]
	page.NextCursor, err = pagination.Encode(string(query.Sort), true, fingerprint, page.Items[len(page.Items)-1].ID)
	if err != nil {
		return ingestiondomain.ContentPage{}, fmt.Errorf("%w: encode content cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return page, nil
}

// GetActive returns one currently visible Content fact. Deleted, expired,
// invalid and duplicate rows intentionally share the external not-found
// result so callers cannot use this reader to inspect non-active state.
func (repository *ContentRepository) GetActive(ctx context.Context, contentID int64) (ingestiondomain.Content, error) {
	if !repository.available() {
		return ingestiondomain.Content{}, sharedrepository.ErrUnavailable
	}
	if contentID <= 0 {
		return ingestiondomain.Content{}, fmt.Errorf("%w: positive content id is required", sharedrepository.ErrInvalidInput)
	}
	content, err := scanContentWithDocumentVersion(repository.queryRow(ctx, `
SELECT `+contentColumns+`, archived_version.document_version_id
FROM contents AS c
LEFT JOIN source_authors AS author ON author.id = c.author_id
`+activeContentDocumentVersionJoin+`
WHERE c.id = $1
  AND c.content_status = 'active'
  AND c.deleted_at IS NULL`, contentID))
	if errors.Is(err, sql.ErrNoRows) {
		return ingestiondomain.Content{}, fmt.Errorf("%w: active content %d", sharedrepository.ErrNotFound, contentID)
	}
	if err != nil {
		return ingestiondomain.Content{}, databaserepository.MapError(err)
	}
	return content, nil
}

func (repository *ContentRepository) MarkDeleted(ctx context.Context, sourceConnectionID int64, externalID string) (ingestiondomain.Content, bool, error) {
	if !repository.available() {
		return ingestiondomain.Content{}, false, sharedrepository.ErrUnavailable
	}
	externalID = ingestiondomain.NormalizeExternalID(externalID)
	if sourceConnectionID <= 0 || externalID == "" {
		return ingestiondomain.Content{}, false, fmt.Errorf("%w: source connection id and external id are required", sharedrepository.ErrInvalidInput)
	}
	var content ingestiondomain.Content
	changed := false
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		var contentID int64
		err := transaction.SQL.QueryRowContext(ctx, `
UPDATE contents
SET content_status = 'deleted', deleted_at = now(),
    duplicate_of_id = NULL, dedupe_reason = NULL, dedupe_version = NULL,
    version = version + 1, updated_at = now()
WHERE source_connection_id = $1
  AND external_id = $2
  AND (content_status <> 'deleted' OR deleted_at IS NULL)
RETURNING id`, sourceConnectionID, externalID).Scan(&contentID)
		if err == nil {
			changed = true
			content, err = selectContentByID(ctx, transaction.SQL, contentID)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return databaserepository.MapError(err)
		}
		content, err = selectContentBySourceExternalID(ctx, transaction.SQL, sourceConnectionID, externalID)
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return nil
		}
		return err
	})
	if err != nil {
		return ingestiondomain.Content{}, false, err
	}
	return content, changed, nil
}

// ExpireBefore changes only currently active ingestion Content facts. It
// deliberately leaves assets and every downstream module untouched; expiry is
// immediately reflected by ListActive's active-status boundary.
func (repository *ContentRepository) ExpireBefore(ctx context.Context, before time.Time) (int, error) {
	if !repository.available() {
		return 0, sharedrepository.ErrUnavailable
	}
	if before.IsZero() {
		return 0, fmt.Errorf("%w: expiry time is required", sharedrepository.ErrInvalidInput)
	}
	expired := 0
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		result, err := transaction.SQL.ExecContext(ctx, `
UPDATE contents
SET content_status = 'expired', version = version + 1, updated_at = now()
WHERE content_status = 'active'
  AND deleted_at IS NULL
  AND published_at < $1`, before.UTC())
		if err != nil {
			return databaserepository.MapError(err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return databaserepository.MapError(err)
		}
		expired = int(affected)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return expired, nil
}

func upsertAuthor(ctx context.Context, executor queryRowExecutor, content ingestiondomain.NormalizedContent) (any, error) {
	if strings.TrimSpace(content.Author.ExternalID) == "" {
		return nil, nil
	}
	var authorID int64
	err := executor.QueryRowContext(ctx, `
INSERT INTO source_authors (source_connection_id, external_id, display_name)
VALUES ($1, $2, $3)
ON CONFLICT (source_connection_id, external_id) DO NOTHING
RETURNING id`, content.SourceConnectionID, content.Author.ExternalID, nullableString(content.Author.DisplayName)).Scan(&authorID)
	if err == nil {
		return authorID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, databaserepository.MapError(err)
	}
	if err := executor.QueryRowContext(ctx, `
SELECT id
FROM source_authors
WHERE source_connection_id = $1 AND external_id = $2`, content.SourceConnectionID, content.Author.ExternalID).Scan(&authorID); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return authorID, nil
}

func appendMetricSnapshot(ctx context.Context, executor sqlExecutor, contentID int64, capturedAt time.Time, metrics sourcedomain.SourceMetrics) error {
	arguments := []any{contentID, capturedAt.UTC()}
	arguments = append(arguments, metricArguments(metrics)...)
	if _, err := executor.ExecContext(ctx, `
INSERT INTO content_metric_snapshots (
    content_id, captured_at, view_count, like_count, comment_count, share_count
)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (content_id, captured_at) DO UPDATE
SET view_count = EXCLUDED.view_count,
    like_count = EXCLUDED.like_count,
    comment_count = EXCLUDED.comment_count,
    share_count = EXCLUDED.share_count`, arguments...); err != nil {
		return databaserepository.MapError(err)
	}
	return nil
}

func selectContentByID(ctx context.Context, executor queryRowExecutor, contentID int64) (ingestiondomain.Content, error) {
	content, err := scanContent(executor.QueryRowContext(ctx, `
SELECT `+contentColumns+`
FROM contents AS c
LEFT JOIN source_authors AS author ON author.id = c.author_id
WHERE c.id = $1`, contentID))
	if errors.Is(err, sql.ErrNoRows) {
		return ingestiondomain.Content{}, fmt.Errorf("%w: content %d", sharedrepository.ErrNotFound, contentID)
	}
	if err != nil {
		return ingestiondomain.Content{}, databaserepository.MapError(err)
	}
	return content, nil
}

func selectContentBySourceExternalID(ctx context.Context, executor queryRowExecutor, sourceConnectionID int64, externalID string) (ingestiondomain.Content, error) {
	content, err := scanContent(executor.QueryRowContext(ctx, `
SELECT `+contentColumns+`
FROM contents AS c
LEFT JOIN source_authors AS author ON author.id = c.author_id
WHERE c.source_connection_id = $1 AND c.external_id = $2`, sourceConnectionID, externalID))
	if errors.Is(err, sql.ErrNoRows) {
		return ingestiondomain.Content{}, fmt.Errorf("%w: content source item", sharedrepository.ErrNotFound)
	}
	if err != nil {
		return ingestiondomain.Content{}, databaserepository.MapError(err)
	}
	return content, nil
}

func (repository *ContentRepository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
}

func (repository *ContentRepository) queryRows(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryContext(ctx, query, args...)
	}
	return repository.runtime.SQL.QueryContext(ctx, query, args...)
}

func (repository *ContentRepository) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryRowContext(ctx, query, args...)
	}
	return repository.runtime.SQL.QueryRowContext(ctx, query, args...)
}

func (repository *ContentRepository) available() bool {
	return repository != nil && repository.runtime != nil && repository.runtime.SQL != nil
}

func contentListParameters(query ingestiondomain.ContentListQuery) (ingestiondomain.ContentListQuery, int64, string, error) {
	query = query.Normalized()
	if query.Limit == 0 {
		query.Limit = contentListDefaultLimit
	}
	if query.Limit > contentListMaximumLimit || query.Validate() != nil {
		return ingestiondomain.ContentListQuery{}, 0, "", fmt.Errorf("%w: invalid content list query", sharedrepository.ErrInvalidInput)
	}
	fingerprint, err := query.ShapeFingerprint()
	if err != nil {
		return ingestiondomain.ContentListQuery{}, 0, "", fmt.Errorf("%w: content shape: %v", sharedrepository.ErrInvalidInput, err)
	}
	cursor, err := pagination.Decode(query.Cursor, string(query.Sort), true, fingerprint)
	if err != nil {
		return ingestiondomain.ContentListQuery{}, 0, "", fmt.Errorf("%w: content cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return query, cursor.ID, fingerprint, nil
}

type contentSQLBuilder struct{ arguments []any }

func (builder *contentSQLBuilder) bind(value any) string {
	builder.arguments = append(builder.arguments, value)
	return fmt.Sprintf("$%d", len(builder.arguments))
}

func contentListStatement(query ingestiondomain.ContentListQuery, cursorID int64) (string, []any) {
	builder := &contentSQLBuilder{}
	monitorID := int64(0)
	if query.MonitorID != nil {
		monitorID = *query.MonitorID
	}
	monitor := builder.bind(monitorID)
	conditions := []string{"c.content_status = 'active'", "c.deleted_at IS NULL"}
	if query.Keyword != "" {
		conditions = append(conditions, "lower(c.title || ' ' || c.excerpt) LIKE "+builder.bind(contentSearchPattern(query.Keyword))+" ESCAPE '\\'")
	}
	if query.SourceConnectionID != nil {
		conditions = append(conditions, "c.source_connection_id = "+builder.bind(*query.SourceConnectionID))
	}
	if query.PublishedFrom != nil {
		conditions = append(conditions, "c.published_at >= "+builder.bind(query.PublishedFrom.UTC()))
	}
	if query.PublishedTo != nil {
		conditions = append(conditions, "c.published_at <= "+builder.bind(query.PublishedTo.UTC()))
	}
	if query.MonitorID != nil {
		conditions = append(conditions, "latest_match.content_id IS NOT NULL")
	}
	if query.Decision != nil {
		conditions = append(conditions, "latest_match.decision = "+builder.bind(string(*query.Decision)))
	}
	if cursorID > 0 {
		cursor := builder.bind(cursorID)
		if query.Sort == ingestiondomain.ContentSortRelevance {
			conditions = append(conditions, `(latest_match.final_score, c.id) < (
    SELECT previous_match.final_score, previous.id
    FROM contents AS previous
    JOIN LATERAL (
        SELECT match.final_score
        FROM monitor_matches AS match
        WHERE match.monitor_id = `+monitor+` AND match.content_id = previous.id
        ORDER BY match.created_at DESC, match.id DESC
        LIMIT 1
    ) AS previous_match ON true
    WHERE previous.id = `+cursor+`)`)
		} else {
			conditions = append(conditions, `(c.published_at, c.id) < (
    SELECT previous.published_at, previous.id FROM contents AS previous WHERE previous.id = `+cursor+`)`)
		}
	}
	orderBy := "c.published_at DESC, c.id DESC"
	if query.Sort == ingestiondomain.ContentSortRelevance {
		orderBy = "latest_match.final_score DESC, c.id DESC"
	}
	statement := `SELECT ` + contentColumns + `,
       archived_version.document_version_id,
       latest_match.final_score::double precision, latest_match.decision
FROM contents AS c
LEFT JOIN source_authors AS author ON author.id = c.author_id
` + activeContentDocumentVersionJoin + `
LEFT JOIN LATERAL (
    SELECT match.content_id, match.final_score, match.decision
    FROM monitor_matches AS match
    WHERE ` + monitor + ` > 0 AND match.monitor_id = ` + monitor + ` AND match.content_id = c.id
    ORDER BY match.created_at DESC, match.id DESC
    LIMIT 1
) AS latest_match ON true
WHERE ` + strings.Join(conditions, " AND ") + `
ORDER BY ` + orderBy + `
LIMIT ` + builder.bind(query.Limit+1)
	return statement, builder.arguments
}

const activeContentDocumentVersionJoin = `LEFT JOIN LATERAL (
    SELECT version.id AS document_version_id
    FROM documents AS document
    JOIN document_versions AS version ON version.id = document.current_document_version_id
    WHERE document.source_connection_id = c.source_connection_id
      AND document.external_work_id = c.external_id
      AND document.document_state = 'active'
      AND version.lifecycle_state = 'readable'
      AND current_rights_action_allowed(
          version.display_private_rights_decision_id,
          document.source_connection_id,
          'document_version',
          version.id::text,
          version.content_sha256,
          'display_private',
          CURRENT_TIMESTAMP
      )
    ORDER BY document.updated_at DESC, document.id DESC
    LIMIT 1
) AS archived_version ON true`

func contentSearchPattern(keyword string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(strings.ToLower(keyword)) + "%"
}

func metricArguments(metrics sourcedomain.SourceMetrics) []any {
	return []any{metricArgument(metrics.ViewCount), metricArgument(metrics.LikeCount), metricArgument(metrics.CommentCount), metricArgument(metrics.ShareCount)}
}

func metricArgument(metric *int64) any {
	if metric == nil {
		return nil
	}
	return *metric
}

func evidencePrefixPattern(sourceConnectionID int64) string {
	return fmt.Sprintf("evidence/v1/%d/%%", sourceConnectionID)
}

func scanContentAsset(scanner interface{ Scan(...any) error }) (ingestiondomain.ContentAsset, error) {
	var asset ingestiondomain.ContentAsset
	var originalURL sql.NullString
	var status string
	if err := scanner.Scan(
		&asset.ID, &asset.Version, &asset.ContentID, &asset.AssetType,
		&asset.ObjectKey, &originalURL, &asset.MIMEType, &asset.SHA256,
		&asset.SizeBytes, &asset.CapturedAt, &status,
	); err != nil {
		return ingestiondomain.ContentAsset{}, err
	}
	asset.CapturedAt = asset.CapturedAt.UTC()
	asset.OriginalURL = originalURL.String
	asset.Status = ingestiondomain.AssetStatus(status)
	if !validAssetStatus(asset.Status) {
		return ingestiondomain.ContentAsset{}, fmt.Errorf("invalid persisted asset status %q", status)
	}
	return asset, nil
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func validateAsset(asset ingestiondomain.ContentAsset) error {
	if asset.ContentID <= 0 || strings.TrimSpace(asset.AssetType) == "" || strings.TrimSpace(asset.ObjectKey) == "" || strings.TrimSpace(asset.MIMEType) == "" || !validSHA256(asset.SHA256) || asset.SizeBytes < 0 || asset.CapturedAt.IsZero() || !validAssetStatus(asset.Status) {
		return errors.New("asset is incomplete or invalid")
	}
	return nil
}

func validAssetStatus(status ingestiondomain.AssetStatus) bool {
	switch status {
	case ingestiondomain.AssetStatusPending, ingestiondomain.AssetStatusAvailable, ingestiondomain.AssetStatusMissing, ingestiondomain.AssetStatusDeletePending, ingestiondomain.AssetStatusDeleted:
		return true
	default:
		return false
	}
}

func safeOriginalURL(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, errors.New("original URL must be a credential-free HTTP(S) URL")
	}
	if parsed.Fragment != "" {
		return nil, errors.New("original URL fragments are not persisted")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return nil, errors.New("original URL query is malformed")
	}
	for key := range query {
		if credentialLikeQueryKey(key) {
			return nil, errors.New("original URL contains a credential-like query key")
		}
	}
	return parsed.String(), nil
}

func credentialLikeQueryKey(key string) bool {
	canonical := strings.ToLower(strings.TrimSpace(key))
	canonical = strings.NewReplacer("-", "", "_", "").Replace(canonical)
	if canonical == "sig" || canonical == "xamzsignature" {
		return true
	}
	for _, sensitive := range []string{"accesskey", "apikey", "authorization", "credential", "password", "secret", "signature", "token"} {
		if strings.Contains(canonical, sensitive) {
			return true
		}
	}
	return false
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type queryRowExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
