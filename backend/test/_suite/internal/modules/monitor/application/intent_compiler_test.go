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
		CompiledProfileID: 701, ConfigVersionID: 301, IntentRevisionID: 401, Reused: false,
	}}
	compiler, err := NewIntentCompiler(repository, fixedIntentClock{now: now})
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
	if result.Profile.SemanticState != "unavailable" || result.Profile.SemanticUnavailableReason != "semantic_generation_unavailable" {
		t.Fatalf("semantic degradation = %#v", result.Profile)
	}
	if repository.command.ProfileHash != result.Profile.ProfileHash || repository.command.ReadyAt != now || repository.command.Task != task {
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
	command PersistPreviewCompiledProfileDTO
	receipt PersistPreviewCompiledProfileReceiptDTO
	err     error
}

func (repository *compiledIntentProfileRepositoryFake) PersistPreviewCompiledProfile(_ context.Context, command PersistPreviewCompiledProfileDTO) (PersistPreviewCompiledProfileReceiptDTO, error) {
	repository.command = command
	return repository.receipt, repository.err
}
