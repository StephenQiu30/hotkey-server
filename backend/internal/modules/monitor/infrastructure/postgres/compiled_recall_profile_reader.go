package postgres

import (
	"context"
	"database/sql"
	"fmt"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/pgvector/pgvector-go"
)

// CompiledRecallProfileReader owns the exact Monitor-side read projection used
// by Ingestion hybrid recall. It never consults editable drafts or legacy rule
// and embedding tables.
type CompiledRecallProfileReader struct{ runtime *database.Runtime }

var _ ingestionapplication.ReadyRecallProfileReader = (*CompiledRecallProfileReader)(nil)

func NewCompiledRecallProfileReader(runtime *database.Runtime) (*CompiledRecallProfileReader, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("%w: database runtime is required", sharedrepository.ErrUnavailable)
	}
	return &CompiledRecallProfileReader{runtime: runtime}, nil
}

func (reader *CompiledRecallProfileReader) ReadReadyRecallProfile(ctx context.Context, query ingestionapplication.ReadyRecallProfileQuery) (ingestionapplication.ReadyRecallProfileDTO, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		return ingestionapplication.ReadyRecallProfileDTO{}, sharedrepository.ErrUnavailable
	}
	if query.MonitorID <= 0 || query.ConfigVersionID <= 0 || query.CompiledProfileID <= 0 || (query.Purpose != "preview" && query.Purpose != "published") {
		return ingestionapplication.ReadyRecallProfileDTO{}, sharedrepository.ErrInvalidInput
	}
	var record compiledRecallProfileRecord
	var monitorVersionID, previewRunID, draftID, draftResourceVersion sql.NullInt64
	var embeddingProfileID, embeddingProfileVersion sql.NullInt64
	var semanticReason, modelVersion, vectorText sql.NullString
	err := reader.runtime.SQL.QueryRowContext(ctx, `
SELECT profile.id,profile.monitor_id,profile.purpose,profile.config_version_id,
       profile.monitor_version_id,profile.preview_run_id,profile.draft_id,profile.draft_resource_version,
       profile.matching_algorithm_version,profile.lexical_algorithm_version,
	       profile.semantic_algorithm_version,profile.structured_algorithm_version,
	       profile.search_normalization_profile_version,
	       CASE WHEN profile.semantic_state='ready' AND model.id IS NOT NULL AND run.id IS NOT NULL
	            THEN 'ready' ELSE 'unavailable' END,
	       CASE WHEN profile.semantic_state='unavailable' THEN profile.semantic_unavailable_reason
	            WHEN model.id IS NULL OR run.id IS NULL THEN 'semantic_receipt_unavailable' END,
	       CASE WHEN model.id IS NOT NULL AND run.id IS NOT NULL THEN embedding.model_profile_id END,
	       CASE WHEN model.id IS NOT NULL AND run.id IS NOT NULL THEN embedding.model_profile_version END,
	       CASE WHEN model.id IS NOT NULL AND run.id IS NOT NULL THEN embedding.model_version END,
	       CASE WHEN model.id IS NOT NULL AND run.id IS NOT NULL THEN embedding.embedding::text END
FROM monitor_compiled_profiles AS profile
JOIN monitor_config_versions AS config
  ON config.id=profile.config_version_id AND config.monitor_id=profile.monitor_id
JOIN monitor_intent_draft_revisions AS revision
  ON revision.id=profile.intent_revision_id AND revision.monitor_id=profile.monitor_id
 AND revision.config_version_id=profile.config_version_id
	LEFT JOIN monitor_compiled_intent_embeddings AS embedding
  ON embedding.compiled_profile_id=profile.id AND embedding.config_version_id=profile.config_version_id
	LEFT JOIN ai_model_profiles AS model
  ON model.id=embedding.model_profile_id AND model.version=embedding.model_profile_version
 AND model.model_version=embedding.model_version AND model.task_type='embedding'
 AND model.embedding_dimensions=1024 AND model.enabled AND model.deleted_at IS NULL
	LEFT JOIN ai_runs AS run
  ON run.id=embedding.ai_run_id AND run.status='succeeded' AND run.task_type='embedding'
 AND run.target_type='monitor_compiled_profile' AND run.target_id=profile.id
 AND run.model_profile_id=embedding.model_profile_id
 AND run.model_profile_version=embedding.model_profile_version
 AND run.model_version=embedding.model_version AND run.input_hash=embedding.input_hash
LEFT JOIN monitor_intent_analysis_runs AS preview_run
  ON preview_run.id=profile.preview_run_id AND preview_run.monitor_id=profile.monitor_id
 AND preview_run.draft_id=profile.draft_id AND preview_run.draft_resource_version=profile.draft_resource_version
WHERE profile.id=$1 AND profile.monitor_id=$2 AND profile.purpose=$3 AND profile.config_version_id=$4
  AND profile.status='ready'
  AND (
    profile.purpose='published' AND profile.monitor_version_id=$5
      AND $6::bigint=0 AND $7::bigint=0 AND $8::bigint=0
      AND config.state='published'
    OR profile.purpose='preview' AND profile.monitor_version_id IS NULL
      AND profile.preview_run_id=$6 AND profile.draft_id=$7 AND profile.draft_resource_version=$8
      AND preview_run.kind='preview' AND preview_run.status IN ('running','succeeded')
      AND config.state='draft'
  )`, query.CompiledProfileID, query.MonitorID, query.Purpose, query.ConfigVersionID,
		query.MonitorVersionID, query.PreviewRunID, query.DraftID, query.DraftResourceVersion).Scan(
		&record.ID, &record.MonitorID, &record.Purpose, &record.ConfigVersionID,
		&monitorVersionID, &previewRunID, &draftID, &draftResourceVersion,
		&record.MatchingAlgorithmVersion, &record.LexicalAlgorithmVersion,
		&record.SemanticAlgorithmVersion, &record.StructuredAlgorithmVersion,
		&record.SearchNormalizationProfileVersion,
		&record.SemanticState, &semanticReason,
		&embeddingProfileID, &embeddingProfileVersion, &modelVersion, &vectorText,
	)
	if err == sql.ErrNoRows {
		return ingestionapplication.ReadyRecallProfileDTO{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return ingestionapplication.ReadyRecallProfileDTO{}, databaserepository.MapError(err)
	}
	record.MonitorVersionID = monitorVersionID.Int64
	record.PreviewRunID = previewRunID.Int64
	record.DraftID = draftID.Int64
	record.DraftResourceVersion = draftResourceVersion.Int64
	record.SemanticUnavailableReason = semanticReason.String
	if record.SemanticState == ingestionapplication.SemanticRecallStateReady {
		if !embeddingProfileID.Valid || !embeddingProfileVersion.Valid || !modelVersion.Valid || !vectorText.Valid {
			return ingestionapplication.ReadyRecallProfileDTO{}, fmt.Errorf("compiled semantic receipt is incomplete")
		}
		var vector pgvector.HalfVector
		if err := vector.Parse(vectorText.String); err != nil {
			return ingestionapplication.ReadyRecallProfileDTO{}, fmt.Errorf("parse compiled semantic vector: %w", err)
		}
		record.EmbeddingProfileID = embeddingProfileID.Int64
		record.EmbeddingProfileVersion = embeddingProfileVersion.Int64
		record.ModelVersion = modelVersion.String
		record.QueryVector = append([]float32(nil), vector.Slice()...)
	}
	clauses, err := reader.readCompiledRecallClauses(ctx, record.ID)
	if err != nil {
		return ingestionapplication.ReadyRecallProfileDTO{}, err
	}
	entities, err := reader.readCompiledRecallEntities(ctx, record.ID)
	if err != nil {
		return ingestionapplication.ReadyRecallProfileDTO{}, err
	}
	return compiledRecallProfileDTO(record, clauses, entities)
}

func (reader *CompiledRecallProfileReader) readCompiledRecallClauses(ctx context.Context, profileID int64) ([]compiledRecallClauseRecord, error) {
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
SELECT operator,field,value,origin
FROM monitor_compiled_clauses WHERE compiled_profile_id=$1 ORDER BY ordinal`, profileID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	result := []compiledRecallClauseRecord{}
	for rows.Next() {
		var record compiledRecallClauseRecord
		if err := rows.Scan(&record.Operator, &record.Field, &record.Value, &record.Origin); err != nil {
			return nil, databaserepository.MapError(err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return result, nil
}

func (reader *CompiledRecallProfileReader) readCompiledRecallEntities(ctx context.Context, profileID int64) ([]compiledRecallEntityRecord, error) {
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
SELECT id,canonical_id FROM monitor_compiled_entities
WHERE compiled_profile_id=$1 ORDER BY ordinal`, profileID)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	result := []compiledRecallEntityRecord{}
	for rows.Next() {
		var record compiledRecallEntityRecord
		if err := rows.Scan(&record.ID, &record.CanonicalID); err != nil {
			_ = rows.Close()
			return nil, databaserepository.MapError(err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, databaserepository.MapError(err)
	}
	if err := rows.Close(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	for index := range result {
		aliasRows, aliasErr := reader.runtime.SQL.QueryContext(ctx, `
SELECT alias FROM monitor_compiled_entity_aliases
WHERE compiled_profile_id=$1 AND compiled_entity_id=$2 ORDER BY ordinal`, profileID, result[index].ID)
		if aliasErr != nil {
			return nil, databaserepository.MapError(aliasErr)
		}
		for aliasRows.Next() {
			var alias string
			if aliasErr = aliasRows.Scan(&alias); aliasErr != nil {
				_ = aliasRows.Close()
				return nil, databaserepository.MapError(aliasErr)
			}
			result[index].Aliases = append(result[index].Aliases, alias)
		}
		if aliasErr = aliasRows.Err(); aliasErr != nil {
			_ = aliasRows.Close()
			return nil, databaserepository.MapError(aliasErr)
		}
		if aliasErr = aliasRows.Close(); aliasErr != nil {
			return nil, databaserepository.MapError(aliasErr)
		}
	}
	return result, nil
}
