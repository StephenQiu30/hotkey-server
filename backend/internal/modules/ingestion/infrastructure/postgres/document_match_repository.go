package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
	"unicode"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedpagination "github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type DocumentMatchRepository struct {
	runtime                     *database.Runtime
	cursorCodec                 *sharedpagination.Codec
	acceptedProjectionScheduler ingestionapplication.AcceptedDocumentMatchProjectionScheduler
}

var (
	_ ingestionapplication.RelevanceDecisionProfileReader = (*DocumentMatchRepository)(nil)
	_ ingestionapplication.DocumentMatchRepository        = (*DocumentMatchRepository)(nil)
	_ ingestionapplication.DocumentMatchReviewAuthorizer  = (*DocumentMatchRepository)(nil)
	_ ingestionapplication.DocumentMatchReader            = (*DocumentMatchRepository)(nil)
)

func NewDocumentMatchRepository(runtime *database.Runtime, schedulers ...ingestionapplication.AcceptedDocumentMatchProjectionScheduler) (*DocumentMatchRepository, error) {
	return NewDocumentMatchRepositoryWithCursorCodec(runtime, ingestionTestCursorCodec(runtime, "document-matches"), schedulers...)
}

func NewDocumentMatchRepositoryWithCursorCodec(runtime *database.Runtime, codec *sharedpagination.Codec, schedulers ...ingestionapplication.AcceptedDocumentMatchProjectionScheduler) (*DocumentMatchRepository, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("document match database runtime is required")
	}
	if codec == nil {
		return nil, fmt.Errorf("document match cursor codec is required")
	}
	if len(schedulers) > 1 || len(schedulers) == 1 && schedulers[0] == nil {
		return nil, fmt.Errorf("accepted document match projection scheduler is invalid")
	}
	repository := &DocumentMatchRepository{runtime: runtime, cursorCodec: codec}
	if len(schedulers) == 1 {
		repository.acceptedProjectionScheduler = schedulers[0]
	}
	return repository, nil
}

func (repository *DocumentMatchRepository) ReadRelevanceDecisionProfile(ctx context.Context, query ingestionapplication.ReadRelevanceDecisionProfileQuery) (ingestionapplication.RelevanceDecisionProfileDTO, error) {
	if repository == nil || repository.runtime == nil || query.RelevanceProfileID <= 0 {
		return ingestionapplication.RelevanceDecisionProfileDTO{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	var record relevanceDecisionProfileRecord
	err := repository.queryRow(ctx, `
SELECT id,version,evaluation_run_id,matching_algorithm_version,reranker_version,calibration_version,status,
       reject_threshold::float8,accept_threshold::float8,calibration_slope::float8,calibration_intercept::float8
FROM relevance_decision_profiles WHERE id=$1`, query.RelevanceProfileID).Scan(
		&record.ID, &record.Version, &record.EvaluationRunID, &record.MatchingAlgorithmVersion, &record.RerankerVersion,
		&record.CalibrationVersion, &record.Status, &record.RejectThreshold, &record.AcceptThreshold,
		&record.CalibrationSlope, &record.CalibrationIntercept,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.RelevanceDecisionProfileDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return ingestionapplication.RelevanceDecisionProfileDTO{}, databaserepository.MapError(err)
	}
	result := relevanceDecisionProfileDTO(record)
	profile := ingestiondomain.RelevanceDecisionProfile{
		ID: result.ID, Version: result.Version, EvaluationRunID: result.EvaluationRunID, MatchingAlgorithmVersion: result.MatchingAlgorithmVersion,
		RerankerVersion: result.RerankerVersion, CalibrationVersion: result.CalibrationVersion,
		Status: ingestiondomain.RelevanceProfileStatus(result.Status), RejectThreshold: result.RejectThreshold,
		AcceptThreshold: result.AcceptThreshold, CalibrationSlope: result.CalibrationSlope,
		CalibrationIntercept: result.CalibrationIntercept,
	}
	if err := profile.Validate(); err != nil {
		return ingestionapplication.RelevanceDecisionProfileDTO{}, fmt.Errorf("stored relevance decision profile: %w", err)
	}
	return result, nil
}

func (repository *DocumentMatchRepository) PersistAutomaticDocumentMatches(ctx context.Context, commands []ingestionapplication.PersistAutomaticDocumentMatchCommand) ([]ingestionapplication.DocumentMatchDecisionDTO, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if len(commands) == 0 {
		return []ingestionapplication.DocumentMatchDecisionDTO{}, nil
	}
	if len(commands) > ingestionapplication.FusedRecallLimit || validateAutomaticDocumentMatchBatch(commands) != nil {
		return nil, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	result := make([]ingestionapplication.DocumentMatchDecisionDTO, 0, len(commands))
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		for _, command := range commands {
			stored, err := persistAutomaticDocumentMatch(transactionCtx, transaction.SQL, command)
			if err != nil {
				return err
			}
			result = append(result, stored)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func persistAutomaticDocumentMatch(ctx context.Context, executor *sql.Tx, command ingestionapplication.PersistAutomaticDocumentMatchCommand) (ingestionapplication.DocumentMatchDecisionDTO, error) {
	reasons, err := json.Marshal(command.ReasonCodes)
	if err != nil {
		return ingestionapplication.DocumentMatchDecisionDTO{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	var id int64
	err = executor.QueryRowContext(ctx, `
INSERT INTO document_match_decisions (
  monitor_id,monitor_version_id,compiled_profile_id,document_version_id,relevance_profile_id,
  matching_algorithm_version,reranker_version,calibration_version,rrf_score,relevance_probability,
  decision,degraded,reason_codes,input_hash,decided_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,
          ARRAY(SELECT jsonb_array_elements_text($13::jsonb)),$14,$15)
ON CONFLICT (monitor_version_id,document_version_id,matching_algorithm_version) DO NOTHING
RETURNING id`, command.MonitorID, command.MonitorVersionID, command.CompiledProfileID,
		command.DocumentVersionID, command.RelevanceProfileID, command.MatchingAlgorithmVersion,
		command.RerankerVersion, command.CalibrationVersion, command.RRFScore, optionalDocumentMatchFloat(command.RelevanceProbability),
		command.Decision, command.Degraded, string(reasons), command.InputHash, command.DecidedAt.UTC()).Scan(&id)
	created := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.DocumentMatchDecisionDTO{}, databaserepository.MapError(err)
	}
	if created {
		for ordinal, signal := range command.Signals {
			if _, err := executor.ExecContext(ctx, `
INSERT INTO document_match_recall_signals
  (match_decision_id,ordinal,channel,rank,raw_score,algorithm_version)
VALUES ($1,$2,$3,$4,$5,$6)`, id, ordinal, signal.Channel, signal.Rank, signal.RawScore, signal.AlgorithmVersion); err != nil {
				return ingestionapplication.DocumentMatchDecisionDTO{}, databaserepository.MapError(err)
			}
		}
	}
	stored, err := readDocumentMatchDecision(ctx, executor, command.MonitorVersionID, command.DocumentVersionID, command.MatchingAlgorithmVersion)
	if err != nil {
		return ingestionapplication.DocumentMatchDecisionDTO{}, err
	}
	if !sameAutomaticDocumentMatch(stored, command) {
		return ingestionapplication.DocumentMatchDecisionDTO{}, sharedrepository.ErrConflict
	}
	return stored, nil
}

func readDocumentMatchDecision(ctx context.Context, executor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, monitorVersionID, documentVersionID int64, algorithmVersion string) (ingestionapplication.DocumentMatchDecisionDTO, error) {
	var record documentMatchDecisionRecord
	err := executor.QueryRowContext(ctx, `
SELECT id,monitor_id,monitor_version_id,compiled_profile_id,document_version_id,relevance_profile_id,
       matching_algorithm_version,reranker_version,calibration_version,rrf_score::float8,
       relevance_probability::float8,decision,degraded,array_to_json(reason_codes)::text,btrim(input_hash),decided_at
FROM document_match_decisions
WHERE monitor_version_id=$1 AND document_version_id=$2 AND matching_algorithm_version=$3`, monitorVersionID, documentVersionID, algorithmVersion).Scan(
		&record.ID, &record.MonitorID, &record.MonitorVersionID, &record.CompiledProfileID,
		&record.DocumentVersionID, &record.RelevanceProfileID, &record.MatchingAlgorithmVersion,
		&record.RerankerVersion, &record.CalibrationVersion, &record.RRFScore,
		&record.RelevanceProbability, &record.Decision, &record.Degraded, &record.ReasonCodesJSON,
		&record.InputHash, &record.DecidedAt,
	)
	if err != nil {
		return ingestionapplication.DocumentMatchDecisionDTO{}, databaserepository.MapError(err)
	}
	rows, err := executor.QueryContext(ctx, `
SELECT channel,rank,raw_score::float8,algorithm_version
FROM document_match_recall_signals WHERE match_decision_id=$1 ORDER BY ordinal`, record.ID)
	if err != nil {
		return ingestionapplication.DocumentMatchDecisionDTO{}, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	signals := []ingestionapplication.DocumentMatchSignalDTO{}
	for rows.Next() {
		var signal ingestionapplication.DocumentMatchSignalDTO
		if err := rows.Scan(&signal.Channel, &signal.Rank, &signal.RawScore, &signal.AlgorithmVersion); err != nil {
			return ingestionapplication.DocumentMatchDecisionDTO{}, databaserepository.MapError(err)
		}
		signals = append(signals, signal)
	}
	if err := rows.Err(); err != nil {
		return ingestionapplication.DocumentMatchDecisionDTO{}, databaserepository.MapError(err)
	}
	return record.dto(signals)
}

func (repository *DocumentMatchRepository) AppendDocumentMatchOverride(ctx context.Context, command ingestionapplication.AppendDocumentMatchOverrideCommand) (ingestionapplication.DocumentMatchOverrideDTO, bool, error) {
	if repository == nil || repository.runtime == nil || !validDocumentMatchOverrideCommand(command) {
		return ingestionapplication.DocumentMatchOverrideDTO{}, false, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	var stored ingestionapplication.DocumentMatchOverrideDTO
	reused := false
	err := repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		var authorizedActorID int64
		if err := transaction.SQL.QueryRowContext(transactionCtx, `
SELECT actor.id
FROM users AS actor
JOIN monitors AS monitor ON monitor.id=$2 AND monitor.deleted_at IS NULL
JOIN document_match_decisions AS decision ON decision.id=$3 AND decision.monitor_id=monitor.id
WHERE actor.id=$1 AND actor.status='active' AND actor.deleted_at IS NULL
  AND actor.role IN ('admin','editor')
FOR SHARE OF actor,monitor,decision`, command.ActorUserID, command.MonitorID, command.MatchDecisionID).Scan(&authorizedActorID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ingestionapplication.ErrDocumentMatchAuthorizationDenied
			}
			return databaserepository.MapError(err)
		}
		existing, err := readDocumentMatchOverrideByKey(transactionCtx, transaction.SQL, command.IdempotencyKey)
		if err == nil {
			var fingerprint string
			if err := transaction.SQL.QueryRowContext(transactionCtx, `SELECT btrim(command_fingerprint) FROM document_match_overrides WHERE id=$1`, existing.ID).Scan(&fingerprint); err != nil {
				return databaserepository.MapError(err)
			}
			if fingerprint != command.CommandFingerprint {
				return sharedrepository.ErrConflict
			}
			stored, reused = existing, true
			return repository.scheduleAcceptedOverrideProjection(transactionCtx, stored)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return databaserepository.MapError(err)
		}
		var monitorID, monitorVersionID, documentVersionID, sequence int64
		var effectiveDecision string
		err = transaction.SQL.QueryRowContext(transactionCtx, `
SELECT decision.id,decision.monitor_id,decision.monitor_version_id,decision.document_version_id,
       COALESCE(latest.sequence_no,0),COALESCE(latest.decision,decision.decision)
FROM document_match_decisions AS decision
LEFT JOIN LATERAL (
  SELECT sequence_no,decision FROM document_match_overrides
  WHERE match_decision_id=decision.id ORDER BY sequence_no DESC LIMIT 1
) AS latest ON true
WHERE decision.id=$1 AND decision.monitor_id=$2
FOR UPDATE OF decision`, command.MatchDecisionID, command.MonitorID).Scan(
			new(int64), &monitorID, &monitorVersionID, &documentVersionID, &sequence, &effectiveDecision,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return sharedrepository.ErrNotFound
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		if sequence != command.ExpectedSequence {
			return sharedrepository.ErrConflict
		}
		var record documentMatchOverrideRecord
		err = transaction.SQL.QueryRowContext(transactionCtx, `
INSERT INTO document_match_overrides (
  match_decision_id,sequence_no,monitor_id,monitor_version_id,document_version_id,
  previous_effective_decision,decision,reason_code,note,actor_user_id,
  idempotency_key,command_fingerprint,created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id,match_decision_id,sequence_no,monitor_id,monitor_version_id,document_version_id,
          previous_effective_decision,decision,reason_code,note,actor_user_id,created_at`,
			command.MatchDecisionID, sequence+1, monitorID, monitorVersionID, documentVersionID,
			effectiveDecision, command.Decision, command.ReasonCode, command.Note, command.ActorUserID,
			command.IdempotencyKey, command.CommandFingerprint, command.DecidedAt.UTC()).Scan(
			&record.ID, &record.MatchDecisionID, &record.Sequence, &record.MonitorID,
			&record.MonitorVersionID, &record.DocumentVersionID, &record.PreviousEffectiveDecision,
			&record.Decision, &record.ReasonCode, &record.Note, &record.ActorUserID, &record.CreatedAt,
		)
		if err != nil {
			return databaserepository.MapError(err)
		}
		stored = documentMatchOverrideDTO(record)
		return repository.scheduleAcceptedOverrideProjection(transactionCtx, stored)
	})
	if err != nil {
		return ingestionapplication.DocumentMatchOverrideDTO{}, false, err
	}
	return stored, reused, nil
}

func (repository *DocumentMatchRepository) scheduleAcceptedOverrideProjection(ctx context.Context, override ingestionapplication.DocumentMatchOverrideDTO) error {
	if override.Decision != "accepted" || repository.acceptedProjectionScheduler == nil {
		return nil
	}
	scheduled, err := repository.acceptedProjectionScheduler.ScheduleAcceptedDocumentMatchProjection(ctx,
		ingestionapplication.ScheduleAcceptedDocumentMatchProjectionCommand{
			DocumentMatchDecisionID: override.MatchDecisionID,
			DocumentVersionID:       override.DocumentVersionID,
			EffectiveSequence:       override.Sequence,
		})
	if err != nil {
		return fmt.Errorf("schedule accepted document match projection: %w", err)
	}
	if scheduled.DocumentMatchDecisionID != override.MatchDecisionID || scheduled.DocumentVersionID != override.DocumentVersionID ||
		scheduled.EffectiveSequence != override.Sequence || scheduled.JobID <= 0 {
		return ingestionapplication.ErrInvalidDocumentMatchContract
	}
	return nil
}

func (repository *DocumentMatchRepository) AuthorizeDocumentMatchReview(ctx context.Context, query ingestionapplication.AuthorizeDocumentMatchReviewQuery) error {
	if repository == nil || repository.runtime == nil || query.ActorUserID <= 0 || query.MonitorID <= 0 || query.MatchDecisionID <= 0 {
		return ingestionapplication.ErrDocumentMatchAuthorizationDenied
	}
	var allowed bool
	err := repository.queryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM users AS actor
  JOIN monitors AS monitor ON monitor.id=$2 AND monitor.deleted_at IS NULL
  JOIN document_match_decisions AS decision ON decision.id=$3 AND decision.monitor_id=monitor.id
  WHERE actor.id=$1 AND actor.status='active' AND actor.deleted_at IS NULL
    AND actor.role IN ('admin','editor')
)`, query.ActorUserID, query.MonitorID, query.MatchDecisionID).Scan(&allowed)
	if err != nil {
		return databaserepository.MapError(err)
	}
	if !allowed {
		return ingestionapplication.ErrDocumentMatchAuthorizationDenied
	}
	return nil
}

func readDocumentMatchOverrideByKey(ctx context.Context, executor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, key string) (ingestionapplication.DocumentMatchOverrideDTO, error) {
	var record documentMatchOverrideRecord
	err := executor.QueryRowContext(ctx, `
SELECT id,match_decision_id,sequence_no,monitor_id,monitor_version_id,document_version_id,
       previous_effective_decision,decision,reason_code,note,actor_user_id,created_at
FROM document_match_overrides WHERE idempotency_key=$1 FOR UPDATE`, key).Scan(
		&record.ID, &record.MatchDecisionID, &record.Sequence, &record.MonitorID,
		&record.MonitorVersionID, &record.DocumentVersionID, &record.PreviousEffectiveDecision,
		&record.Decision, &record.ReasonCode, &record.Note, &record.ActorUserID, &record.CreatedAt,
	)
	return documentMatchOverrideDTO(record), err
}

func validateAutomaticDocumentMatchBatch(commands []ingestionapplication.PersistAutomaticDocumentMatchCommand) error {
	first := commands[0]
	seen := make(map[int64]struct{}, len(commands))
	for _, command := range commands {
		decision := ingestiondomain.DocumentMatchDecision{
			ID: 1, MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID,
			CompiledProfileID: command.CompiledProfileID, DocumentVersionID: command.DocumentVersionID,
			RelevanceProfileID: command.RelevanceProfileID, MatchingAlgorithmVersion: command.MatchingAlgorithmVersion,
			RerankerVersion: command.RerankerVersion, CalibrationVersion: command.CalibrationVersion,
			InputHash: command.InputHash, RRFScore: command.RRFScore, RelevanceProbability: command.RelevanceProbability,
			Decision: ingestiondomain.MatchDecision(command.Decision), Degraded: command.Degraded,
			ReasonCodes: command.ReasonCodes, CreatedAt: command.DecidedAt,
		}
		if decision.Validate() != nil || command.DecidedAt.IsZero() || len(command.Signals) > 3 ||
			command.MonitorID != first.MonitorID || command.MonitorVersionID != first.MonitorVersionID ||
			command.CompiledProfileID != first.CompiledProfileID || command.RelevanceProfileID != first.RelevanceProfileID ||
			command.MatchingAlgorithmVersion != first.MatchingAlgorithmVersion {
			return ingestionapplication.ErrInvalidDocumentMatchContract
		}
		if _, duplicate := seen[command.DocumentVersionID]; duplicate {
			return ingestionapplication.ErrInvalidDocumentMatchContract
		}
		seen[command.DocumentVersionID] = struct{}{}
		channels := make(map[string]struct{}, len(command.Signals))
		for _, signal := range command.Signals {
			if (signal.Channel != "lexical" && signal.Channel != "semantic" && signal.Channel != "structured") ||
				signal.Rank < 1 || signal.Rank > 100 || strings.TrimSpace(signal.AlgorithmVersion) == "" ||
				math.IsNaN(signal.RawScore) || math.IsInf(signal.RawScore, 0) {
				return ingestionapplication.ErrInvalidDocumentMatchContract
			}
			if _, duplicate := channels[signal.Channel]; duplicate {
				return ingestionapplication.ErrInvalidDocumentMatchContract
			}
			channels[signal.Channel] = struct{}{}
		}
	}
	return nil
}

func sameAutomaticDocumentMatch(stored ingestionapplication.DocumentMatchDecisionDTO, command ingestionapplication.PersistAutomaticDocumentMatchCommand) bool {
	return stored.MonitorID == command.MonitorID && stored.MonitorVersionID == command.MonitorVersionID &&
		stored.CompiledProfileID == command.CompiledProfileID && stored.DocumentVersionID == command.DocumentVersionID &&
		stored.RelevanceProfileID == command.RelevanceProfileID && stored.MatchingAlgorithmVersion == command.MatchingAlgorithmVersion &&
		stored.RerankerVersion == command.RerankerVersion && stored.CalibrationVersion == command.CalibrationVersion &&
		stored.InputHash == command.InputHash && stored.RRFScore == command.RRFScore &&
		sameOptionalDocumentMatchFloat(stored.RelevanceProbability, command.RelevanceProbability) && stored.Decision == command.Decision &&
		stored.Degraded == command.Degraded && reflect.DeepEqual(stored.ReasonCodes, command.ReasonCodes) &&
		reflect.DeepEqual(stored.Signals, command.Signals)
}

func validDocumentMatchOverrideCommand(command ingestionapplication.AppendDocumentMatchOverrideCommand) bool {
	return command.ActorUserID > 0 && command.MonitorID > 0 && command.MatchDecisionID > 0 &&
		command.ExpectedSequence >= 0 &&
		(command.Decision == "accepted" || command.Decision == "rejected") && validDocumentMatchReasonRecord(command.ReasonCode) &&
		validDocumentMatchIdempotencyRecord(command.IdempotencyKey) && validDocumentMatchNoteRecord(command.Note) &&
		validDocumentMatchHashRecord(command.CommandFingerprint) && command.DecidedAt.After(time.Time{})
}

func validDocumentMatchReasonRecord(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > 64 {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			index > 0 && (character == '_' || character == ':' || character == '-') {
			continue
		}
		return false
	}
	return true
}

func validDocumentMatchIdempotencyRecord(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > 128 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validDocumentMatchNoteRecord(value string) bool {
	if len([]byte(value)) > 8000 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return false
		}
	}
	return true
}

func validDocumentMatchHashRecord(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}

func sameOptionalDocumentMatchFloat(left, right *float64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func optionalDocumentMatchFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func (repository *DocumentMatchRepository) withTransaction(ctx context.Context, operation func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return operation(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, operation)
}

func (repository *DocumentMatchRepository) queryRow(ctx context.Context, query string, arguments ...any) *sql.Row {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryRowContext(ctx, query, arguments...)
	}
	return repository.runtime.SQL.QueryRowContext(ctx, query, arguments...)
}
