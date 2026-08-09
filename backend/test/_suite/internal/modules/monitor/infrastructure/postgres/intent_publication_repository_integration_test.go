package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
)

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
	if _, err := repository.PersistPreviewCompiledProfile(context.Background(), previewCommand); err != nil {
		t.Fatalf("persist preview profile: %v", err)
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
	publication, err := monitorapplication.NewIntentPublicationService(repository)
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
	if err := runtime.SQL.QueryRow(`
SELECT purpose,status,btrim(profile_hash) FROM monitor_compiled_profiles WHERE id=$1`, prepared.Publication.CompiledProfileID).Scan(&purpose, &status, &hash); err != nil {
		t.Fatal(err)
	}
	if purpose != "published" || status != "ready" || hash != prepared.Publication.ProfileHash {
		t.Fatalf("published profile = %s/%s/%s", purpose, status, hash)
	}
}
