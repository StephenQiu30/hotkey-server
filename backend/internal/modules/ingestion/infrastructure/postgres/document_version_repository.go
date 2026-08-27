package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// DocumentVersionRepository persists only document identity and immutable
// version metadata. It never receives or stores document body bytes.
type DocumentVersionRepository struct{ runtime *database.Runtime }

var _ ingestionapplication.DocumentVersionRepository = (*DocumentVersionRepository)(nil)

func NewDocumentVersionRepository(runtime *database.Runtime) *DocumentVersionRepository {
	return &DocumentVersionRepository{runtime: runtime}
}

type documentVersionExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (repository *DocumentVersionRepository) ResolveDocument(ctx context.Context, identity ingestionapplication.DocumentIdentityDTO) (ingestionapplication.DocumentDTO, bool, error) {
	if !repository.available() {
		return ingestionapplication.DocumentDTO{}, false, sharedrepository.ErrUnavailable
	}
	if err := ingestionapplication.ValidateDocumentIdentityDTO(identity); err != nil {
		return ingestionapplication.DocumentDTO{}, false, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	var document ingestionapplication.DocumentDTO
	created := false
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, executor documentVersionExecutor) error {
		var lockedSourceID int64
		if err := executor.QueryRowContext(transactionCtx, `
SELECT id FROM source_connections WHERE id=$1 FOR UPDATE`, identity.SourceConnectionID).Scan(&lockedSourceID); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: source connection %d", sharedrepository.ErrNotFound, identity.SourceConnectionID)
		} else if err != nil {
			return databaserepository.MapError(err)
		}

		var err error
		document, err = scanDocumentRow(executor.QueryRowContext(transactionCtx, `
SELECT document.id,document.version,document.source_connection_id,document.document_key,document.external_work_id,
       document.current_document_version_id,document.document_state,document.created_at,document.updated_at
FROM document_identity_keys AS identity_key
JOIN documents AS document
  ON document.id=identity_key.document_id AND document.source_connection_id=identity_key.source_connection_id
WHERE identity_key.source_connection_id=$1 AND document.document_state='active'
  AND (
    ($2::text IS NOT NULL AND identity_key.identity_kind='external_id' AND identity_key.identity_value=$2)
    OR ($3::text<>'' AND identity_key.identity_kind='canonical_url' AND identity_key.identity_value=$3)
    OR ($4::text<>'' AND identity_key.identity_kind='content_sha256' AND identity_key.identity_value=$4)
  )
ORDER BY
  CASE
    WHEN identity_key.identity_kind='external_id' THEN 0
    WHEN identity_key.identity_kind='canonical_url' THEN 1
    ELSE 2
  END,
  document.id
LIMIT 1
FOR UPDATE OF document`, identity.SourceConnectionID, documentOptionalString(identity.ExternalWorkID), identity.CanonicalURL, identity.ContentSHA256))
		if err == nil {
			return bindDocumentIdentityKeys(transactionCtx, executor, document.ID, identity)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return databaserepository.MapError(err)
		}

		result, err := executor.ExecContext(transactionCtx, `
INSERT INTO documents (source_connection_id, document_key, external_work_id)
VALUES ($1,$2,$3)
ON CONFLICT (source_connection_id, document_key) DO NOTHING`,
			identity.SourceConnectionID, identity.DocumentKey, documentOptionalString(identity.ExternalWorkID))
		if err != nil {
			return databaserepository.MapError(err)
		}
		createdRows, err := result.RowsAffected()
		if err != nil {
			return databaserepository.MapError(err)
		}
		created = createdRows == 1
		document, err = scanDocumentRow(executor.QueryRowContext(transactionCtx, `
SELECT id,version,source_connection_id,document_key,external_work_id,
       current_document_version_id,document_state,created_at,updated_at
FROM documents WHERE source_connection_id=$1 AND document_key=$2`,
			identity.SourceConnectionID, identity.DocumentKey))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: resolved document disappeared", sharedrepository.ErrConflict)
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		return bindDocumentIdentityKeys(transactionCtx, executor, document.ID, identity)
	})
	if err != nil {
		return ingestionapplication.DocumentDTO{}, false, err
	}
	if document.State != ingestionapplication.DocumentStateActive || document.SourceConnectionID != identity.SourceConnectionID {
		return ingestionapplication.DocumentDTO{}, false, fmt.Errorf("%w: document identity is inactive or inconsistent", sharedrepository.ErrConflict)
	}
	return document, created, nil
}

func bindDocumentIdentityKeys(ctx context.Context, executor documentVersionExecutor, documentID int64, identity ingestionapplication.DocumentIdentityDTO) error {
	keys := make([][2]string, 0, 3)
	if identity.ExternalWorkID != nil {
		keys = append(keys, [2]string{"external_id", *identity.ExternalWorkID})
	}
	if identity.CanonicalURL != "" {
		keys = append(keys, [2]string{"canonical_url", identity.CanonicalURL})
	}
	if identity.ContentSHA256 != "" {
		keys = append(keys, [2]string{"content_sha256", identity.ContentSHA256})
	}
	for _, key := range keys {
		if _, err := executor.ExecContext(ctx, `
INSERT INTO document_identity_keys (source_connection_id,document_id,identity_kind,identity_value)
VALUES ($1,$2,$3,$4)
ON CONFLICT (source_connection_id,identity_kind,identity_value) DO NOTHING`,
			identity.SourceConnectionID, documentID, key[0], key[1]); err != nil {
			return databaserepository.MapError(err)
		}
		var boundDocumentID int64
		if err := executor.QueryRowContext(ctx, `
SELECT document_id FROM document_identity_keys
WHERE source_connection_id=$1 AND identity_kind=$2 AND identity_value=$3`,
			identity.SourceConnectionID, key[0], key[1]).Scan(&boundDocumentID); err != nil {
			return databaserepository.MapError(err)
		}
		if boundDocumentID != documentID {
			return fmt.Errorf("%w: document identity keys resolve to different documents", sharedrepository.ErrConflict)
		}
	}
	return nil
}

func (repository *DocumentVersionRepository) AppendDocumentVersion(ctx context.Context, draft ingestionapplication.DocumentVersionDraftDTO) (ingestionapplication.DocumentVersionDTO, bool, error) {
	if !repository.available() {
		return ingestionapplication.DocumentVersionDTO{}, false, sharedrepository.ErrUnavailable
	}
	if err := ingestionapplication.ValidateDocumentVersionDraftDTO(draft); err != nil {
		return ingestionapplication.DocumentVersionDTO{}, false, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	warningsJSON, err := json.Marshal(draft.QualityWarnings)
	if err != nil {
		return ingestionapplication.DocumentVersionDTO{}, false, fmt.Errorf("%w: encode quality warnings", sharedrepository.ErrInvalidInput)
	}
	var stored ingestionapplication.DocumentVersionDTO
	created := false
	err = repository.withTransaction(ctx, func(transactionCtx context.Context, executor documentVersionExecutor) error {
		var lockedDocumentID int64
		err := executor.QueryRowContext(transactionCtx, `
SELECT id FROM documents
WHERE id=$1 AND source_connection_id=$2 AND document_state='active'
FOR UPDATE`, draft.DocumentID, draft.SourceConnectionID).Scan(&lockedDocumentID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: active document %d", sharedrepository.ErrNotFound, draft.DocumentID)
		}
		if err != nil {
			return databaserepository.MapError(err)
		}

		existing, err := scanDocumentVersionRow(executor.QueryRowContext(transactionCtx, `
SELECT `+documentVersionColumns+`
FROM document_versions
WHERE document_id=$1 AND source_observation_id=$2
  AND content_sha256=$3 AND extractor_profile_version=$4`,
			draft.DocumentID, draft.SourceObservationID, draft.ContentSHA256, draft.ExtractorProfileVersion))
		switch {
		case err == nil:
			if !sameDocumentVersionFacts(existing, draft) {
				return fmt.Errorf("%w: source observation already has different immutable document facts", sharedrepository.ErrConflict)
			}
			stored = existing
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return databaserepository.MapError(err)
		}

		var revisionNo int64
		if err := executor.QueryRowContext(transactionCtx, `
SELECT COALESCE(MAX(revision_no),0)+1
FROM document_versions WHERE document_id=$1`, draft.DocumentID).Scan(&revisionNo); err != nil {
			return databaserepository.MapError(err)
		}
		stored, err = scanDocumentVersionRow(executor.QueryRowContext(transactionCtx, `
INSERT INTO document_versions (
  document_id,source_observation_id,revision_no,version_key,body_origin,completeness,
  word_count,language,truncated,quality_score,quality_warnings,content_sha256,
  extractor_version,extractor_profile_version,extractor_profile_sha256,lifecycle_state,captured_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
  ARRAY(SELECT jsonb_array_elements_text($11::jsonb)),$12,$13,$14,$15,$16,$17
) RETURNING `+documentVersionColumns,
			draft.DocumentID, draft.SourceObservationID, revisionNo, draft.VersionKey,
			string(draft.BodyOrigin), string(draft.Completeness), draft.WordCount, draft.Language,
			draft.Truncated, documentOptionalFloat(draft.QualityScore), string(warningsJSON), draft.ContentSHA256,
			draft.ExtractorVersion, draft.ExtractorProfileVersion, draft.ExtractorProfileSHA256,
			string(draft.LifecycleState), draft.CapturedAt.UTC()))
		if err != nil {
			return databaserepository.MapError(err)
		}
		created = true
		return nil
	})
	if err != nil {
		return ingestionapplication.DocumentVersionDTO{}, false, err
	}
	return stored, created, nil
}

func (repository *DocumentVersionRepository) GetDocumentVersion(ctx context.Context, id int64) (ingestionapplication.DocumentVersionDTO, error) {
	if !repository.available() {
		return ingestionapplication.DocumentVersionDTO{}, sharedrepository.ErrUnavailable
	}
	if id <= 0 {
		return ingestionapplication.DocumentVersionDTO{}, fmt.Errorf("%w: invalid document version id", sharedrepository.ErrInvalidInput)
	}
	stored, err := scanDocumentVersionRow(repository.executor(ctx).QueryRowContext(ctx, `
SELECT `+documentVersionColumns+` FROM document_versions WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.DocumentVersionDTO{}, fmt.Errorf("%w: document version %d", sharedrepository.ErrNotFound, id)
	}
	if err != nil {
		return ingestionapplication.DocumentVersionDTO{}, databaserepository.MapError(err)
	}
	return stored, nil
}

func (repository *DocumentVersionRepository) CompareAndSwapDocumentVersionLifecycle(ctx context.Context, transition ingestionapplication.TransitionDocumentVersionCommand) (ingestionapplication.DocumentVersionDTO, error) {
	if !repository.available() {
		return ingestionapplication.DocumentVersionDTO{}, sharedrepository.ErrUnavailable
	}
	if err := ingestionapplication.ValidateTransitionDocumentVersionCommand(transition); err != nil {
		return ingestionapplication.DocumentVersionDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	displayDecisionID := documentOptionalInt64(transition.DisplayPrivateRightsDecisionID)
	var updated ingestionapplication.DocumentVersionDTO
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, executor documentVersionExecutor) error {
		var err error
		updated, err = scanDocumentVersionRow(executor.QueryRowContext(transactionCtx, `
UPDATE document_versions AS target
SET lifecycle_state=$3,
    display_private_rights_decision_id=CASE
      WHEN $3::text='readable' THEN $4
      ELSE target.display_private_rights_decision_id
    END,
    version=target.version+1, updated_at=now()
WHERE target.id=$1 AND target.version=$2
  AND EXISTS (
    SELECT 1 FROM documents
    WHERE id=target.document_id AND document_state='active'
  )
  AND (
    $3::text NOT IN ('derived_available','readable')
    OR EXISTS (
      SELECT 1
      FROM derived_artifacts AS artifact
      JOIN documents AS artifact_document ON artifact_document.id=target.document_id
      WHERE artifact.document_version_id=target.id
        AND artifact.lifecycle_state='derived_available' AND artifact.active
        AND artifact.source_connection_id=artifact_document.source_connection_id
        AND artifact.available_at IS NOT NULL
        AND artifact.failure_code IS NULL
        AND artifact.retention_until>now()
        AND current_rights_action_allowed(
          artifact.store_derived_rights_decision_id,
          artifact_document.source_connection_id,
          'document_version', target.id::text, target.content_sha256,
          'store_derived', now()
        )
        AND current_rights_action_allowed(
          artifact.retain_rights_decision_id,
          artifact_document.source_connection_id,
          'document_version', target.id::text, target.content_sha256,
          'retain', now()
        )
    )
  )
  AND (
    $3::text<>'readable'
    OR EXISTS (
      SELECT 1
      FROM source_rights_decisions AS display_decision
      JOIN documents AS display_document ON display_document.id=target.document_id
      WHERE display_decision.id=$4
        AND current_rights_action_allowed(
          display_decision.id,
          display_document.source_connection_id,
          'document_version', target.id::text, target.content_sha256,
          'display_private', now()
        )
    )
  )
RETURNING `+documentVersionColumns,
			transition.DocumentVersionID, transition.ExpectedVersion, string(transition.To), displayDecisionID))
		if errors.Is(err, sql.ErrNoRows) {
			var exists bool
			if existsErr := executor.QueryRowContext(transactionCtx, `SELECT EXISTS(SELECT 1 FROM document_versions WHERE id=$1)`, transition.DocumentVersionID).Scan(&exists); existsErr != nil {
				return databaserepository.MapError(existsErr)
			}
			if !exists {
				return fmt.Errorf("%w: document version %d", sharedrepository.ErrNotFound, transition.DocumentVersionID)
			}
			return fmt.Errorf("%w: document version lifecycle precondition", sharedrepository.ErrConflict)
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		if transition.To == ingestionapplication.DocumentReadable {
			_, err = executor.ExecContext(transactionCtx, `
UPDATE documents AS document
SET current_document_version_id=$1, version=document.version+1, updated_at=now()
WHERE document.id=$2 AND document.document_state='active'
  AND EXISTS (
    SELECT 1 FROM document_versions AS readable_version
    WHERE readable_version.id=$1
      AND readable_version.document_id=document.id
      AND readable_version.lifecycle_state='readable'
      AND current_rights_action_allowed(
        readable_version.display_private_rights_decision_id,
        document.source_connection_id,
        'document_version', readable_version.id::text, readable_version.content_sha256,
        'display_private', now()
      )
      AND EXISTS (
        SELECT 1 FROM derived_artifacts AS artifact
        WHERE artifact.document_version_id=readable_version.id
          AND artifact.source_connection_id=document.source_connection_id
          AND artifact.lifecycle_state='derived_available' AND artifact.active
          AND artifact.available_at IS NOT NULL AND artifact.failure_code IS NULL
          AND artifact.retention_until>now()
          AND current_rights_action_allowed(
            artifact.store_derived_rights_decision_id,
            document.source_connection_id,
            'document_version', readable_version.id::text, readable_version.content_sha256,
            'store_derived', now()
          )
          AND current_rights_action_allowed(
            artifact.retain_rights_decision_id,
            document.source_connection_id,
            'document_version', readable_version.id::text, readable_version.content_sha256,
            'retain', now()
          )
      )
  )
  AND (
    document.current_document_version_id IS NULL
    OR EXISTS (
      SELECT 1 FROM document_versions AS current_version
      WHERE current_version.id=document.current_document_version_id
        AND current_version.revision_no <= $3
    )
  )`, updated.ID, updated.DocumentID, updated.RevisionNo)
		} else {
			_, err = executor.ExecContext(transactionCtx, `
UPDATE documents AS document
SET current_document_version_id=(
      SELECT candidate.id FROM document_versions AS candidate
      WHERE candidate.document_id=document.id AND candidate.lifecycle_state='readable'
        AND current_rights_action_allowed(
          candidate.display_private_rights_decision_id,
          document.source_connection_id,
          'document_version', candidate.id::text, candidate.content_sha256,
          'display_private', now()
        )
        AND EXISTS (
          SELECT 1 FROM derived_artifacts AS artifact
          WHERE artifact.document_version_id=candidate.id
            AND artifact.source_connection_id=document.source_connection_id
            AND artifact.lifecycle_state='derived_available' AND artifact.active
            AND artifact.available_at IS NOT NULL AND artifact.failure_code IS NULL
            AND artifact.retention_until>now()
            AND current_rights_action_allowed(
              artifact.store_derived_rights_decision_id,
              document.source_connection_id,
              'document_version', candidate.id::text, candidate.content_sha256,
              'store_derived', now()
            )
            AND current_rights_action_allowed(
              artifact.retain_rights_decision_id,
              document.source_connection_id,
              'document_version', candidate.id::text, candidate.content_sha256,
              'retain', now()
            )
        )
      ORDER BY candidate.revision_no DESC,candidate.id DESC LIMIT 1
    ),
    version=document.version+1, updated_at=now()
WHERE document.id=$1 AND document.current_document_version_id=$2`, updated.DocumentID, updated.ID)
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		return nil
	})
	if err != nil {
		return ingestionapplication.DocumentVersionDTO{}, err
	}
	return updated, nil
}

const documentVersionColumns = `
id,version,document_id,source_observation_id,revision_no,version_key,
body_origin,completeness,word_count,language,truncated,quality_score,
array_to_json(quality_warnings)::text,content_sha256,extractor_version,
extractor_profile_version,extractor_profile_sha256,display_private_rights_decision_id,lifecycle_state,
captured_at,created_at,updated_at`

type documentRow struct {
	id, version, sourceConnectionID int64
	documentKey                     string
	externalWorkID                  sql.NullString
	currentVersionID                sql.NullInt64
	state                           string
	createdAt, updatedAt            time.Time
}

func scanDocumentRow(scanner interface{ Scan(...any) error }) (ingestionapplication.DocumentDTO, error) {
	var row documentRow
	if err := scanner.Scan(
		&row.id, &row.version, &row.sourceConnectionID, &row.documentKey,
		&row.externalWorkID, &row.currentVersionID, &row.state, &row.createdAt, &row.updatedAt,
	); err != nil {
		return ingestionapplication.DocumentDTO{}, err
	}
	return documentDTOFromRow(row)
}

func documentDTOFromRow(row documentRow) (ingestionapplication.DocumentDTO, error) {
	document := ingestionapplication.DocumentDTO{
		ID: row.id, Version: row.version, SourceConnectionID: row.sourceConnectionID,
		DocumentKey: row.documentKey, ExternalWorkID: documentOptionalStringValue(row.externalWorkID),
		CurrentVersionID: documentOptionalInt64Value(row.currentVersionID),
		State:            row.state,
		CreatedAt:        row.createdAt.UTC(), UpdatedAt: row.updatedAt.UTC(),
	}
	if err := ingestionapplication.ValidateDocumentDTO(document); err != nil {
		return ingestionapplication.DocumentDTO{}, fmt.Errorf("invalid document row: %w", err)
	}
	return document, nil
}

type documentVersionRow struct {
	id, version, documentID, sourceObservationID, revisionNo int64
	versionKey, bodyOrigin, completeness, language           string
	wordCount                                                int
	truncated                                                bool
	qualityScore                                             sql.NullFloat64
	warningsJSON                                             string
	contentSHA256, extractorVersion                          string
	extractorProfileVersion, extractorProfileSHA256          string
	displayPrivateDecisionID                                 sql.NullInt64
	lifecycleState                                           string
	capturedAt, createdAt, updatedAt                         time.Time
}

func scanDocumentVersionRow(scanner interface{ Scan(...any) error }) (ingestionapplication.DocumentVersionDTO, error) {
	var row documentVersionRow
	if err := scanner.Scan(
		&row.id, &row.version, &row.documentID, &row.sourceObservationID,
		&row.revisionNo, &row.versionKey, &row.bodyOrigin, &row.completeness,
		&row.wordCount, &row.language, &row.truncated, &row.qualityScore,
		&row.warningsJSON, &row.contentSHA256, &row.extractorVersion,
		&row.extractorProfileVersion, &row.extractorProfileSHA256, &row.displayPrivateDecisionID, &row.lifecycleState,
		&row.capturedAt, &row.createdAt, &row.updatedAt,
	); err != nil {
		return ingestionapplication.DocumentVersionDTO{}, err
	}
	return documentVersionDTOFromRow(row)
}

func documentVersionDTOFromRow(row documentVersionRow) (ingestionapplication.DocumentVersionDTO, error) {
	warnings := make([]string, 0)
	if err := json.Unmarshal([]byte(row.warningsJSON), &warnings); err != nil {
		return ingestionapplication.DocumentVersionDTO{}, fmt.Errorf("decode document version row warnings: %w", err)
	}
	version := ingestionapplication.DocumentVersionDTO{
		ID: row.id, Version: row.version, DocumentID: row.documentID,
		SourceObservationID: row.sourceObservationID, RevisionNo: row.revisionNo,
		VersionKey: row.versionKey, BodyOrigin: row.bodyOrigin,
		Completeness: row.completeness, WordCount: row.wordCount,
		Language: row.language, Truncated: row.truncated,
		QualityScore: documentOptionalFloatValue(row.qualityScore), QualityWarnings: warnings,
		ContentSHA256: row.contentSHA256, ExtractorVersion: row.extractorVersion,
		ExtractorProfileVersion: row.extractorProfileVersion, ExtractorProfileSHA256: row.extractorProfileSHA256,
		DisplayPrivateRightsDecisionID: documentOptionalInt64Value(row.displayPrivateDecisionID),
		LifecycleState:                 row.lifecycleState,
		CapturedAt:                     row.capturedAt.UTC(), CreatedAt: row.createdAt.UTC(), UpdatedAt: row.updatedAt.UTC(),
	}
	if err := ingestionapplication.ValidateDocumentVersionDTO(version); err != nil {
		return ingestionapplication.DocumentVersionDTO{}, fmt.Errorf("invalid document version row: %w", err)
	}
	return version, nil
}

func (repository *DocumentVersionRepository) executor(ctx context.Context) documentVersionExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

func (repository *DocumentVersionRepository) withTransaction(ctx context.Context, operation func(context.Context, documentVersionExecutor) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return operation(ctx, transaction.SQL)
	}
	return repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		return operation(transactionCtx, transaction.SQL)
	})
}

func (repository *DocumentVersionRepository) available() bool {
	return repository != nil && repository.runtime != nil && repository.runtime.SQL != nil
}

func sameDocumentVersionFacts(version ingestionapplication.DocumentVersionDTO, draft ingestionapplication.DocumentVersionDraftDTO) bool {
	return version.DocumentID == draft.DocumentID && version.SourceObservationID == draft.SourceObservationID &&
		version.VersionKey == draft.VersionKey && version.BodyOrigin == draft.BodyOrigin && version.Completeness == draft.Completeness &&
		version.WordCount == draft.WordCount && version.Language == draft.Language && version.Truncated == draft.Truncated &&
		documentQualityScoresEqual(version.QualityScore, draft.QualityScore) && slices.Equal(version.QualityWarnings, draft.QualityWarnings) &&
		version.ContentSHA256 == draft.ContentSHA256 && version.ExtractorVersion == draft.ExtractorVersion &&
		version.ExtractorProfileVersion == draft.ExtractorProfileVersion && version.ExtractorProfileSHA256 == draft.ExtractorProfileSHA256 &&
		version.CapturedAt.Equal(draft.CapturedAt.UTC())
}

func documentQualityScoresEqual(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func optionalDocumentStringsEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func documentOptionalFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func documentOptionalString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func documentOptionalInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func documentOptionalFloatValue(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}

func documentOptionalInt64Value(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func documentOptionalStringValue(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
