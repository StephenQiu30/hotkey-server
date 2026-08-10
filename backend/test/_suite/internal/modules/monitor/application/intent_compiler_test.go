package application

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIntentCompilerPersistsExactPreviewProfileWithApprovedFactsOnly(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC)
	reviewer := int64(88)
	reviewedAt := now.Add(-time.Hour)
	draft := intentDraftFixture()
	draft.Objective = "Track AI acquisition announcements"
	draft.Entities = []IntentEntityDTO{{CanonicalID: "company:hotkey", DisplayName: "HotKey", Aliases: []string{"Hot Key"}}}
	draft.Candidates = []ExpansionCandidateDTO{
		{ID: "approved", Value: "takeover", Source: "llm", Reason: "related wording", ModelVersion: "model-v1", PromptVersion: "prompt-v1", InputHash: strings.Repeat("a", 64), Similarity: .8, Risk: "low", ApprovalStatus: "approved", ReviewerUserID: &reviewer, ReviewedAt: &reviewedAt},
		{ID: "pending", Value: "rumor", Source: "llm", Reason: "unreviewed wording", ModelVersion: "model-v1", PromptVersion: "prompt-v1", InputHash: strings.Repeat("a", 64), Similarity: .7, Risk: "high", ApprovalStatus: "pending"},
	}
	task := IntentAnalysisTaskDTO{Run: IntentRunReferenceDTO{
		RunID: 501, Kind: "preview", MonitorID: draft.MonitorID, DraftID: draft.DraftID,
		DraftResourceVersion: draft.ResourceVersion,
	}, AnalysisProfile: "hybrid-preview-v1", SampleLimit: 20}
	validatedDraft, err := intentDraftFromDTO(draft)
	if err != nil {
		t.Fatalf("intentDraftFromDTO(): %v", err)
	}
	task.Run.InputHash = intentPreviewInputHash(validatedDraft, task.AnalysisProfile, task.SampleLimit)
	repository := &compiledIntentProfileRepositoryFake{receipt: PersistPreviewCompiledProfileReceiptDTO{
		CompiledProfileID: 701, ConfigVersionID: 301, IntentRevisionID: 401, Status: "building",
		SemanticState: IntentSemanticStateUnavailable, SemanticUnavailableReason: IntentSemanticGenerationUnavailable,
	}}
	embeddings := &compiledIntentEmbeddingProducerFake{result: ProduceCompiledIntentEmbeddingResult{
		CompiledProfileID: 701, Availability: IntentEmbeddingAvailabilityDegraded, UnavailableReason: IntentSemanticModelUnavailable,
	}}
	compiler, err := NewIntentCompiler(repository, embeddings, fixedIntentClock{now: now})
	if err != nil {
		t.Fatalf("NewIntentCompiler(): %v", err)
	}

	result, err := compiler.CompilePreview(context.Background(), CompilePreviewIntentProfileCommand{
		Preview: PreparedIntentPreviewDTO{Task: task, Draft: draft},
	})
	if err != nil {
		t.Fatalf("CompilePreview(): %v", err)
	}
	if result.Profile.CompiledProfileID != 701 || result.Profile.ConfigVersionID != 301 || result.Profile.IntentRevisionID != 401 || result.Profile.ProfileHash == "" {
		t.Fatalf("compiled result = %#v", result)
	}
	if result.Profile.SemanticState != "unavailable" || result.Profile.SemanticUnavailableReason != IntentSemanticModelUnavailable {
		t.Fatalf("semantic degradation = %#v", result.Profile)
	}
	if repository.command.ProfileHash != result.Profile.ProfileHash || repository.command.ReadyAt != now || repository.command.Task != task ||
		repository.complete.SemanticUnavailableReason != IntentSemanticModelUnavailable || embeddings.command.Input == "" {
		t.Fatalf("persist command identity = %#v", repository.command)
	}
	if !containsCompiledClause(repository.command.Clauses, "should", "term", "takeover", "approved_candidate") {
		t.Fatalf("approved candidate was not compiled: %#v", repository.command.Clauses)
	}
	if containsCompiledValue(repository.command.Clauses, "rumor") {
		t.Fatalf("pending candidate influenced compiled profile: %#v", repository.command.Clauses)
	}
	if !containsCompiledClause(repository.command.Clauses, "should", "phrase", draft.Objective, "objective_derived") {
		t.Fatalf("objective clause missing: %#v", repository.command.Clauses)
	}
	if len(repository.command.Entities) != 1 || !reflect.DeepEqual(repository.command.Entities[0].Aliases, []string{"Hot Key", "HotKey"}) {
		t.Fatalf("compiled entities = %#v", repository.command.Entities)
	}

	repeated, err := compiler.CompilePreview(context.Background(), CompilePreviewIntentProfileCommand{Preview: PreparedIntentPreviewDTO{Task: task, Draft: draft}})
	if err != nil || repeated.Profile.ProfileHash != result.Profile.ProfileHash {
		t.Fatalf("deterministic compile = %#v / %v", repeated, err)
	}
}

func TestIntentCompilerFinalizesSemanticReadyAfterExactEmbeddingReceipt(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 11, 0, 0, 0, time.UTC)
	draft := intentDraftFixture()
	task := IntentAnalysisTaskDTO{Run: IntentRunReferenceDTO{
		RunID: 502, Kind: "preview", MonitorID: draft.MonitorID, DraftID: draft.DraftID,
		DraftResourceVersion: draft.ResourceVersion,
	}, AnalysisProfile: "hybrid-preview-v1", SampleLimit: 20}
	validatedDraft, err := intentDraftFromDTO(draft)
	if err != nil {
		t.Fatal(err)
	}
	task.Run.InputHash = intentPreviewInputHash(validatedDraft, task.AnalysisProfile, task.SampleLimit)
	repository := &compiledIntentProfileRepositoryFake{receipt: PersistPreviewCompiledProfileReceiptDTO{
		CompiledProfileID: 702, ConfigVersionID: 302, IntentRevisionID: 402, Status: "building",
		SemanticState: IntentSemanticStateUnavailable, SemanticUnavailableReason: IntentSemanticGenerationUnavailable,
	}}
	embeddings := &compiledIntentEmbeddingProducerFake{result: ProduceCompiledIntentEmbeddingResult{
		CompiledProfileID: 702, Availability: IntentEmbeddingAvailabilityReady,
		Receipt: &CompiledIntentEmbeddingReceiptDTO{
			EmbeddingID: 802, CompiledProfileID: 702, ConfigVersionID: 302,
			ModelProfileID: 81, ModelProfileVersion: 2, ModelVersion: "embedding-v1",
			InputHash: "placeholder", AIRunID: 91, CreatedAt: now,
		},
	}}
	compiler, err := NewIntentCompiler(repository, embeddings, fixedIntentClock{now: now})
	if err != nil {
		t.Fatal(err)
	}
	// The embedding input hash is produced by the compiler. Let the fake echo
	// the exact command instead of weakening the production validation.
	embeddings.result.Receipt.InputHash = ""
	embeddings.err = nil
	embeddings.echoInputHash = true
	result, err := compiler.CompilePreview(context.Background(), CompilePreviewIntentProfileCommand{
		Preview: PreparedIntentPreviewDTO{Task: task, Draft: draft},
	})
	if err != nil {
		t.Fatalf("CompilePreview(): %v", err)
	}
	if result.Profile.SemanticState != IntentSemanticStateReady || result.Profile.SemanticUnavailableReason != "" ||
		repository.complete.SemanticState != IntentSemanticStateReady || embeddings.command.InputHash == "" {
		t.Fatalf("semantic ready result=%#v complete=%#v", result.Profile, repository.complete)
	}
}

func containsCompiledClause(items []CompiledIntentClauseDTO, operator, field, value, origin string) bool {
	for _, item := range items {
		if item.Operator == operator && item.Field == field && item.Value == value && item.Origin == origin {
			return true
		}
	}
	return false
}

func containsCompiledValue(items []CompiledIntentClauseDTO, value string) bool {
	for _, item := range items {
		if item.Value == value {
			return true
		}
	}
	return false
}

type compiledIntentProfileRepositoryFake struct {
	command  PersistPreviewCompiledProfileDTO
	complete CompletePreviewCompiledProfileDTO
	receipt  PersistPreviewCompiledProfileReceiptDTO
	err      error
}

func (repository *compiledIntentProfileRepositoryFake) PersistPreviewCompiledProfile(_ context.Context, command PersistPreviewCompiledProfileDTO) (PersistPreviewCompiledProfileReceiptDTO, error) {
	repository.command = command
	return repository.receipt, repository.err
}

func (repository *compiledIntentProfileRepositoryFake) CompletePreviewCompiledProfile(_ context.Context, command CompletePreviewCompiledProfileDTO) (CompletePreviewCompiledProfileReceiptDTO, error) {
	repository.complete = command
	return CompletePreviewCompiledProfileReceiptDTO{
		CompiledProfileID: command.CompiledProfileID, Status: "ready", SemanticState: command.SemanticState,
		SemanticUnavailableReason: command.SemanticUnavailableReason,
	}, repository.err
}

type compiledIntentEmbeddingProducerFake struct {
	command       ProduceCompiledIntentEmbeddingCommand
	result        ProduceCompiledIntentEmbeddingResult
	err           error
	echoInputHash bool
}

func (producer *compiledIntentEmbeddingProducerFake) ProduceCompiledIntentEmbedding(_ context.Context, command ProduceCompiledIntentEmbeddingCommand) (ProduceCompiledIntentEmbeddingResult, error) {
	producer.command = command
	if producer.echoInputHash && producer.result.Receipt != nil {
		producer.result.Receipt.InputHash = command.InputHash
	}
	return producer.result, producer.err
}
