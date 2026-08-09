package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"regexp"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// DerivedArtifactRepository persists immutable projection receipts and their
// lifecycle metadata. Projection bytes never cross this boundary.
type DerivedArtifactRepository struct{ runtime *database.Runtime }

var _ ingestionapplication.DerivedArtifactRepository = (*DerivedArtifactRepository)(nil)

func NewDerivedArtifactRepository(runtime *database.Runtime) *DerivedArtifactRepository {
	return &DerivedArtifactRepository{runtime: runtime}
}

func (repository *DerivedArtifactRepository) Reserve(ctx context.Context, command ingestionapplication.ReserveDerivedArtifactCommand) (ingestionapplication.ReserveDerivedArtifactResult, error) {
	if !repository.available() {
		return ingestionapplication.ReserveDerivedArtifactResult{}, sharedrepository.ErrUnavailable
	}
	if err := ingestionapplication.ValidateReserveDerivedArtifactCommand(command); err != nil {
		return ingestionapplication.ReserveDerivedArtifactResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}

	var result ingestionapplication.ReserveDerivedArtifactResult
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, executor documentVersionExecutor) error {
		locked, err := lockProjectionDocumentVersion(transactionCtx, executor, command.DocumentVersionID)
		if err != nil {
			return err
		}
		if command.ArtifactType == ingestionapplication.DerivedArtifactPlaintext && command.SHA256 != locked.version.ContentSHA256 {
			return fmt.Errorf("%w: plaintext projection digest does not match document version", sharedrepository.ErrConflict)
		}

		row, err := scanDerivedArtifactRow(executor.QueryRowContext(transactionCtx, `
SELECT `+derivedArtifactColumns+`
FROM derived_artifacts
WHERE document_version_id=$1 AND artifact_type=$2 AND transformer_profile_sha256=$3
FOR UPDATE`, command.DocumentVersionID, string(command.ArtifactType), command.TransformerProfileSHA256))
		switch {
		case err == nil:
			result, err = reserveDerivedArtifactResult(row, locked)
			if err != nil {
				return err
			}
			existingResult := result
			if row.storeDerivedRightsDecisionID != command.StoreDerivedRightsDecisionID || row.retainRightsDecisionID != command.RetainRightsDecisionID {
				result = ingestionapplication.ReserveDerivedArtifactResult{}
				return fmt.Errorf("%w: immutable artifact rights receipts differ", sharedrepository.ErrConflict)
			}
			if !sameDerivedArtifactProjectionFacts(row, locked.documentID, command) {
				return fmt.Errorf("%w: %w", sharedrepository.ErrConflict, ingestionapplication.ErrDerivedArtifactContentConflict)
			}
			result = ingestionapplication.ReserveDerivedArtifactResult{}
			if row.lifecycleState != ingestionapplication.DerivedArtifactPending &&
				row.lifecycleState != ingestionapplication.DerivedArtifactFailed &&
				row.lifecycleState != ingestionapplication.DerivedArtifactAvailable {
				result = ingestionapplication.ReserveDerivedArtifactResult{}
				return fmt.Errorf("%w: immutable artifact is not publishable", sharedrepository.ErrConflict)
			}
			currentRights, rightsErr := exactDerivedArtifactRightsCurrent(transactionCtx, executor, row, locked.version)
			if rightsErr != nil {
				result = ingestionapplication.ReserveDerivedArtifactResult{}
				return rightsErr
			}
			if !currentRights {
				result = ingestionapplication.ReserveDerivedArtifactResult{}
				return fmt.Errorf("%w: derived artifact rights are no longer current", sharedrepository.ErrConflict)
			}
			if row.lifecycleState == ingestionapplication.DerivedArtifactAvailable {
				result = existingResult
				return nil
			}
			if locked.version.Version != command.ExpectedDocumentVersion {
				result = ingestionapplication.ReserveDerivedArtifactResult{}
				return fmt.Errorf("%w: document version changed before artifact reserve", sharedrepository.ErrConflict)
			}
			if row.lifecycleState == ingestionapplication.DerivedArtifactFailed {
				row, err = scanDerivedArtifactRow(executor.QueryRowContext(transactionCtx, `
UPDATE derived_artifacts
SET lifecycle_state='derive_pending',failure_code=NULL,active=false,available_at=NULL,updated_at=now()
WHERE id=$1 AND lifecycle_state='derive_failed'
RETURNING `+derivedArtifactColumns, row.id))
				if err != nil {
					return mapDerivedArtifactDatabaseError(err)
				}
				result, err = reserveDerivedArtifactResult(row, locked)
				return err
			}
			result = existingResult
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return mapDerivedArtifactDatabaseError(err)
		}

		if locked.version.Version != command.ExpectedDocumentVersion {
			return fmt.Errorf("%w: document version changed before artifact reserve", sharedrepository.ErrConflict)
		}
		if !documentVersionAllowsProjection(locked.version.LifecycleState) {
			return fmt.Errorf("%w: document version lifecycle is not projectable", sharedrepository.ErrConflict)
		}
		retentionUntil, currentRights, err := currentReservationRights(transactionCtx, executor,
			command.StoreDerivedRightsDecisionID, command.RetainRightsDecisionID, locked)
		if err != nil {
			return err
		}
		if !currentRights {
			return fmt.Errorf("%w: derived artifact requires exact current rights allows", sharedrepository.ErrConflict)
		}
		vaultRelativePath := derivedArtifactRelativePath(
			locked.documentID, command.DocumentVersionID, command.ArtifactType, command.TransformerProfileSHA256,
		)
		row, err = scanDerivedArtifactRow(executor.QueryRowContext(transactionCtx, `
INSERT INTO derived_artifacts (
  source_connection_id,document_version_id,store_derived_rights_decision_id,
  retain_rights_decision_id,artifact_type,transformer_profile_sha256,
  vault_relative_path,mime_type,sha256,size_bytes,retention_until
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING `+derivedArtifactColumns,
			locked.sourceConnectionID, command.DocumentVersionID, command.StoreDerivedRightsDecisionID,
			command.RetainRightsDecisionID, string(command.ArtifactType), command.TransformerProfileSHA256,
			vaultRelativePath, command.MIMEType, command.SHA256, command.SizeBytes, retentionUntil))
		if err != nil {
			return mapDerivedArtifactDatabaseError(err)
		}
		result, err = reserveDerivedArtifactResult(row, locked)
		return err
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func (repository *DerivedArtifactRepository) Commit(ctx context.Context, command ingestionapplication.CommitDerivedArtifactCommand) (ingestionapplication.CommitDerivedArtifactResult, error) {
	if !repository.available() {
		return ingestionapplication.CommitDerivedArtifactResult{}, sharedrepository.ErrUnavailable
	}
	if command.ArtifactID <= 0 {
		return ingestionapplication.CommitDerivedArtifactResult{}, fmt.Errorf("%w: invalid derived artifact id", sharedrepository.ErrInvalidInput)
	}
	if err := ingestionapplication.ValidateProjectionReceiptDTO(command.Receipt); err != nil {
		return ingestionapplication.CommitDerivedArtifactResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}

	var result ingestionapplication.CommitDerivedArtifactResult
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, executor documentVersionExecutor) error {
		var documentVersionID int64
		if err := executor.QueryRowContext(transactionCtx, `
SELECT document_version_id FROM derived_artifacts WHERE id=$1`, command.ArtifactID).Scan(&documentVersionID); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: derived artifact %d", sharedrepository.ErrNotFound, command.ArtifactID)
		} else if err != nil {
			return mapDerivedArtifactDatabaseError(err)
		}
		locked, err := lockProjectionDocumentVersion(transactionCtx, executor, documentVersionID)
		if err != nil {
			return err
		}
		row, err := scanDerivedArtifactRow(executor.QueryRowContext(transactionCtx, `
SELECT `+derivedArtifactColumns+` FROM derived_artifacts WHERE id=$1 FOR UPDATE`, command.ArtifactID))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: derived artifact %d", sharedrepository.ErrNotFound, command.ArtifactID)
		}
		if err != nil {
			return mapDerivedArtifactDatabaseError(err)
		}
		result, err = commitDerivedArtifactResult(row, locked)
		if err != nil {
			return err
		}
		existingResult := result
		if !derivedArtifactReceiptMatches(row, locked.documentID, command.Receipt) {
			return fmt.Errorf("%w: %w", sharedrepository.ErrConflict, ingestionapplication.ErrDerivedArtifactContentConflict)
		}
		result = ingestionapplication.CommitDerivedArtifactResult{}
		currentRights, err := exactDerivedArtifactRightsCurrent(transactionCtx, executor, row, locked.version)
		if err != nil {
			return err
		}
		if !currentRights {
			result = ingestionapplication.CommitDerivedArtifactResult{}
			return fmt.Errorf("%w: derived artifact rights changed before commit", sharedrepository.ErrConflict)
		}
		displayRightsCurrent, err := readableDocumentDisplayRightsCurrent(transactionCtx, executor, locked)
		if err != nil {
			return err
		}
		if !displayRightsCurrent {
			return fmt.Errorf("%w: readable document display rights changed before commit", sharedrepository.ErrConflict)
		}
		if row.lifecycleState == ingestionapplication.DerivedArtifactAvailable {
			result = existingResult
			return nil
		}
		if row.lifecycleState != ingestionapplication.DerivedArtifactPending {
			result = ingestionapplication.CommitDerivedArtifactResult{}
			return fmt.Errorf("%w: derived artifact is not pending", sharedrepository.ErrConflict)
		}

		if _, err := executor.ExecContext(transactionCtx, `
UPDATE derived_artifacts AS old
SET lifecycle_state='retention_blocked',active=false,available_at=NULL,failure_code=NULL,updated_at=now()
FROM document_versions AS version
WHERE old.document_version_id=$1 AND old.artifact_type=$2 AND old.active AND old.id<>$3
  AND version.id=old.document_version_id
  AND NOT (
    current_rights_action_is_allowed(
      old.source_connection_id,'document_version',version.id::text,version.content_sha256,'store_derived',now()
    )
    AND current_rights_retention_days(
      old.source_connection_id,'document_version',version.id::text,version.content_sha256,now()
    ) IS NOT NULL
    AND old.retention_until<=version.captured_at+current_rights_retention_days(
      old.source_connection_id,'document_version',version.id::text,version.content_sha256,now()
    )*interval '24 hours'
    AND old.retention_until>now()
  )`, row.documentVersionID, row.artifactType, row.id); err != nil {
			return mapDerivedArtifactDatabaseError(err)
		}
		if _, err := executor.ExecContext(transactionCtx, `
UPDATE derived_artifacts
SET active=false,updated_at=now()
WHERE document_version_id=$1 AND artifact_type=$2 AND active AND id<>$3`,
			row.documentVersionID, row.artifactType, row.id); err != nil {
			return mapDerivedArtifactDatabaseError(err)
		}
		row, err = scanDerivedArtifactRow(executor.QueryRowContext(transactionCtx, `
UPDATE derived_artifacts
SET lifecycle_state='derived_available',active=true,available_at=now(),failure_code=NULL,updated_at=now()
WHERE id=$1 AND lifecycle_state='derive_pending'
RETURNING `+derivedArtifactColumns, row.id))
		if err != nil {
			return mapDerivedArtifactDatabaseError(err)
		}
		result, err = commitDerivedArtifactResult(row, locked)
		return err
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func (repository *DerivedArtifactRepository) MarkFailed(ctx context.Context, command ingestionapplication.MarkDerivedArtifactFailedCommand) (ingestionapplication.DerivedArtifactDTO, error) {
	if !repository.available() {
		return ingestionapplication.DerivedArtifactDTO{}, sharedrepository.ErrUnavailable
	}
	if command.ArtifactID <= 0 || !derivedArtifactFailureCodePattern.MatchString(command.FailureCode) {
		return ingestionapplication.DerivedArtifactDTO{}, fmt.Errorf("%w: invalid derived artifact failure command", sharedrepository.ErrInvalidInput)
	}
	var artifact ingestionapplication.DerivedArtifactDTO
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, executor documentVersionExecutor) error {
		row, err := scanDerivedArtifactRow(executor.QueryRowContext(transactionCtx, `
SELECT `+derivedArtifactColumns+` FROM derived_artifacts WHERE id=$1 FOR UPDATE`, command.ArtifactID))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: derived artifact %d", sharedrepository.ErrNotFound, command.ArtifactID)
		}
		if err != nil {
			return mapDerivedArtifactDatabaseError(err)
		}
		if row.lifecycleState == ingestionapplication.DerivedArtifactFailed && row.failureCode.String == command.FailureCode {
			artifact, err = derivedArtifactDTOFromRow(row)
			return err
		}
		if row.lifecycleState != ingestionapplication.DerivedArtifactPending {
			return fmt.Errorf("%w: derived artifact is not pending", sharedrepository.ErrConflict)
		}
		row, err = scanDerivedArtifactRow(executor.QueryRowContext(transactionCtx, `
UPDATE derived_artifacts
SET lifecycle_state='derive_failed',active=false,available_at=NULL,failure_code=$2,updated_at=now()
WHERE id=$1 AND lifecycle_state='derive_pending'
RETURNING `+derivedArtifactColumns, command.ArtifactID, command.FailureCode))
		if err != nil {
			return mapDerivedArtifactDatabaseError(err)
		}
		artifact, err = derivedArtifactDTOFromRow(row)
		return err
	})
	return artifact, err
}

func (repository *DerivedArtifactRepository) Quarantine(ctx context.Context, command ingestionapplication.QuarantineDerivedArtifactCommand) (ingestionapplication.DerivedArtifactDTO, error) {
	if !repository.available() {
		return ingestionapplication.DerivedArtifactDTO{}, sharedrepository.ErrUnavailable
	}
	if command.ArtifactID <= 0 {
		return ingestionapplication.DerivedArtifactDTO{}, fmt.Errorf("%w: invalid derived artifact quarantine command", sharedrepository.ErrInvalidInput)
	}
	var artifact ingestionapplication.DerivedArtifactDTO
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, executor documentVersionExecutor) error {
		row, err := scanDerivedArtifactRow(executor.QueryRowContext(transactionCtx, `
SELECT `+derivedArtifactColumns+` FROM derived_artifacts WHERE id=$1 FOR UPDATE`, command.ArtifactID))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: derived artifact %d", sharedrepository.ErrNotFound, command.ArtifactID)
		}
		if err != nil {
			return mapDerivedArtifactDatabaseError(err)
		}
		if row.lifecycleState == ingestionapplication.DerivedArtifactQuarantined {
			artifact, err = derivedArtifactDTOFromRow(row)
			return err
		}
		switch row.lifecycleState {
		case ingestionapplication.DerivedArtifactPending, ingestionapplication.DerivedArtifactFailed, ingestionapplication.DerivedArtifactAvailable:
		default:
			return fmt.Errorf("%w: derived artifact cannot be quarantined", sharedrepository.ErrConflict)
		}
		row, err = scanDerivedArtifactRow(executor.QueryRowContext(transactionCtx, `
UPDATE derived_artifacts
SET lifecycle_state='quarantined',active=false,available_at=NULL,failure_code=NULL,updated_at=now()
WHERE id=$1
RETURNING `+derivedArtifactColumns, command.ArtifactID))
		if err != nil {
			return mapDerivedArtifactDatabaseError(err)
		}
		artifact, err = derivedArtifactDTOFromRow(row)
		return err
	})
	return artifact, err
}

const derivedArtifactColumns = `
id,source_connection_id,document_version_id,store_derived_rights_decision_id,
retain_rights_decision_id,artifact_type,transformer_profile_sha256,vault_relative_path,
mime_type,sha256,size_bytes,lifecycle_state,active,failure_code,available_at,
retention_until,created_at,updated_at`

var derivedArtifactFailureCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type projectionDocumentVersionLock struct {
	documentID, sourceConnectionID int64
	version                        ingestionapplication.DocumentVersionDTO
}

type derivedArtifactRow struct {
	id, sourceConnectionID, documentVersionID                 int64
	storeDerivedRightsDecisionID, retainRightsDecisionID      int64
	artifactType, transformerProfileSHA256, vaultRelativePath string
	mimeType, sha256, lifecycleState                          string
	sizeBytes                                                 int64
	active                                                    bool
	failureCode                                               sql.NullString
	availableAt                                               sql.NullTime
	retentionUntil, createdAt, updatedAt                      time.Time
}

func scanDerivedArtifactRow(scanner interface{ Scan(...any) error }) (derivedArtifactRow, error) {
	var row derivedArtifactRow
	err := scanner.Scan(
		&row.id, &row.sourceConnectionID, &row.documentVersionID,
		&row.storeDerivedRightsDecisionID, &row.retainRightsDecisionID,
		&row.artifactType, &row.transformerProfileSHA256, &row.vaultRelativePath,
		&row.mimeType, &row.sha256, &row.sizeBytes, &row.lifecycleState, &row.active,
		&row.failureCode, &row.availableAt, &row.retentionUntil, &row.createdAt, &row.updatedAt,
	)
	return row, err
}

func derivedArtifactDTOFromRow(row derivedArtifactRow) (ingestionapplication.DerivedArtifactDTO, error) {
	artifact := ingestionapplication.DerivedArtifactDTO{
		ID: row.id, SourceConnectionID: row.sourceConnectionID, DocumentVersionID: row.documentVersionID,
		StoreDerivedRightsDecisionID: row.storeDerivedRightsDecisionID,
		RetainRightsDecisionID:       row.retainRightsDecisionID,
		ArtifactType:                 row.artifactType,
		TransformerProfileSHA256:     row.transformerProfileSHA256,
		MIMEType:                     row.mimeType, SHA256: row.sha256, SizeBytes: row.sizeBytes,
		LifecycleState: row.lifecycleState, Active: row.active,
		FailureCode: row.failureCode.String, AvailableAt: optionalArtifactTime(row.availableAt),
		RetentionUntil: row.retentionUntil.UTC(), CreatedAt: row.createdAt.UTC(), UpdatedAt: row.updatedAt.UTC(),
	}
	if err := ingestionapplication.ValidateDerivedArtifactDTO(artifact); err != nil {
		return ingestionapplication.DerivedArtifactDTO{}, fmt.Errorf("invalid derived artifact row: %w", err)
	}
	return artifact, nil
}

func lockProjectionDocumentVersion(ctx context.Context, executor documentVersionExecutor, documentVersionID int64) (projectionDocumentVersionLock, error) {
	var locked projectionDocumentVersionLock
	err := executor.QueryRowContext(ctx, `
SELECT version.document_id,document.source_connection_id
FROM document_versions AS version
JOIN documents AS document ON document.id=version.document_id
WHERE version.id=$1 AND document.document_state='active'
FOR UPDATE OF version,document`, documentVersionID).Scan(&locked.documentID, &locked.sourceConnectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return projectionDocumentVersionLock{}, fmt.Errorf("%w: active document version %d", sharedrepository.ErrNotFound, documentVersionID)
	}
	if err != nil {
		return projectionDocumentVersionLock{}, mapDerivedArtifactDatabaseError(err)
	}
	locked.version, err = scanDocumentVersionRow(executor.QueryRowContext(ctx, `
SELECT `+documentVersionColumns+` FROM document_versions WHERE id=$1`, documentVersionID))
	if err != nil {
		return projectionDocumentVersionLock{}, mapDerivedArtifactDatabaseError(err)
	}
	return locked, nil
}

func reserveDerivedArtifactResult(row derivedArtifactRow, locked projectionDocumentVersionLock) (ingestionapplication.ReserveDerivedArtifactResult, error) {
	artifact, err := derivedArtifactDTOFromRow(row)
	if err != nil {
		return ingestionapplication.ReserveDerivedArtifactResult{}, err
	}
	return ingestionapplication.ReserveDerivedArtifactResult{
		Artifact: artifact, DocumentID: locked.documentID,
		VaultRelativePath: row.vaultRelativePath, DocumentVersion: locked.version,
	}, nil
}

func commitDerivedArtifactResult(row derivedArtifactRow, locked projectionDocumentVersionLock) (ingestionapplication.CommitDerivedArtifactResult, error) {
	artifact, err := derivedArtifactDTOFromRow(row)
	if err != nil {
		return ingestionapplication.CommitDerivedArtifactResult{}, err
	}
	return ingestionapplication.CommitDerivedArtifactResult{
		Artifact: artifact, DocumentID: locked.documentID, DocumentVersion: locked.version,
	}, nil
}

func sameDerivedArtifactProjectionFacts(row derivedArtifactRow, documentID int64, command ingestionapplication.ReserveDerivedArtifactCommand) bool {
	return row.documentVersionID == command.DocumentVersionID && row.artifactType == string(command.ArtifactType) &&
		row.transformerProfileSHA256 == command.TransformerProfileSHA256 &&
		row.vaultRelativePath == derivedArtifactRelativePath(documentID, command.DocumentVersionID, command.ArtifactType, command.TransformerProfileSHA256) &&
		row.mimeType == command.MIMEType && row.sha256 == command.SHA256 && row.sizeBytes == command.SizeBytes
}

func derivedArtifactReceiptMatches(row derivedArtifactRow, documentID int64, receipt ingestionapplication.ProjectionReceiptDTO) bool {
	return documentID == receipt.DocumentID && row.documentVersionID == receipt.DocumentVersionID && row.artifactType == string(receipt.ArtifactType) &&
		row.transformerProfileSHA256 == receipt.TransformerProfileSHA256 && row.vaultRelativePath == receipt.VaultRelativePath &&
		row.mimeType == receipt.MIMEType && row.sha256 == receipt.SHA256 && row.sizeBytes == receipt.SizeBytes
}

func currentReservationRights(ctx context.Context, executor documentVersionExecutor, storeDecisionID, retainDecisionID int64, locked projectionDocumentVersionLock) (time.Time, bool, error) {
	var storeAllowed, retainAllowed bool
	var currentRetentionDays, selectedRetentionDays sql.NullInt64
	var retentionUntil sql.NullTime
	var retentionCurrent bool
	err := executor.QueryRowContext(ctx, `
SELECT
  current_rights_action_allowed($1,$3,'document_version',$4,$5,'store_derived',now()),
  current_rights_action_allowed($2,$3,'document_version',$4,$5,'retain',now()),
  current_rights_retention_days($3,'document_version',$4,$5,now()),
  selected.retention_days,
  version.captured_at+selected.retention_days*interval '24 hours',
  version.captured_at+selected.retention_days*interval '24 hours'>now()
FROM document_versions AS version
JOIN source_rights_decisions AS selected ON selected.id=$2
WHERE version.id=$6`, storeDecisionID, retainDecisionID, locked.sourceConnectionID,
		fmt.Sprintf("%d", locked.version.ID), locked.version.ContentSHA256, locked.version.ID).Scan(
		&storeAllowed, &retainAllowed, &currentRetentionDays, &selectedRetentionDays, &retentionUntil, &retentionCurrent,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, mapDerivedArtifactDatabaseError(err)
	}
	allowed := storeAllowed && retainAllowed && currentRetentionDays.Valid && selectedRetentionDays.Valid &&
		currentRetentionDays.Int64 == selectedRetentionDays.Int64 && retentionUntil.Valid && retentionCurrent
	return retentionUntil.Time.UTC(), allowed, nil
}

func exactDerivedArtifactRightsCurrent(ctx context.Context, executor documentVersionExecutor, row derivedArtifactRow, version ingestionapplication.DocumentVersionDTO) (bool, error) {
	locked := projectionDocumentVersionLock{sourceConnectionID: row.sourceConnectionID, version: version}
	retentionUntil, allowed, err := currentReservationRights(
		ctx, executor, row.storeDerivedRightsDecisionID, row.retainRightsDecisionID, locked,
	)
	if err != nil || !allowed {
		return false, err
	}
	return row.retentionUntil.Equal(retentionUntil), nil
}

func readableDocumentDisplayRightsCurrent(ctx context.Context, executor documentVersionExecutor, locked projectionDocumentVersionLock) (bool, error) {
	if locked.version.LifecycleState != ingestionapplication.DocumentReadable {
		return true, nil
	}
	if locked.version.DisplayPrivateRightsDecisionID == nil {
		return false, nil
	}
	var allowed bool
	err := executor.QueryRowContext(ctx, `
SELECT current_rights_action_allowed(
  $1,$2,'document_version',$3,$4,'display_private',now()
)`, *locked.version.DisplayPrivateRightsDecisionID, locked.sourceConnectionID,
		fmt.Sprintf("%d", locked.version.ID), locked.version.ContentSHA256).Scan(&allowed)
	if err != nil {
		return false, mapDerivedArtifactDatabaseError(err)
	}
	return allowed, nil
}

func documentVersionAllowsProjection(state string) bool {
	switch state {
	case ingestionapplication.DocumentPolicyPending, ingestionapplication.DocumentPolicyBlocked,
		ingestionapplication.DocumentDerivedPending, ingestionapplication.DocumentDerivedFailed,
		ingestionapplication.DocumentDerivedAvailable, ingestionapplication.DocumentReadable:
		return true
	default:
		return false
	}
}

func derivedArtifactRelativePath(documentID, documentVersionID int64, artifactType string, profileSHA string) string {
	extension := "txt"
	if artifactType == ingestionapplication.DerivedArtifactMarkdown {
		extension = "md"
	}
	return path.Join(
		"documents", fmt.Sprintf("%d", documentID), fmt.Sprintf("%d", documentVersionID),
		artifactType, profileSHA+"."+extension,
	)
}

func optionalArtifactTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func mapDerivedArtifactDatabaseError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: derived artifact state changed", sharedrepository.ErrConflict)
	}
	return databaserepository.MapError(err)
}

func (repository *DerivedArtifactRepository) withTransaction(ctx context.Context, operation func(context.Context, documentVersionExecutor) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return operation(ctx, transaction.SQL)
	}
	return repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		return operation(transactionCtx, transaction.SQL)
	})
}

func (repository *DerivedArtifactRepository) available() bool {
	return repository != nil && repository.runtime != nil && repository.runtime.SQL != nil
}
