package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

func TestIntentExpansionProcessorUsesExactImmutableDraftAndRealModelOutput(t *testing.T) {
	t.Parallel()
	task := intentExpansionTaskFixture(41)
	preparer := &intentExpansionPreparerFake{result: monitorapplication.PrepareIntentExpansionResult{
		Expansion: monitorapplication.PreparedIntentExpansionDTO{Task: task, Draft: intentExpansionDraftFixture()},
	}}
	runner := &intentExpansionStructuredRunnerFake{result: intelligenceapplication.StructuredExecutionResult{
		Status: "succeeded",
		Run:    intelligencedomain.Run{ID: 901, TaskType: intelligencedomain.TaskTypeTermExpansion, ModelVersion: "actual-model-2026-08"},
		Result: json.RawMessage(`{"terms":[
			{"term":"e\u0301lan disruption","language":"en","reason":"semantic wording related to the monitoring objective","similarity":0.84,"risk":"low"},
			{"term":"ÉLAN DISRUPTION","language":"en","reason":"case variant","similarity":0.70,"risk":"medium"},
			{"term":"launch","language":"en","reason":"already supplied wording","similarity":0.90,"risk":"low"},
			{"term":"test environment outage","language":"en","reason":"excluded context","similarity":0.60,"risk":"high"},
			{"term":"service interruption","language":"en","reason":"adjacent incident terminology","similarity":0.79,"risk":"medium"}
		]}`),
	}}
	processor, err := newIntentAnalysisCompositeProcessor(preparer, runner)
	if err != nil {
		t.Fatalf("newIntentAnalysisCompositeProcessor(): %v", err)
	}

	candidates, err := processor.GenerateExpansion(context.Background(), task)
	if err != nil {
		t.Fatalf("GenerateExpansion(): %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v, want normalized unique and conflict-filtered pair", candidates)
	}
	if candidates[0].Value != "élan disruption" || candidates[1].Value != "service interruption" {
		t.Fatalf("normalized candidate values = %#v", candidates)
	}
	for _, candidate := range candidates {
		if candidate.Source != "llm" || candidate.ModelVersion != "actual-model-2026-08" || candidate.PromptVersion != IntentExpansionPromptVersion || candidate.InputHash != task.Run.InputHash || candidate.ApprovalStatus != "pending" {
			t.Fatalf("candidate provenance = %#v", candidate)
		}
		if strings.Contains(strings.ToLower(candidate.Reason), "probability") || strings.Contains(candidate.Reason, "%") {
			t.Fatalf("candidate reason is probability-shaped: %q", candidate.Reason)
		}
	}
	if candidates[0].ID != deterministicIntentExpansionCandidateID(task.Run.RunID, "élan disruption") {
		t.Fatalf("candidate ID = %q, want exact run plus normalized term identity", candidates[0].ID)
	}
	if preparer.query.Task.Run.RunID != task.Run.RunID || preparer.query.Task.Run.DraftID != task.Run.DraftID || preparer.query.Task.Run.DraftResourceVersion != task.Run.DraftResourceVersion {
		t.Fatalf("preparation query = %#v", preparer.query)
	}
	if runner.input.TargetID != task.Run.RunID || runner.input.TargetType != IntentExpansionTargetType || runner.input.InputHash != task.Run.InputHash || runner.input.PromptVersion != IntentExpansionPromptVersion {
		t.Fatalf("AI execution provenance input = %#v", runner.input)
	}
	var modelInput map[string]json.RawMessage
	if err := json.Unmarshal(runner.input.Input, &modelInput); err != nil {
		t.Fatalf("decode model input: %v", err)
	}
	if string(modelInput["objective"]) != `"Track launch disruption"` || modelInput["raw_body"] != nil || modelInput["model_version"] != nil {
		t.Fatalf("bounded immutable model input = %s", runner.input.Input)
	}

	repeated, err := processor.GenerateExpansion(context.Background(), task)
	if err != nil || repeated[0].ID != candidates[0].ID {
		t.Fatalf("same run deterministic IDs = %#v / %v", repeated, err)
	}
	otherTask := intentExpansionTaskFixture(42)
	preparer.result.Expansion.Task = otherTask
	other, err := processor.GenerateExpansion(context.Background(), otherTask)
	if err != nil || other[0].ID == candidates[0].ID {
		t.Fatalf("other exact run candidate IDs = %#v / %v", other, err)
	}
}

func TestIntentExpansionProcessorFailsClosedForUnavailableOrUnsafeModelOutput(t *testing.T) {
	t.Parallel()
	task := intentExpansionTaskFixture(51)
	preparer := &intentExpansionPreparerFake{result: monitorapplication.PrepareIntentExpansionResult{
		Expansion: monitorapplication.PreparedIntentExpansionDTO{Task: task, Draft: intentExpansionDraftFixture()},
	}}
	tests := []struct {
		name   string
		result intelligenceapplication.StructuredExecutionResult
		err    error
	}{
		{name: "model unavailable", result: intelligenceapplication.StructuredExecutionResult{Status: "degraded", ReasonCode: "ai_model_unavailable"}},
		{name: "provider failure", err: errors.New("provider response contains sensitive detail")},
		{name: "missing model language", result: successfulIntentExpansionResult(`{"terms":[{"term":"outage","reason":"semantic wording","similarity":0.8,"risk":"low"}]}`)},
		{name: "probability shaped reason", result: successfulIntentExpansionResult(`{"terms":[{"term":"outage","language":"en","reason":"90% probability this is true","similarity":0.9,"risk":"low"}]}`)},
		{name: "unknown output field", result: successfulIntentExpansionResult(`{"terms":[{"term":"outage","language":"en","reason":"semantic wording","similarity":0.8,"risk":"low","fact":true}]}`)},
		{name: "only must not conflicts", result: successfulIntentExpansionResult(`{"terms":[{"term":"test environment outage","language":"en","reason":"excluded wording","similarity":0.8,"risk":"high"}]}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &intentExpansionStructuredRunnerFake{result: test.result, err: test.err}
			processor, constructorErr := newIntentAnalysisCompositeProcessor(preparer, runner)
			if constructorErr != nil {
				t.Fatal(constructorErr)
			}
			if candidates, err := processor.GenerateExpansion(context.Background(), task); err == nil || len(candidates) != 0 {
				t.Fatalf("GenerateExpansion() = %#v / %v, want fail-closed empty result", candidates, err)
			}
		})
	}
}

func TestIntentAnalysisCompositeProcessorAdvertisesOnlyImplementedExpansion(t *testing.T) {
	t.Parallel()
	processor, err := newIntentAnalysisCompositeProcessor(&intentExpansionPreparerFake{}, &intentExpansionStructuredRunnerFake{})
	if err != nil {
		t.Fatal(err)
	}
	if !processor.Available("expansion") || processor.Available("preview") || processor.Available("unknown") {
		t.Fatalf("availability expansion/preview/unknown = %t/%t/%t", processor.Available("expansion"), processor.Available("preview"), processor.Available("unknown"))
	}
	if _, err := processor.EvaluatePreview(context.Background(), intentExpansionTaskFixture(61)); !errors.Is(err, ErrIntentPreviewProcessorUnavailable) {
		t.Fatalf("EvaluatePreview() error = %v", err)
	}
}

func successfulIntentExpansionResult(output string) intelligenceapplication.StructuredExecutionResult {
	return intelligenceapplication.StructuredExecutionResult{
		Status: "succeeded",
		Run:    intelligencedomain.Run{ID: 902, TaskType: intelligencedomain.TaskTypeTermExpansion, ModelVersion: "actual-model-v1"},
		Result: json.RawMessage(output),
	}
}

func intentExpansionTaskFixture(runID int64) monitorapplication.IntentAnalysisTaskDTO {
	return monitorapplication.IntentAnalysisTaskDTO{
		Run: monitorapplication.IntentRunReferenceDTO{
			RunID: runID, Kind: "expansion", MonitorID: 7, DraftID: 11, DraftResourceVersion: 3,
			InputHash: strings.Repeat("a", 64),
		},
		AnalysisProfile: "monitor-intent-expansion-v1",
	}
}

func intentExpansionDraftFixture() monitorapplication.IntentDraftDTO {
	return monitorapplication.IntentDraftDTO{
		MonitorID: 7, DraftID: 11, ResourceVersion: 3, Objective: "Track launch disruption",
		Clauses: []monitorapplication.IntentClauseDTO{
			{Operator: "must", Field: "action", Value: "launch"},
			{Operator: "must_not", Field: "location", Value: "test environment"},
		},
		Entities: []monitorapplication.IntentEntityDTO{{CanonicalID: "product:hotkey", DisplayName: "HotKey", Aliases: []string{"Hot Key"}}},
		Examples: []monitorapplication.IntentExampleDTO{
			{Label: "positive", Text: "The product launch is unavailable"},
			{Label: "negative", Text: "A keyboard shortcut tutorial"},
		},
	}
}

type intentExpansionPreparerFake struct {
	query  monitorapplication.PrepareIntentExpansionQuery
	result monitorapplication.PrepareIntentExpansionResult
	err    error
}

func (fake *intentExpansionPreparerFake) PrepareIntentExpansion(_ context.Context, query monitorapplication.PrepareIntentExpansionQuery) (monitorapplication.PrepareIntentExpansionResult, error) {
	fake.query = query
	return fake.result, fake.err
}

type intentExpansionStructuredRunnerFake struct {
	input  intelligenceapplication.StructuredExecutionInput
	result intelligenceapplication.StructuredExecutionResult
	err    error
}

func (fake *intentExpansionStructuredRunnerFake) ExecuteStructured(_ context.Context, input intelligenceapplication.StructuredExecutionInput) (intelligenceapplication.StructuredExecutionResult, error) {
	fake.input = input
	return fake.result, fake.err
}
