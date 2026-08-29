package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const rawEvidenceDeletionLeaseTimeout = 15 * time.Minute

type RawEvidenceRetentionRepository struct{ runtime *database.Runtime }

var _ sourceapplication.RawEvidenceRetentionRepository = (*RawEvidenceRetentionRepository)(nil)

func NewRawEvidenceRetentionRepository(runtime *database.Runtime) *RawEvidenceRetentionRepository {
	return &RawEvidenceRetentionRepository{runtime: runtime}
}

func (repository *RawEvidenceRetentionRepository) ClaimExpired(ctx context.Context, at time.Time, limit int) ([]sourceapplication.RawEvidenceRetentionCandidateDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if at.IsZero() || limit < 1 || limit > sourceapplication.MaximumRawEvidenceRetentionBatch {
		return nil, fmt.Errorf("%w: invalid raw evidence retention claim", sharedrepository.ErrInvalidInput)
	}
	at = at.UTC()
	claimed := make([]sourceapplication.RawEvidenceRetentionCandidateDTO, 0, limit)
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		rows, err := transaction.SQL.QueryContext(transactionCtx, `
SELECT snapshot.id,snapshot.source_connection_id,btrim(snapshot.snapshot_key),snapshot.object_key,
       btrim(snapshot.payload_sha256),snapshot.retention_until,policy.id,decision.policy_revision,
       CASE WHEN NOT current_rights_action_is_allowed(
              snapshot.source_connection_id,'raw_response',btrim(snapshot.snapshot_key),snapshot.payload_sha256,'store_raw',$1
            ) OR current_rights_retention_days(
              snapshot.source_connection_id,'raw_response',btrim(snapshot.snapshot_key),snapshot.payload_sha256,$1
            ) IS NULL
            THEN 'RIGHTS_REVOKED' ELSE 'RETENTION_EXPIRED' END
FROM evidence_snapshots AS snapshot
JOIN source_rights_decisions AS decision ON decision.id=snapshot.retain_rights_decision_id
JOIN source_rights_policies AS policy ON policy.id=decision.policy_id
WHERE snapshot.lifecycle_state IN ('raw_available','retention_blocked')
  AND (
      NOT current_rights_action_is_allowed(
        snapshot.source_connection_id,'raw_response',btrim(snapshot.snapshot_key),snapshot.payload_sha256,'store_raw',$1
      )
      OR current_rights_retention_days(
        snapshot.source_connection_id,'raw_response',btrim(snapshot.snapshot_key),snapshot.payload_sha256,$1
      ) IS NULL
      OR snapshot.retention_until <= $1 AND NOT EXISTS (
          SELECT 1 FROM evidence_retention_exceptions AS exception
          WHERE exception.evidence_snapshot_id=snapshot.id
            AND exception.revoked_at IS NULL
            AND exception.approved_at <= $1
            AND (exception.expires_at IS NULL OR exception.expires_at > $1)
      )
  )
  AND NOT EXISTS (
      SELECT 1 FROM evidence_deletion_audits AS audit
      WHERE audit.evidence_snapshot_id=snapshot.id AND audit.event_type='delete_succeeded'
  )
  AND (
      snapshot.lifecycle_state='raw_available'
      OR COALESCE((
          SELECT latest.event_type='delete_failed' OR latest.occurred_at <= $2
      FROM evidence_deletion_audits AS latest
      WHERE latest.evidence_snapshot_id=snapshot.id
      ORDER BY latest.attempt_no DESC,latest.id DESC LIMIT 1
      ),true)
  )
ORDER BY snapshot.retention_until,snapshot.id
LIMIT $3
FOR UPDATE OF snapshot SKIP LOCKED`, at, at.Add(-rawEvidenceDeletionLeaseTimeout), limit)
		if err != nil {
			return databaserepository.MapError(err)
		}
		selected := make([]sourceapplication.RawEvidenceRetentionCandidateDTO, 0, limit)
		for rows.Next() {
			var candidate sourceapplication.RawEvidenceRetentionCandidateDTO
			if err := rows.Scan(&candidate.SnapshotID, &candidate.SourceConnectionID, &candidate.EvidenceKey,
				&candidate.ObjectKey, &candidate.PayloadSHA256, &candidate.RetentionUntil,
				&candidate.RetentionPolicyID, &candidate.RetentionPolicyVersion, &candidate.ReasonCode); err != nil {
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
UPDATE evidence_snapshots
SET lifecycle_state='retention_blocked',available_at=NULL,failure_code=NULL,updated_at=$2
WHERE id=$1 AND lifecycle_state IN ('raw_available','retention_blocked')`, candidate.SnapshotID, at); err != nil {
				return databaserepository.MapError(err)
			}
			if err := transaction.SQL.QueryRowContext(transactionCtx, `
SELECT COALESCE(max(attempt_no),0)+1 FROM evidence_deletion_audits WHERE evidence_snapshot_id=$1`, candidate.SnapshotID).Scan(&candidate.AttemptNo); err != nil {
				return databaserepository.MapError(err)
			}
			if err := candidate.Validate(at); err != nil {
				return fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
			}
			if _, err := transaction.SQL.ExecContext(transactionCtx, `
INSERT INTO evidence_deletion_audits (
  evidence_snapshot_id,source_connection_id,retention_policy_id,retention_policy_version,
  attempt_no,event_type,object_key,payload_sha256,reason_code,occurred_at
) VALUES ($1,$2,$3,$4,$5,'delete_claimed',$6,$7,$8,$9)`,
				candidate.SnapshotID, candidate.SourceConnectionID, candidate.RetentionPolicyID, candidate.RetentionPolicyVersion,
				candidate.AttemptNo, candidate.ObjectKey, candidate.PayloadSHA256, candidate.ReasonCode, at); err != nil {
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

func (repository *RawEvidenceRetentionRepository) CompleteDeletion(ctx context.Context, command sourceapplication.CompleteRawEvidenceDeletionCommand) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	if command.SnapshotID <= 0 || command.AttemptNo <= 0 || command.ObjectKey == "" || command.PayloadSHA256 == "" || command.DeletedAt.IsZero() {
		return fmt.Errorf("%w: invalid raw evidence deletion completion", sharedrepository.ErrInvalidInput)
	}
	return repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		identity, err := lockRawEvidenceDeletionAttempt(transactionCtx, transaction.SQL, command.SnapshotID, command.AttemptNo)
		if err != nil {
			return err
		}
		if identity.ObjectKey != command.ObjectKey || identity.PayloadSHA256 != command.PayloadSHA256 || identity.LifecycleState != "retention_blocked" {
			return fmt.Errorf("%w: raw evidence deletion identity changed", sharedrepository.ErrConflict)
		}
		if _, err := transaction.SQL.ExecContext(transactionCtx, `
INSERT INTO evidence_deletion_audits (
  evidence_snapshot_id,source_connection_id,retention_policy_id,retention_policy_version,
  attempt_no,event_type,object_key,payload_sha256,reason_code,already_missing,occurred_at
) VALUES ($1,$2,$3,$4,$5,'delete_succeeded',$6,$7,$8,$9,$10)`,
			command.SnapshotID, identity.SourceConnectionID, identity.PolicyID, identity.PolicyVersion,
			command.AttemptNo, command.ObjectKey, command.PayloadSHA256, identity.ReasonCode, command.AlreadyMissing, command.DeletedAt.UTC()); err != nil {
			return databaserepository.MapError(err)
		}
		result, err := transaction.SQL.ExecContext(transactionCtx, `
UPDATE evidence_snapshots
SET lifecycle_state='tombstoned',available_at=NULL,failure_code=NULL,updated_at=$2
WHERE id=$1 AND lifecycle_state='retention_blocked'`, command.SnapshotID, command.DeletedAt.UTC())
		if err != nil {
			return databaserepository.MapError(err)
		}
		return requireRawEvidenceRetentionMutation(result)
	})
}

func (repository *RawEvidenceRetentionRepository) FailDeletion(ctx context.Context, command sourceapplication.FailRawEvidenceDeletionCommand) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	if command.SnapshotID <= 0 || command.AttemptNo <= 0 || command.ObjectKey == "" || command.PayloadSHA256 == "" || command.FailedAt.IsZero() ||
		(command.FailureCode != sourceapplication.RawEvidenceDeleteObjectFailed && command.FailureCode != sourceapplication.RawEvidenceDeleteIntegrityFailed) {
		return fmt.Errorf("%w: invalid raw evidence deletion failure", sharedrepository.ErrInvalidInput)
	}
	return repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		identity, err := lockRawEvidenceDeletionAttempt(transactionCtx, transaction.SQL, command.SnapshotID, command.AttemptNo)
		if err != nil {
			return err
		}
		if identity.ObjectKey != command.ObjectKey || identity.PayloadSHA256 != command.PayloadSHA256 || identity.LifecycleState != "retention_blocked" {
			return fmt.Errorf("%w: raw evidence deletion identity changed", sharedrepository.ErrConflict)
		}
		_, err = transaction.SQL.ExecContext(transactionCtx, `
INSERT INTO evidence_deletion_audits (
  evidence_snapshot_id,source_connection_id,retention_policy_id,retention_policy_version,
  attempt_no,event_type,object_key,payload_sha256,reason_code,occurred_at
) VALUES ($1,$2,$3,$4,$5,'delete_failed',$6,$7,$8,$9)`,
			command.SnapshotID, identity.SourceConnectionID, identity.PolicyID, identity.PolicyVersion,
			command.AttemptNo, command.ObjectKey, command.PayloadSHA256, command.FailureCode, command.FailedAt.UTC())
		return databaserepository.MapError(err)
	})
}

type rawEvidenceDeletionIdentity struct {
	SourceConnectionID int64
	PolicyID           int64
	PolicyVersion      int64
	ObjectKey          string
	PayloadSHA256      string
	LifecycleState     string
	ReasonCode         string
}

func lockRawEvidenceDeletionAttempt(ctx context.Context, executor *sql.Tx, snapshotID int64, attemptNo int) (rawEvidenceDeletionIdentity, error) {
	var identity rawEvidenceDeletionIdentity
	err := executor.QueryRowContext(ctx, `
SELECT snapshot.source_connection_id,audit.retention_policy_id,audit.retention_policy_version,
       snapshot.object_key,btrim(snapshot.payload_sha256),snapshot.lifecycle_state,audit.reason_code
FROM evidence_snapshots AS snapshot
JOIN evidence_deletion_audits AS audit
  ON audit.evidence_snapshot_id=snapshot.id AND audit.attempt_no=$2 AND audit.event_type='delete_claimed'
WHERE snapshot.id=$1
FOR UPDATE OF snapshot`, snapshotID, attemptNo).Scan(
		&identity.SourceConnectionID, &identity.PolicyID, &identity.PolicyVersion,
		&identity.ObjectKey, &identity.PayloadSHA256, &identity.LifecycleState, &identity.ReasonCode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return rawEvidenceDeletionIdentity{}, fmt.Errorf("%w: raw evidence deletion attempt", sharedrepository.ErrNotFound)
	}
	if err != nil {
		return rawEvidenceDeletionIdentity{}, databaserepository.MapError(err)
	}
	return identity, nil
}

func requireRawEvidenceRetentionMutation(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return databaserepository.MapError(err)
	}
	if affected != 1 {
		return fmt.Errorf("%w: raw evidence retention state changed", sharedrepository.ErrConflict)
	}
	return nil
}
