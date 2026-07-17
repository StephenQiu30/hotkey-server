package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

func TestEventSummaryServiceValidatesCitationsAgainstInputSnapshot(t *testing.T) {
	source := eventIntelligenceSourceFixture()
	runner := &eventIntelligenceRunnerStub{result: intelligenceapplication.EventIntelligenceResult{
		Status: "succeeded", Run: intelligencedomain.Run{ID: 41},
		Result: json.RawMessage(`{"title_zh":"可信摘要","title_en":"Trusted summary","sentences":[{"text":"有证据的事实","evidence":[{"content_id":2,"locator":"excerpt","excerpt":"forged"}]}]}`),
	}}
	result, err := NewEventSummaryService(&eventIntelligenceReaderStub{source: source}, runner).Generate(context.Background(), source.Event.ID)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.RunID != 41 || result.Summary.Degraded || result.Summary.Version != "event-summary-v1" || result.Summary.Sentences[0].Evidence[0].Excerpt != "trusted evidence" {
		t.Fatalf("Generate() = %#v, want verified summary with trusted citation metadata", result)
	}
	if runner.input.TaskType != intelligencedomain.TaskTypeEventSummary || runner.input.EventID != source.Event.ID || len(runner.input.Evidence) != 2 {
		t.Fatalf("runner input = %#v, want bounded event summary input", runner.input)
	}
}

func TestEventSummaryServiceRejectsCitationOutsideInputSnapshot(t *testing.T) {
	source := eventIntelligenceSourceFixture()
	runner := &eventIntelligenceRunnerStub{result: intelligenceapplication.EventIntelligenceResult{
		Status: "succeeded", Run: intelligencedomain.Run{ID: 41},
		Result: json.RawMessage(`{"title_zh":"不可信摘要","sentences":[{"text":"不可信事实","evidence":[{"content_id":999,"locator":"excerpt"}]}]}`),
	}}
	if _, err := NewEventSummaryService(&eventIntelligenceReaderStub{source: source}, runner).Generate(context.Background(), source.Event.ID); err == nil {
		t.Fatal("Generate() error = nil, want rejection for outside citation")
	}
}

func TestEventSummaryServiceReturnsRepresentativeFallback(t *testing.T) {
	source := eventIntelligenceSourceFixture()
	runner := &eventIntelligenceRunnerStub{result: intelligenceapplication.EventIntelligenceResult{Status: "degraded", ReasonCode: "ai_unavailable"}}
	result, err := NewEventSummaryService(&eventIntelligenceReaderStub{source: source}, runner).Generate(context.Background(), source.Event.ID)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !result.Summary.Degraded || result.ReasonCode != "ai_unavailable" || result.RunID != 0 || result.Summary.Sentences[0].Evidence[0].ContentID != 2 {
		t.Fatalf("Generate() = %#v, want representative fact-only fallback", result)
	}
}

func eventIntelligenceSourceFixture() EventIntelligenceSource {
	now := time.Now().UTC()
	representative := int64(2)
	return EventIntelligenceSource{
		Event:    domain.Event{ID: 7, EventKey: "evt-7", TitleZH: "事件", TitleEN: "Event", RepresentativeContentID: &representative, FirstSeenAt: now, LastSeenAt: now},
		Evidence: []domain.EvidenceRef{{ContentID: 9, Locator: "title", Excerpt: "secondary evidence"}, {ContentID: 2, Locator: "excerpt", Excerpt: "trusted evidence"}},
	}
}

type eventIntelligenceReaderStub struct {
	source EventIntelligenceSource
	err    error
}

func (stub *eventIntelligenceReaderStub) LoadEventIntelligenceSource(_ context.Context, _ int64) (EventIntelligenceSource, error) {
	return stub.source, stub.err
}

type eventIntelligenceRunnerStub struct {
	input  intelligenceapplication.EventIntelligenceInput
	result intelligenceapplication.EventIntelligenceResult
	err    error
}

func (stub *eventIntelligenceRunnerStub) Execute(_ context.Context, input intelligenceapplication.EventIntelligenceInput) (intelligenceapplication.EventIntelligenceResult, error) {
	stub.input = input
	return stub.result, stub.err
}
