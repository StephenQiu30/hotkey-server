package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedpagination "github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func (repository *DocumentMatchRepository) ListDocumentMatches(ctx context.Context, query ingestionapplication.ListDocumentMatchesQuery) (ingestionapplication.DocumentMatchPageResult, error) {
	if repository == nil || repository.runtime == nil || query.ActorUserID <= 0 || query.MonitorID <= 0 || query.Limit <= 0 ||
		query.Limit > ingestionapplication.MaximumDocumentMatchPageSize {
		return ingestionapplication.DocumentMatchPageResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("document-match-list-v1:%d:%s", query.MonitorID, query.EffectiveDecision))))
	cursor, err := sharedpagination.Decode(query.Cursor, "document_match_decision_id", true, fingerprint)
	if err != nil {
		return ingestionapplication.DocumentMatchPageResult{}, fmt.Errorf("%w: document match cursor", sharedrepository.ErrInvalidInput)
	}
	result := ingestionapplication.DocumentMatchPageResult{Items: []ingestionapplication.DocumentMatchListItemDTO{}}
	err = repository.withTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		if err := authorizeDocumentMatchList(transactionCtx, transaction.SQL, query.ActorUserID, query.MonitorID); err != nil {
			return err
		}
		rows, err := transaction.SQL.QueryContext(transactionCtx, `
SELECT decision.id,decision.monitor_id,decision.monitor_version_id,decision.compiled_profile_id,
       decision.document_version_id,decision.relevance_profile_id,decision.matching_algorithm_version,
       decision.reranker_version,decision.calibration_version,decision.rrf_score::float8,
       decision.relevance_probability::float8,decision.decision,decision.degraded,
       array_to_json(decision.reason_codes)::text,btrim(decision.input_hash),decision.decided_at,
       COALESCE(latest.decision,decision.decision),COALESCE(latest.sequence_no,0),
       COALESCE((
           SELECT jsonb_agg(jsonb_build_object(
               'channel',signal.channel,'rank',signal.rank,'raw_score',signal.raw_score::float8,
               'algorithm_version',signal.algorithm_version
           ) ORDER BY signal.ordinal)
           FROM document_match_recall_signals AS signal WHERE signal.match_decision_id=decision.id
       ),'[]'::jsonb)::text
FROM document_match_decisions AS decision
LEFT JOIN LATERAL (
    SELECT sequence_no,decision FROM document_match_overrides
    WHERE match_decision_id=decision.id ORDER BY sequence_no DESC LIMIT 1
) AS latest ON true
WHERE decision.monitor_id=$1 AND ($2::bigint=0 OR decision.id<$2)
  AND ($3='' OR COALESCE(latest.decision,decision.decision)=$3)
ORDER BY decision.id DESC
LIMIT $4`, query.MonitorID, cursor.ID, query.EffectiveDecision, query.Limit+1)
		if err != nil {
			return databaserepository.MapError(err)
		}
		defer rows.Close()
		for rows.Next() {
			var record documentMatchDecisionRecord
			var effectiveDecision, signalsJSON string
			var overrideSequence int64
			if err := rows.Scan(
				&record.ID, &record.MonitorID, &record.MonitorVersionID, &record.CompiledProfileID,
				&record.DocumentVersionID, &record.RelevanceProfileID, &record.MatchingAlgorithmVersion,
				&record.RerankerVersion, &record.CalibrationVersion, &record.RRFScore,
				&record.RelevanceProbability, &record.Decision, &record.Degraded, &record.ReasonCodesJSON,
				&record.InputHash, &record.DecidedAt, &effectiveDecision, &overrideSequence, &signalsJSON,
			); err != nil {
				return databaserepository.MapError(err)
			}
			signals, err := documentMatchSignalsFromJSON(signalsJSON)
			if err != nil {
				return err
			}
			automatic, err := record.dto(signals)
			if err != nil {
				return err
			}
			result.Items = append(result.Items, ingestionapplication.DocumentMatchListItemDTO{
				Automatic: automatic, EffectiveDecision: effectiveDecision, OverrideSequence: overrideSequence,
			})
		}
		if err := rows.Err(); err != nil {
			return databaserepository.MapError(err)
		}
		return nil
	})
	if err != nil {
		return ingestionapplication.DocumentMatchPageResult{}, err
	}
	if len(result.Items) > query.Limit {
		result.Items = result.Items[:query.Limit]
		result.NextCursor, err = sharedpagination.Encode(
			"document_match_decision_id", true, fingerprint, result.Items[len(result.Items)-1].Automatic.ID,
		)
		if err != nil {
			return ingestionapplication.DocumentMatchPageResult{}, fmt.Errorf("%w: encode document match cursor", sharedrepository.ErrInvalidInput)
		}
	}
	return result, nil
}

func authorizeDocumentMatchList(ctx context.Context, transaction *sql.Tx, actorUserID, monitorID int64) error {
	var authorizedActorID int64
	err := transaction.QueryRowContext(ctx, `
SELECT actor.id
FROM users AS actor
JOIN monitors AS monitor ON monitor.id=$2 AND monitor.deleted_at IS NULL
WHERE actor.id=$1 AND actor.status='active' AND actor.deleted_at IS NULL
  AND actor.role IN ('viewer','editor','admin')
FOR SHARE OF actor,monitor`, actorUserID, monitorID).Scan(&authorizedActorID)
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.ErrDocumentMatchAuthorizationDenied
	}
	if err != nil {
		return databaserepository.MapError(err)
	}
	return nil
}
