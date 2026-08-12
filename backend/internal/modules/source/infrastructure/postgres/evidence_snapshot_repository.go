package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type EvidenceSnapshotRepository struct {
	runtime                     *database.Runtime
	documentGenerationScheduler sourceapplication.SourceDocumentGenerationScheduler
}

var _ sourceapplication.EvidenceSnapshotRepository = (*EvidenceSnapshotRepository)(nil)

func NewEvidenceSnapshotRepository(runtime *database.Runtime, scheduler sourceapplication.SourceDocumentGenerationScheduler) (*EvidenceSnapshotRepository, error) {
	if runtime == nil || runtime.SQL == nil || runtime.GORM == nil {
		return nil, fmt.Errorf("evidence snapshot database runtime is required")
	}
	if scheduler == nil {
		return nil, fmt.Errorf("source document generation scheduler is required")
	}
	return &EvidenceSnapshotRepository{runtime: runtime, documentGenerationScheduler: scheduler}, nil
}

func (repository *EvidenceSnapshotRepository) Reserve(ctx context.Context, command sourceapplication.ReserveEvidenceSnapshotCommand) (sourceapplication.PersistedEvidenceSnapshotDTO, error) {
	if !repository.available() {
		return sourceapplication.PersistedEvidenceSnapshotDTO{}, sharedrepository.ErrUnavailable
	}
	if err := validateEvidenceReservation(command); err != nil {
		return sourceapplication.PersistedEvidenceSnapshotDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	redirectChainJSON, err := encodeRedirectChain(command.RedirectChain)
	if err != nil {
		return sourceapplication.PersistedEvidenceSnapshotDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	responseHeadersJSON, err := encodeResponseHeaders(command.ResponseHeaders)
	if err != nil {
		return sourceapplication.PersistedEvidenceSnapshotDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}

	var stored evidenceSnapshotRecord
	err = repository.withTransaction(ctx, func(transactionCtx context.Context, executor evidenceSnapshotExecutor) error {
		var collectionRunID any
		if command.CollectionRunID > 0 {
			var storedRunID int64
			if err := executor.QueryRowContext(transactionCtx, `
SELECT id FROM collection_runs
WHERE id=$1 AND source_connection_id=$2
		FOR KEY SHARE`, command.CollectionRunID, command.SourceConnectionID).Scan(&storedRunID); errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: source collection run is missing", sharedrepository.ErrConstraint)
			} else if err != nil {
				return databaserepository.MapError(err)
			}
			collectionRunID = storedRunID
		}

		_, err := executor.ExecContext(transactionCtx, `
INSERT INTO evidence_snapshots (
  source_connection_id,collection_run_id,store_raw_rights_decision_id,retain_rights_decision_id,
  snapshot_key,object_key,payload_sha256,collector_profile_version,mime_type,size_bytes,
  response_status,requested_url,final_url,redirect_chain,response_headers,captured_at,retention_until
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15::jsonb,$16,$17)
ON CONFLICT (source_connection_id,snapshot_key) DO NOTHING`,
			command.SourceConnectionID, collectionRunID, command.StoreRawRightsDecisionID, command.RetainRightsDecisionID,
			command.EvidenceKey, command.ObjectKey, command.PayloadSHA256, command.CollectorProfileVersion,
			command.MIMEType, command.SizeBytes, command.ResponseStatus, command.RequestedURL, command.FinalURL,
			string(redirectChainJSON), string(responseHeadersJSON), command.CapturedAt.UTC(), command.RetentionUntil.UTC())
		if err != nil {
			return databaserepository.MapError(err)
		}

		stored, err = scanEvidenceSnapshotRecord(executor.QueryRowContext(transactionCtx, `
SELECT `+evidenceSnapshotColumns+`
FROM evidence_snapshots
WHERE source_connection_id=$1 AND snapshot_key=$2
FOR UPDATE`, command.SourceConnectionID, command.EvidenceKey))
		if errors.Is(err, sql.ErrNoRows) {
			return evidenceConflict("reserved evidence snapshot disappeared")
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		if !sameEvidenceIdentity(stored, command) {
			return evidenceConflict("endpoint-scoped evidence key has different immutable identity facts")
		}

		switch domain.EvidenceLifecycleState(stored.LifecycleState) {
		case domain.EvidenceLifecyclePending, domain.EvidenceLifecycleAvailable:
			return nil
		case domain.EvidenceLifecycleFailed:
			stored, err = scanEvidenceSnapshotRecord(executor.QueryRowContext(transactionCtx, `
UPDATE evidence_snapshots
SET lifecycle_state='raw_pending',failure_code=NULL,available_at=NULL,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND lifecycle_state='raw_failed'
RETURNING `+evidenceSnapshotColumns, stored.ID))
			if errors.Is(err, sql.ErrNoRows) {
				return evidenceConflict("failed evidence snapshot changed before retry")
			}
			if err != nil {
				return databaserepository.MapError(err)
			}
			return nil
		default:
			return evidenceConflict("evidence snapshot lifecycle cannot be reserved")
		}
	})
	if err != nil {
		return sourceapplication.PersistedEvidenceSnapshotDTO{}, err
	}
	result, err := stored.persistenceDTO()
	if err != nil {
		return sourceapplication.PersistedEvidenceSnapshotDTO{}, fmt.Errorf("%w: map persisted evidence snapshot: %v", sharedrepository.ErrConstraint, err)
	}
	return result, nil
}

func (repository *EvidenceSnapshotRepository) Commit(ctx context.Context, command sourceapplication.CommitEvidenceSnapshotCommand) (sourceapplication.CommitEvidenceSnapshotResult, error) {
	if !repository.available() {
		return sourceapplication.CommitEvidenceSnapshotResult{}, sharedrepository.ErrUnavailable
	}
	if command.SnapshotID <= 0 {
		return sourceapplication.CommitEvidenceSnapshotResult{}, fmt.Errorf("%w: evidence snapshot id is required", sharedrepository.ErrInvalidInput)
	}
	if err := validateEvidenceStoreResult(command.StoreResult); err != nil {
		return sourceapplication.CommitEvidenceSnapshotResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	if err := (sourceapplication.ScheduleSourceDocumentGenerationCommand{
		EvidenceReferences: []sourceapplication.CommittedEvidenceReferenceDTO{},
		TraceID:            command.TraceID, ScheduledAt: command.DocumentGenerationScheduledAt,
	}).Validate(); err != nil {
		return sourceapplication.CommitEvidenceSnapshotResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}

	var committed evidenceSnapshotRecord
	committedReferences := make([]sourceapplication.CommittedEvidenceReferenceDTO, 0, len(command.Observations))
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, executor evidenceSnapshotExecutor) error {
		stored, err := scanEvidenceSnapshotRecord(executor.QueryRowContext(transactionCtx, `
SELECT `+evidenceSnapshotColumns+`
FROM evidence_snapshots WHERE id=$1
FOR UPDATE`, command.SnapshotID))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: evidence snapshot %d", sharedrepository.ErrNotFound, command.SnapshotID)
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		if !evidenceStoreResultMatches(stored, command.StoreResult) {
			return evidenceConflict("object-store result does not match reserved evidence")
		}
		if stored.LifecycleState != string(domain.EvidenceLifecyclePending) && stored.LifecycleState != string(domain.EvidenceLifecycleAvailable) {
			return evidenceConflict("evidence snapshot is not committable")
		}
		if err := validateObservationCommitSet(command.Observations, stored); err != nil {
			return err
		}
		if stored.LifecycleState == string(domain.EvidenceLifecycleAvailable) {
			committedReferences, err = verifyCommittedObservationSet(transactionCtx, executor, stored, command.Observations)
			if err != nil {
				return err
			}
			committed = stored
		} else {
			for _, observation := range command.Observations {
				collectionItemID, err := findObservationCollectionItem(transactionCtx, executor, observation)
				if err != nil {
					return err
				}
				observationID, err := appendSourceObservation(transactionCtx, executor, collectionItemID, observation)
				if err != nil {
					return err
				}
				reference, err := appendEvidenceLocator(transactionCtx, executor, observation.SourceConnectionID, observationID, stored.ID, observation.Evidence)
				if err != nil {
					return err
				}
				if err := appendObservationParties(
					transactionCtx, executor, observation.SourceConnectionID, observationID,
					reference.EvidenceReferenceID, observation.Parties,
				); err != nil {
					return err
				}
				committedReferences = append(committedReferences, reference)
			}

			committed, err = scanEvidenceSnapshotRecord(executor.QueryRowContext(transactionCtx, `
UPDATE evidence_snapshots
SET lifecycle_state='raw_available',available_at=COALESCE(available_at,CURRENT_TIMESTAMP),
    failure_code=NULL,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND lifecycle_state IN ('raw_pending','raw_available')
RETURNING `+evidenceSnapshotColumns, stored.ID))
			if errors.Is(err, sql.ErrNoRows) {
				return evidenceConflict("evidence snapshot changed before commit")
			}
			if err != nil {
				// evidence_snapshots_lifecycle performs the transaction-time current
				// store_raw/retain authorization check. Its failure rolls back every
				// observation and locator appended above.
				return databaserepository.MapError(err)
			}
		}

		scheduleCommand := sourceapplication.ScheduleSourceDocumentGenerationCommand{
			EvidenceReferences: documentSourceEvidenceReferences(committedReferences),
			TraceID:            command.TraceID, ScheduledAt: command.DocumentGenerationScheduledAt,
		}
		scheduleResult, err := repository.documentGenerationScheduler.Schedule(transactionCtx, scheduleCommand)
		if err != nil {
			return err
		}
		if err := sourceapplication.ValidateSourceDocumentGenerationScheduleResult(scheduleCommand, scheduleResult); err != nil {
			return fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
		}
		return nil
	})
	if err != nil {
		return sourceapplication.CommitEvidenceSnapshotResult{}, err
	}
	snapshot, err := committed.persistenceDTO()
	if err != nil {
		return sourceapplication.CommitEvidenceSnapshotResult{}, fmt.Errorf("%w: map committed evidence snapshot: %v", sharedrepository.ErrConstraint, err)
	}
	return sourceapplication.CommitEvidenceSnapshotResult{
		Snapshot: snapshot, EvidenceReferences: copyCommittedEvidenceReferences(committedReferences),
	}, nil
}

func copyCommittedEvidenceReferences(references []sourceapplication.CommittedEvidenceReferenceDTO) []sourceapplication.CommittedEvidenceReferenceDTO {
	copyOfReferences := make([]sourceapplication.CommittedEvidenceReferenceDTO, len(references))
	copy(copyOfReferences, references)
	return copyOfReferences
}

func documentSourceEvidenceReferences(references []sourceapplication.CommittedEvidenceReferenceDTO) []sourceapplication.CommittedEvidenceReferenceDTO {
	result := make([]sourceapplication.CommittedEvidenceReferenceDTO, 0, len(references))
	for _, reference := range references {
		if reference.Usage == "document_source" {
			result = append(result, reference)
		}
	}
	return result
}

var evidenceFailureCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

func (repository *EvidenceSnapshotRepository) MarkFailed(ctx context.Context, snapshotID int64, failureCode string) error {
	if !repository.available() {
		return sharedrepository.ErrUnavailable
	}
	if snapshotID <= 0 || !evidenceFailureCodePattern.MatchString(failureCode) {
		return fmt.Errorf("%w: evidence snapshot id or failure code is invalid", sharedrepository.ErrInvalidInput)
	}
	return repository.withTransaction(ctx, func(transactionCtx context.Context, executor evidenceSnapshotExecutor) error {
		var lifecycleState string
		var storedFailureCode sql.NullString
		err := executor.QueryRowContext(transactionCtx, `
SELECT lifecycle_state,failure_code
FROM evidence_snapshots WHERE id=$1
FOR UPDATE`, snapshotID).Scan(&lifecycleState, &storedFailureCode)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: evidence snapshot %d", sharedrepository.ErrNotFound, snapshotID)
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		switch domain.EvidenceLifecycleState(lifecycleState) {
		case domain.EvidenceLifecyclePending:
			result, err := executor.ExecContext(transactionCtx, `
UPDATE evidence_snapshots
SET lifecycle_state='raw_failed',failure_code=$2,available_at=NULL,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND lifecycle_state='raw_pending'`, snapshotID, failureCode)
			if err != nil {
				return databaserepository.MapError(err)
			}
			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return databaserepository.MapError(err)
			}
			if rowsAffected != 1 {
				return evidenceConflict("evidence snapshot changed before failure mark")
			}
			return nil
		case domain.EvidenceLifecycleFailed:
			if storedFailureCode.Valid && storedFailureCode.String == failureCode {
				return nil
			}
			return evidenceConflict("evidence snapshot already records a different failure")
		default:
			return evidenceConflict("evidence snapshot lifecycle cannot be marked failed")
		}
	})
}

type evidenceSnapshotExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (repository *EvidenceSnapshotRepository) available() bool {
	return repository != nil && repository.runtime != nil && repository.runtime.SQL != nil && repository.runtime.GORM != nil && repository.documentGenerationScheduler != nil
}

func (repository *EvidenceSnapshotRepository) withTransaction(ctx context.Context, function func(context.Context, evidenceSnapshotExecutor) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return function(ctx, transaction.SQL)
	}
	return repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		return function(transactionCtx, transaction.SQL)
	})
}

func findObservationCollectionItem(ctx context.Context, executor evidenceSnapshotExecutor, observation sourceapplication.SourceObservationDTO) (*int64, error) {
	var collectionItemID int64
	err := executor.QueryRowContext(ctx, `
SELECT item.id
FROM collection_run_items AS item
JOIN collection_runs AS run
  ON run.id=item.run_id AND run.source_connection_id=item.source_connection_id
WHERE item.run_id=$1 AND item.source_connection_id=$2 AND item.external_id=$3
	  AND item.source_code=$4 AND item.content_type=$5 AND item.outcome='captured'
FOR KEY SHARE`, observation.CollectionRunID, observation.SourceConnectionID, observation.ExternalID,
		observation.SourceCode, observation.ContentType).Scan(&collectionItemID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	return &collectionItemID, nil
}

func appendSourceObservation(ctx context.Context, executor evidenceSnapshotExecutor, collectionItemID *int64, observation sourceapplication.SourceObservationDTO) (int64, error) {
	stored, err := scanSourceObservationRecord(executor.QueryRowContext(ctx, `
INSERT INTO source_observations (
  source_connection_id,collection_run_item_id,external_id,upstream_identity,source_code,content_type,
  title,language,author_snapshot,source_record_url,canonical_url,discussion_url,body_origin,
  completeness,published_at,discovered_at,captured_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
ON CONFLICT DO NOTHING
RETURNING `+sourceObservationColumns,
		observation.SourceConnectionID, nullInt64(collectionItemID), observation.ExternalID, observation.UpstreamIdentity,
		observation.SourceCode, observation.ContentType, observation.Title, observation.Language, nullString(observation.Author),
		nullString(observation.SourceRecordURL), nullString(observation.CanonicalURL), nullString(observation.DiscussionURL),
		observation.BodyOrigin, observation.Completeness, nullTime(observation.PublishedAt),
		observation.DiscoveredAt.UTC(), observation.CapturedAt.UTC()))
	if errors.Is(err, sql.ErrNoRows) {
		stored, err = scanSourceObservationRecord(executor.QueryRowContext(ctx, `
SELECT `+sourceObservationColumns+`
FROM source_observations
WHERE source_connection_id=$1 AND external_id=$2 AND upstream_identity=$3
FOR KEY SHARE`, observation.SourceConnectionID, observation.ExternalID, observation.UpstreamIdentity))
		if errors.Is(err, sql.ErrNoRows) {
			return 0, evidenceConflict("legacy collection item is already bound to different observation facts")
		}
	}
	if err != nil {
		return 0, databaserepository.MapError(err)
	}
	if !sameObservationContentFacts(stored, observation) {
		return 0, evidenceConflict("source observation identity has different immutable content facts")
	}
	return stored.ID, nil
}

type observationLocatorCommitIdentity struct {
	ExternalID       string
	UpstreamIdentity string
	LocatorType      string
	LocatorValue     string
}

func validateObservationCommitSet(observations []sourceapplication.SourceObservationDTO, snapshot evidenceSnapshotRecord) error {
	seen := make(map[observationLocatorCommitIdentity]struct{}, len(observations))
	for _, observation := range observations {
		if err := validateSourceObservation(observation, snapshot); err != nil {
			return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
		}
		if snapshot.LifecycleState == string(domain.EvidenceLifecyclePending) &&
			(!snapshot.CollectionRunID.Valid || observation.CollectionRunID != snapshot.CollectionRunID.Int64) {
			return fmt.Errorf("%w: source observation collection run does not match first evidence capture", sharedrepository.ErrInvalidInput)
		}
		identity := observationLocatorCommitIdentity{
			ExternalID: observation.ExternalID, UpstreamIdentity: observation.UpstreamIdentity,
			LocatorType: observation.Evidence.LocatorType, LocatorValue: observation.Evidence.LocatorValue,
		}
		if _, duplicate := seen[identity]; duplicate {
			return evidenceConflict("observation commit set contains duplicate locator facts")
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func verifyCommittedObservationSet(ctx context.Context, executor evidenceSnapshotExecutor, snapshot evidenceSnapshotRecord, observations []sourceapplication.SourceObservationDTO) ([]sourceapplication.CommittedEvidenceReferenceDTO, error) {
	references := make([]sourceapplication.CommittedEvidenceReferenceDTO, 0, len(observations))
	for _, observation := range observations {
		storedObservation, err := scanSourceObservationRecord(executor.QueryRowContext(ctx, `
SELECT `+sourceObservationColumns+`
FROM source_observations
WHERE source_connection_id=$1 AND external_id=$2 AND upstream_identity=$3
FOR KEY SHARE`, observation.SourceConnectionID, observation.ExternalID, observation.UpstreamIdentity))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, evidenceConflict("available evidence snapshot has different observation facts")
		}
		if err != nil {
			return nil, databaserepository.MapError(err)
		}
		if !sameObservationContentFacts(storedObservation, observation) {
			return nil, evidenceConflict("available evidence snapshot has different observation facts")
		}

		storedLocator, err := scanEvidenceLocatorRecord(executor.QueryRowContext(ctx, `
SELECT `+evidenceLocatorColumns+`
FROM source_observation_evidences
WHERE source_observation_id=$1 AND evidence_snapshot_id=$2 AND locator_type=$3 AND locator_value=$4
FOR KEY SHARE`, storedObservation.ID, snapshot.ID, observation.Evidence.LocatorType, observation.Evidence.LocatorValue))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, evidenceConflict("available evidence snapshot has different locator facts")
		}
		if err != nil {
			return nil, databaserepository.MapError(err)
		}
		if !sameEvidenceLocatorFacts(storedLocator, snapshot.SourceConnectionID, storedObservation.ID, snapshot.ID, observation.Evidence) {
			return nil, evidenceConflict("available evidence snapshot has different locator facts")
		}
		if err := verifyObservationParties(ctx, executor, snapshot.SourceConnectionID, storedObservation.ID, observation.Parties); err != nil {
			return nil, err
		}
		references = append(references, storedLocator.committedReferenceDTO())
	}

	var persistedCount int
	if err := executor.QueryRowContext(ctx, `
SELECT count(*) FROM source_observation_evidences
WHERE source_connection_id=$1 AND evidence_snapshot_id=$2`, snapshot.SourceConnectionID, snapshot.ID).Scan(&persistedCount); err != nil {
		return nil, databaserepository.MapError(err)
	}
	if persistedCount != len(observations) {
		return nil, evidenceConflict("available evidence snapshot has a different observation commit set")
	}
	return references, nil
}

func appendEvidenceLocator(ctx context.Context, executor evidenceSnapshotExecutor, sourceConnectionID, observationID, snapshotID int64, reference sourceapplication.RawEvidenceReferenceDTO) (sourceapplication.CommittedEvidenceReferenceDTO, error) {
	if reference.Usage == "" {
		reference.Usage = "document_source"
	}
	stored, err := scanEvidenceLocatorRecord(executor.QueryRowContext(ctx, `
INSERT INTO source_observation_evidences (
  source_connection_id,source_observation_id,evidence_snapshot_id,usage,locator_type,locator_value,
  byte_start,byte_end,selected_payload_sha256,selector_version
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (source_observation_id,evidence_snapshot_id,locator_type,locator_value) DO NOTHING
RETURNING `+evidenceLocatorColumns,
		sourceConnectionID, observationID, snapshotID, reference.Usage, reference.LocatorType, reference.LocatorValue,
		nullInt64(reference.ByteStart), nullInt64(reference.ByteEnd), reference.SelectedPayloadSHA256, reference.SelectorVersion))
	if errors.Is(err, sql.ErrNoRows) {
		stored, err = scanEvidenceLocatorRecord(executor.QueryRowContext(ctx, `
SELECT `+evidenceLocatorColumns+`
FROM source_observation_evidences
WHERE source_observation_id=$1 AND evidence_snapshot_id=$2 AND locator_type=$3 AND locator_value=$4
FOR KEY SHARE`, observationID, snapshotID, reference.LocatorType, reference.LocatorValue))
	}
	if err != nil {
		return sourceapplication.CommittedEvidenceReferenceDTO{}, databaserepository.MapError(err)
	}
	if !sameEvidenceLocatorFacts(stored, sourceConnectionID, observationID, snapshotID, reference) {
		return sourceapplication.CommittedEvidenceReferenceDTO{}, evidenceConflict("observation locator identity has different immutable facts")
	}
	return stored.committedReferenceDTO(), nil
}

func evidenceConflict(message string) error {
	return fmt.Errorf("%w: %s", domain.ErrRawEvidenceConflict, message)
}
