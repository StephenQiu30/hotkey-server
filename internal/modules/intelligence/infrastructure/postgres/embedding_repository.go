package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/pgvector/pgvector-go"
)

type EmbeddingTarget string

const (
	EmbeddingTargetContent EmbeddingTarget = "content"
	EmbeddingTargetMonitor EmbeddingTarget = "monitor"
	EmbeddingTargetEvent   EmbeddingTarget = "event"
	EmbeddingTargetTopic   EmbeddingTarget = "topic"
)

type EmbeddingWrite struct {
	Target                                        EmbeddingTarget
	TargetID, ModelProfileID, ModelProfileVersion int64
	ModelVersion, InputHash, QueryText            string
	Vector                                        []float32
}

type EmbeddingMatch struct {
	TargetID, ModelProfileVersion int64
	ModelVersion                  string
	Distance                      float64
}

// EmbeddingCompletion binds an embedding to the validating run that produced
// it. A vector has no standalone success path: provenance, budget settlement
// and activation are all committed together.
type EmbeddingCompletion struct {
	RunID      int64
	Write      EmbeddingWrite
	Usage      intelligencedomain.Usage
	LatencyMS  int64
	FinishedAt time.Time
}

// CompleteEmbedding retires any prior active vector and writes its successor
// while settling exactly the reserved PLAN-008 budget unit. Its advisory lock
// order is fixed: ai-budget -> ai-run -> ai-embedding.
func (repository *Repository) CompleteEmbedding(ctx context.Context, completion EmbeddingCompletion) (int64, error) {
	input := completion.Write
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || completion.RunID <= 0 || completion.LatencyMS < 0 ||
		input.TargetID <= 0 || input.ModelProfileID <= 0 || input.ModelProfileVersion <= 0 || strings.TrimSpace(input.ModelVersion) == "" || !validEmbeddingHash(input.InputHash) {
		return 0, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if _, err := completion.Usage.TotalTokens(); err != nil {
		return 0, err
	}
	if err := intelligencedomain.ValidateEmbedding(input.Vector); err != nil {
		return 0, err
	}
	specification, ok := embeddingSpecificationFor(input.Target)
	if !ok || input.Target == EmbeddingTargetMonitor && strings.TrimSpace(input.QueryText) == "" {
		return 0, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if completion.FinishedAt.IsZero() {
		completion.FinishedAt = time.Now().UTC()
	} else {
		completion.FinishedAt = completion.FinishedAt.UTC()
	}
	reference, err := repository.embeddingRunReference(ctx, repository.queryer(ctx), completion.RunID, false)
	if err != nil {
		return 0, err
	}
	var embeddingID int64
	err = repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockBudget(ctx, transaction.SQL, reference.ModelProfileID, reference.BudgetDay); err != nil {
			return err
		}
		if err := lockRun(ctx, transaction.SQL, reference.ReuseKey); err != nil {
			return err
		}
		locked, err := repository.embeddingRunReference(ctx, transaction.SQL, completion.RunID, true)
		if err != nil {
			return err
		}
		if locked != reference || locked.Status != intelligencedomain.RunStatusValidating ||
			locked.TaskType != intelligencedomain.TaskTypeEmbedding || locked.TargetType != string(input.Target) || locked.TargetID != input.TargetID ||
			locked.ModelProfileID != input.ModelProfileID || locked.ModelProfileVersion != input.ModelProfileVersion ||
			locked.ModelVersion != input.ModelVersion || locked.InputHash != input.InputHash {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		if err := lockEmbedding(ctx, transaction.SQL, input.Target, input.TargetID, input.ModelProfileID); err != nil {
			return err
		}
		// Claim captured the immutable profile/model facts before reserving the
		// run. Do not take a profile row lock after the budget lock: Claim locks
		// that row before it reaches the budget and the inverse ordering would
		// deadlock. A later disable/delete only removes a vector from serving
		// queries; it must not strand an already validating run or erase history.
		if _, err := transaction.SQL.ExecContext(ctx, `UPDATE `+specification.table+` SET active=false WHERE `+specification.targetColumn+`=$1 AND model_profile_id=$2 AND active`, input.TargetID, input.ModelProfileID); err != nil {
			return fmt.Errorf("deactivate prior %s embedding: %w", input.Target, err)
		}
		query, arguments := embeddingUpsert(specification, input, completion.RunID)
		if err := transaction.SQL.QueryRowContext(ctx, query, arguments...).Scan(&embeddingID); err != nil {
			return fmt.Errorf("write %s embedding: %w", input.Target, err)
		}
		tokens, err := completion.Usage.TotalTokens()
		if err != nil {
			return err
		}
		if err := settleReservedBudget(ctx, transaction.SQL, locked.runReference, locked.ReservedCost); err != nil {
			return err
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE ai_runs
SET status='succeeded',tokens=$1,cost=$2::numeric,latency_ms=$3,reserved_cost=0,
    error_code=NULL,lease_expires_at=NULL,finished_at=$4
WHERE id=$5`, tokens, locked.ReservedCost, completion.LatencyMS, completion.FinishedAt, completion.RunID); err != nil {
			return fmt.Errorf("complete embedding AI run: %w", err)
		}
		return nil
	})
	return embeddingID, err
}

// ActiveEmbeddingForRun returns only a vector that remains active and whose
// producer run is the exact succeeded run selected by reuse. It purposefully
// does not fall back to profile/input hash matches with weaker provenance.
func (repository *Repository) ActiveEmbeddingForRun(ctx context.Context, target EmbeddingTarget, targetID, runID int64) ([]float32, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || targetID <= 0 || runID <= 0 {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	specification, ok := embeddingSpecificationFor(target)
	if !ok {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	var vector pgvector.HalfVector
	err := repository.queryer(ctx).QueryRowContext(ctx, `
SELECT e.embedding
FROM `+specification.table+` e
JOIN ai_runs r ON r.id=e.ai_run_id
WHERE e.active AND e.`+specification.targetColumn+`=$1 AND e.ai_run_id=$2 AND r.status='succeeded'`, targetID, runID).Scan(&vector)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read active %s embedding for AI run: %w", target, err)
	}
	return append([]float32(nil), vector.Slice()...), nil
}

// NearestEmbeddings only serves vectors for an enabled, non-deleted profile
// and the caller-selected semantic model version. Stale vectors cannot leak
// into an otherwise valid similarity response.
func (repository *Repository) NearestEmbeddings(ctx context.Context, target EmbeddingTarget, profileID int64, modelVersion string, vector []float32, limit int) ([]EmbeddingMatch, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || profileID <= 0 || strings.TrimSpace(modelVersion) == "" || limit < 1 || limit > 100 {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if err := intelligencedomain.ValidateEmbedding(vector); err != nil {
		return nil, err
	}
	specification, ok := embeddingSpecificationFor(target)
	if !ok {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	rows, err := repository.queryRows(ctx, `
SELECT e.`+specification.targetColumn+`,e.model_profile_version,e.model_version,e.embedding <=> $1::halfvec
FROM `+specification.table+` e
JOIN ai_model_profiles p ON p.id=e.model_profile_id
WHERE e.active AND e.model_profile_id=$2 AND e.model_version=$3
  AND p.enabled AND p.deleted_at IS NULL
ORDER BY e.embedding <=> $1::halfvec
LIMIT $4`, pgvector.NewVector(vector), profileID, modelVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("query nearest %s embeddings: %w", target, err)
	}
	defer rows.Close()
	var matches []EmbeddingMatch
	for rows.Next() {
		var match EmbeddingMatch
		if err := rows.Scan(&match.TargetID, &match.ModelProfileVersion, &match.ModelVersion, &match.Distance); err != nil {
			return nil, fmt.Errorf("scan nearest %s embedding: %w", target, err)
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nearest %s embeddings: %w", target, err)
	}
	return matches, nil
}

type embeddingSpecification struct {
	table, targetColumn string
	withQueryText       bool
}

func embeddingSpecificationFor(target EmbeddingTarget) (embeddingSpecification, bool) {
	switch target {
	case EmbeddingTargetContent:
		return embeddingSpecification{table: "content_embeddings", targetColumn: "content_id"}, true
	case EmbeddingTargetMonitor:
		return embeddingSpecification{table: "monitor_embeddings", targetColumn: "monitor_id", withQueryText: true}, true
	case EmbeddingTargetEvent:
		return embeddingSpecification{table: "event_embeddings", targetColumn: "event_id"}, true
	case EmbeddingTargetTopic:
		return embeddingSpecification{table: "topic_embeddings", targetColumn: "topic_id"}, true
	default:
		return embeddingSpecification{}, false
	}
}

func embeddingUpsert(specification embeddingSpecification, input EmbeddingWrite, runID int64) (string, []any) {
	vector := pgvector.NewVector(input.Vector)
	if specification.withQueryText {
		return `INSERT INTO monitor_embeddings (
 monitor_id,model_profile_id,model_version,input_hash,query_text,embedding,active,model_profile_version,ai_run_id
) VALUES ($1,$2,$3,$4,$5,$6,true,$7,$8)
ON CONFLICT (monitor_id,model_profile_id,model_version,input_hash) DO UPDATE
SET query_text=EXCLUDED.query_text,embedding=EXCLUDED.embedding,active=true,model_profile_version=EXCLUDED.model_profile_version,ai_run_id=EXCLUDED.ai_run_id
RETURNING id`, []any{input.TargetID, input.ModelProfileID, input.ModelVersion, input.InputHash, input.QueryText, vector, input.ModelProfileVersion, runID}
	}
	return `INSERT INTO ` + specification.table + ` (
 ` + specification.targetColumn + `,model_profile_id,model_version,input_hash,embedding,active,model_profile_version,ai_run_id
) VALUES ($1,$2,$3,$4,$5,true,$6,$7)
ON CONFLICT (` + specification.targetColumn + `,model_profile_id,model_version,input_hash) DO UPDATE
SET embedding=EXCLUDED.embedding,active=true,model_profile_version=EXCLUDED.model_profile_version,ai_run_id=EXCLUDED.ai_run_id
RETURNING id`, []any{input.TargetID, input.ModelProfileID, input.ModelVersion, input.InputHash, vector, input.ModelProfileVersion, runID}
}

type embeddingRunReference struct {
	runReference
	TaskType                            intelligencedomain.TaskType
	TargetType, ModelVersion, InputHash string
	TargetID, ModelProfileVersion       int64
}

func (repository *Repository) embeddingRunReference(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, runID int64, lock bool) (embeddingRunReference, error) {
	query := `SELECT model_profile_id,budget_day::text,reuse_key,reserved_cost::text,status,task_type,target_type,target_id,model_profile_version,model_version,input_hash FROM ai_runs WHERE id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	var reference embeddingRunReference
	if err := queryer.QueryRowContext(ctx, query, runID).Scan(
		&reference.ModelProfileID, &reference.BudgetDay, &reference.ReuseKey, &reference.ReservedCost, &reference.Status,
		&reference.TaskType, &reference.TargetType, &reference.TargetID, &reference.ModelProfileVersion, &reference.ModelVersion, &reference.InputHash,
	); err != nil {
		if err == sql.ErrNoRows {
			return embeddingRunReference{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
		return embeddingRunReference{}, fmt.Errorf("read embedding AI run: %w", err)
	}
	return reference, nil
}

func lockEmbedding(ctx context.Context, queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, target EmbeddingTarget, targetID, profileID int64) error {
	if _, err := queryer.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, fmt.Sprintf("ai-embedding:%s:%d:%d", target, targetID, profileID)); err != nil {
		return fmt.Errorf("lock %s embedding: %w", target, err)
	}
	return nil
}

func validEmbeddingHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}
