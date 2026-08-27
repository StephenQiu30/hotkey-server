package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestDocumentRecallProjectionWriterFeedsExactHybridReadersIdempotently(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "hybrid-recall-projection", 73)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	plaintext := []byte("authorized normalized document body")
	projection := newDerivedArtifactSaga(t, runtime, newKnowledgeProjectionPublisher(t, t.TempDir()), fixture.documentVersions)
	projected, err := projection.Project(context.Background(), ingestionapplication.ProjectDocumentCommand{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID, ExpectedDocumentVersion: fixture.persisted.DocumentVersion.Version,
		ArtifactType: ingestionapplication.DocumentProjectionPlaintext, TransformerProfileSHA256: strings.Repeat("7", 64),
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		ProjectionBytes: plaintext,
	})
	if err != nil {
		t.Fatalf("Project(plaintext) error = %v", err)
	}

	adapter, err := ingestionpostgres.NewDocumentRecallProjectionWriter(runtime)
	if err != nil {
		t.Fatalf("NewDocumentRecallProjectionWriter() error = %v", err)
	}
	service, err := ingestionapplication.NewDocumentRecallProjectionService(adapter)
	if err != nil {
		t.Fatalf("NewDocumentRecallProjectionService() error = %v", err)
	}
	indexedAt := time.Now().UTC().Truncate(time.Microsecond)
	searchCommand := ingestionapplication.PersistDocumentSearchProjectionCommand{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID, DerivedArtifactID: projected.Artifact.ID,
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		NormalizationProfileVersion: "canonical-search-v1", NormalizedTextSHA256: fixture.persisted.DocumentVersion.ContentSHA256,
		Plaintext: string(plaintext), EntityKeys: []string{"OpenAI"}, ActionKeys: []string{"Launch"},
		LocationKeys: []string{"San Francisco"}, RegionKeys: []string{"US"}, IndexedAt: indexedAt,
	}
	firstSearch, err := service.PersistSearchProjection(context.Background(), searchCommand)
	if err != nil {
		t.Fatalf("PersistSearchProjection(first) error = %v", err)
	}
	if firstSearch.DocumentVersionID != searchCommand.DocumentVersionID || firstSearch.SourceConnectionID != fixture.sourceID ||
		firstSearch.DerivedArtifactID != projected.Artifact.ID ||
		firstSearch.StoreDerivedRightsDecisionID != storeDecisionID || firstSearch.RetainRightsDecisionID != retainDecisionID ||
		firstSearch.NormalizationProfileVersion != searchCommand.NormalizationProfileVersion ||
		firstSearch.NormalizedTextSHA256 != searchCommand.NormalizedTextSHA256 ||
		!firstSearch.IndexedAt.Equal(indexedAt) || !firstSearch.RetentionUntil.After(indexedAt) ||
		firstSearch.LifecycleState != ingestionapplication.RecallAssetLifecycleActive {
		t.Fatalf("PersistSearchProjection(first) receipt = %#v", firstSearch)
	}
	secondSearch, err := service.PersistSearchProjection(context.Background(), searchCommand)
	if err != nil || secondSearch.ProjectionID != firstSearch.ProjectionID || secondSearch.Created {
		t.Fatalf("PersistSearchProjection(retry) = %#v/%v", secondSearch, err)
	}
	upgraded := searchCommand
	upgraded.NormalizationProfileVersion = "canonical-search-v2"
	upgradedSearch, err := service.PersistSearchProjection(context.Background(), upgraded)
	if err != nil || upgradedSearch.ProjectionID == firstSearch.ProjectionID || !upgradedSearch.Created {
		t.Fatalf("PersistSearchProjection(profile upgrade) = %#v/%v", upgradedSearch, err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO document_version_search_indexes (
  document_version_id,source_connection_id,derived_artifact_id,
  store_derived_rights_decision_id,retain_rights_decision_id,
  normalization_profile_version,normalized_text_sha256,
  title_search_vector,body_search_vector,title_trigrams,body_trigrams,
  entity_keys,action_keys,location_keys,region_keys,retention_until,indexed_at
)
SELECT document_version_id,source_connection_id,derived_artifact_id,
       store_derived_rights_decision_id,retain_rights_decision_id,
       'canonical-search-overlong',normalized_text_sha256,
       title_search_vector,body_search_vector,title_trigrams,body_trigrams,
       entity_keys,action_keys,location_keys,region_keys,$2,$1
FROM document_version_search_indexes WHERE id=$3`, indexedAt, indexedAt.Add(31*24*time.Hour), firstSearch.ProjectionID); err == nil {
		t.Fatal("overlong search projection retention was accepted")
	}
	conflicting := searchCommand
	conflicting.EntityKeys = []string{"different"}
	if _, err := service.PersistSearchProjection(context.Background(), conflicting); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("PersistSearchProjection(conflicting facts) error = %v", err)
	}

	embedDecisionID := createDocumentEmbeddingRights(t, runtime, fixture, 2)
	modelProfileID, modelProfileVersion, modelVersion := createDocumentEmbeddingProfile(t, runtime)
	aiRunID := createDocumentEmbeddingRun(t, runtime, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, modelProfileID, modelProfileVersion, modelVersion)
	vector := make([]float32, 1024)
	vector[0] = 1
	embeddingCommand := ingestionapplication.PersistDocumentEmbeddingReceiptCommand{
		DocumentVersionID:          fixture.persisted.DocumentVersion.ID,
		EmbedLocalRightsDecisionID: embedDecisionID, RetainRightsDecisionID: retainDecisionID,
		ModelProfileID: modelProfileID, ModelProfileVersion: modelProfileVersion, ModelVersion: modelVersion,
		NormalizedTextSHA256: fixture.persisted.DocumentVersion.ContentSHA256,
		Embedding:            vector, AIRunID: aiRunID, CreatedAt: indexedAt,
	}
	firstEmbedding, err := service.PersistEmbeddingReceipt(context.Background(), embeddingCommand)
	if err != nil {
		t.Fatalf("PersistEmbeddingReceipt(first) error = %v", err)
	}
	if firstEmbedding.DocumentVersionID != embeddingCommand.DocumentVersionID || firstEmbedding.SourceConnectionID != fixture.sourceID ||
		firstEmbedding.EmbedLocalRightsDecisionID != embedDecisionID || firstEmbedding.RetainRightsDecisionID != retainDecisionID ||
		firstEmbedding.ModelProfileID != modelProfileID || firstEmbedding.ModelProfileVersion != modelProfileVersion ||
		firstEmbedding.ModelVersion != modelVersion || firstEmbedding.NormalizedTextSHA256 != embeddingCommand.NormalizedTextSHA256 ||
		firstEmbedding.AIRunID != aiRunID || !firstEmbedding.CreatedAt.Equal(indexedAt) ||
		!firstEmbedding.RetentionUntil.After(indexedAt) || firstEmbedding.LifecycleState != ingestionapplication.RecallAssetLifecycleActive {
		t.Fatalf("PersistEmbeddingReceipt(first) receipt = %#v", firstEmbedding)
	}
	secondEmbedding, err := service.PersistEmbeddingReceipt(context.Background(), embeddingCommand)
	if err != nil || secondEmbedding.EmbeddingID != firstEmbedding.EmbeddingID || secondEmbedding.Created {
		t.Fatalf("PersistEmbeddingReceipt(retry) = %#v/%v", secondEmbedding, err)
	}
	overlongProfileID, overlongProfileVersion, overlongModelVersion := createDocumentEmbeddingProfile(t, runtime)
	overlongRunID := createDocumentEmbeddingRun(t, runtime, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, overlongProfileID, overlongProfileVersion, overlongModelVersion)
	if _, err := runtime.SQL.Exec(`
INSERT INTO document_version_embeddings (
  document_version_id,source_connection_id,embed_local_rights_decision_id,retain_rights_decision_id,
  model_profile_id,model_profile_version,model_version,normalized_text_sha256,
  embedding,ai_run_id,retention_until,created_at
)
SELECT document_version_id,source_connection_id,embed_local_rights_decision_id,retain_rights_decision_id,
       $2,$3,$4,normalized_text_sha256,embedding,$5,$6,$1
FROM document_version_embeddings WHERE id=$7`, indexedAt, overlongProfileID, overlongProfileVersion,
		overlongModelVersion, overlongRunID, indexedAt.Add(31*24*time.Hour), firstEmbedding.EmbeddingID); err == nil {
		t.Fatal("overlong embedding retention was accepted")
	}

	reader, err := ingestionpostgres.NewHybridDocumentRecallReader(runtime)
	if err != nil {
		t.Fatalf("NewHybridDocumentRecallReader() error = %v", err)
	}
	capturedAt := documentVersionCapturedAt(73)
	lexicalQuery := ingestionapplication.LexicalRecallQueryDTO{
		ConfigVersionID: 1, CompiledProfileID: 1, SearchNormalizationProfileVersion: "canonical-search-v1",
		Must: []ingestionapplication.RecallFilterDTO{
			{Operator: "must", Field: "language", Value: "EN"},
			{Operator: "must", Field: "source", Value: fmt.Sprint(fixture.sourceID)},
			{Operator: "must", Field: "action", Value: "Launch"},
			{Operator: "must", Field: "location", Value: "San Francisco"},
			{Operator: "must", Field: "region", Value: "US"},
			{Operator: "must", Field: "time_window", Value: capturedAt.Add(-time.Minute).Format(time.RFC3339) + "/" + capturedAt.Add(time.Minute).Format(time.RFC3339)},
		},
		Should: []ingestionapplication.RecallFilterDTO{
			{Operator: "should", Field: "term", Value: "not-present"},
			{Operator: "should", Field: "term", Value: "authorized"},
		},
		AlgorithmVersion: ingestionapplication.LexicalRecallAlgorithmVersion, Limit: ingestionapplication.LexicalRecallLimit,
	}
	lexicalHits, err := reader.RecallLexical(context.Background(), lexicalQuery)
	if err != nil || len(lexicalHits) != 1 || lexicalHits[0].DocumentVersionID != fixture.persisted.DocumentVersion.ID || lexicalHits[0].Rank != 1 {
		t.Fatalf("RecallLexical() = %#v/%v", lexicalHits, err)
	}
	structuredHits, err := reader.RecallStructured(context.Background(), ingestionapplication.StructuredRecallQueryDTO{
		ConfigVersionID: 1, CompiledProfileID: 1, SearchNormalizationProfileVersion: "canonical-search-v1",
		Should:           []ingestionapplication.RecallFilterDTO{{Operator: "should", Field: "action", Value: "launch"}},
		Entities:         []ingestionapplication.RecallEntityDTO{{CanonicalID: "openai"}},
		AlgorithmVersion: ingestionapplication.StructuredRecallAlgorithmVersion, Limit: ingestionapplication.StructuredRecallLimit,
	})
	if err != nil || len(structuredHits) != 1 || structuredHits[0].DocumentVersionID != fixture.persisted.DocumentVersion.ID {
		t.Fatalf("RecallStructured() = %#v/%v", structuredHits, err)
	}
	semanticHits, err := reader.RecallSemantic(context.Background(), ingestionapplication.SemanticRecallQueryDTO{
		ConfigVersionID: 1, CompiledProfileID: 1, SearchNormalizationProfileVersion: "canonical-search-v1",
		EmbeddingProfileID: modelProfileID, EmbeddingProfileVersion: modelProfileVersion, ModelVersion: modelVersion,
		QueryVector: vector, AlgorithmVersion: ingestionapplication.SemanticRecallAlgorithmVersion, Limit: ingestionapplication.SemanticRecallLimit,
	})
	if err != nil || len(semanticHits) != 1 || semanticHits[0].DocumentVersionID != fixture.persisted.DocumentVersion.ID || semanticHits[0].RawScore < 0.99 {
		t.Fatalf("RecallSemantic() = %#v/%v", semanticHits, err)
	}
	excludedHits, err := reader.RecallLexical(context.Background(), ingestionapplication.LexicalRecallQueryDTO{
		ConfigVersionID: 1, CompiledProfileID: 1, SearchNormalizationProfileVersion: "canonical-search-v1",
		Should:           []ingestionapplication.RecallFilterDTO{{Operator: "should", Field: "term", Value: "authorized"}},
		MustNot:          []ingestionapplication.RecallFilterDTO{{Operator: "must_not", Field: "term", Value: "document"}},
		AlgorithmVersion: ingestionapplication.LexicalRecallAlgorithmVersion, Limit: ingestionapplication.LexicalRecallLimit,
	})
	if err != nil || len(excludedHits) != 0 {
		t.Fatalf("RecallLexical(MUST_NOT) = %#v/%v", excludedHits, err)
	}

	var bodyLikeColumns int
	if err := runtime.SQL.QueryRow(`
SELECT count(*) FROM information_schema.columns
WHERE table_schema='public' AND table_name IN ('document_version_search_indexes','document_version_embeddings')
  AND column_name IN ('body','plaintext','content','markdown')`).Scan(&bodyLikeColumns); err != nil {
		t.Fatalf("inspect recall asset columns: %v", err)
	}
	if bodyLikeColumns != 0 {
		t.Fatalf("recall asset body-like columns = %d", bodyLikeColumns)
	}
	revocationPolicy := createDocumentRightsPolicy(t, runtime, fixture.sourceID, 3, time.Now().UTC().Add(-time.Hour))
	insertDocumentRightsDecisionWithOutcome(t, runtime, revocationPolicy, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, "store_derived", "deny", nil, nil, fixture.persisted.DocumentVersion.ID)
	insertDocumentRightsDecisionWithOutcome(t, runtime, revocationPolicy, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, "embed_local", "deny", nil, nil, fixture.persisted.DocumentVersion.ID)
	revokedHits, err := reader.RecallLexical(context.Background(), lexicalQuery)
	if err != nil || len(revokedHits) != 0 {
		t.Fatalf("RecallLexical(revoked before projection scrub) = %#v/%v", revokedHits, err)
	}
	scrubbedAt := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := runtime.SQL.Exec(`
UPDATE document_version_search_indexes SET lifecycle_state='tombstoned',
  title_search_vector=NULL,body_search_vector=NULL,title_trigrams=NULL,body_trigrams=NULL,
  entity_keys=NULL,action_keys=NULL,location_keys=NULL,region_keys=NULL,
  tombstoned_at=$2,purge_reason='rights_revoked'
WHERE document_version_id=$1 AND lifecycle_state='active'`, fixture.persisted.DocumentVersion.ID, scrubbedAt); err != nil {
		t.Fatalf("scrub revoked search projections: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE document_version_embeddings SET lifecycle_state='tombstoned',embedding=NULL,
  tombstoned_at=$2,purge_reason='rights_revoked'
WHERE document_version_id=$1 AND lifecycle_state='active'`, fixture.persisted.DocumentVersion.ID, scrubbedAt); err != nil {
		t.Fatalf("scrub revoked embeddings: %v", err)
	}
	var scrubbedSearch, scrubbedEmbeddings int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM document_version_search_indexes WHERE document_version_id=$1 AND lifecycle_state='tombstoned' AND title_search_vector IS NULL AND body_trigrams IS NULL`, fixture.persisted.DocumentVersion.ID).Scan(&scrubbedSearch); err != nil {
		t.Fatalf("count scrubbed search projections: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM document_version_embeddings WHERE document_version_id=$1 AND lifecycle_state='tombstoned' AND embedding IS NULL`, fixture.persisted.DocumentVersion.ID).Scan(&scrubbedEmbeddings); err != nil {
		t.Fatalf("count scrubbed embeddings: %v", err)
	}
	if scrubbedSearch != 2 || scrubbedEmbeddings != 1 {
		t.Fatalf("scrubbed search/embedding counts = %d/%d", scrubbedSearch, scrubbedEmbeddings)
	}
}

func TestSemanticDocumentRecallReportsUnavailableForExactEmptyCorpus(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	profileID, profileVersion, modelVersion := createDocumentEmbeddingProfile(t, runtime)
	reader, _ := ingestionpostgres.NewHybridDocumentRecallReader(runtime)
	vector := make([]float32, 1024)
	vector[0] = 1
	_, err := reader.RecallSemantic(context.Background(), ingestionapplication.SemanticRecallQueryDTO{
		ConfigVersionID: 1, CompiledProfileID: 1, SearchNormalizationProfileVersion: "canonical-search-v1",
		EmbeddingProfileID: profileID, EmbeddingProfileVersion: profileVersion, ModelVersion: modelVersion,
		QueryVector: vector, AlgorithmVersion: ingestionapplication.SemanticRecallAlgorithmVersion, Limit: ingestionapplication.SemanticRecallLimit,
	})
	if !errors.Is(err, ingestionapplication.ErrSemanticRecallUnavailable) {
		t.Fatalf("RecallSemantic(empty exact corpus) error = %v", err)
	}
}

func createDocumentEmbeddingRights(t *testing.T, runtime *database.Runtime, fixture derivedArtifactDocumentFixture, revision int64) int64 {
	t.Helper()
	policy := createDocumentRightsPolicy(t, runtime, fixture.sourceID, revision, time.Now().UTC().Add(-time.Hour))
	return insertDocumentRightsDecision(t, runtime, policy, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, "embed_local", nil, nil, fixture.persisted.DocumentVersion.ID)
}

func createDocumentEmbeddingProfile(t *testing.T, runtime *database.Runtime) (int64, int64, string) {
	t.Helper()
	modelVersion := "document-embedding-v1"
	var profileID, profileVersion int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO ai_model_profiles (
  name,task_type,provider,model_name,credential_ref,model_version,embedding_dimensions,
  timeout_seconds,max_attempts,max_cost,fallback_priority,enabled
) VALUES ('document-recall-' || md5(random()::text),'embedding','openai','text-embedding-3-large',
  'env:OPENAI_API_KEY',$1,1024,30,1,0.1000,100,true)
RETURNING id,version`, modelVersion).Scan(&profileID, &profileVersion); err != nil {
		t.Fatalf("create document embedding profile: %v", err)
	}
	return profileID, profileVersion, modelVersion
}

func createDocumentEmbeddingRun(t *testing.T, runtime *database.Runtime, documentVersionID int64, inputHash string, profileID, profileVersion int64, modelVersion string) int64 {
	t.Helper()
	reuseKey := fmt.Sprintf("%064x", time.Now().UnixNano())
	var runID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO ai_runs (
  workspace_key,skill_id,task_type,target_type,target_id,target_version,runtime_version,model_profile_id,prompt_version,schema_version,input_hash,status,
  model_profile_version,model_version,parameters_version,input_schema_version,evidence_set_hash,reuse_key,
  attempt,max_attempts,budget_day,cost
) VALUES ('default','content.embedding.v1','embedding','document_version',$1,1,'structured-provider-v1',$2,'document-recall-v1','v1',$3,'succeeded',
  $4,$5,'document-recall-v1','v1',repeat('e',64),$6,1,1,current_date,0.0100)
RETURNING id`, documentVersionID, profileID, inputHash, profileVersion, modelVersion, reuseKey).Scan(&runID); err != nil {
		t.Fatalf("create document embedding run: %v", err)
	}
	return runID
}
