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

const derivedArtifactDeletionLeaseTimeout = 15 * time.Minute

type DerivedArtifactRetentionRepository struct{ runtime *database.Runtime }

var _ ingestionapplication.DerivedArtifactRetentionRepository = (*DerivedArtifactRetentionRepository)(nil)

func NewDerivedArtifactRetentionRepository(runtime *database.Runtime) *DerivedArtifactRetentionRepository {
	return &DerivedArtifactRetentionRepository{runtime: runtime}
}

func (repository *DerivedArtifactRetentionRepository) available() bool {
	return repository != nil && repository.runtime != nil && repository.runtime.SQL != nil
}

func (repository *DerivedArtifactRetentionRepository) ClaimExpired(ctx context.Context, at time.Time, limit int) ([]ingestionapplication.DerivedArtifactRetentionCandidateDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if at.IsZero() || limit < 1 || limit > ingestionapplication.MaximumDerivedArtifactRetentionBatch {
		return nil, fmt.Errorf("%w: invalid derived artifact retention claim", sharedrepository.ErrInvalidInput)
	}
	at = at.UTC()
	claimed := make([]ingestionapplication.DerivedArtifactRetentionCandidateDTO, 0, limit)
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		rows, err := transaction.SQL.QueryContext(transactionCtx, `
SELECT artifact.id,artifact.source_connection_id,document.id,artifact.document_version_id,
       artifact.artifact_type,btrim(artifact.transformer_profile_sha256),artifact.vault_relative_path,
       artifact.mime_type,btrim(artifact.sha256),artifact.size_bytes,artifact.retention_until,
       policy.id,decision.policy_revision,
       CASE WHEN NOT current_rights_action_is_allowed(
              artifact.source_connection_id,'document_version',version.id::text,version.content_sha256,'store_derived',$1
            ) OR current_rights_retention_days(
              artifact.source_connection_id,'document_version',version.id::text,version.content_sha256,$1
            ) IS NULL
            THEN 'RIGHTS_REVOKED' ELSE 'RETENTION_EXPIRED' END
FROM derived_artifacts AS artifact
JOIN document_versions AS version ON version.id=artifact.document_version_id
JOIN documents AS document ON document.id=version.document_id
JOIN source_rights_decisions AS decision ON decision.id=artifact.retain_rights_decision_id
JOIN source_rights_policies AS policy ON policy.id=decision.policy_id
WHERE artifact.lifecycle_state IN ('derived_available','retention_blocked')
  AND (
      NOT current_rights_action_is_allowed(
        artifact.source_connection_id,'document_version',version.id::text,version.content_sha256,'store_derived',$1
      )
      OR current_rights_retention_days(
        artifact.source_connection_id,'document_version',version.id::text,version.content_sha256,$1
      ) IS NULL
      OR artifact.retention_until <= $1
  )
  AND NOT EXISTS (
      SELECT 1 FROM derived_artifact_deletion_audits AS audit
      WHERE audit.derived_artifact_id=artifact.id AND audit.event_type='delete_succeeded'
  )
  AND (
      artifact.lifecycle_state='derived_available'
      OR COALESCE((
          SELECT latest.event_type='delete_failed' OR latest.occurred_at <= $2
          FROM derived_artifact_deletion_audits AS latest
          WHERE latest.derived_artifact_id=artifact.id
          ORDER BY latest.attempt_no DESC,latest.id DESC LIMIT 1
      ),true)
  )
ORDER BY artifact.retention_until,artifact.id
LIMIT $3
FOR UPDATE OF artifact SKIP LOCKED`, at, at.Add(-derivedArtifactDeletionLeaseTimeout), limit)
		if err != nil {
			return databaserepository.MapError(err)
		}
		selected := make([]ingestionapplication.DerivedArtifactRetentionCandidateDTO, 0, limit)
		for rows.Next() {
			var candidate ingestionapplication.DerivedArtifactRetentionCandidateDTO
			if err := rows.Scan(
				&candidate.ArtifactID, &candidate.SourceConnectionID, &candidate.DocumentID, &candidate.DocumentVersionID,
				&candidate.ArtifactType, &candidate.TransformerProfileSHA256, &candidate.VaultRelativePath,
				&candidate.MIMEType, &candidate.SHA256, &candidate.SizeBytes, &candidate.RetentionUntil,
				&candidate.RetentionPolicyID, &candidate.RetentionPolicyVersion, &candidate.ReasonCode,
			); err != nil {
				_ = rows.Close()
				return databaserepository.MapError(err)
			}
			selected = append(selected, candidate)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return databaserepository.MapError(err)
		}
		if err := rows.Close(); err != nil {
			return databaserepository.MapError(err)
		}
		for _, candidate := range selected {
			if _, err := transaction.SQL.ExecContext(transactionCtx, `
UPDATE derived_artifacts
SET lifecycle_state='retention_blocked',active=false,available_at=NULL,failure_code=NULL,updated_at=$2
WHERE id=$1 AND lifecycle_state IN ('derived_available','retention_blocked')`, candidate.ArtifactID, at); err != nil {
				return databaserepository.MapError(err)
			}
			if _, err := transaction.SQL.ExecContext(transactionCtx, `
UPDATE document_versions
SET version=version+1,lifecycle_state='retention_blocked'
WHERE id=$1 AND lifecycle_state='readable'`, candidate.DocumentVersionID); err != nil {
				return databaserepository.MapError(err)
			}
			if err := transaction.SQL.QueryRowContext(transactionCtx, `
SELECT COALESCE(max(attempt_no),0)+1
FROM derived_artifact_deletion_audits WHERE derived_artifact_id=$1`, candidate.ArtifactID).Scan(&candidate.AttemptNo); err != nil {
				return databaserepository.MapError(err)
			}
			if err := candidate.Validate(at); err != nil {
				return fmt.Errorf("%w: %w", sharedrepository.ErrConstraint, err)
			}
			if _, err := transaction.SQL.ExecContext(transactionCtx, `
INSERT INTO derived_artifact_deletion_audits (
  derived_artifact_id,source_connection_id,retention_policy_id,retention_policy_version,
  attempt_no,event_type,vault_relative_path,sha256,size_bytes,reason_code,occurred_at
) VALUES ($1,$2,$3,$4,$5,'delete_claimed',$6,$7,$8,$9,$10)`,
				candidate.ArtifactID, candidate.SourceConnectionID, candidate.RetentionPolicyID, candidate.RetentionPolicyVersion,
				candidate.AttemptNo, candidate.VaultRelativePath, candidate.SHA256, candidate.SizeBytes, candidate.ReasonCode, at); err != nil {
				return databaserepository.MapError(err)
			}
			claimed = append(claimed, candidate)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (repository *DerivedArtifactRetentionRepository) CompleteDeletion(ctx context.Context, command ingestionapplication.CompleteDerivedArtifactDeletionCommand) error {
	if !repository.available() {
		return sharedrepository.ErrUnavailable
	}
	if command.ArtifactID <= 0 || command.AttemptNo <= 0 || command.VaultRelativePath == "" ||
		command.SHA256 == "" || command.SizeBytes <= 0 || command.DeletedAt.IsZero() {
		return fmt.Errorf("%w: invalid derived artifact deletion completion", sharedrepository.ErrInvalidInput)
	}
	return repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		identity, err := lockDerivedArtifactDeletionAttempt(transactionCtx, transaction.SQL, command.ArtifactID, command.AttemptNo)
		if err != nil {
			return err
		}
		if identity.VaultRelativePath != command.VaultRelativePath || identity.SHA256 != command.SHA256 ||
			identity.SizeBytes != command.SizeBytes || identity.LifecycleState != ingestionapplication.DerivedArtifactRetentionBlocked {
			return fmt.Errorf("%w: derived artifact deletion identity changed", sharedrepository.ErrConflict)
		}
		if _, err := transaction.SQL.ExecContext(transactionCtx, `
INSERT INTO derived_artifact_deletion_audits (
  derived_artifact_id,source_connection_id,retention_policy_id,retention_policy_version,
  attempt_no,event_type,vault_relative_path,sha256,size_bytes,reason_code,already_missing,occurred_at
) VALUES ($1,$2,$3,$4,$5,'delete_succeeded',$6,$7,$8,$9,$10,$11)`,
			command.ArtifactID, identity.SourceConnectionID, identity.PolicyID, identity.PolicyVersion,
			command.AttemptNo, command.VaultRelativePath, command.SHA256, command.SizeBytes,
			identity.ReasonCode, command.AlreadyMissing, command.DeletedAt.UTC()); err != nil {
			return databaserepository.MapError(err)
		}
		result, err := transaction.SQL.ExecContext(transactionCtx, `
UPDATE derived_artifacts
SET lifecycle_state='tombstoned',active=false,available_at=NULL,failure_code=NULL,updated_at=$2
WHERE id=$1 AND lifecycle_state='retention_blocked'`, command.ArtifactID, command.DeletedAt.UTC())
		if err != nil {
			return databaserepository.MapError(err)
		}
		return requireDerivedArtifactRetentionMutation(result)
	})
}

func (repository *DerivedArtifactRetentionRepository) FailDeletion(ctx context.Context, command ingestionapplication.FailDerivedArtifactDeletionCommand) error {
	if !repository.available() {
		return sharedrepository.ErrUnavailable
	}
	if command.ArtifactID <= 0 || command.AttemptNo <= 0 || command.VaultRelativePath == "" || command.SHA256 == "" ||
		command.SizeBytes <= 0 || command.FailedAt.IsZero() ||
		(command.FailureCode != ingestionapplication.DerivedArtifactDeleteVaultFailed &&
			command.FailureCode != ingestionapplication.DerivedArtifactDeleteIntegrityFailed) {
		return fmt.Errorf("%w: invalid derived artifact deletion failure", sharedrepository.ErrInvalidInput)
	}
	return repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		identity, err := lockDerivedArtifactDeletionAttempt(transactionCtx, transaction.SQL, command.ArtifactID, command.AttemptNo)
		if err != nil {
			return err
		}
		if identity.VaultRelativePath != command.VaultRelativePath || identity.SHA256 != command.SHA256 ||
			identity.SizeBytes != command.SizeBytes || identity.LifecycleState != ingestionapplication.DerivedArtifactRetentionBlocked {
			return fmt.Errorf("%w: derived artifact deletion identity changed", sharedrepository.ErrConflict)
		}
		_, err = transaction.SQL.ExecContext(transactionCtx, `
INSERT INTO derived_artifact_deletion_audits (
  derived_artifact_id,source_connection_id,retention_policy_id,retention_policy_version,
  attempt_no,event_type,vault_relative_path,sha256,size_bytes,reason_code,occurred_at
) VALUES ($1,$2,$3,$4,$5,'delete_failed',$6,$7,$8,$9,$10)`,
			command.ArtifactID, identity.SourceConnectionID, identity.PolicyID, identity.PolicyVersion,
			command.AttemptNo, command.VaultRelativePath, command.SHA256, command.SizeBytes,
			command.FailureCode, command.FailedAt.UTC())
		return databaserepository.MapError(err)
	})
}

type derivedArtifactDeletionIdentity struct {
	SourceConnectionID int64
	PolicyID           int64
	PolicyVersion      int64
	VaultRelativePath  string
	SHA256             string
	SizeBytes          int64
	LifecycleState     string
	ReasonCode         string
}

func lockDerivedArtifactDeletionAttempt(ctx context.Context, executor *sql.Tx, artifactID int64, attemptNo int) (derivedArtifactDeletionIdentity, error) {
	var identity derivedArtifactDeletionIdentity
	err := executor.QueryRowContext(ctx, `
SELECT artifact.source_connection_id,audit.retention_policy_id,audit.retention_policy_version,
       artifact.vault_relative_path,btrim(artifact.sha256),artifact.size_bytes,artifact.lifecycle_state,audit.reason_code
FROM derived_artifacts AS artifact
JOIN derived_artifact_deletion_audits AS audit
  ON audit.derived_artifact_id=artifact.id AND audit.attempt_no=$2 AND audit.event_type='delete_claimed'
WHERE artifact.id=$1
FOR UPDATE OF artifact`, artifactID, attemptNo).Scan(
		&identity.SourceConnectionID, &identity.PolicyID, &identity.PolicyVersion,
		&identity.VaultRelativePath, &identity.SHA256, &identity.SizeBytes, &identity.LifecycleState, &identity.ReasonCode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return derivedArtifactDeletionIdentity{}, fmt.Errorf("%w: derived artifact deletion attempt", sharedrepository.ErrNotFound)
	}
	if err != nil {
		return derivedArtifactDeletionIdentity{}, databaserepository.MapError(err)
	}
	return identity, nil
}

func requireDerivedArtifactRetentionMutation(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return databaserepository.MapError(err)
	}
	if affected != 1 {
		return fmt.Errorf("%w: derived artifact retention state changed", sharedrepository.ErrConflict)
	}
	return nil
}
