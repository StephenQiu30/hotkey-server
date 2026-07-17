package application

import (
	"context"
	"encoding/json"
	"testing"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

func TestEventClaimExtractionServicePersistsOnlyVerifiedFacts(t *testing.T) {
	source := eventIntelligenceSourceFixture()
	store := &eventIntelligenceFactStoreStub{}
	runner := &eventIntelligenceRunnerStub{result: intelligenceapplication.EventIntelligenceResult{
		Status: "succeeded", Run: intelligencedomain.Run{ID: 51},
		Result: json.RawMessage(`{"entities":[{"entity_key":"acme","entity_type":"organization","canonical_name":"Acme"}],"claims":[{"claim":"Acme announced a release.","evidence":[{"content_id":2,"locator":"excerpt","excerpt":"forged","stance":"supports","confidence":90}]}]}`),
	}}
	result, err := NewEventClaimExtractionService(&eventIntelligenceReaderStub{source: source}, runner, store).Extract(context.Background(), source.Event.ID)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if result.Status != "succeeded" || result.RunID != 51 || runner.input.TaskType != intelligencedomain.TaskTypeEntityClaimExtraction || store.calls != 1 {
		t.Fatalf("Extract() = %#v, runner=%#v store calls=%d", result, runner.input, store.calls)
	}
	if len(store.facts.Entities) != 1 || store.facts.Entities[0].Entity.Key != "acme" || len(store.facts.Claims) != 1 || store.facts.Claims[0].Status != "single_source" || store.facts.Claims[0].Evidence[0].Excerpt != "trusted evidence" {
		t.Fatalf("persisted facts = %#v, want trusted model candidates", store.facts)
	}
}

func TestEventClaimExtractionServiceRejectsOutsideCitationAndSkipsDegradedRun(t *testing.T) {
	source := eventIntelligenceSourceFixture()
	store := &eventIntelligenceFactStoreStub{}
	runner := &eventIntelligenceRunnerStub{result: intelligenceapplication.EventIntelligenceResult{
		Status: "succeeded", Run: intelligencedomain.Run{ID: 51},
		Result: json.RawMessage(`{"entities":[],"claims":[{"claim":"Unsupported","evidence":[{"content_id":999,"locator":"excerpt"}]}]}`),
	}}
	if _, err := NewEventClaimExtractionService(&eventIntelligenceReaderStub{source: source}, runner, store).Extract(context.Background(), source.Event.ID); err == nil || store.calls != 0 {
		t.Fatalf("Extract(outside citation) error/calls = %v/%d, want rejection before persistence", err, store.calls)
	}
	runner.result = intelligenceapplication.EventIntelligenceResult{Status: "degraded", ReasonCode: "ai_unavailable"}
	result, err := NewEventClaimExtractionService(&eventIntelligenceReaderStub{source: source}, runner, store).Extract(context.Background(), source.Event.ID)
	if err != nil || result.Status != "degraded" || result.ReasonCode != "ai_unavailable" || store.calls != 0 {
		t.Fatalf("Extract(degraded) = %#v / %v calls=%d", result, err, store.calls)
	}
}

type eventIntelligenceFactStoreStub struct {
	facts ExtractedEventFacts
	calls int
	err   error
}

func (stub *eventIntelligenceFactStoreStub) PersistExtractedFacts(_ context.Context, facts ExtractedEventFacts) (PersistedEventFacts, error) {
	stub.calls++
	stub.facts = facts
	return PersistedEventFacts{}, stub.err
}
