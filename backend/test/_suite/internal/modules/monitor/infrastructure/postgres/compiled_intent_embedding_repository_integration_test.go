package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/infrastructure/postgres"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
)

func TestCompiledIntentEmbeddingCommitsWithItsExactSucceededAIRun(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := insertIntentRepositoryDraft(t, runtime, false)
	intentRepository, err := monitorpostgres.NewIntentRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	reserved, err := intentRepository.ReserveAndEnqueue(context.Background(), intentPreviewReservation(fixture, now))
	if err != nil {
		t.Fatal(err)
	}
	running := reserved.Run
	running.Status, running.StartedAt = "running", pointerToTime(now.Add(time.Second))
	if _, err := intentRepository.SaveTransition(context.Background(), monitorapplication.IntentRunTransitionDTO{Expected: reserved.Run, Next: running}); err != nil {
		t.Fatal(err)
	}
	profile, err := intentRepository.PersistPreviewCompiledProfile(context.Background(), monitorapplication.PersistPreviewCompiledProfileDTO{
		Task: monitorapplication.IntentAnalysisTaskDTO{Run: monitorapplication.IntentRunReferenceDTO{
			RunID: running.ID, Kind: "preview", MonitorID: fixture.monitorID, DraftID: fixture.draftID,
			DraftResourceVersion: 1, InputHash: running.InputHash,
		}, AnalysisProfile: "preview-v1", SampleLimit: 25},
		CompilerVersion: monitorapplication.IntentCompilerVersion, MatchingAlgorithmVersion: "rrf-k60-v1",
		LexicalAlgorithmVersion: "fts-trgm-dice-v1", SemanticAlgorithmVersion: "halfvec-cosine-v1",
		StructuredAlgorithmVersion: "entity-hard-rule-v1", SearchNormalizationProfileVersion: monitorapplication.IntentSearchNormalizationProfileVersion,
		SemanticState: "unavailable", SemanticUnavailableReason: monitorapplication.IntentSemanticGenerationUnavailable,
		ProfileHash: strings.Repeat("a", 64), ReadyAt: now.Add(time.Second),
		Clauses: []monitorapplication.CompiledIntentClauseDTO{{
			Operator: "must", Field: "term", Value: "PostgreSQL", NormalizedValue: "postgresql", Origin: "intent_clause",
		}},
	})
	if err != nil {
		t.Fatalf("persist preview profile: %v", err)
	}
	modelProfileID, modelProfileVersion, modelVersion := createCompiledIntentEmbeddingProfile(t, runtime.SQL)
	inputHash := strings.Repeat("b", 64)
	intelligenceRepository := intelligencepostgres.NewRepository(runtime)
	claimed, err := intelligenceRepository.Claim(context.Background(), intelligencepostgres.ClaimInput{
		TaskType: intelligencedomain.TaskTypeEmbedding, TargetType: "monitor_compiled_profile", TargetID: profile.CompiledProfileID,
		ModelProfileID: modelProfileID, PromptVersion: "compiled-intent-embedding-v1", InputSchemaVersion: "compiled-intent-input-v1",
		SchemaVersion: "embedding-output-v1", ParametersVersion: "compiled-intent-nfc-1024-v1",
		InputHash: inputHash, EvidenceSetHash: inputHash, Now: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("claim exact embedding run: %v", err)
	}
	if _, err := intelligenceRepository.Transition(context.Background(), claimed.Run.ID, intelligencedomain.RunStatusRunning, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := intelligenceRepository.Transition(context.Background(), claimed.Run.ID, intelligencedomain.RunStatusValidating, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	projection, err := monitorapplication.NewCompiledIntentEmbeddingProjectionService(intentRepository)
	if err != nil {
		t.Fatal(err)
	}
	vector := make([]float32, 1024)
	vector[0] = 1
	if err := intelligenceRepository.CompleteProjectedEmbedding(context.Background(), intelligencepostgres.ProjectedEmbeddingCompletion{
		RunID: claimed.Run.ID, TargetType: "monitor_compiled_profile", TargetID: profile.CompiledProfileID,
		ModelProfileID: modelProfileID, ModelProfileVersion: modelProfileVersion, ModelVersion: modelVersion,
		InputHash: inputHash, Vector: vector, Usage: intelligencedomain.Usage{}, LatencyMS: 7, FinishedAt: now.Add(5 * time.Second),
	}, func(transactionContext context.Context) error {
		_, persistErr := projection.Persist(transactionContext, monitorapplication.PersistCompiledIntentEmbeddingCommand{
			CompiledProfileID: profile.CompiledProfileID, ConfigVersionID: fixture.configID,
			ModelProfileID: modelProfileID, ModelProfileVersion: modelProfileVersion, ModelVersion: modelVersion,
			InputHash: inputHash, Embedding: vector, AIRunID: claimed.Run.ID, CreatedAt: now.Add(5 * time.Second),
		})
		return persistErr
	}); err != nil {
		t.Fatalf("complete projected embedding transaction: %v", err)
	}
	receipt, err := projection.Read(context.Background(), monitorapplication.ReadCompiledIntentEmbeddingQuery{
		CompiledProfileID: profile.CompiledProfileID, ConfigVersionID: fixture.configID,
		ModelProfileID: modelProfileID, ModelProfileVersion: modelProfileVersion, ModelVersion: modelVersion,
		InputHash: inputHash, AIRunID: claimed.Run.ID,
	})
	if err != nil || receipt.AIRunID != claimed.Run.ID || receipt.CompiledProfileID != profile.CompiledProfileID {
		t.Fatalf("read committed embedding = %#v / %v", receipt, err)
	}
	var runStatus string
	if err := runtime.SQL.QueryRow(`SELECT status FROM ai_runs WHERE id=$1`, claimed.Run.ID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus != "succeeded" {
		t.Fatalf("AI run status = %q, want succeeded", runStatus)
	}
}

func pointerToTime(value time.Time) *time.Time { return &value }
