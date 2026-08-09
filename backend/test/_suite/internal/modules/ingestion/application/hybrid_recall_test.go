package application

import (
	"context"
	"errors"
	"testing"
)

type lexicalRecallStub struct {
	hits      []RecallHitDTO
	wantLimit int
	lastQuery LexicalRecallQueryDTO
}

func (stub *lexicalRecallStub) RecallLexical(_ context.Context, query LexicalRecallQueryDTO) ([]RecallHitDTO, error) {
	stub.lastQuery = query
	if query.Limit != stub.wantLimit {
		return nil, errors.New("unexpected lexical limit")
	}
	return append([]RecallHitDTO(nil), stub.hits...), nil
}

type structuredRecallStub struct {
	hits      []RecallHitDTO
	wantLimit int
}

func (stub *structuredRecallStub) RecallStructured(_ context.Context, query StructuredRecallQueryDTO) ([]RecallHitDTO, error) {
	if query.Limit != stub.wantLimit {
		return nil, errors.New("unexpected structured limit")
	}
	return append([]RecallHitDTO(nil), stub.hits...), nil
}

type semanticRecallStub struct {
	hits      []RecallHitDTO
	err       error
	wantLimit int
}

type readyRecallProfileStub struct {
	profile ReadyRecallProfileDTO
	err     error
}

func (stub readyRecallProfileStub) ReadReadyRecallProfile(_ context.Context, query ReadyRecallProfileQuery) (ReadyRecallProfileDTO, error) {
	if stub.err != nil {
		return ReadyRecallProfileDTO{}, stub.err
	}
	if query.MonitorID != stub.profile.MonitorID || query.Purpose != stub.profile.Purpose ||
		query.ConfigVersionID != stub.profile.ConfigVersionID || query.MonitorVersionID != stub.profile.MonitorVersionID ||
		query.CompiledProfileID != stub.profile.CompiledProfileID {
		return ReadyRecallProfileDTO{}, errors.New("unexpected exact profile identity")
	}
	return stub.profile, nil
}

func (stub *semanticRecallStub) RecallSemantic(_ context.Context, query SemanticRecallQueryDTO) ([]RecallHitDTO, error) {
	if query.Limit != stub.wantLimit {
		return nil, errors.New("unexpected semantic limit")
	}
	if stub.err != nil {
		return nil, stub.err
	}
	return append([]RecallHitDTO(nil), stub.hits...), nil
}

func TestHybridRecallFusesBoundedChannelsAndHardExcludesMustNot(t *testing.T) {
	lexical := &lexicalRecallStub{wantLimit: 100, hits: []RecallHitDTO{
		{DocumentVersionID: 101, Rank: 1, RawScore: 0.99},
		{DocumentVersionID: 102, Rank: 2, RawScore: 0.70, HardExcluded: true, ExclusionReasons: []string{"must_not:term:rumor"}},
	}}
	structured := &structuredRecallStub{wantLimit: 50, hits: []RecallHitDTO{
		{DocumentVersionID: 103, Rank: 1, RawScore: 1},
	}}
	semantic := &semanticRecallStub{wantLimit: 100, hits: []RecallHitDTO{
		{DocumentVersionID: 103, Rank: 1, RawScore: 0.91},
	}}
	query, profile := validHybridRecallQuery()
	service, err := NewHybridRecallService(readyRecallProfileStub{profile: profile}, lexical, structured, semantic)
	if err != nil {
		t.Fatalf("NewHybridRecallService() error = %v", err)
	}

	result, err := service.Recall(context.Background(), query)
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if result.Degraded || len(result.DegradationReasons) != 0 {
		t.Fatalf("unexpected degraded result: %#v", result)
	}
	if len(result.Candidates) != 2 || result.Candidates[0].DocumentVersionID != 103 || result.Candidates[1].DocumentVersionID != 101 {
		t.Fatalf("candidates = %#v, want 103 then 101 and no hard-excluded 102", result.Candidates)
	}
	if len(result.Candidates[0].Signals) != 2 || result.Candidates[0].Signals[0].AlgorithmVersion == "" {
		t.Fatalf("channel provenance was not preserved: %#v", result.Candidates[0].Signals)
	}
	if len(lexical.lastQuery.MustNot) != 1 || lexical.lastQuery.MustNot[0].Field != "term" {
		t.Fatalf("MUST_NOT was not passed as a hard lexical filter: %#v", lexical.lastQuery.MustNot)
	}
}

func TestHybridRecallMarksSemanticUnavailableWithoutFabricatingScore(t *testing.T) {
	query, profile := validHybridRecallQuery()
	service, err := NewHybridRecallService(
		readyRecallProfileStub{profile: profile},
		&lexicalRecallStub{wantLimit: 100, hits: []RecallHitDTO{{DocumentVersionID: 201, Rank: 1, RawScore: 0.8}}},
		&structuredRecallStub{wantLimit: 50},
		nil,
	)
	if err != nil {
		t.Fatalf("NewHybridRecallService() error = %v", err)
	}
	result, err := service.Recall(context.Background(), query)
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if !result.Degraded || len(result.DegradationReasons) != 1 || result.DegradationReasons[0] != "semantic_reader_unavailable" {
		t.Fatalf("degradation = %#v", result)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].SemanticScore != nil {
		t.Fatalf("semantic score must stay nil: %#v", result.Candidates)
	}
}

func TestHybridRecallSemanticFailureDegradesButRequiredChannelFailureDoesNot(t *testing.T) {
	semantic := &semanticRecallStub{wantLimit: 100, err: ErrSemanticRecallUnavailable}
	query, profile := validHybridRecallQuery()
	service, _ := NewHybridRecallService(
		readyRecallProfileStub{profile: profile},
		&lexicalRecallStub{wantLimit: 100, hits: []RecallHitDTO{{DocumentVersionID: 301, Rank: 1, RawScore: 0.7}}},
		&structuredRecallStub{wantLimit: 50}, semantic,
	)
	result, err := service.Recall(context.Background(), query)
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if !result.Degraded || result.DegradationReasons[0] != "semantic_recall_unavailable" || result.Candidates[0].SemanticScore != nil {
		t.Fatalf("semantic failure was not explicit and scoreless: %#v", result)
	}
}

func TestHybridRecallUsesPersistedSemanticUnavailabilityWithoutCallingReader(t *testing.T) {
	query, profile := validHybridRecallQuery()
	profile.SemanticState = SemanticRecallStateUnavailable
	profile.SemanticUnavailableReason = "semantic_model_unavailable"
	profile.Semantic = nil
	service, _ := NewHybridRecallService(
		readyRecallProfileStub{profile: profile},
		&lexicalRecallStub{wantLimit: 100, hits: []RecallHitDTO{{DocumentVersionID: 401, Rank: 1, RawScore: 0.6}}},
		&structuredRecallStub{wantLimit: 50}, &semanticRecallStub{wantLimit: 100},
	)
	result, err := service.Recall(context.Background(), query)
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if !result.Degraded || len(result.DegradationReasons) != 1 || result.DegradationReasons[0] != "semantic_model_unavailable" ||
		len(result.Candidates) != 1 || result.Candidates[0].SemanticScore != nil {
		t.Fatalf("result = %#v", result)
	}
}

func validHybridRecallQuery() (HybridRecallQuery, ReadyRecallProfileDTO) {
	vector := make([]float32, 1024)
	vector[0] = 1
	profile := ReadyRecallProfileDTO{
		MonitorID: 2, Purpose: "published", ConfigVersionID: 3, MonitorVersionID: 3, CompiledProfileID: 4,
		MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		LexicalAlgorithmVersion:  LexicalRecallAlgorithmVersion, SemanticAlgorithmVersion: SemanticRecallAlgorithmVersion, StructuredAlgorithmVersion: StructuredRecallAlgorithmVersion,
		SearchNormalizationProfileVersion: "canonical-search-v1",
		SemanticState:                     SemanticRecallStateReady,
		Clauses: []RecallClauseDTO{
			{Operator: "must", Field: "term", Value: "earthquake", Origin: "objective_derived"},
			{Operator: "must_not", Field: "term", Value: "rumor", Origin: "intent_clause"},
		},
		Entities: []RecallEntityDTO{{CanonicalID: "entity:earthquake", Aliases: []string{"quake"}}},
		Semantic: &SemanticRecallProfileDTO{EmbeddingProfileID: 8, EmbeddingProfileVersion: 1, ModelVersion: "embed-v1", QueryVector: vector},
	}
	return HybridRecallQuery{MonitorID: 2, Purpose: "published", ConfigVersionID: 3, MonitorVersionID: 3, CompiledProfileID: 4}, profile
}
