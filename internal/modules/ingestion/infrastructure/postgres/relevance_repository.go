package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// RelevanceRepository owns immutable relevance results and the human feedback
// that may be collected against them. It deliberately keeps the monitor and
// Content joins private: callers cannot use this adapter as a general-purpose
// monitor or identity repository.
type RelevanceRepository struct{ runtime *database.Runtime }

var _ ingestiondomain.RelevanceRepository = (*RelevanceRepository)(nil)

func NewRelevanceRepository(runtime *database.Runtime) *RelevanceRepository {
	return &RelevanceRepository{runtime: runtime}
}

func (repository *RelevanceRepository) UpsertSnapshot(ctx context.Context, input ingestiondomain.RelevanceSnapshotInput) (ingestiondomain.RelevanceSnapshot, bool, error) {
	if !repository.available() {
		return ingestiondomain.RelevanceSnapshot{}, false, sharedrepository.ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return ingestiondomain.RelevanceSnapshot{}, false, fmt.Errorf("%w: relevance snapshot: %v", sharedrepository.ErrInvalidInput, err)
	}
	reasonCodes, err := json.Marshal(input.ReasonCodes)
	if err != nil {
		return ingestiondomain.RelevanceSnapshot{}, false, fmt.Errorf("%w: relevance reason codes", sharedrepository.ErrInvalidInput)
	}
	recallPaths, err := json.Marshal(input.RecallPaths)
	if err != nil {
		return ingestiondomain.RelevanceSnapshot{}, false, fmt.Errorf("%w: relevance recall paths", sharedrepository.ErrInvalidInput)
	}

	var stored ingestiondomain.RelevanceSnapshot
	created := false
	err = repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := ensureSnapshotReferences(ctx, transaction.SQL, input); err != nil {
			return err
		}

		var snapshotID int64
		err := transaction.SQL.QueryRowContext(ctx, `
INSERT INTO monitor_matches (
    monitor_id, monitor_config_version_id, content_id,
    rule_score, semantic_score, llm_score, final_score, decision,
    reason_codes, explanation, algorithm_version,
    input_hash, scoring_version, recall_paths, degraded, decision_origin,
    embedding_model_profile_id, embedding_model_profile_version, embedding_model_version
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    ARRAY(SELECT jsonb_array_elements_text($9::jsonb)), $10::jsonb, $11,
    $12, $13, ARRAY(SELECT jsonb_array_elements_text($14::jsonb)), $15, $16,
    $17, $18, $19
)
ON CONFLICT (monitor_config_version_id, content_id, input_hash, scoring_version) DO NOTHING
RETURNING id`,
			input.MonitorID, input.MonitorConfigVersionID, input.ContentID,
			input.RuleScore, optionalFloat(input.SemanticScore), optionalFloat(input.LLMScore), input.FinalScore, string(input.Decision),
			string(reasonCodes), string(input.Explanation), input.ScoringVersion,
			input.InputHash, input.ScoringVersion, string(recallPaths), input.Degraded, string(input.DecisionOrigin),
			optionalInt64(input.EmbeddingModelProfileID), optionalInt64(input.EmbeddingModelProfileVersion), optionalString(input.EmbeddingModelVersion),
		).Scan(&snapshotID)
		switch {
		case err == nil:
			created = true
		case errors.Is(err, sql.ErrNoRows):
			stored, err = selectSnapshotByUnique(ctx, transaction.SQL, input.MonitorConfigVersionID, input.ContentID, input.InputHash, input.ScoringVersion)
			if err != nil {
				return err
			}
			return nil
		default:
			return sharedrepository.MapError(err)
		}

		stored, err = selectSnapshotByID(ctx, transaction.SQL, snapshotID)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return ingestiondomain.RelevanceSnapshot{}, false, err
	}
	return stored, created, nil
}

func (repository *RelevanceRepository) ApplySuccessfulReview(ctx context.Context, input ingestiondomain.SuccessfulReviewInput) (ingestiondomain.RelevanceSnapshot, error) {
	if !repository.available() {
		return ingestiondomain.RelevanceSnapshot{}, sharedrepository.ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return ingestiondomain.RelevanceSnapshot{}, fmt.Errorf("%w: successful relevance review: %v", sharedrepository.ErrInvalidInput, err)
	}
	reasonCodes, err := json.Marshal(input.ReasonCodes)
	if err != nil {
		return ingestiondomain.RelevanceSnapshot{}, fmt.Errorf("%w: relevance review reason codes", sharedrepository.ErrInvalidInput)
	}
	var stored ingestiondomain.RelevanceSnapshot
	err = repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := ensureReviewRun(ctx, transaction.SQL, input); err != nil {
			return err
		}
		stored, err = scanSnapshot(transaction.SQL.QueryRowContext(ctx, `
UPDATE monitor_matches
SET llm_score = $1,
    final_score = $2,
    decision = $3,
    reason_codes = ARRAY(SELECT jsonb_array_elements_text($4::jsonb)),
    decision_origin = 'ai',
    review_ai_run_id = $5::bigint,
    explanation = jsonb_set(
        explanation,
        '{provenance}',
        COALESCE(explanation->'provenance', '{}'::jsonb) || jsonb_build_object('review_ai_run_id', $5::bigint),
        true
    ),
    version = version + 1,
    updated_at = now()
WHERE id = $6 AND version = $7 AND manual_locked = false
  AND decision = 'review' AND decision_origin = 'rule' AND review_ai_run_id IS NULL
RETURNING `+snapshotColumns("monitor_matches"),
			input.LLMScore, input.FinalScore, string(input.Decision), string(reasonCodes), input.ReviewAIRunID, input.SnapshotID, input.ExpectedVersion,
		))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: relevance snapshot changed or is not eligible for AI review", sharedrepository.ErrConflict)
		}
		if err != nil {
			return sharedrepository.MapError(err)
		}
		return nil
	})
	if err != nil {
		return ingestiondomain.RelevanceSnapshot{}, err
	}
	return stored, nil
}

func (repository *RelevanceRepository) ListLatestSnapshots(ctx context.Context, monitorID int64, query ingestiondomain.RelevanceSnapshotListQuery) (ingestiondomain.RelevanceSnapshotPage, error) {
	if !repository.available() {
		return ingestiondomain.RelevanceSnapshotPage{}, sharedrepository.ErrUnavailable
	}
	if monitorID <= 0 || query.Validate() != nil {
		return ingestiondomain.RelevanceSnapshotPage{}, fmt.Errorf("%w: relevance snapshot list", sharedrepository.ErrInvalidInput)
	}
	var decision any
	if query.Decision != nil {
		decision = string(*query.Decision)
	}
	var cursorScore any
	var cursorID int64
	if query.Cursor != nil {
		cursorScore = query.Cursor.FinalScore
		cursorID = query.Cursor.ID
	}

	rows, err := repository.queryRows(ctx, `
WITH latest AS (
    SELECT DISTINCT ON (match.monitor_config_version_id, match.content_id) match.*
    FROM monitor_matches AS match
    JOIN contents AS content ON content.id = match.content_id
    WHERE match.monitor_id = $1
      AND content.content_status = 'active'
      AND content.deleted_at IS NULL
    ORDER BY match.monitor_config_version_id, match.content_id, match.created_at DESC, match.id DESC
)
SELECT `+snapshotColumns("match")+`
FROM latest AS match
WHERE ($2::varchar IS NULL OR match.decision = $2)
  AND ($3::numeric IS NULL OR (match.final_score, match.id) < ($3, $4))
ORDER BY match.final_score DESC, match.id DESC
LIMIT $5`, monitorID, decision, cursorScore, cursorID, query.Limit+1)
	if err != nil {
		return ingestiondomain.RelevanceSnapshotPage{}, sharedrepository.MapError(err)
	}
	defer rows.Close()

	page := ingestiondomain.RelevanceSnapshotPage{Items: make([]ingestiondomain.RelevanceSnapshot, 0, query.Limit)}
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return ingestiondomain.RelevanceSnapshotPage{}, sharedrepository.MapError(err)
		}
		if len(page.Items) == query.Limit {
			page.Next = &ingestiondomain.RelevanceSnapshotCursor{FinalScore: page.Items[len(page.Items)-1].FinalScore, ID: page.Items[len(page.Items)-1].ID}
			break
		}
		page.Items = append(page.Items, snapshot)
	}
	if err := rows.Err(); err != nil {
		return ingestiondomain.RelevanceSnapshotPage{}, sharedrepository.MapError(err)
	}
	return page, nil
}

func (repository *RelevanceRepository) UpsertFeedback(ctx context.Context, input ingestiondomain.RelevanceFeedbackInput) (ingestiondomain.RelevanceFeedback, error) {
	if !repository.available() {
		return ingestiondomain.RelevanceFeedback{}, sharedrepository.ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return ingestiondomain.RelevanceFeedback{}, fmt.Errorf("%w: relevance feedback: %v", sharedrepository.ErrInvalidInput, err)
	}
	var stored ingestiondomain.RelevanceFeedback
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := ensureFeedbackReferences(ctx, transaction.SQL, input); err != nil {
			return err
		}

		var existingID, existingVersion int64
		err := transaction.SQL.QueryRowContext(ctx, `
SELECT id, version
FROM monitor_match_feedbacks
WHERE monitor_config_version_id = $1 AND content_id = $2 AND actor_user_id = $3
FOR UPDATE`, input.MonitorConfigVersionID, input.ContentID, input.ActorUserID).Scan(&existingID, &existingVersion)
		switch {
		case err == nil:
			if input.ExpectedVersion == nil || *input.ExpectedVersion != existingVersion {
				return fmt.Errorf("%w: relevance feedback version", sharedrepository.ErrConflict)
			}
			stored, err = updateFeedback(ctx, transaction.SQL, existingID, existingVersion, input)
			return err
		case errors.Is(err, sql.ErrNoRows):
			if input.ExpectedVersion != nil {
				return fmt.Errorf("%w: relevance feedback does not exist", sharedrepository.ErrConflict)
			}
			stored, err = insertFeedback(ctx, transaction.SQL, input)
			return err
		default:
			return sharedrepository.MapError(err)
		}
	})
	if err != nil {
		return ingestiondomain.RelevanceFeedback{}, err
	}
	return stored, nil
}

func (repository *RelevanceRepository) UpsertPendingSuggestion(ctx context.Context, input ingestiondomain.RelevanceSuggestionInput) (ingestiondomain.RelevanceSuggestion, bool, error) {
	if !repository.available() {
		return ingestiondomain.RelevanceSuggestion{}, false, sharedrepository.ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return ingestiondomain.RelevanceSuggestion{}, false, fmt.Errorf("%w: relevance suggestion: %v", sharedrepository.ErrInvalidInput, err)
	}
	input.Value = strings.TrimSpace(input.Value)
	var stored ingestiondomain.RelevanceSuggestion
	created := false
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := ensurePublishedMonitorConfig(ctx, transaction.SQL, input.MonitorID, input.MonitorConfigVersionID); err != nil {
			return err
		}
		var wasCreated bool
		var reviewedByUserID sql.NullInt64
		err := transaction.SQL.QueryRowContext(ctx, `
INSERT INTO monitor_feedback_suggestions (
    monitor_id, monitor_config_version_id, suggestion_type, value, support_count
)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (monitor_config_version_id, suggestion_type, value) WHERE status = 'pending' DO UPDATE
SET support_count = EXCLUDED.support_count, version = monitor_feedback_suggestions.version + 1, updated_at = now()
RETURNING id, version, monitor_id, monitor_config_version_id, suggestion_type, value,
          support_count, status, reviewed_by_user_id, created_at, updated_at, (xmax = 0)`,
			input.MonitorID, input.MonitorConfigVersionID, string(input.SuggestionType), input.Value, input.SupportCount,
		).Scan(
			&stored.ID, &stored.Version, &stored.MonitorID, &stored.MonitorConfigVersionID, &stored.SuggestionType, &stored.Value,
			&stored.SupportCount, &stored.Status, &reviewedByUserID, &stored.CreatedAt, &stored.UpdatedAt, &wasCreated,
		)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		created = wasCreated
		stored.ReviewedByUserID = optionalInt64Value(reviewedByUserID)
		stored.CreatedAt = stored.CreatedAt.UTC()
		stored.UpdatedAt = stored.UpdatedAt.UTC()
		return nil
	})
	if err != nil {
		return ingestiondomain.RelevanceSuggestion{}, false, err
	}
	return stored, created, nil
}

func (repository *RelevanceRepository) ReviewSuggestion(ctx context.Context, suggestionID, reviewerID, expectedVersion int64, status ingestiondomain.SuggestionStatus) (ingestiondomain.RelevanceSuggestion, error) {
	if !repository.available() {
		return ingestiondomain.RelevanceSuggestion{}, sharedrepository.ErrUnavailable
	}
	if suggestionID <= 0 || reviewerID <= 0 || expectedVersion <= 0 || (status != ingestiondomain.SuggestionStatusApproved && status != ingestiondomain.SuggestionStatusRejected) {
		return ingestiondomain.RelevanceSuggestion{}, fmt.Errorf("%w: relevance suggestion review", sharedrepository.ErrInvalidInput)
	}
	var stored ingestiondomain.RelevanceSuggestion
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		var active bool
		if err := transaction.SQL.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND status = 'active' AND deleted_at IS NULL)`, reviewerID).Scan(&active); err != nil {
			return sharedrepository.MapError(err)
		}
		if !active {
			return fmt.Errorf("%w: active reviewer %d", sharedrepository.ErrNotFound, reviewerID)
		}
		var reviewedByUserID sql.NullInt64
		err := transaction.SQL.QueryRowContext(ctx, `
UPDATE monitor_feedback_suggestions
SET status = $1, reviewed_by_user_id = $2, version = version + 1, updated_at = now()
WHERE id = $3 AND version = $4 AND status = 'pending'
RETURNING id, version, monitor_id, monitor_config_version_id, suggestion_type, value,
          support_count, status, reviewed_by_user_id, created_at, updated_at`,
			string(status), reviewerID, suggestionID, expectedVersion,
		).Scan(
			&stored.ID, &stored.Version, &stored.MonitorID, &stored.MonitorConfigVersionID, &stored.SuggestionType, &stored.Value,
			&stored.SupportCount, &stored.Status, &reviewedByUserID, &stored.CreatedAt, &stored.UpdatedAt,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: pending relevance suggestion", sharedrepository.ErrConflict)
		}
		if err != nil {
			return sharedrepository.MapError(err)
		}
		stored.ReviewedByUserID = optionalInt64Value(reviewedByUserID)
		stored.CreatedAt = stored.CreatedAt.UTC()
		stored.UpdatedAt = stored.UpdatedAt.UTC()
		return nil
	})
	if err != nil {
		return ingestiondomain.RelevanceSuggestion{}, err
	}
	return stored, nil
}

func ensureSnapshotReferences(ctx context.Context, executor queryRowExecutor, input ingestiondomain.RelevanceSnapshotInput) error {
	if err := ensurePublishedMonitorConfigAndContent(ctx, executor, input.MonitorID, input.MonitorConfigVersionID, input.ContentID); err != nil {
		return err
	}
	if input.EmbeddingModelProfileID == nil {
		return nil
	}
	var version int64
	var modelVersion string
	var taskType string
	err := executor.QueryRowContext(ctx, `
SELECT version, model_version, task_type
FROM ai_model_profiles
WHERE id = $1`, *input.EmbeddingModelProfileID).Scan(&version, &modelVersion, &taskType)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: embedding model profile", sharedrepository.ErrInvalidInput)
	}
	if err != nil {
		return sharedrepository.MapError(err)
	}
	if taskType != "embedding" || version != *input.EmbeddingModelProfileVersion || modelVersion != *input.EmbeddingModelVersion {
		return fmt.Errorf("%w: stale or incompatible embedding provenance", sharedrepository.ErrInvalidInput)
	}
	return nil
}

func ensureReviewRun(ctx context.Context, executor queryRowExecutor, input ingestiondomain.SuccessfulReviewInput) error {
	var runInputHash, snapshotInputHash, structuredResult string
	err := executor.QueryRowContext(ctx, `
SELECT run.input_hash, match.input_hash, run.structured_result::text
FROM ai_runs AS run
JOIN monitor_matches AS match ON match.id = $2
WHERE run.id = $1 AND run.task_type = 'relevance_review' AND run.target_type = 'monitor_match'
  AND run.target_id = match.id AND run.status = 'succeeded'`, input.ReviewAIRunID, input.SnapshotID).Scan(&runInputHash, &snapshotInputHash, &structuredResult)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: relevance review run", sharedrepository.ErrInvalidInput)
	}
	if err != nil {
		return sharedrepository.MapError(err)
	}
	var output struct {
		Decision    ingestiondomain.MatchDecision `json:"decision"`
		Score       float64                       `json:"score"`
		ReasonCodes []string                      `json:"reason_codes"`
	}
	if runInputHash != snapshotInputHash || json.Unmarshal([]byte(structuredResult), &output) != nil ||
		output.Decision != input.Decision || output.Score != input.LLMScore || !sameStringSlices(output.ReasonCodes, input.ReasonCodes) {
		return fmt.Errorf("%w: relevance review provenance does not match its result", sharedrepository.ErrInvalidInput)
	}
	return nil
}

func ensureFeedbackReferences(ctx context.Context, executor queryRowExecutor, input ingestiondomain.RelevanceFeedbackInput) error {
	if err := ensureHistoricalMonitorConfigAndContent(ctx, executor, input.MonitorID, input.MonitorConfigVersionID, input.ContentID); err != nil {
		return err
	}
	var activeActor bool
	err := executor.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND status = 'active' AND deleted_at IS NULL)`, input.ActorUserID).Scan(&activeActor)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	if !activeActor {
		return fmt.Errorf("%w: active feedback actor %d", sharedrepository.ErrNotFound, input.ActorUserID)
	}
	if input.MonitorMatchID == nil {
		return nil
	}
	var matching bool
	err = executor.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM monitor_matches
    WHERE id = $1 AND monitor_id = $2 AND monitor_config_version_id = $3 AND content_id = $4
)`, *input.MonitorMatchID, input.MonitorID, input.MonitorConfigVersionID, input.ContentID).Scan(&matching)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	if !matching {
		return fmt.Errorf("%w: relevance snapshot", sharedrepository.ErrNotFound)
	}
	return nil
}

func ensureHistoricalMonitorConfigAndContent(ctx context.Context, executor queryRowExecutor, monitorID, configID, contentID int64) error {
	var configExists bool
	err := executor.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM monitor_config_versions
    WHERE id = $1 AND monitor_id = $2 AND state IN ('published', 'superseded')
)`, configID, monitorID).Scan(&configExists)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	if !configExists {
		return fmt.Errorf("%w: historical monitor configuration", sharedrepository.ErrNotFound)
	}
	var active bool
	err = executor.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM contents
    WHERE id = $1 AND content_status = 'active' AND deleted_at IS NULL
)`, contentID).Scan(&active)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	if !active {
		return fmt.Errorf("%w: active content %d", sharedrepository.ErrNotFound, contentID)
	}
	return nil
}

func ensurePublishedMonitorConfig(ctx context.Context, executor queryRowExecutor, monitorID, configID int64) error {
	var exists bool
	err := executor.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM monitors AS monitor
    JOIN monitor_config_versions AS config ON config.id = monitor.published_config_version_id
    WHERE monitor.id = $1 AND monitor.status = 'active'
      AND config.id = $2 AND config.monitor_id = monitor.id AND config.state = 'published'
)`, monitorID, configID).Scan(&exists)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	if !exists {
		return fmt.Errorf("%w: active published monitor configuration", sharedrepository.ErrNotFound)
	}
	return nil
}

func ensurePublishedMonitorConfigAndContent(ctx context.Context, executor queryRowExecutor, monitorID, configID, contentID int64) error {
	if err := ensurePublishedMonitorConfig(ctx, executor, monitorID, configID); err != nil {
		return err
	}
	var active bool
	err := executor.QueryRowContext(ctx, `
SELECT EXISTS(
    SELECT 1 FROM contents
    WHERE id = $1 AND content_status = 'active' AND deleted_at IS NULL
)`, contentID).Scan(&active)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	if !active {
		return fmt.Errorf("%w: active content %d", sharedrepository.ErrNotFound, contentID)
	}
	return nil
}

func insertFeedback(ctx context.Context, executor queryRowExecutor, input ingestiondomain.RelevanceFeedbackInput) (ingestiondomain.RelevanceFeedback, error) {
	return scanFeedback(executor.QueryRowContext(ctx, `
INSERT INTO monitor_match_feedbacks (
    monitor_id, monitor_config_version_id, content_id, monitor_match_id, actor_user_id, feedback_type
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, version, monitor_id, monitor_config_version_id, content_id, monitor_match_id,
          actor_user_id, feedback_type, created_at, updated_at`,
		input.MonitorID, input.MonitorConfigVersionID, input.ContentID, optionalInt64(input.MonitorMatchID), input.ActorUserID, string(input.FeedbackType),
	))
}

func updateFeedback(ctx context.Context, executor queryRowExecutor, id, expectedVersion int64, input ingestiondomain.RelevanceFeedbackInput) (ingestiondomain.RelevanceFeedback, error) {
	feedback, err := scanFeedback(executor.QueryRowContext(ctx, `
UPDATE monitor_match_feedbacks
SET monitor_match_id = $1, feedback_type = $2, version = version + 1, updated_at = now()
WHERE id = $3 AND version = $4
RETURNING id, version, monitor_id, monitor_config_version_id, content_id, monitor_match_id,
          actor_user_id, feedback_type, created_at, updated_at`,
		optionalInt64(input.MonitorMatchID), string(input.FeedbackType), id, expectedVersion,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return ingestiondomain.RelevanceFeedback{}, fmt.Errorf("%w: relevance feedback version", sharedrepository.ErrConflict)
	}
	return feedback, err
}

func selectSnapshotByID(ctx context.Context, executor queryRowExecutor, snapshotID int64) (ingestiondomain.RelevanceSnapshot, error) {
	snapshot, err := scanSnapshot(executor.QueryRowContext(ctx, `SELECT `+snapshotColumns("match")+` FROM monitor_matches AS match WHERE match.id = $1`, snapshotID))
	if errors.Is(err, sql.ErrNoRows) {
		return ingestiondomain.RelevanceSnapshot{}, fmt.Errorf("%w: relevance snapshot %d", sharedrepository.ErrNotFound, snapshotID)
	}
	if err != nil {
		return ingestiondomain.RelevanceSnapshot{}, sharedrepository.MapError(err)
	}
	return snapshot, nil
}

func selectSnapshotByUnique(ctx context.Context, executor queryRowExecutor, configID, contentID int64, inputHash, scoringVersion string) (ingestiondomain.RelevanceSnapshot, error) {
	snapshot, err := scanSnapshot(executor.QueryRowContext(ctx, `
SELECT `+snapshotColumns("match")+`
FROM monitor_matches AS match
WHERE match.monitor_config_version_id = $1 AND match.content_id = $2
  AND match.input_hash = $3 AND match.scoring_version = $4`, configID, contentID, inputHash, scoringVersion))
	if errors.Is(err, sql.ErrNoRows) {
		return ingestiondomain.RelevanceSnapshot{}, fmt.Errorf("%w: relevance snapshot retry", sharedrepository.ErrNotFound)
	}
	if err != nil {
		return ingestiondomain.RelevanceSnapshot{}, sharedrepository.MapError(err)
	}
	return snapshot, nil
}

const snapshotColumnsTemplate = `
%[1]s.id, %[1]s.version, %[1]s.monitor_id, %[1]s.monitor_config_version_id, %[1]s.content_id,
%[1]s.input_hash, %[1]s.scoring_version, to_json(%[1]s.recall_paths)::text,
%[1]s.rule_score, %[1]s.semantic_score, %[1]s.llm_score, %[1]s.final_score,
%[1]s.decision, to_json(%[1]s.reason_codes)::text, %[1]s.explanation::text,
%[1]s.degraded, %[1]s.decision_origin,
%[1]s.embedding_model_profile_id, %[1]s.embedding_model_profile_version, %[1]s.embedding_model_version, %[1]s.review_ai_run_id,
%[1]s.created_at, %[1]s.updated_at`

func snapshotColumns(alias string) string { return fmt.Sprintf(snapshotColumnsTemplate, alias) }

func scanSnapshot(scanner interface{ Scan(...any) error }) (ingestiondomain.RelevanceSnapshot, error) {
	var snapshot ingestiondomain.RelevanceSnapshot
	var recallPaths, reasonCodes, explanation string
	var semanticScore, llmScore sql.NullFloat64
	var decision, origin string
	var embeddingID, embeddingVersion, reviewRunID sql.NullInt64
	var embeddingModelVersion sql.NullString
	if err := scanner.Scan(
		&snapshot.ID, &snapshot.Version, &snapshot.MonitorID, &snapshot.MonitorConfigVersionID, &snapshot.ContentID,
		&snapshot.InputHash, &snapshot.ScoringVersion, &recallPaths,
		&snapshot.RuleScore, &semanticScore, &llmScore, &snapshot.FinalScore,
		&decision, &reasonCodes, &explanation,
		&snapshot.Degraded, &origin,
		&embeddingID, &embeddingVersion, &embeddingModelVersion, &reviewRunID,
		&snapshot.CreatedAt, &snapshot.UpdatedAt,
	); err != nil {
		return ingestiondomain.RelevanceSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(recallPaths), &snapshot.RecallPaths); err != nil {
		return ingestiondomain.RelevanceSnapshot{}, fmt.Errorf("decode persisted recall paths: %w", err)
	}
	if err := json.Unmarshal([]byte(reasonCodes), &snapshot.ReasonCodes); err != nil {
		return ingestiondomain.RelevanceSnapshot{}, fmt.Errorf("decode persisted reason codes: %w", err)
	}
	snapshot.Explanation = json.RawMessage(explanation)
	snapshot.Decision = ingestiondomain.MatchDecision(decision)
	snapshot.DecisionOrigin = ingestiondomain.DecisionOrigin(origin)
	snapshot.SemanticScore = optionalFloat64Value(semanticScore)
	snapshot.LLMScore = optionalFloat64Value(llmScore)
	snapshot.EmbeddingModelProfileID = optionalInt64Value(embeddingID)
	snapshot.EmbeddingModelProfileVersion = optionalInt64Value(embeddingVersion)
	snapshot.EmbeddingModelVersion = optionalStringValue(embeddingModelVersion)
	snapshot.ReviewAIRunID = optionalInt64Value(reviewRunID)
	snapshot.CreatedAt = snapshot.CreatedAt.UTC()
	snapshot.UpdatedAt = snapshot.UpdatedAt.UTC()
	return snapshot, nil
}

func scanFeedback(scanner interface{ Scan(...any) error }) (ingestiondomain.RelevanceFeedback, error) {
	var feedback ingestiondomain.RelevanceFeedback
	var matchID sql.NullInt64
	var feedbackType string
	if err := scanner.Scan(
		&feedback.ID, &feedback.Version, &feedback.MonitorID, &feedback.MonitorConfigVersionID, &feedback.ContentID, &matchID,
		&feedback.ActorUserID, &feedbackType, &feedback.CreatedAt, &feedback.UpdatedAt,
	); err != nil {
		return ingestiondomain.RelevanceFeedback{}, sharedrepository.MapError(err)
	}
	feedback.MonitorMatchID = optionalInt64Value(matchID)
	feedback.FeedbackType = ingestiondomain.FeedbackType(feedbackType)
	feedback.CreatedAt = feedback.CreatedAt.UTC()
	feedback.UpdatedAt = feedback.UpdatedAt.UTC()
	return feedback, nil
}

func (repository *RelevanceRepository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
}

func (repository *RelevanceRepository) queryRows(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryContext(ctx, query, args...)
	}
	return repository.runtime.SQL.QueryContext(ctx, query, args...)
}

func (repository *RelevanceRepository) available() bool {
	return repository != nil && repository.runtime != nil && repository.runtime.SQL != nil
}

func optionalFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalFloat64Value(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}

func optionalInt64Value(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func optionalStringValue(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func sameStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
