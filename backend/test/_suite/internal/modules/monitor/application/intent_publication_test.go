package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIntentPublicationStagesExactPreviewFactsAndCompletesPublishedProfile(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 21, 0, 0, 0, time.UTC)
	repository := &intentPublicationRepositoryFake{candidate: PublishableIntentProfileDTO{
		Exists: true, MonitorID: 7, ConfigVersionID: 301, PreviewRunID: 501, DraftID: 101,
		DraftResourceVersion: 4, IntentRevisionID: 401, PreviewCompiledProfileID: 701,
		CompilerVersion:          IntentCompilerVersion,
		MatchingAlgorithmVersion: "rrf-k60-v1", LexicalAlgorithmVersion: "fts-trgm-dice-v1",
		SemanticAlgorithmVersion: "halfvec-cosine-v1", StructuredAlgorithmVersion: "entity-hard-rule-v1",
		SearchNormalizationProfileVersion: IntentSearchNormalizationProfileVersion,
		SemanticState:                     "unavailable", SemanticUnavailableReason: IntentSemanticGenerationUnavailable,
		PreviewProfileHash: strings.Repeat("a", 64),
		Clauses: []CompiledIntentClauseDTO{
			{Operator: "must", Field: "action", Value: "acquisition", NormalizedValue: "acquisition", Origin: "intent_clause"},
			{Operator: "must_not", Field: "term", Value: "jobs", NormalizedValue: "jobs", Origin: "intent_clause"},
			{Operator: "should", Field: "phrase", Value: "Track AI acquisitions", NormalizedValue: "track ai acquisitions", Origin: "objective_derived"},
			{Operator: "must", Field: "language", Value: "en", NormalizedValue: "en", Origin: "intent_clause"},
		},
		Entities: []CompiledIntentEntityDTO{{CanonicalID: "company:hotkey", Aliases: []string{"Hot Key", "HotKey"}, NormalizedAliases: []string{"hot key", "hotkey"}}},
	}}
	repository.stageReceipt = StagePublishedIntentProfileReceiptDTO{CompiledProfileID: 801, Reused: false}
	service, err := NewIntentPublicationService(repository)
	if err != nil {
		t.Fatalf("NewIntentPublicationService(): %v", err)
	}

	prepared, err := service.Prepare(context.Background(), PrepareIntentPublicationCommand{MonitorID: 7, ConfigVersionID: 301})
	if err != nil {
		t.Fatalf("Prepare(): %v", err)
	}
	if !prepared.Publication.Enabled || prepared.Publication.CompiledProfileID != 801 || prepared.Publication.ProfileHash == "" {
		t.Fatalf("publication = %#v", prepared.Publication)
	}
	wantTerms := []CompiledCollectionTermDTO{
		{Value: "acquisition"}, {Value: "jobs", Excluded: true},
		{Value: "Track AI acquisitions"}, {Value: "Hot Key"}, {Value: "HotKey"},
	}
	if !reflect.DeepEqual(prepared.Publication.CollectionTerms, wantTerms) {
		t.Fatalf("collection terms = %#v, want %#v", prepared.Publication.CollectionTerms, wantTerms)
	}
	if !reflect.DeepEqual(prepared.Publication.LocaleClauses, []IntentClauseDTO{{Operator: "must", Field: "language", Value: "en"}}) {
		t.Fatalf("locale clauses = %#v", prepared.Publication.LocaleClauses)
	}
	if repository.stage.ProfileHash != prepared.Publication.ProfileHash || repository.stage.ConfigVersionID != 301 {
		t.Fatalf("stage command = %#v", repository.stage)
	}

	completedAt := now.Add(time.Minute)
	err = service.Complete(context.Background(), CompleteIntentPublicationCommand{
		Publication: prepared.Publication, PreviousConfigVersionID: 201, PublishedAt: completedAt,
	})
	if err != nil {
		t.Fatalf("Complete(): %v", err)
	}
	if repository.complete.CompiledProfileID != 801 || repository.complete.ConfigVersionID != 301 || repository.complete.PreviousConfigVersionID != 201 || !repository.complete.PublishedAt.Equal(completedAt) {
		t.Fatalf("complete command = %#v", repository.complete)
	}
}

func TestIntentPublicationKeepsLegacyPathOnlyWhenNoIntentDraftExists(t *testing.T) {
	t.Parallel()
	repository := &intentPublicationRepositoryFake{candidate: PublishableIntentProfileDTO{Exists: false}}
	service, err := NewIntentPublicationService(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Prepare(context.Background(), PrepareIntentPublicationCommand{MonitorID: 7, ConfigVersionID: 301})
	if err != nil || result.Publication.Enabled || repository.stages != 0 {
		t.Fatalf("legacy preparation = %#v / %v / stages=%d", result, err, repository.stages)
	}

	repository.candidate = PublishableIntentProfileDTO{Exists: true, MonitorID: 7, ConfigVersionID: 301}
	if _, err := service.Prepare(context.Background(), PrepareIntentPublicationCommand{MonitorID: 7, ConfigVersionID: 301}); !errors.Is(err, ErrIntentPublicationUnavailable) {
		t.Fatalf("incomplete v2 intent fell back to legacy: %v", err)
	}
}

type intentPublicationRepositoryFake struct {
	candidate    PublishableIntentProfileDTO
	stage        StagePublishedIntentProfileDTO
	stageReceipt StagePublishedIntentProfileReceiptDTO
	complete     CompletePublishedIntentProfileDTO
	stages       int
	err          error
}

func (repository *intentPublicationRepositoryFake) ReadPublishableIntentProfile(_ context.Context, _ ReadPublishableIntentProfileQuery) (PublishableIntentProfileDTO, error) {
	return repository.candidate, repository.err
}

func (repository *intentPublicationRepositoryFake) StagePublishedIntentProfile(_ context.Context, command StagePublishedIntentProfileDTO) (StagePublishedIntentProfileReceiptDTO, error) {
	repository.stages++
	repository.stage = command
	return repository.stageReceipt, repository.err
}

func (repository *intentPublicationRepositoryFake) CompletePublishedIntentProfile(_ context.Context, command CompletePublishedIntentProfileDTO) error {
	repository.complete = command
	return repository.err
}
