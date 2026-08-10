package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	"github.com/pgvector/pgvector-go"
)

type compiledIntentEmbeddingRecord struct {
	ID, CompiledProfileID, ConfigVersionID       int64
	ModelProfileID, ModelProfileVersion, AIRunID int64
	ModelVersion, InputHash                      string
	CreatedAt                                    sql.NullTime
}

func (repository *IntentRepository) PersistCompiledIntentEmbedding(ctx context.Context, command monitorapplication.PersistCompiledIntentEmbeddingCommand) (monitorapplication.CompiledIntentEmbeddingReceiptDTO, error) {
	if repository == nil || command.CompiledProfileID <= 0 || command.ConfigVersionID <= 0 || command.ModelProfileID <= 0 ||
		command.ModelProfileVersion <= 0 || !intentAnalysisProfilePattern.MatchString(command.ModelVersion) ||
		validateIntentRecordHash(command.InputHash) != nil || len(command.Embedding) != 1024 || command.AIRunID <= 0 || command.CreatedAt.IsZero() {
		return monitorapplication.CompiledIntentEmbeddingReceiptDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	command.CreatedAt = command.CreatedAt.UTC().Truncate(1000)
	var receipt monitorapplication.CompiledIntentEmbeddingReceiptDTO
	err := repository.withIntentTransaction(ctx, func(transactionCtx context.Context, executor intentExecutor) error {
		result, err := executor.ExecContext(transactionCtx, `
UPDATE monitor_compiled_profiles
SET semantic_state='ready',semantic_unavailable_reason=NULL
WHERE id=$1 AND config_version_id=$2 AND purpose='preview' AND status='building'
  AND (semantic_state='ready' OR semantic_state='unavailable'
       AND semantic_unavailable_reason IN ('semantic_generation_unavailable','semantic_model_unavailable'))`,
			command.CompiledProfileID, command.ConfigVersionID)
		if err != nil {
			return mapIntentDatabaseError(err)
		}
		if changed, rowsErr := result.RowsAffected(); rowsErr != nil || changed != 1 {
			return monitorapplication.ErrCompiledIntentProfileConflict
		}
		var embeddingID int64
		insertErr := executor.QueryRowContext(transactionCtx, `
INSERT INTO monitor_compiled_intent_embeddings (
  compiled_profile_id,config_version_id,model_profile_id,model_profile_version,
  model_version,input_hash,embedding,ai_run_id,created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (compiled_profile_id) DO NOTHING
RETURNING id`, command.CompiledProfileID, command.ConfigVersionID, command.ModelProfileID,
			command.ModelProfileVersion, command.ModelVersion, command.InputHash,
			pgvector.NewHalfVector(command.Embedding), command.AIRunID, command.CreatedAt).Scan(&embeddingID)
		created := insertErr == nil
		if insertErr != nil && !errors.Is(insertErr, sql.ErrNoRows) {
			return mapIntentDatabaseError(insertErr)
		}
		record, err := readCompiledIntentEmbeddingRecord(transactionCtx, executor, command.CompiledProfileID)
		if err != nil {
			return err
		}
		if embeddingID > 0 && record.ID != embeddingID {
			return monitorapplication.ErrCompiledIntentProfileConflict
		}
		receipt = compiledIntentEmbeddingReceiptDTO(record, created)
		return nil
	})
	return receipt, err
}

func (repository *IntentRepository) ReadCompiledIntentEmbedding(ctx context.Context, query monitorapplication.ReadCompiledIntentEmbeddingQuery) (monitorapplication.CompiledIntentEmbeddingReceiptDTO, error) {
	if repository == nil || query.CompiledProfileID <= 0 || query.ConfigVersionID <= 0 || query.ModelProfileID <= 0 ||
		query.ModelProfileVersion <= 0 || !intentAnalysisProfilePattern.MatchString(query.ModelVersion) ||
		validateIntentRecordHash(query.InputHash) != nil || query.AIRunID <= 0 {
		return monitorapplication.CompiledIntentEmbeddingReceiptDTO{}, monitorapplication.ErrInvalidIntentContract
	}
	record, err := readCompiledIntentEmbeddingRecord(ctx, repository.intentExecutor(ctx), query.CompiledProfileID)
	if err != nil {
		return monitorapplication.CompiledIntentEmbeddingReceiptDTO{}, err
	}
	if record.ConfigVersionID != query.ConfigVersionID || record.ModelProfileID != query.ModelProfileID ||
		record.ModelProfileVersion != query.ModelProfileVersion || record.ModelVersion != query.ModelVersion ||
		record.InputHash != query.InputHash || record.AIRunID != query.AIRunID {
		return monitorapplication.CompiledIntentEmbeddingReceiptDTO{}, monitorapplication.ErrCompiledIntentProfileConflict
	}
	return compiledIntentEmbeddingReceiptDTO(record, false), nil
}

func readCompiledIntentEmbeddingRecord(ctx context.Context, executor intentExecutor, compiledProfileID int64) (compiledIntentEmbeddingRecord, error) {
	var record compiledIntentEmbeddingRecord
	var inputHash string
	err := executor.QueryRowContext(ctx, `
SELECT embedding.id,embedding.compiled_profile_id,embedding.config_version_id,
       embedding.model_profile_id,embedding.model_profile_version,embedding.model_version,
       btrim(embedding.input_hash),embedding.ai_run_id,embedding.created_at
FROM monitor_compiled_intent_embeddings AS embedding
JOIN monitor_compiled_profiles AS profile
  ON profile.id=embedding.compiled_profile_id AND profile.config_version_id=embedding.config_version_id
JOIN ai_runs AS run ON run.id=embedding.ai_run_id AND run.status='succeeded'
JOIN ai_model_profiles AS model
  ON model.id=embedding.model_profile_id AND model.version=embedding.model_profile_version
 AND model.model_version=embedding.model_version AND model.enabled AND model.deleted_at IS NULL
WHERE embedding.compiled_profile_id=$1 AND profile.semantic_state='ready'
  AND profile.status IN ('building','ready')`, compiledProfileID).Scan(
		&record.ID, &record.CompiledProfileID, &record.ConfigVersionID, &record.ModelProfileID,
		&record.ModelProfileVersion, &record.ModelVersion, &inputHash, &record.AIRunID, &record.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return compiledIntentEmbeddingRecord{}, monitorapplication.ErrIntentPublicationUnavailable
	}
	if err != nil {
		return compiledIntentEmbeddingRecord{}, mapIntentDatabaseError(err)
	}
	record.InputHash = strings.TrimSpace(inputHash)
	return record, nil
}

func compiledIntentEmbeddingReceiptDTO(record compiledIntentEmbeddingRecord, created bool) monitorapplication.CompiledIntentEmbeddingReceiptDTO {
	return monitorapplication.CompiledIntentEmbeddingReceiptDTO{
		EmbeddingID: record.ID, CompiledProfileID: record.CompiledProfileID, ConfigVersionID: record.ConfigVersionID,
		ModelProfileID: record.ModelProfileID, ModelProfileVersion: record.ModelProfileVersion,
		ModelVersion: record.ModelVersion, InputHash: record.InputHash, AIRunID: record.AIRunID,
		CreatedAt: record.CreatedAt.Time.UTC(), Created: created,
	}
}
