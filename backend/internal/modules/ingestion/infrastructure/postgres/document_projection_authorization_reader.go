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

// DocumentProjectionAuthorizationReader resolves the exact single-action
// decisions needed after a DocumentVersion exists. Selection and conflict
// handling happen in one PostgreSQL snapshot; DerivedArtifact reservation
// rechecks the returned IDs immediately before committing external effects.
type DocumentProjectionAuthorizationReader struct {
	runtime *database.Runtime
}

var _ ingestionapplication.DocumentProjectionAuthorizationReader = (*DocumentProjectionAuthorizationReader)(nil)

func NewDocumentProjectionAuthorizationReader(runtime *database.Runtime) *DocumentProjectionAuthorizationReader {
	return &DocumentProjectionAuthorizationReader{runtime: runtime}
}

type documentProjectionAuthorizationExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// documentProjectionAuthorizationRecord is private persistence state. The
// mapper below is the only place that creates the cross-module Application
// DTO, preventing policy and nullable SQL facts from leaking through it.
type documentProjectionAuthorizationRecord struct {
	sourceConnectionID int64
	documentVersionID  int64
	contentSHA256      string
	decisionAt         time.Time

	storeDerivedDecisionID sql.NullInt64
	retainDecisionID       sql.NullInt64
	displayDecisionID      sql.NullInt64
	selectedRetentionDays  sql.NullInt64
	currentRetentionDays   sql.NullInt64
	storeDerivedAllowed    bool
	retainAllowed          bool
	displayAllowed         bool
}

func (reader *DocumentProjectionAuthorizationReader) ReadDocumentProjectionAuthorization(
	ctx context.Context,
	query ingestionapplication.DocumentProjectionAuthorizationQuery,
) (ingestionapplication.DocumentProjectionAuthorizationDTO, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		return ingestionapplication.DocumentProjectionAuthorizationDTO{}, sharedrepository.ErrUnavailable
	}
	if err := ingestionapplication.ValidateDocumentProjectionAuthorizationQuery(query); err != nil {
		return ingestionapplication.DocumentProjectionAuthorizationDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}

	record, err := scanDocumentProjectionAuthorizationRecord(reader.executor(ctx).QueryRowContext(ctx, `
WITH target AS (
  SELECT
    document.source_connection_id,
    version.id AS document_version_id,
    btrim(version.content_sha256) AS content_sha256
  FROM document_versions AS version
  JOIN documents AS document ON document.id=version.document_id
  WHERE version.id=$2
    AND document.source_connection_id=$1
    AND version.content_sha256=$3
), terminal AS (
  SELECT decision.*
  FROM target
  JOIN source_rights_decisions AS decision
    ON decision.source_connection_id=target.source_connection_id
   AND decision.subject_type='document_version'
   AND decision.subject_key=target.document_version_id::text
   AND decision.input_digest=target.content_sha256
   AND decision.action IN ('store_derived','retain','display_private')
  WHERE decision.effective_from<=$4
    AND (decision.expires_at IS NULL OR $4<decision.expires_at)
    AND NOT EXISTS (
      SELECT 1
      FROM source_rights_decisions AS superseding
      WHERE superseding.supersedes_decision_id=decision.id
        AND superseding.effective_from<=$4
    )
), highest_priority AS (
  SELECT action,max(priority_rank) AS priority_rank
  FROM terminal
  GROUP BY action
), highest_terminal AS (
  SELECT terminal.*
  FROM terminal
  JOIN highest_priority AS highest
    ON highest.action=terminal.action
   AND highest.priority_rank=terminal.priority_rank
), allowed_action AS (
  SELECT action
  FROM highest_terminal
  GROUP BY action
  HAVING bool_and(decision='allow')
), selected AS (
  SELECT DISTINCT ON (terminal.action)
    terminal.id,terminal.action,terminal.retention_days
  FROM highest_terminal AS terminal
  JOIN allowed_action AS allowed ON allowed.action=terminal.action
  WHERE terminal.decision='allow'
  ORDER BY terminal.action,
           CASE WHEN terminal.action='retain' THEN terminal.retention_days END ASC NULLS LAST,
           terminal.effective_from DESC,
           terminal.id DESC
)
SELECT
  target.source_connection_id,
  target.document_version_id,
  target.content_sha256,
  $4::timestamptz AS decision_at,
  store_decision.id,
  retain_decision.id,
  display_decision.id,
  retain_decision.retention_days,
  current_rights_retention_days(
    target.source_connection_id,'document_version',target.document_version_id::text,
    target.content_sha256,$4
  ) AS current_retention_days,
  current_rights_action_allowed(
    store_decision.id,target.source_connection_id,'document_version',target.document_version_id::text,
    target.content_sha256,'store_derived',$4
  ) AS store_derived_allowed,
  current_rights_action_allowed(
    retain_decision.id,target.source_connection_id,'document_version',target.document_version_id::text,
    target.content_sha256,'retain',$4
  ) AS retain_allowed,
  CASE WHEN display_decision.id IS NULL THEN false ELSE current_rights_action_allowed(
    display_decision.id,target.source_connection_id,'document_version',target.document_version_id::text,
    target.content_sha256,'display_private',$4
  ) END AS display_allowed
FROM target
LEFT JOIN selected AS store_decision ON store_decision.action='store_derived'
LEFT JOIN selected AS retain_decision ON retain_decision.action='retain'
LEFT JOIN selected AS display_decision ON display_decision.action='display_private'`,
		query.SourceConnectionID,
		query.DocumentVersionID,
		query.ContentSHA256,
		query.DecisionAt.UTC(),
	))
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.DocumentProjectionAuthorizationDTO{}, fmt.Errorf(
			"%w: document version projection identity was not found",
			sharedrepository.ErrNotFound,
		)
	}
	if err != nil {
		return ingestionapplication.DocumentProjectionAuthorizationDTO{}, databaserepository.MapError(err)
	}
	return documentProjectionAuthorizationDTOFromRecord(record, query)
}

func (reader *DocumentProjectionAuthorizationReader) executor(ctx context.Context) documentProjectionAuthorizationExecutor {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL
	}
	return reader.runtime.SQL
}

func scanDocumentProjectionAuthorizationRecord(row *sql.Row) (documentProjectionAuthorizationRecord, error) {
	var record documentProjectionAuthorizationRecord
	err := row.Scan(
		&record.sourceConnectionID,
		&record.documentVersionID,
		&record.contentSHA256,
		&record.decisionAt,
		&record.storeDerivedDecisionID,
		&record.retainDecisionID,
		&record.displayDecisionID,
		&record.selectedRetentionDays,
		&record.currentRetentionDays,
		&record.storeDerivedAllowed,
		&record.retainAllowed,
		&record.displayAllowed,
	)
	return record, err
}

func documentProjectionAuthorizationDTOFromRecord(
	record documentProjectionAuthorizationRecord,
	query ingestionapplication.DocumentProjectionAuthorizationQuery,
) (ingestionapplication.DocumentProjectionAuthorizationDTO, error) {
	if !record.storeDerivedDecisionID.Valid || !record.retainDecisionID.Valid ||
		!record.storeDerivedAllowed || !record.retainAllowed ||
		!record.selectedRetentionDays.Valid || !record.currentRetentionDays.Valid ||
		record.selectedRetentionDays.Int64 != record.currentRetentionDays.Int64 {
		return ingestionapplication.DocumentProjectionAuthorizationDTO{}, fmt.Errorf(
			"%w: exact derived storage rights are not currently allowed",
			sharedrepository.ErrConflict,
		)
	}
	if record.displayDecisionID.Valid && !record.displayAllowed {
		return ingestionapplication.DocumentProjectionAuthorizationDTO{}, fmt.Errorf(
			"%w: selected display decision is not current",
			sharedrepository.ErrConstraint,
		)
	}
	var displayDecisionID *int64
	if record.displayDecisionID.Valid {
		value := record.displayDecisionID.Int64
		displayDecisionID = &value
	}
	result := ingestionapplication.DocumentProjectionAuthorizationDTO{
		SourceConnectionID:             record.sourceConnectionID,
		DocumentVersionID:              record.documentVersionID,
		ContentSHA256:                  record.contentSHA256,
		DecisionAt:                     record.decisionAt.UTC(),
		StoreDerivedRightsDecisionID:   record.storeDerivedDecisionID.Int64,
		RetainRightsDecisionID:         record.retainDecisionID.Int64,
		DisplayPrivateRightsDecisionID: displayDecisionID,
	}
	if err := ingestionapplication.ValidateDocumentProjectionAuthorizationDTO(result, query); err != nil {
		return ingestionapplication.DocumentProjectionAuthorizationDTO{}, fmt.Errorf(
			"%w: persisted document projection authorization is invalid: %v",
			sharedrepository.ErrConstraint,
			err,
		)
	}
	return result, nil
}
