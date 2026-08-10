package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/pgvector/pgvector-go"
)

// DocumentRecallProjectionWriter synchronously derives and stores retrieval
// assets from exact receipts. Canonical plaintext is an SQL argument only; it
// is never selected back, put in a persistence record, job, or log field.
type DocumentRecallProjectionWriter struct{ runtime *database.Runtime }

var _ ingestionapplication.DocumentRecallProjectionWriter = (*DocumentRecallProjectionWriter)(nil)

func NewDocumentRecallProjectionWriter(runtime *database.Runtime) (*DocumentRecallProjectionWriter, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("%w: database runtime is required", sharedrepository.ErrUnavailable)
	}
	return &DocumentRecallProjectionWriter{runtime: runtime}, nil
}

func (writer *DocumentRecallProjectionWriter) PersistDocumentSearchProjection(ctx context.Context, command ingestionapplication.PersistDocumentSearchProjectionCommand) (ingestionapplication.DocumentSearchProjectionResult, error) {
	if writer == nil || writer.runtime == nil || writer.runtime.SQL == nil {
		return ingestionapplication.DocumentSearchProjectionResult{}, sharedrepository.ErrUnavailable
	}
	var record documentSearchProjectionRecord
	err := writer.queryRow(ctx, persistDocumentSearchProjectionSQL,
		command.DocumentVersionID, command.DerivedArtifactID,
		command.StoreDerivedRightsDecisionID, command.RetainRightsDecisionID,
		command.NormalizationProfileVersion, command.NormalizedTextSHA256, command.Plaintext,
		command.EntityKeys, command.ActionKeys, command.LocationKeys, command.RegionKeys,
		command.IndexedAt.UTC(),
	).Scan(
		&record.ID, &record.DocumentVersionID, &record.SourceConnectionID, &record.DerivedArtifactID,
		&record.StoreDerivedRightsDecisionID, &record.RetainRightsDecisionID,
		&record.NormalizationProfileVersion, &record.NormalizedTextSHA256,
		&record.RetentionUntil, &record.IndexedAt, &record.LifecycleState, &record.Created,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.DocumentSearchProjectionResult{}, fmt.Errorf(
			"%w: exact plaintext receipt, rights or immutable search projection conflicts",
			sharedrepository.ErrConflict,
		)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ingestionapplication.DocumentSearchProjectionResult{}, err
		}
		return ingestionapplication.DocumentSearchProjectionResult{}, databaserepository.MapError(err)
	}
	return documentSearchProjectionResult(record), nil
}

func (writer *DocumentRecallProjectionWriter) PersistDocumentEmbeddingReceipt(ctx context.Context, command ingestionapplication.PersistDocumentEmbeddingReceiptCommand) (ingestionapplication.DocumentEmbeddingReceiptResult, error) {
	if writer == nil || writer.runtime == nil || writer.runtime.SQL == nil {
		return ingestionapplication.DocumentEmbeddingReceiptResult{}, sharedrepository.ErrUnavailable
	}
	var record documentEmbeddingReceiptRecord
	err := writer.queryRow(ctx, persistDocumentEmbeddingReceiptSQL,
		command.DocumentVersionID, command.EmbedLocalRightsDecisionID, command.RetainRightsDecisionID,
		command.ModelProfileID, command.ModelProfileVersion, command.ModelVersion,
		command.NormalizedTextSHA256, pgvector.NewHalfVector(command.Embedding), command.AIRunID,
		command.CreatedAt.UTC(),
	).Scan(
		&record.ID, &record.DocumentVersionID, &record.SourceConnectionID,
		&record.EmbedLocalRightsDecisionID, &record.RetainRightsDecisionID,
		&record.ModelProfileID, &record.ModelProfileVersion, &record.ModelVersion,
		&record.NormalizedTextSHA256, &record.AIRunID, &record.RetentionUntil, &record.CreatedAt,
		&record.LifecycleState, &record.Created,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.DocumentEmbeddingReceiptResult{}, fmt.Errorf(
			"%w: exact embedding run, rights or immutable embedding receipt conflicts",
			sharedrepository.ErrConflict,
		)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ingestionapplication.DocumentEmbeddingReceiptResult{}, err
		}
		return ingestionapplication.DocumentEmbeddingReceiptResult{}, databaserepository.MapError(err)
	}
	return documentEmbeddingReceiptResult(record), nil
}

func (writer *DocumentRecallProjectionWriter) ReadDocumentEmbeddingReceipt(ctx context.Context, query ingestionapplication.ReadDocumentEmbeddingReceiptQuery) (ingestionapplication.DocumentEmbeddingReceiptResult, error) {
	if writer == nil || writer.runtime == nil || writer.runtime.SQL == nil {
		return ingestionapplication.DocumentEmbeddingReceiptResult{}, sharedrepository.ErrUnavailable
	}
	var record documentEmbeddingReceiptRecord
	err := writer.queryRow(ctx, `
SELECT embedding.id,embedding.document_version_id,embedding.source_connection_id,
       embedding.embed_local_rights_decision_id,embedding.retain_rights_decision_id,
       embedding.model_profile_id,embedding.model_profile_version,embedding.model_version,
       btrim(embedding.normalized_text_sha256),embedding.ai_run_id,embedding.retention_until,
       embedding.created_at,embedding.lifecycle_state,false
FROM document_version_embeddings AS embedding
JOIN ai_runs AS run ON run.id=embedding.ai_run_id AND run.status='succeeded'
JOIN ai_model_profiles AS model ON model.id=embedding.model_profile_id
    AND model.version=embedding.model_profile_version AND model.model_version=embedding.model_version
    AND model.enabled AND model.deleted_at IS NULL
WHERE embedding.document_version_id=$1 AND embedding.embed_local_rights_decision_id=$2
  AND embedding.retain_rights_decision_id=$3 AND embedding.model_profile_id=$4
  AND embedding.model_profile_version=$5 AND embedding.model_version=$6
  AND embedding.normalized_text_sha256=$7 AND embedding.ai_run_id=$8
  AND embedding.lifecycle_state='active' AND embedding.retention_until>CURRENT_TIMESTAMP
  AND current_rights_action_allowed(
      embedding.embed_local_rights_decision_id,embedding.source_connection_id,'document_version',
      embedding.document_version_id::text,embedding.normalized_text_sha256,'embed_local',CURRENT_TIMESTAMP
  )
  AND current_rights_action_allowed(
      embedding.retain_rights_decision_id,embedding.source_connection_id,'document_version',
      embedding.document_version_id::text,embedding.normalized_text_sha256,'retain',CURRENT_TIMESTAMP
  )`, query.DocumentVersionID, query.EmbedLocalRightsDecisionID, query.RetainRightsDecisionID,
		query.ModelProfileID, query.ModelProfileVersion, query.ModelVersion, query.NormalizedTextSHA256, query.AIRunID,
	).Scan(
		&record.ID, &record.DocumentVersionID, &record.SourceConnectionID,
		&record.EmbedLocalRightsDecisionID, &record.RetainRightsDecisionID,
		&record.ModelProfileID, &record.ModelProfileVersion, &record.ModelVersion,
		&record.NormalizedTextSHA256, &record.AIRunID, &record.RetentionUntil, &record.CreatedAt,
		&record.LifecycleState, &record.Created,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ingestionapplication.DocumentEmbeddingReceiptResult{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return ingestionapplication.DocumentEmbeddingReceiptResult{}, databaserepository.MapError(err)
	}
	return documentEmbeddingReceiptResult(record), nil
}

func (writer *DocumentRecallProjectionWriter) queryRow(ctx context.Context, query string, arguments ...any) *sql.Row {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryRowContext(ctx, query, arguments...)
	}
	return writer.runtime.SQL.QueryRowContext(ctx, query, arguments...)
}

const persistDocumentSearchProjectionSQL = `
WITH exact AS MATERIALIZED (
  SELECT version.id AS document_version_id,document.source_connection_id,
         observation.title,artifact.id AS derived_artifact_id,
         LEAST(
           artifact.retention_until,
           $12::timestamptz + current_rights_retention_days(
             document.source_connection_id,'document_version',version.id::text,version.content_sha256,$12::timestamptz
           )*interval '24 hours'
         ) AS retention_until
  FROM document_versions AS version
  JOIN documents AS document ON document.id=version.document_id
  JOIN source_observations AS observation ON observation.id=version.source_observation_id
  JOIN derived_artifacts AS artifact
    ON artifact.id=$2 AND artifact.document_version_id=version.id
   AND artifact.source_connection_id=document.source_connection_id
  WHERE version.id=$1 AND version.content_sha256=$6
    AND version.lifecycle_state NOT IN ('policy_pending','policy_blocked','derive_failed','retention_blocked','quarantined','tombstoned')
    AND artifact.artifact_type='plaintext' AND artifact.sha256=$6
    AND artifact.lifecycle_state='derived_available' AND artifact.active
    AND artifact.store_derived_rights_decision_id=$3 AND artifact.retain_rights_decision_id=$4
    AND artifact.retention_until>$12::timestamptz
    AND current_rights_action_allowed($3,document.source_connection_id,'document_version',version.id::text,version.content_sha256,'store_derived',$12::timestamptz)
    AND current_rights_action_allowed($4,document.source_connection_id,'document_version',version.id::text,version.content_sha256,'retain',$12::timestamptz)
    AND current_rights_retention_days(document.source_connection_id,'document_version',version.id::text,version.content_sha256,$12::timestamptz) IS NOT NULL
), prepared AS MATERIALIZED (
  SELECT exact.*,
         to_tsvector('simple',COALESCE(exact.title,'')) AS title_search_vector,
         to_tsvector('simple',$7::text) AS body_search_vector,
         ARRAY(SELECT DISTINCT value FROM unnest(show_trgm(COALESCE(exact.title,''))) AS value ORDER BY value) AS title_trigrams,
         ARRAY(SELECT DISTINCT value FROM unnest(show_trgm($7::text)) AS value ORDER BY value) AS body_trigrams
  FROM exact
), inserted AS (
  INSERT INTO document_version_search_indexes (
    document_version_id,source_connection_id,derived_artifact_id,
    store_derived_rights_decision_id,retain_rights_decision_id,
    normalization_profile_version,normalized_text_sha256,
    title_search_vector,body_search_vector,title_trigrams,body_trigrams,
    entity_keys,action_keys,location_keys,region_keys,retention_until,indexed_at
  )
  SELECT document_version_id,source_connection_id,derived_artifact_id,$3,$4,$5,$6,
         title_search_vector,body_search_vector,title_trigrams,body_trigrams,
         $8::text[],$9::text[],$10::text[],$11::text[],retention_until,$12::timestamptz
  FROM prepared
  ON CONFLICT (document_version_id,normalization_profile_version,normalized_text_sha256) DO NOTHING
  RETURNING id,document_version_id,source_connection_id,derived_artifact_id,
            store_derived_rights_decision_id,retain_rights_decision_id,
            normalization_profile_version,normalized_text_sha256,
            retention_until,indexed_at,lifecycle_state,true AS created
), existing AS (
  SELECT stored.id,stored.document_version_id,stored.source_connection_id,stored.derived_artifact_id,
         stored.store_derived_rights_decision_id,stored.retain_rights_decision_id,
         stored.normalization_profile_version,stored.normalized_text_sha256,
         stored.retention_until,stored.indexed_at,stored.lifecycle_state,false AS created
  FROM document_version_search_indexes AS stored
  JOIN prepared
    ON prepared.document_version_id=stored.document_version_id
   AND stored.normalization_profile_version=$5 AND stored.normalized_text_sha256=$6
  WHERE NOT EXISTS (SELECT 1 FROM inserted)
    AND stored.lifecycle_state='active' AND stored.retention_until>$12::timestamptz
    AND stored.source_connection_id=prepared.source_connection_id
    AND stored.derived_artifact_id=prepared.derived_artifact_id
    AND stored.store_derived_rights_decision_id=$3 AND stored.retain_rights_decision_id=$4
    AND stored.title_search_vector=prepared.title_search_vector
    AND stored.body_search_vector=prepared.body_search_vector
    AND stored.title_trigrams=prepared.title_trigrams AND stored.body_trigrams=prepared.body_trigrams
    AND stored.entity_keys=$8::text[] AND stored.action_keys=$9::text[]
    AND stored.location_keys=$10::text[] AND stored.region_keys=$11::text[]
    AND current_rights_action_allowed($3,stored.source_connection_id,'document_version',stored.document_version_id::text,$6,'store_derived',$12::timestamptz)
    AND current_rights_action_allowed($4,stored.source_connection_id,'document_version',stored.document_version_id::text,$6,'retain',$12::timestamptz)
)
SELECT * FROM inserted UNION ALL SELECT * FROM existing LIMIT 1`

const persistDocumentEmbeddingReceiptSQL = `
WITH exact AS MATERIALIZED (
  SELECT version.id AS document_version_id,document.source_connection_id,
         LEAST(
  $10::timestamptz + current_rights_retention_days(
             document.source_connection_id,'document_version',version.id::text,version.content_sha256,$10::timestamptz
           )*interval '24 hours',
           search.retention_until
         ) AS retention_until
  FROM document_versions AS version
  JOIN documents AS document ON document.id=version.document_id
  JOIN ai_model_profiles AS model
    ON model.id=$4 AND model.version=$5 AND model.model_version=$6
   AND model.task_type='embedding' AND model.embedding_dimensions=1024
   AND model.enabled AND model.deleted_at IS NULL
  JOIN ai_runs AS run
    ON run.id=$9 AND run.status='succeeded' AND run.task_type='embedding'
   AND run.target_type='document_version' AND run.target_id=version.id
   AND run.model_profile_id=$4 AND run.model_profile_version=$5 AND run.model_version=$6
   AND run.input_hash=$7
  JOIN LATERAL (
    SELECT max(projection.retention_until) AS retention_until
    FROM document_version_search_indexes AS projection
    WHERE projection.document_version_id=version.id
      AND projection.source_connection_id=document.source_connection_id
      AND projection.normalized_text_sha256=$7
      AND projection.lifecycle_state='active' AND projection.retention_until>$10::timestamptz
      AND current_rights_action_allowed(
        projection.store_derived_rights_decision_id,projection.source_connection_id,
        'document_version',version.id::text,version.content_sha256,'store_derived',$10::timestamptz
      )
      AND current_rights_action_allowed(
        projection.retain_rights_decision_id,projection.source_connection_id,
        'document_version',version.id::text,version.content_sha256,'retain',$10::timestamptz
      )
  ) AS search ON search.retention_until IS NOT NULL
  WHERE version.id=$1 AND version.content_sha256=$7
    AND version.lifecycle_state NOT IN ('policy_pending','policy_blocked','derive_failed','retention_blocked','quarantined','tombstoned')
    AND current_rights_action_allowed($2,document.source_connection_id,'document_version',version.id::text,version.content_sha256,'embed_local',$10::timestamptz)
    AND current_rights_action_allowed($3,document.source_connection_id,'document_version',version.id::text,version.content_sha256,'retain',$10::timestamptz)
    AND current_rights_retention_days(document.source_connection_id,'document_version',version.id::text,version.content_sha256,$10::timestamptz) IS NOT NULL
), inserted AS (
  INSERT INTO document_version_embeddings (
    document_version_id,source_connection_id,embed_local_rights_decision_id,retain_rights_decision_id,
    model_profile_id,model_profile_version,model_version,normalized_text_sha256,
    embedding,ai_run_id,retention_until,created_at
  )
  SELECT document_version_id,source_connection_id,$2,$3,$4,$5,$6,$7,$8::halfvec,$9,retention_until,$10::timestamptz
  FROM exact WHERE retention_until>$10::timestamptz
  ON CONFLICT (document_version_id,model_profile_id,model_profile_version,model_version,normalized_text_sha256) DO NOTHING
  RETURNING id,document_version_id,source_connection_id,
            embed_local_rights_decision_id,retain_rights_decision_id,
            model_profile_id,model_profile_version,model_version,
            normalized_text_sha256,ai_run_id,retention_until,created_at,lifecycle_state,true AS created
), existing AS (
  SELECT stored.id,stored.document_version_id,stored.source_connection_id,
         stored.embed_local_rights_decision_id,stored.retain_rights_decision_id,
         stored.model_profile_id,stored.model_profile_version,stored.model_version,
         stored.normalized_text_sha256,stored.ai_run_id,stored.retention_until,stored.created_at,
         stored.lifecycle_state,false AS created
  FROM document_version_embeddings AS stored
  JOIN exact ON exact.document_version_id=stored.document_version_id
  WHERE NOT EXISTS (SELECT 1 FROM inserted)
    AND stored.lifecycle_state='active' AND stored.retention_until>$10::timestamptz
    AND stored.source_connection_id=exact.source_connection_id
    AND stored.embed_local_rights_decision_id=$2 AND stored.retain_rights_decision_id=$3
    AND stored.model_profile_id=$4 AND stored.model_profile_version=$5 AND stored.model_version=$6
    AND stored.normalized_text_sha256=$7 AND stored.embedding=$8::halfvec AND stored.ai_run_id=$9
    AND current_rights_action_allowed($2,stored.source_connection_id,'document_version',stored.document_version_id::text,$7,'embed_local',$10::timestamptz)
    AND current_rights_action_allowed($3,stored.source_connection_id,'document_version',stored.document_version_id::text,$7,'retain',$10::timestamptz)
)
SELECT * FROM inserted UNION ALL SELECT * FROM existing LIMIT 1`
