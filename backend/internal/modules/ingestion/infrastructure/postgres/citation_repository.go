package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// CitationRepository reads one immutable DocumentVersion and evaluates its
// rights in the same PostgreSQL statement. It never selects object keys,
// Vault paths, credentials, or raw payloads.
type CitationRepository struct{ runtime *database.Runtime }

var _ ingestionapplication.CitationReader = (*CitationRepository)(nil)

func NewCitationRepository(runtime *database.Runtime) *CitationRepository {
	return &CitationRepository{runtime: runtime}
}

type citationQueryExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// citationRecord is private to the PostgreSQL adapter. Application DTOs are
// populated explicitly so persistence-only nullable and rights fields cannot
// cross the adapter boundary by accident.
type citationRecord struct {
	documentID             int64
	documentVersionID      int64
	sourceConnectionID     int64
	documentState          string
	documentLifecycleState string
	observationState       string
	sourceType             string
	sourceName             string
	title                  string
	author                 sql.NullString
	sourceRecordURL        sql.NullString
	canonicalURL           sql.NullString
	discussionURL          sql.NullString
	bodyOrigin             string
	completeness           string
	language               string
	publishedAt            sql.NullTime
	capturedAt             time.Time
	contentSHA256          string
	displayPrivateAllowed  bool
	rightsEvaluatedAt      time.Time

	artifactID               sql.NullInt64
	artifactType             sql.NullString
	transformerProfileSHA256 sql.NullString
	mimeType                 sql.NullString
	artifactSHA256           sql.NullString
	sizeBytes                sql.NullInt64
	artifactLifecycleState   sql.NullString
	artifactActive           sql.NullBool
	failureCode              sql.NullString
	availableAt              sql.NullTime
	retentionUntil           sql.NullTime
	storeDerivedAllowed      bool
	retainAllowed            bool
	currentRetentionDays     sql.NullInt64
}

func (repository *CitationRepository) ReadCitation(ctx context.Context, documentVersionID int64) (ingestionapplication.CitationReadDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return ingestionapplication.CitationReadDTO{}, sharedrepository.ErrUnavailable
	}
	if documentVersionID <= 0 {
		return ingestionapplication.CitationReadDTO{}, fmt.Errorf("%w: invalid document version id", sharedrepository.ErrInvalidInput)
	}
	record, err := scanCitationRecord(repository.executor(ctx).QueryRowContext(ctx, `
SELECT
  document.id,
  document_version.id,
  document.source_connection_id,
  document.document_state,
  document_version.lifecycle_state,
  observation.observation_state,
  source.source_type,
  source.name,
  observation.title,
  observation.author_snapshot,
  observation.source_record_url,
  observation.canonical_url,
  observation.discussion_url,
  document_version.body_origin,
  document_version.completeness,
  document_version.language,
  observation.published_at,
  document_version.captured_at,
  btrim(document_version.content_sha256),
  current_rights_action_allowed(
    document_version.display_private_rights_decision_id,
    document.source_connection_id,
    'document_version', document_version.id::text, document_version.content_sha256,
    'display_private', CURRENT_TIMESTAMP
  ) AS display_private_allowed,
  CURRENT_TIMESTAMP AS rights_evaluated_at,
  artifact.id,
  artifact.artifact_type,
  btrim(artifact.transformer_profile_sha256),
  artifact.mime_type,
  btrim(artifact.sha256),
  artifact.size_bytes,
  artifact.lifecycle_state,
  artifact.active,
  artifact.failure_code,
  artifact.available_at,
  artifact.retention_until,
  CASE WHEN artifact.id IS NULL THEN false ELSE current_rights_action_allowed(
    artifact.store_derived_rights_decision_id,
    document.source_connection_id,
    'document_version', document_version.id::text, document_version.content_sha256,
    'store_derived', CURRENT_TIMESTAMP
  ) END AS store_derived_allowed,
  CASE WHEN artifact.id IS NULL THEN false ELSE current_rights_action_allowed(
    artifact.retain_rights_decision_id,
    document.source_connection_id,
    'document_version', document_version.id::text, document_version.content_sha256,
    'retain', CURRENT_TIMESTAMP
  ) END AS retain_allowed,
  CASE WHEN artifact.id IS NULL THEN NULL ELSE current_rights_retention_days(
    document.source_connection_id,
    'document_version', document_version.id::text, document_version.content_sha256,
    CURRENT_TIMESTAMP
  ) END AS current_retention_days
FROM document_versions AS document_version
JOIN documents AS document ON document.id = document_version.document_id
JOIN source_observations AS observation ON observation.id = document_version.source_observation_id
JOIN source_connections AS source ON source.id = document.source_connection_id
LEFT JOIN LATERAL (
  SELECT candidate.*
  FROM derived_artifacts AS candidate
  WHERE candidate.document_version_id = document_version.id
    AND candidate.source_connection_id = document.source_connection_id
    AND candidate.artifact_type = 'markdown'
  ORDER BY candidate.active DESC,
           (candidate.lifecycle_state = 'derived_available') DESC,
           candidate.created_at DESC,
           candidate.id DESC
  LIMIT 1
) AS artifact ON true
WHERE document_version.id = $1`, documentVersionID))
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.CitationReadDTO{}, fmt.Errorf("%w: document version %d", sharedrepository.ErrNotFound, documentVersionID)
	}
	if err != nil {
		return ingestionapplication.CitationReadDTO{}, databaserepository.MapError(err)
	}
	return citationReadDTO(record), nil
}

func (repository *CitationRepository) executor(ctx context.Context) citationQueryExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

func scanCitationRecord(row *sql.Row) (citationRecord, error) {
	var record citationRecord
	err := row.Scan(
		&record.documentID, &record.documentVersionID, &record.sourceConnectionID,
		&record.documentState, &record.documentLifecycleState, &record.observationState,
		&record.sourceType, &record.sourceName, &record.title, &record.author,
		&record.sourceRecordURL, &record.canonicalURL, &record.discussionURL,
		&record.bodyOrigin, &record.completeness, &record.language, &record.publishedAt,
		&record.capturedAt, &record.contentSHA256, &record.displayPrivateAllowed, &record.rightsEvaluatedAt,
		&record.artifactID, &record.artifactType, &record.transformerProfileSHA256, &record.mimeType,
		&record.artifactSHA256, &record.sizeBytes, &record.artifactLifecycleState, &record.artifactActive,
		&record.failureCode, &record.availableAt, &record.retentionUntil,
		&record.storeDerivedAllowed, &record.retainAllowed, &record.currentRetentionDays,
	)
	return record, err
}

func citationReadDTO(record citationRecord) ingestionapplication.CitationReadDTO {
	result := ingestionapplication.CitationReadDTO{
		DocumentID: record.documentID, DocumentVersionID: record.documentVersionID,
		SourceConnectionID: record.sourceConnectionID, DocumentState: record.documentState,
		DocumentLifecycleState: record.documentLifecycleState,
		ObservationState:       record.observationState,
		SourceType:             record.sourceType, SourceName: record.sourceName, Title: record.title,
		Author: citationOptionalString(record.author), SourceRecordURL: citationOptionalString(record.sourceRecordURL),
		CanonicalURL: citationOptionalString(record.canonicalURL), DiscussionURL: citationOptionalString(record.discussionURL),
		BodyOrigin: record.bodyOrigin, Completeness: record.completeness,
		Language: record.language, PublishedAt: citationOptionalTime(record.publishedAt), CapturedAt: record.capturedAt.UTC(),
		ContentSHA256: record.contentSHA256, DisplayPrivateAllowed: record.displayPrivateAllowed,
		RightsEvaluatedAt: record.rightsEvaluatedAt.UTC(),
	}
	if record.artifactID.Valid {
		result.Artifact = &ingestionapplication.CitationArtifactReadDTO{
			ArtifactType: record.artifactType.String, TransformerProfileSHA256: record.transformerProfileSHA256.String,
			MIMEType: record.mimeType.String, SHA256: record.artifactSHA256.String, SizeBytes: record.sizeBytes.Int64,
			LifecycleState: record.artifactLifecycleState.String, Active: record.artifactActive.Bool,
			FailureCode: citationOptionalString(record.failureCode), AvailableAt: citationOptionalTime(record.availableAt),
			RetentionUntil: citationTime(record.retentionUntil), StoreDerivedAllowed: record.storeDerivedAllowed,
			RetainAllowed: record.retainAllowed, CurrentRetentionDays: citationOptionalInt(record.currentRetentionDays),
		}
	}
	return result
}

func citationOptionalString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func citationOptionalTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func citationTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func citationOptionalInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}
