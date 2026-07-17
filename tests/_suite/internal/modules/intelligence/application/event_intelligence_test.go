package application

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

func TestEventIntelligenceServiceBuildsStableEvidenceBoundRun(t *testing.T) {
	runner := &eventIntelligenceRunnerStub{result: StructuredExecutionResult{
		Status: "succeeded", Run: domain.Run{ID: 41, Status: domain.RunStatusSucceeded},
		Result: json.RawMessage(`{"title_zh":"事件","sentences":[{"text":"事实","evidence":[{"content_id":2,"locator":"title"}]}]}`),
	}}
	service := NewEventIntelligenceService(runner)
	input := EventIntelligenceInput{
		TaskType: domain.TaskTypeEventSummary, EventID: 7, EventKey: "evt_7",
		Evidence: []EventIntelligenceEvidence{
			{ContentID: 9, Locator: "body:1", Excerpt: "second"},
			{ContentID: 2, Locator: "title", Excerpt: "first"},
		},
	}
	result, err := service.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != "succeeded" || result.Run.ID != 41 || string(result.Result) != string(runner.result.Result) {
		t.Fatalf("Execute() = %#v, want successful safe result", result)
	}
	if runner.input.TaskType != domain.TaskTypeEventSummary || runner.input.TargetType != "event" || runner.input.TargetID != 7 ||
		runner.input.PromptVersion != eventSummaryPromptVersion || runner.input.InputSchemaVersion != "v1" || runner.input.SchemaVersion != "v1" ||
		runner.input.ParametersVersion != eventSummaryParametersVersion || len(runner.input.InputHash) != 64 || len(runner.input.EvidenceSetHash) != 64 {
		t.Fatalf("run input = %#v, want versioned event summary run", runner.input)
	}
	var payload struct {
		Evidence []EventIntelligenceEvidence `json:"evidence"`
	}
	if err := json.Unmarshal(runner.input.Input, &payload); err != nil {
		t.Fatalf("decode run input: %v", err)
	}
	if len(payload.Evidence) != 2 || payload.Evidence[0].ContentID != 2 || payload.Evidence[1].ContentID != 9 {
		t.Fatalf("canonical evidence = %#v, want stable content ordering", payload.Evidence)
	}

	runner.input = StructuredExecutionInput{}
	_, err = service.Execute(context.Background(), EventIntelligenceInput{
		TaskType: input.TaskType, EventID: input.EventID, EventKey: input.EventKey,
		Evidence: []EventIntelligenceEvidence{input.Evidence[1], input.Evidence[0]},
	})
	if err != nil {
		t.Fatalf("Execute(reordered evidence) error = %v", err)
	}
	if runner.input.InputHash == "" || runner.input.InputHash != resultInputHash(t, service, input) {
		t.Fatalf("reordered input hash = %q, want stable hash", runner.input.InputHash)
	}
}

func TestEventIntelligenceServiceKeepsSafeDegradationAndRejectsInvalidInput(t *testing.T) {
	runner := &eventIntelligenceRunnerStub{result: StructuredExecutionResult{Status: "degraded", ReasonCode: "ai_unavailable"}}
	service := NewEventIntelligenceService(runner)
	result, err := service.Execute(context.Background(), EventIntelligenceInput{
		TaskType: domain.TaskTypeEntityClaimExtraction, EventID: 7, EventKey: "evt_7",
		Evidence: []EventIntelligenceEvidence{{ContentID: 2, Locator: "title", Excerpt: "safe"}},
	})
	if err != nil || result.Status != "degraded" || result.ReasonCode != "ai_unavailable" || len(result.Result) != 0 {
		t.Fatalf("Execute(degraded) = %#v / %v, want safe degradation", result, err)
	}
	if _, err := service.Execute(context.Background(), EventIntelligenceInput{TaskType: domain.TaskTypeEventSummary, EventID: 7, EventKey: "evt_7"}); err == nil {
		t.Fatal("Execute(without evidence) error = nil, want rejection")
	}
}

type eventIntelligenceRunnerStub struct {
	input  StructuredExecutionInput
	result StructuredExecutionResult
	err    error
}

func (stub *eventIntelligenceRunnerStub) ExecuteStructured(_ context.Context, input StructuredExecutionInput) (StructuredExecutionResult, error) {
	stub.input = input
	return stub.result, stub.err
}

func resultInputHash(t *testing.T, service *EventIntelligenceService, input EventIntelligenceInput) string {
	t.Helper()
	prepared, err := service.prepare(input)
	if err != nil {
		t.Fatalf("prepare() error = %v", err)
	}
	return prepared.InputHash
}
