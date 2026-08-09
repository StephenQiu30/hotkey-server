package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/pgvector/pgvector-go"
)

// HybridDocumentRecallReader owns exact DocumentVersion retrieval. It reads
// only versioned search/embedding projections and current rights; it never
// consults legacy contents, monitor_rules or content_embeddings.
type HybridDocumentRecallReader struct{ runtime *database.Runtime }

var (
	_ ingestionapplication.LexicalDocumentRecallReader    = (*HybridDocumentRecallReader)(nil)
	_ ingestionapplication.StructuredDocumentRecallReader = (*HybridDocumentRecallReader)(nil)
	_ ingestionapplication.SemanticDocumentRecallReader   = (*HybridDocumentRecallReader)(nil)
)

func NewHybridDocumentRecallReader(runtime *database.Runtime) (*HybridDocumentRecallReader, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("%w: database runtime is required", sharedrepository.ErrUnavailable)
	}
	return &HybridDocumentRecallReader{runtime: runtime}, nil
}

func (reader *HybridDocumentRecallReader) RecallLexical(ctx context.Context, query ingestionapplication.LexicalRecallQueryDTO) ([]ingestionapplication.RecallHitDTO, error) {
	if err := reader.validate(query.ConfigVersionID, query.CompiledProfileID, query.SearchNormalizationProfileVersion, query.AlgorithmVersion, ingestionapplication.LexicalRecallAlgorithmVersion, query.Limit, ingestionapplication.LexicalRecallLimit); err != nil {
		return nil, err
	}
	filters, err := compileDocumentRecallFilters(query.Must, query.Should, query.MustNot, query.Entities)
	if err != nil {
		return nil, err
	}
	if len(filters.PositiveTerms) == 0 {
		return []ingestionapplication.RecallHitDTO{}, nil
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, lexicalDocumentRecallSQL,
		query.SearchNormalizationProfileVersion,
		filters.PositiveTerms, filters.MustTerms, filters.MustNotTerms,
		filters.MustLanguages, filters.MustNotLanguages, filters.MustSources, filters.MustNotSources,
		filters.MustActions, filters.MustNotActions, filters.MustLocations, filters.MustNotLocations,
		filters.MustRegions, filters.MustNotRegions,
		filters.MustTimeStarts, filters.MustTimeEnds, filters.MustNotTimeStarts, filters.MustNotTimeEnds,
		query.Limit,
	)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	return scanDocumentRecallHits(rows)
}

func (reader *HybridDocumentRecallReader) RecallStructured(ctx context.Context, query ingestionapplication.StructuredRecallQueryDTO) ([]ingestionapplication.RecallHitDTO, error) {
	if err := reader.validate(query.ConfigVersionID, query.CompiledProfileID, query.SearchNormalizationProfileVersion, query.AlgorithmVersion, ingestionapplication.StructuredRecallAlgorithmVersion, query.Limit, ingestionapplication.StructuredRecallLimit); err != nil {
		return nil, err
	}
	filters, err := compileDocumentRecallFilters(query.Must, query.Should, query.MustNot, query.Entities)
	if err != nil {
		return nil, err
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, structuredDocumentRecallSQL,
		query.SearchNormalizationProfileVersion,
		filters.MustTerms, filters.MustNotTerms,
		filters.MustLanguages, filters.MustNotLanguages, filters.MustSources, filters.MustNotSources,
		filters.MustActions, filters.ShouldActions, filters.MustNotActions,
		filters.MustLocations, filters.ShouldLocations, filters.MustNotLocations,
		filters.MustRegions, filters.ShouldRegions, filters.MustNotRegions,
		filters.EntityKeys,
		filters.MustTimeStarts, filters.MustTimeEnds, filters.MustNotTimeStarts, filters.MustNotTimeEnds,
		query.Limit,
	)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	return scanDocumentRecallHits(rows)
}

func (reader *HybridDocumentRecallReader) RecallSemantic(ctx context.Context, query ingestionapplication.SemanticRecallQueryDTO) ([]ingestionapplication.RecallHitDTO, error) {
	if err := reader.validate(query.ConfigVersionID, query.CompiledProfileID, query.SearchNormalizationProfileVersion, query.AlgorithmVersion, ingestionapplication.SemanticRecallAlgorithmVersion, query.Limit, ingestionapplication.SemanticRecallLimit); err != nil {
		return nil, err
	}
	if !validSemanticRecallVector(query.QueryVector) || query.EmbeddingProfileID <= 0 || query.EmbeddingProfileVersion <= 0 || strings.TrimSpace(query.ModelVersion) == "" {
		return nil, sharedrepository.ErrInvalidInput
	}
	filters, err := compileDocumentRecallFilters(query.Must, nil, query.MustNot, nil)
	if err != nil {
		return nil, err
	}
	var eligibleEmbeddings int
	err = reader.runtime.SQL.QueryRowContext(ctx, semanticDocumentCorpusAvailabilitySQL,
		query.EmbeddingProfileID, query.EmbeddingProfileVersion, query.ModelVersion,
		query.SearchNormalizationProfileVersion,
	).Scan(&eligibleEmbeddings)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, databaserepository.MapError(err)
	}
	if eligibleEmbeddings == 0 {
		return nil, ingestionapplication.ErrSemanticRecallUnavailable
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, semanticDocumentRecallSQL,
		pgvector.NewHalfVector(query.QueryVector), query.EmbeddingProfileID, query.EmbeddingProfileVersion, query.ModelVersion,
		query.SearchNormalizationProfileVersion,
		filters.MustTerms, filters.MustNotTerms,
		filters.MustLanguages, filters.MustNotLanguages, filters.MustSources, filters.MustNotSources,
		filters.MustActions, filters.MustNotActions, filters.MustLocations, filters.MustNotLocations,
		filters.MustRegions, filters.MustNotRegions,
		filters.MustTimeStarts, filters.MustTimeEnds, filters.MustNotTimeStarts, filters.MustNotTimeEnds,
		query.Limit,
	)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, databaserepository.MapError(err)
	}
	defer rows.Close()
	return scanDocumentRecallHits(rows)
}

func (reader *HybridDocumentRecallReader) validate(configVersionID, compiledProfileID int64, normalizationVersion, gotAlgorithm, wantAlgorithm string, limit, wantLimit int) error {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		if wantAlgorithm == ingestionapplication.SemanticRecallAlgorithmVersion {
			return ingestionapplication.ErrSemanticRecallUnavailable
		}
		return sharedrepository.ErrUnavailable
	}
	if configVersionID <= 0 || compiledProfileID <= 0 || strings.TrimSpace(normalizationVersion) == "" || gotAlgorithm != wantAlgorithm || limit != wantLimit {
		return sharedrepository.ErrInvalidInput
	}
	return nil
}

func scanDocumentRecallHits(rows *sql.Rows) ([]ingestionapplication.RecallHitDTO, error) {
	result := []ingestionapplication.RecallHitDTO{}
	for rows.Next() {
		var record documentRecallHitRecord
		if err := rows.Scan(&record.DocumentVersionID, &record.Rank, &record.RawScore); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			return nil, databaserepository.MapError(err)
		}
		result = append(result, documentRecallHitDTO(record))
	}
	if err := rows.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, databaserepository.MapError(err)
	}
	return result, nil
}

func compileDocumentRecallFilters(must, should, mustNot []ingestionapplication.RecallFilterDTO, entities []ingestionapplication.RecallEntityDTO) (documentRecallFilterRecord, error) {
	var result documentRecallFilterRecord
	for _, filter := range must {
		if err := appendDocumentRecallFilter(&result, filter, "must"); err != nil {
			return documentRecallFilterRecord{}, err
		}
	}
	for _, filter := range should {
		if err := appendDocumentRecallFilter(&result, filter, "should"); err != nil {
			return documentRecallFilterRecord{}, err
		}
	}
	for _, filter := range mustNot {
		if err := appendDocumentRecallFilter(&result, filter, "must_not"); err != nil {
			return documentRecallFilterRecord{}, err
		}
	}
	for _, entity := range entities {
		result.EntityKeys = append(result.EntityKeys, normalizedRecallKey(entity.CanonicalID))
		for _, alias := range entity.Aliases {
			result.EntityKeys = append(result.EntityKeys, normalizedRecallKey(alias))
			result.PositiveTerms = append(result.PositiveTerms, normalizedRecallKey(alias))
		}
	}
	normalizeRecallFilterSlices(&result)
	return result, nil
}

func appendDocumentRecallFilter(target *documentRecallFilterRecord, filter ingestionapplication.RecallFilterDTO, operator string) error {
	value := normalizedRecallKey(filter.Value)
	if value == "" || filter.Operator != operator {
		return sharedrepository.ErrInvalidInput
	}
	switch filter.Field {
	case "term", "phrase":
		switch operator {
		case "must":
			target.MustTerms = append(target.MustTerms, value)
			target.PositiveTerms = append(target.PositiveTerms, value)
		case "should":
			target.PositiveTerms = append(target.PositiveTerms, value)
		case "must_not":
			target.MustNotTerms = append(target.MustNotTerms, value)
		}
	case "language":
		appendRecallByOperator(operator, value, &target.MustLanguages, nil, &target.MustNotLanguages)
	case "source":
		appendRecallByOperator(operator, value, &target.MustSources, nil, &target.MustNotSources)
	case "action":
		appendRecallByOperator(operator, value, &target.MustActions, &target.ShouldActions, &target.MustNotActions)
	case "location":
		appendRecallByOperator(operator, value, &target.MustLocations, &target.ShouldLocations, &target.MustNotLocations)
	case "region":
		appendRecallByOperator(operator, value, &target.MustRegions, &target.ShouldRegions, &target.MustNotRegions)
	case "time_window":
		start, end, err := parseRecallTimeWindow(filter.Value)
		if err != nil || operator == "should" {
			return sharedrepository.ErrInvalidInput
		}
		if operator == "must" {
			target.MustTimeStarts, target.MustTimeEnds = append(target.MustTimeStarts, start), append(target.MustTimeEnds, end)
		} else {
			target.MustNotTimeStarts, target.MustNotTimeEnds = append(target.MustNotTimeStarts, start), append(target.MustNotTimeEnds, end)
		}
	default:
		return sharedrepository.ErrInvalidInput
	}
	return nil
}

func appendRecallByOperator(operator, value string, must, should, mustNot *[]string) {
	switch operator {
	case "must":
		*must = append(*must, value)
	case "should":
		if should != nil {
			*should = append(*should, value)
		}
	case "must_not":
		*mustNot = append(*mustNot, value)
	}
}

func parseRecallTimeWindow(value string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 2 {
		return "", "", sharedrepository.ErrInvalidInput
	}
	start, startErr := time.Parse(time.RFC3339, parts[0])
	end, endErr := time.Parse(time.RFC3339, parts[1])
	if startErr != nil || endErr != nil || !end.After(start) {
		return "", "", sharedrepository.ErrInvalidInput
	}
	return start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), nil
}

func normalizedRecallKey(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func normalizeRecallFilterSlices(record *documentRecallFilterRecord) {
	for _, values := range []*[]string{
		&record.PositiveTerms, &record.MustTerms, &record.MustNotTerms,
		&record.MustLanguages, &record.MustNotLanguages, &record.MustSources, &record.MustNotSources,
		&record.MustActions, &record.ShouldActions, &record.MustNotActions,
		&record.MustLocations, &record.ShouldLocations, &record.MustNotLocations,
		&record.MustRegions, &record.ShouldRegions, &record.MustNotRegions, &record.EntityKeys,
		&record.MustTimeStarts, &record.MustTimeEnds, &record.MustNotTimeStarts, &record.MustNotTimeEnds,
	} {
		*values = sortedUniqueRecallKeys(*values)
	}
}

func sortedUniqueRecallKeys(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func validSemanticRecallVector(vector []float32) bool {
	if len(vector) != 1024 {
		return false
	}
	norm := float64(0)
	for _, value := range vector {
		converted := float64(value)
		if math.IsNaN(converted) || math.IsInf(converted, 0) {
			return false
		}
		norm += converted * converted
	}
	return norm > 0
}

const lexicalDocumentRecallSQL = `
WITH eligible AS MATERIALIZED (
  SELECT search.document_version_id,search.title_search_vector,search.body_search_vector,
         search.title_trigrams,search.body_trigrams
  FROM document_version_search_indexes AS search
  JOIN document_versions AS version ON version.id=search.document_version_id
  JOIN documents AS document ON document.id=version.document_id AND document.source_connection_id=search.source_connection_id
  JOIN source_connections AS source ON source.id=search.source_connection_id
  WHERE search.lifecycle_state='active' AND search.normalization_profile_version=$1
    AND search.retention_until>now()
    AND version.lifecycle_state NOT IN ('retention_blocked','quarantined','tombstoned')
    AND current_rights_action_allowed(search.store_derived_rights_decision_id,search.source_connection_id,'document_version',version.id::text,version.content_sha256,'store_derived',now())
    AND current_rights_action_allowed(search.retain_rights_decision_id,search.source_connection_id,'document_version',version.id::text,version.content_sha256,'retain',now())
    AND NOT EXISTS (SELECT 1 FROM unnest($3::text[]) AS required(value) WHERE NOT (
      search.title_search_vector @@ plainto_tsquery('simple',required.value)
      OR search.body_search_vector @@ plainto_tsquery('simple',required.value)
      OR show_trgm(required.value) <@ search.title_trigrams OR show_trgm(required.value) <@ search.body_trigrams))
    AND NOT EXISTS (SELECT 1 FROM unnest($4::text[]) AS forbidden(value) WHERE
      search.title_search_vector @@ plainto_tsquery('simple',forbidden.value)
      OR search.body_search_vector @@ plainto_tsquery('simple',forbidden.value)
      OR show_trgm(forbidden.value) <@ search.title_trigrams OR show_trgm(forbidden.value) <@ search.body_trigrams)
    AND (cardinality($5::text[])=0 OR lower(COALESCE(version.language,''))=ANY($5::text[]))
    AND NOT lower(COALESCE(version.language,''))=ANY($6::text[])
    AND (cardinality($7::text[])=0 OR lower(source.source_type)=ANY($7::text[]) OR source.id::text=ANY($7::text[]))
    AND NOT (lower(source.source_type)=ANY($8::text[]) OR source.id::text=ANY($8::text[]))
    AND $9::text[] <@ search.action_keys AND NOT search.action_keys && $10::text[]
    AND $11::text[] <@ search.location_keys AND NOT search.location_keys && $12::text[]
    AND $13::text[] <@ search.region_keys AND NOT search.region_keys && $14::text[]
    AND NOT EXISTS (SELECT 1 FROM generate_subscripts($15::text[],1) n WHERE NOT (version.captured_at >= ($15::text[])[n]::timestamptz AND version.captured_at < ($16::text[])[n]::timestamptz))
    AND NOT EXISTS (SELECT 1 FROM generate_subscripts($17::text[],1) n WHERE version.captured_at >= ($17::text[])[n]::timestamptz AND version.captured_at < ($18::text[])[n]::timestamptz)
), query_trigrams AS MATERIALIZED (
  SELECT show_trgm(array_to_string($2::text[],' ')) AS values
), scored AS (
  SELECT eligible.document_version_id,
    lexical.fts_score + lexical.title_dice + lexical.body_dice AS raw_score
  FROM eligible
  CROSS JOIN query_trigrams
  CROSS JOIN LATERAL (
    SELECT
      COALESCE((SELECT sum(ts_rank_cd(eligible.title_search_vector,plainto_tsquery('simple',term)) + ts_rank_cd(eligible.body_search_vector,plainto_tsquery('simple',term))) FROM unnest($2::text[]) term),0)::float8 AS fts_score,
      CASE WHEN cardinality(eligible.title_trigrams)+cardinality(query_trigrams.values)=0 THEN 0 ELSE
        (2.0*(SELECT count(*) FROM (SELECT unnest(eligible.title_trigrams) INTERSECT SELECT unnest(query_trigrams.values)) common)) /
        (cardinality(eligible.title_trigrams)+cardinality(query_trigrams.values)) END::float8 AS title_dice,
      CASE WHEN cardinality(eligible.body_trigrams)+cardinality(query_trigrams.values)=0 THEN 0 ELSE
        (2.0*(SELECT count(*) FROM (SELECT unnest(eligible.body_trigrams) INTERSECT SELECT unnest(query_trigrams.values)) common)) /
        (cardinality(eligible.body_trigrams)+cardinality(query_trigrams.values)) END::float8 AS body_dice
  ) lexical
  WHERE EXISTS (
    SELECT 1 FROM unnest($2::text[]) AS candidate(value)
    WHERE eligible.title_search_vector @@ plainto_tsquery('simple',candidate.value)
       OR eligible.body_search_vector @@ plainto_tsquery('simple',candidate.value)
       OR show_trgm(candidate.value) && eligible.title_trigrams
       OR show_trgm(candidate.value) && eligible.body_trigrams
  )
), ranked AS (
  SELECT document_version_id,row_number() OVER (ORDER BY raw_score DESC,document_version_id ASC)::integer AS rank,raw_score
  FROM scored
)
SELECT document_version_id,rank,raw_score FROM ranked ORDER BY rank LIMIT $19`

const structuredDocumentRecallSQL = `
WITH eligible AS MATERIALIZED (
  SELECT search.document_version_id,search.entity_keys,search.action_keys,search.location_keys,search.region_keys,
         search.title_search_vector,search.body_search_vector,search.title_trigrams,search.body_trigrams,
         version.language,version.captured_at,source.source_type,source.id AS source_id
  FROM document_version_search_indexes AS search
  JOIN document_versions AS version ON version.id=search.document_version_id
  JOIN documents AS document ON document.id=version.document_id AND document.source_connection_id=search.source_connection_id
  JOIN source_connections AS source ON source.id=search.source_connection_id
  WHERE search.lifecycle_state='active' AND search.normalization_profile_version=$1 AND search.retention_until>now()
    AND version.lifecycle_state NOT IN ('retention_blocked','quarantined','tombstoned')
    AND current_rights_action_allowed(search.store_derived_rights_decision_id,search.source_connection_id,'document_version',version.id::text,version.content_sha256,'store_derived',now())
    AND current_rights_action_allowed(search.retain_rights_decision_id,search.source_connection_id,'document_version',version.id::text,version.content_sha256,'retain',now())
    AND NOT EXISTS (SELECT 1 FROM unnest($2::text[]) required(value) WHERE NOT (search.title_search_vector @@ plainto_tsquery('simple',required.value) OR search.body_search_vector @@ plainto_tsquery('simple',required.value) OR show_trgm(required.value)<@search.title_trigrams OR show_trgm(required.value)<@search.body_trigrams))
    AND NOT EXISTS (SELECT 1 FROM unnest($3::text[]) forbidden(value) WHERE search.title_search_vector @@ plainto_tsquery('simple',forbidden.value) OR search.body_search_vector @@ plainto_tsquery('simple',forbidden.value) OR show_trgm(forbidden.value)<@search.title_trigrams OR show_trgm(forbidden.value)<@search.body_trigrams)
    AND (cardinality($4::text[])=0 OR lower(COALESCE(version.language,''))=ANY($4::text[])) AND NOT lower(COALESCE(version.language,''))=ANY($5::text[])
    AND (cardinality($6::text[])=0 OR lower(source.source_type)=ANY($6::text[]) OR source.id::text=ANY($6::text[])) AND NOT (lower(source.source_type)=ANY($7::text[]) OR source.id::text=ANY($7::text[]))
    AND $8::text[]<@search.action_keys AND NOT search.action_keys&&$10::text[]
    AND $11::text[]<@search.location_keys AND NOT search.location_keys&&$13::text[]
    AND $14::text[]<@search.region_keys AND NOT search.region_keys&&$16::text[]
    AND NOT EXISTS (SELECT 1 FROM generate_subscripts($18::text[],1) n WHERE NOT (version.captured_at>=($18::text[])[n]::timestamptz AND version.captured_at<($19::text[])[n]::timestamptz))
    AND NOT EXISTS (SELECT 1 FROM generate_subscripts($20::text[],1) n WHERE version.captured_at>=($20::text[])[n]::timestamptz AND version.captured_at<($21::text[])[n]::timestamptz)
), scored AS (
  SELECT document_version_id,
    (SELECT count(*) FROM (SELECT unnest(entity_keys) INTERSECT SELECT unnest($17::text[])) AS common)::float8
    + (SELECT count(*) FROM (SELECT unnest(action_keys) INTERSECT SELECT unnest($9::text[])) AS common)::float8
    + (SELECT count(*) FROM (SELECT unnest(location_keys) INTERSECT SELECT unnest($12::text[])) AS common)::float8
    + (SELECT count(*) FROM (SELECT unnest(region_keys) INTERSECT SELECT unnest($15::text[])) AS common)::float8
    + cardinality($8::text[]) + cardinality($11::text[]) + cardinality($14::text[])
    AS raw_score
  FROM eligible
), ranked AS (
  SELECT document_version_id,row_number() OVER (ORDER BY raw_score DESC,document_version_id ASC)::integer AS rank,raw_score
  FROM scored WHERE raw_score>0
)
SELECT document_version_id,rank,raw_score FROM ranked ORDER BY rank LIMIT $22`

const semanticDocumentCorpusAvailabilitySQL = `
SELECT count(*)
FROM document_version_embeddings AS embedding
JOIN document_versions AS version ON version.id=embedding.document_version_id
JOIN ai_model_profiles AS model ON model.id=embedding.model_profile_id
JOIN document_version_search_indexes AS search
  ON search.document_version_id=embedding.document_version_id
 AND search.normalized_text_sha256=embedding.normalized_text_sha256
 AND search.normalization_profile_version=$4 AND search.lifecycle_state='active'
WHERE embedding.lifecycle_state='active' AND embedding.retention_until>now()
  AND search.retention_until>now()
  AND embedding.model_profile_id=$1 AND embedding.model_profile_version=$2 AND embedding.model_version=$3
  AND model.version=$2 AND model.model_version=$3 AND model.task_type='embedding'
  AND model.embedding_dimensions=1024 AND model.enabled AND model.deleted_at IS NULL
  AND current_rights_action_allowed(embedding.embed_local_rights_decision_id,embedding.source_connection_id,'document_version',version.id::text,version.content_sha256,'embed_local',now())
  AND current_rights_action_allowed(embedding.retain_rights_decision_id,embedding.source_connection_id,'document_version',version.id::text,version.content_sha256,'retain',now())
  AND current_rights_action_allowed(search.store_derived_rights_decision_id,search.source_connection_id,'document_version',version.id::text,version.content_sha256,'store_derived',now())
  AND current_rights_action_allowed(search.retain_rights_decision_id,search.source_connection_id,'document_version',version.id::text,version.content_sha256,'retain',now())`

const semanticDocumentRecallSQL = `
WITH eligible AS MATERIALIZED (
  SELECT embedding.document_version_id,embedding.embedding <=> $1::halfvec AS cosine_distance
  FROM document_version_embeddings AS embedding
  JOIN document_versions AS version ON version.id=embedding.document_version_id
  JOIN documents AS document ON document.id=version.document_id AND document.source_connection_id=embedding.source_connection_id
  JOIN source_connections AS source ON source.id=embedding.source_connection_id
  JOIN ai_model_profiles AS model ON model.id=embedding.model_profile_id
  JOIN document_version_search_indexes AS search
    ON search.document_version_id=embedding.document_version_id AND search.normalized_text_sha256=embedding.normalized_text_sha256
   AND search.normalization_profile_version=$5 AND search.lifecycle_state='active'
  WHERE embedding.lifecycle_state='active' AND embedding.retention_until>now() AND search.retention_until>now()
    AND embedding.model_profile_id=$2 AND embedding.model_profile_version=$3 AND embedding.model_version=$4
    AND model.version=$3 AND model.model_version=$4 AND model.task_type='embedding' AND model.embedding_dimensions=1024 AND model.enabled AND model.deleted_at IS NULL
    AND version.lifecycle_state NOT IN ('retention_blocked','quarantined','tombstoned')
    AND current_rights_action_allowed(embedding.embed_local_rights_decision_id,embedding.source_connection_id,'document_version',version.id::text,version.content_sha256,'embed_local',now())
    AND current_rights_action_allowed(embedding.retain_rights_decision_id,embedding.source_connection_id,'document_version',version.id::text,version.content_sha256,'retain',now())
    AND current_rights_action_allowed(search.store_derived_rights_decision_id,search.source_connection_id,'document_version',version.id::text,version.content_sha256,'store_derived',now())
    AND current_rights_action_allowed(search.retain_rights_decision_id,search.source_connection_id,'document_version',version.id::text,version.content_sha256,'retain',now())
    AND NOT EXISTS (SELECT 1 FROM unnest($6::text[]) required(value) WHERE NOT (search.title_search_vector@@plainto_tsquery('simple',required.value) OR search.body_search_vector@@plainto_tsquery('simple',required.value) OR show_trgm(required.value)<@search.title_trigrams OR show_trgm(required.value)<@search.body_trigrams))
    AND NOT EXISTS (SELECT 1 FROM unnest($7::text[]) forbidden(value) WHERE search.title_search_vector@@plainto_tsquery('simple',forbidden.value) OR search.body_search_vector@@plainto_tsquery('simple',forbidden.value) OR show_trgm(forbidden.value)<@search.title_trigrams OR show_trgm(forbidden.value)<@search.body_trigrams)
    AND (cardinality($8::text[])=0 OR lower(COALESCE(version.language,''))=ANY($8::text[])) AND NOT lower(COALESCE(version.language,''))=ANY($9::text[])
    AND (cardinality($10::text[])=0 OR lower(source.source_type)=ANY($10::text[]) OR source.id::text=ANY($10::text[])) AND NOT (lower(source.source_type)=ANY($11::text[]) OR source.id::text=ANY($11::text[]))
    AND $12::text[]<@search.action_keys AND NOT search.action_keys&&$13::text[]
    AND $14::text[]<@search.location_keys AND NOT search.location_keys&&$15::text[]
    AND $16::text[]<@search.region_keys AND NOT search.region_keys&&$17::text[]
    AND NOT EXISTS (SELECT 1 FROM generate_subscripts($18::text[],1) n WHERE NOT (version.captured_at>=($18::text[])[n]::timestamptz AND version.captured_at<($19::text[])[n]::timestamptz))
    AND NOT EXISTS (SELECT 1 FROM generate_subscripts($20::text[],1) n WHERE version.captured_at>=($20::text[])[n]::timestamptz AND version.captured_at<($21::text[])[n]::timestamptz)
  ORDER BY embedding.embedding <=> $1::halfvec,embedding.document_version_id
  LIMIT $22
), ranked AS (
  SELECT document_version_id,row_number() OVER (ORDER BY cosine_distance ASC,document_version_id ASC)::integer AS rank,(1-cosine_distance)::float8 AS raw_score
  FROM eligible
)
SELECT document_version_id,rank,raw_score FROM ranked ORDER BY rank`
