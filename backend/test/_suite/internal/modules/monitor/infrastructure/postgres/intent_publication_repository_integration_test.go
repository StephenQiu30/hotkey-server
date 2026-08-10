package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
)

type intentPublicationBackfillScheduler struct{}

func (intentPublicationBackfillScheduler) SchedulePublishedIntentBackfill(_ context.Context, command monitorapplication.SchedulePublishedIntentBackfillCommand) (monitorapplication.SchedulePublishedIntentBackfillResult, error) {
	return monitorapplication.SchedulePublishedIntentBackfillResult{
		MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID,
		CompiledProfileID: command.CompiledProfileID, JobID: 701, Created: true,
	}, nil
}

func TestIntentPublicationRepositoryPromotesSuccessfulExactPreviewInsidePublishTransaction(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := insertIntentRepositoryDraft(t, runtime, false)
	repository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	reserved, err := repository.ReserveAndEnqueue(context.Background(), intentPreviewReservation(fixture, now))
	if err != nil {
		t.Fatal(err)
	}
	startedAt := now.Add(time.Second)
	running := reserved.Run
	running.Status, running.StartedAt = "running", &startedAt
	if _, err := repository.SaveTransition(context.Background(), monitorapplication.IntentRunTransitionDTO{Expected: reserved.Run, Next: running}); err != nil {
		t.Fatal(err)
	}
	previewCommand := monitorapplication.PersistPreviewCompiledProfileDTO{
		Task: monitorapplication.IntentAnalysisTaskDTO{
			Run: monitorapplication.IntentRunReferenceDTO{
				RunID: running.ID, Kind: "preview", MonitorID: fixture.monitorID, DraftID: fixture.draftID,
				DraftResourceVersion: 1, InputHash: running.InputHash,
			}, AnalysisProfile: "preview-v1", SampleLimit: 25,
		},
		CompilerVersion:          monitorapplication.IntentCompilerVersion,
		MatchingAlgorithmVersion: "rrf-k60-v1", LexicalAlgorithmVersion: "fts-trgm-dice-v1",
		SemanticAlgorithmVersion: "halfvec-cosine-v1", StructuredAlgorithmVersion: "entity-hard-rule-v1",
		SearchNormalizationProfileVersion: monitorapplication.IntentSearchNormalizationProfileVersion,
		SemanticState:                     "unavailable", SemanticUnavailableReason: monitorapplication.IntentSemanticGenerationUnavailable,
		ProfileHash: strings.Repeat("a", 64), ReadyAt: startedAt,
		Clauses: []monitorapplication.CompiledIntentClauseDTO{
			{Operator: "must", Field: "action", Value: "launch", NormalizedValue: "launch", Origin: "intent_clause"},
			{Operator: "should", Field: "phrase", Value: "Track launch disruption", NormalizedValue: "track launch disruption", Origin: "objective_derived"},
		},
		Entities: []monitorapplication.CompiledIntentEntityDTO{{
			CanonicalID: "product:hotkey", Aliases: []string{"Hot Key", "HotKey"}, NormalizedAliases: []string{"hot key", "hotkey"},
		}},
	}
	previewReceipt, err := repository.PersistPreviewCompiledProfile(context.Background(), previewCommand)
	if err != nil {
		t.Fatalf("persist preview profile: %v", err)
	}
	inputHash := strings.Repeat("e", 64)
	modelProfileID, modelProfileVersion, modelVersion := createCompiledIntentEmbeddingProfile(t, runtime.SQL)
	aiRunID := createCompiledIntentEmbeddingRun(t, runtime.SQL, previewReceipt.CompiledProfileID, inputHash, modelProfileID, modelProfileVersion, modelVersion)
	vector := make([]float32, 1024)
	vector[0] = 1
	projection, err := monitorapplication.NewCompiledIntentEmbeddingProjectionService(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projection.Persist(context.Background(), monitorapplication.PersistCompiledIntentEmbeddingCommand{
		CompiledProfileID: previewReceipt.CompiledProfileID, ConfigVersionID: fixture.configID,
		ModelProfileID: modelProfileID, ModelProfileVersion: modelProfileVersion, ModelVersion: modelVersion,
		InputHash: inputHash, Embedding: vector, AIRunID: aiRunID, CreatedAt: startedAt,
	}); err != nil {
		t.Fatalf("persist preview embedding: %v", err)
	}
	if _, err := repository.CompletePreviewCompiledProfile(context.Background(), monitorapplication.CompletePreviewCompiledProfileDTO{
		CompiledProfileID: previewReceipt.CompiledProfileID, ConfigVersionID: fixture.configID,
		IntentRevisionID: previewReceipt.IntentRevisionID, ProfileHash: previewCommand.ProfileHash,
		SemanticState: monitorapplication.IntentSemanticStateReady, ReadyAt: previewCommand.ReadyAt,
	}); err != nil {
		t.Fatalf("complete preview profile: %v", err)
	}
	completedAt := startedAt.Add(time.Second)
	succeeded := running
	succeeded.Status, succeeded.CompletedAt = "succeeded", &completedAt
	if _, err := repository.CompletePreview(context.Background(), monitorapplication.CompletePreviewRunMutationDTO{
		Transition:        monitorapplication.IntentRunTransitionDTO{Expected: running, Next: succeeded},
		Preview:           monitorapplication.IntentPreviewDTO{Warnings: []string{"no_historical_candidates"}},
		ResultFingerprint: strings.Repeat("b", 64),
	}); err != nil {
		t.Fatalf("complete exact preview: %v", err)
	}
	publication, err := monitorapplication.NewIntentPublicationService(repository, intentPublicationBackfillScheduler{})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := publication.Prepare(context.Background(), monitorapplication.PrepareIntentPublicationCommand{
		MonitorID: fixture.monitorID, ConfigVersionID: fixture.configID,
	})
	if err != nil {
		t.Fatalf("Prepare(): %v", err)
	}
	if !prepared.Publication.Enabled || prepared.Publication.CompiledProfileID <= 0 || len(prepared.Publication.CollectionTerms) != 4 {
		t.Fatalf("prepared publication = %#v", prepared.Publication)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitor_config_versions
SET state='published',config_hash=$2,published_at=$3,version=version+1
WHERE id=$1`, fixture.configID, strings.Repeat("c", 64), completedAt); err != nil {
		t.Fatalf("publish fixture config: %v", err)
	}
	if err := publication.Complete(context.Background(), monitorapplication.CompleteIntentPublicationCommand{
		Publication: prepared.Publication, PublishedAt: completedAt,
	}); err != nil {
		t.Fatalf("Complete(): %v", err)
	}
	var purpose, status, hash string
	var sourcePreviewID int64
	if err := runtime.SQL.QueryRow(`
SELECT purpose,status,btrim(profile_hash),source_preview_compiled_profile_id
FROM monitor_compiled_profiles WHERE id=$1`, prepared.Publication.CompiledProfileID).Scan(&purpose, &status, &hash, &sourcePreviewID); err != nil {
		t.Fatal(err)
	}
	if purpose != "published" || status != "ready" || hash != prepared.Publication.ProfileHash || sourcePreviewID != previewReceipt.CompiledProfileID {
		t.Fatalf("published profile = %s/%s/%s", purpose, status, hash)
	}
	var embeddingCount int
	if err := runtime.SQL.QueryRow(`
SELECT count(*) FROM monitor_compiled_intent_embeddings
WHERE compiled_profile_id IN ($1,$2) AND ai_run_id=$3 AND input_hash=$4`,
		previewReceipt.CompiledProfileID, prepared.Publication.CompiledProfileID, aiRunID, inputHash).Scan(&embeddingCount); err != nil {
		t.Fatal(err)
	}
	if embeddingCount != 2 {
		t.Fatalf("preview/published embedding count=%d, want copied exact pair", embeddingCount)
	}
}

func createCompiledIntentEmbeddingProfile(t *testing.T, databaseExecutor interface {
	QueryRow(string, ...any) *sql.Row
}) (int64, int64, string) {
	t.Helper()
	modelVersion := "intent-embedding-v1"
	var profileID, profileVersion int64
	if err := databaseExecutor.QueryRow(`
INSERT INTO ai_model_profiles (
  name,task_type,provider,model_name,credential_ref,model_version,embedding_dimensions,
  timeout_seconds,max_attempts,max_cost,fallback_priority,enabled
) VALUES ('intent-embedding-' || md5(random()::text),'embedding','openai','text-embedding-test',
  'env:OPENAI_API_KEY',$1,1024,30,1,0.1000,100,true)
RETURNING id,version`, modelVersion).Scan(&profileID, &profileVersion); err != nil {
		t.Fatalf("create intent embedding profile: %v", err)
	}
	return profileID, profileVersion, modelVersion
}

func createCompiledIntentEmbeddingRun(t *testing.T, databaseExecutor interface {
	QueryRow(string, ...any) *sql.Row
}, compiledProfileID int64, inputHash string, profileID, profileVersion int64, modelVersion string) int64 {
	t.Helper()
	var runID int64
	if err := databaseExecutor.QueryRow(`
INSERT INTO ai_runs (
  task_type,target_type,target_id,model_profile_id,prompt_version,schema_version,input_hash,status,
  model_profile_version,model_version,parameters_version,input_schema_version,evidence_set_hash,reuse_key,
  attempt,max_attempts,budget_day,cost
) VALUES ('embedding','monitor_compiled_profile',$1,$2,'compiled-intent-embedding-v1','embedding-output-v1',$3,'succeeded',
  $4,$5,'compiled-intent-nfc-1024-v1','compiled-intent-input-v1',$3,$6,1,1,current_date,0.0100)
RETURNING id`, compiledProfileID, profileID, inputHash, profileVersion, modelVersion,
		fmt.Sprintf("%064x", time.Now().UnixNano())).Scan(&runID); err != nil {
		t.Fatalf("create compiled intent embedding run: %v", err)
	}
	return runID
}
