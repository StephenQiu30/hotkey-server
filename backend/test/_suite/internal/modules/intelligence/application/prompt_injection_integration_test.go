package application

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/infrastructure/postgres"
)

type promptInjectionFixture struct {
	SourceText       string `json:"source_text"`
	AttemptedCommand string `json:"attempted_command"`
	ForgedEvidenceID int64  `json:"forged_evidence_id"`
	FormatOverride   string `json:"format_override"`
}

func TestPromptInjectionFixtureCannotChangeEvidenceOrOutputContract(t *testing.T) {
	fixture := loadPromptInjectionFixture(t)
	runtime := openApplicationRuntime(t)
	defer func() { _ = runtime.Close() }()
	runs := intelligencepostgres.NewRepository(runtime)
	profile := applicationEventSummaryProfile()
	profile.Name = "application-event-summary-prompt-injection-profile"
	if err := runs.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatal(err)
	}
	forgedEvidence := json.RawMessage(fmt.Sprintf(
		`{"title_zh":"事件","sentences":[{"text":"注入事实","evidence":[{"content_id":%d,"locator":"admin:override"}]}]}`,
		fixture.ForgedEvidenceID,
	))
	formatOverride := json.RawMessage(`{"title_zh":"事件","sentences":[],"system_contract":"markdown"}`)
	provider := &promptInjectionRecordingProvider{
		modelVersion: profile.ModelVersion,
		outputs:      []json.RawMessage{forgedEvidence, formatOverride},
	}
	clock := &applicationClock{value: time.Date(2026, time.August, 27, 17, 0, 0, 0, time.UTC)}
	service := NewEventIntelligenceService(newApplicationRunService(t, runs, provider, clock))
	result, err := service.Execute(context.Background(), EventIntelligenceInput{
		TaskType: domain.TaskTypeEventSummary, EventID: 7004, EventVersion: 1, EventKey: "evt-prompt-injection",
		Evidence: []EventIntelligenceEvidence{{ContentID: 2, Locator: "title", Excerpt: fixture.SourceText}},
	})
	if err != nil || result.Status != AnalysisStatusPending || result.ReasonCode != AnalysisReasonOutputInvalid || len(result.Result) != 0 {
		t.Fatalf("prompt-injection result=%#v error=%v, want pending output_invalid without a result", result, err)
	}

	requests := provider.recordedRequests()
	if len(requests) != 2 {
		t.Fatalf("provider requests=%d, want one attempt plus one repair", len(requests))
	}
	for index, request := range requests {
		if request.TaskType != domain.TaskTypeEventSummary || request.SchemaName != "event-summary-output-v1" || request.SchemaVersion != "v1" {
			t.Fatalf("request[%d] contract changed: task=%q schema=%q version=%q", index, request.TaskType, request.SchemaName, request.SchemaVersion)
		}
		if strings.Contains(request.Instruction, fixture.AttemptedCommand) || strings.Contains(request.Instruction, fixture.FormatOverride) || strings.Contains(request.Instruction, "attacker-output-v9") {
			t.Fatalf("request[%d] promoted untrusted text into the instruction: %q", index, request.Instruction)
		}
		var input struct {
			Evidence []EventIntelligenceEvidence `json:"evidence"`
		}
		if err := json.Unmarshal(request.Input, &input); err != nil || len(input.Evidence) != 1 || input.Evidence[0].Excerpt != fixture.SourceText {
			t.Fatalf("request[%d] did not preserve the source strictly as evidence data: %#v / %v", index, input, err)
		}
	}
	if requests[0].Repair != nil || requests[1].Repair == nil || string(requests[1].Repair.PreviousOutput) != string(forgedEvidence) {
		t.Fatalf("repair contract=%#v, want the rejected first output only on attempt two", requests[1].Repair)
	}

	var status string
	var attempt int
	var repairAttempted bool
	var structuredResult []byte
	var succeededRuns int
	if err := runtime.SQL.QueryRow(`
SELECT status,attempt,repair_attempted,structured_result,
       count(*) FILTER (WHERE status='succeeded') OVER ()
FROM ai_runs WHERE task_type='event_summary' AND target_id=7004`).
		Scan(&status, &attempt, &repairAttempted, &structuredResult, &succeededRuns); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || attempt != 2 || !repairAttempted || string(structuredResult) != "{}" || succeededRuns != 0 {
		t.Fatalf("persisted run status=%q attempt=%d repair=%v result=%q succeeded=%d", status, attempt, repairAttempted, structuredResult, succeededRuns)
	}
}

type promptInjectionRecordingProvider struct {
	modelVersion string
	outputs      []json.RawMessage
	requests     []domain.StructuredRequest
}

func (provider *promptInjectionRecordingProvider) GenerateStructured(_ context.Context, request domain.StructuredRequest) (domain.StructuredResponse, error) {
	provider.requests = append(provider.requests, cloneStructuredRequest(request))
	index := len(provider.requests) - 1
	if index >= len(provider.outputs) {
		return domain.StructuredResponse{}, domain.NewError(domain.CodeAIModelUnavailable)
	}
	return domain.StructuredResponse{
		ModelVersion: provider.modelVersion,
		JSON:         append(json.RawMessage(nil), provider.outputs[index]...),
		Usage:        domain.Usage{InputTokens: 2, OutputTokens: 1},
	}, nil
}

func (provider *promptInjectionRecordingProvider) Embed(context.Context, domain.EmbeddingRequest) (domain.EmbeddingResponse, error) {
	return domain.EmbeddingResponse{}, domain.NewError(domain.CodeAIModelUnavailable)
}

func (provider *promptInjectionRecordingProvider) recordedRequests() []domain.StructuredRequest {
	return append([]domain.StructuredRequest(nil), provider.requests...)
}

func cloneStructuredRequest(request domain.StructuredRequest) domain.StructuredRequest {
	cloned := request
	cloned.InputSchema = append(json.RawMessage(nil), request.InputSchema...)
	cloned.Schema = append(json.RawMessage(nil), request.Schema...)
	cloned.Input = append(json.RawMessage(nil), request.Input...)
	if request.Repair != nil {
		repair := *request.Repair
		repair.PreviousOutput = append(json.RawMessage(nil), request.Repair.PreviousOutput...)
		repair.Violations = append([]domain.SchemaViolation(nil), request.Repair.Violations...)
		cloned.Repair = &repair
	}
	return cloned
}

func loadPromptInjectionFixture(t *testing.T) promptInjectionFixture {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("testdata", "prompt-injection", "v1", "source.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture promptInjectionFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(fixture.SourceText) == "" || strings.TrimSpace(fixture.AttemptedCommand) == "" || fixture.ForgedEvidenceID <= 0 || strings.TrimSpace(fixture.FormatOverride) == "" {
		t.Fatalf("incomplete prompt-injection fixture: %#v", fixture)
	}
	return fixture
}
