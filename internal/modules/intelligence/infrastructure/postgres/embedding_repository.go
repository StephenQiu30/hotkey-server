package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

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

// ReplaceEmbedding serializes writers for one target/profile pair, retires the
// previous active vector, and writes the replacement in the same transaction.
// It intentionally relies on target FKs rather than reading another module's
// tables, so a missing target is a normal database constraint outcome.
func (repository *Repository) ReplaceEmbedding(ctx context.Context, input EmbeddingWrite) (int64, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || input.TargetID <= 0 || input.ModelProfileID <= 0 || input.ModelProfileVersion <= 0 ||
		strings.TrimSpace(input.ModelVersion) == "" || !validEmbeddingHash(input.InputHash) {
		return 0, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if err := intelligencedomain.ValidateEmbedding(input.Vector); err != nil {
		return 0, err
	}
	specification, ok := embeddingSpecificationFor(input.Target)
	if !ok || input.Target == EmbeddingTargetMonitor && strings.TrimSpace(input.QueryText) == "" {
		return 0, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	var embeddingID int64
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockEmbedding(ctx, transaction.SQL, input.Target, input.TargetID, input.ModelProfileID); err != nil {
			return err
		}
		profile, deleted, err := readProfile(ctx, transaction.SQL, input.ModelProfileID, true)
		if err != nil {
			return err
		}
		if deleted || !profile.Enabled || profile.TaskType != intelligencedomain.TaskTypeEmbedding || profile.Version != input.ModelProfileVersion || profile.ModelVersion != input.ModelVersion {
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `UPDATE `+specification.table+` SET active=false WHERE `+specification.targetColumn+`=$1 AND model_profile_id=$2 AND active`, input.TargetID, input.ModelProfileID); err != nil {
			return fmt.Errorf("deactivate prior %s embedding: %w", input.Target, err)
		}
		query, arguments := embeddingUpsert(specification, input)
		if err := transaction.SQL.QueryRowContext(ctx, query, arguments...).Scan(&embeddingID); err != nil {
			return fmt.Errorf("write %s embedding: %w", input.Target, err)
		}
		return nil
	})
	return embeddingID, err
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

func embeddingUpsert(specification embeddingSpecification, input EmbeddingWrite) (string, []any) {
	vector := pgvector.NewVector(input.Vector)
	if specification.withQueryText {
		return `INSERT INTO monitor_embeddings (
 monitor_id,model_profile_id,model_version,input_hash,query_text,embedding,active,model_profile_version
) VALUES ($1,$2,$3,$4,$5,$6,true,$7)
ON CONFLICT (monitor_id,model_profile_id,model_version,input_hash) DO UPDATE
SET query_text=EXCLUDED.query_text,embedding=EXCLUDED.embedding,active=true,model_profile_version=EXCLUDED.model_profile_version
RETURNING id`, []any{input.TargetID, input.ModelProfileID, input.ModelVersion, input.InputHash, input.QueryText, vector, input.ModelProfileVersion}
	}
	return `INSERT INTO ` + specification.table + ` (
 ` + specification.targetColumn + `,model_profile_id,model_version,input_hash,embedding,active,model_profile_version
) VALUES ($1,$2,$3,$4,$5,true,$6)
ON CONFLICT (` + specification.targetColumn + `,model_profile_id,model_version,input_hash) DO UPDATE
SET embedding=EXCLUDED.embedding,active=true,model_profile_version=EXCLUDED.model_profile_version
RETURNING id`, []any{input.TargetID, input.ModelProfileID, input.ModelVersion, input.InputHash, vector, input.ModelProfileVersion}
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
