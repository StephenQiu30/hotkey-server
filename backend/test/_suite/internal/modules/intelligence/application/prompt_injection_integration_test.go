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
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
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
	beforeFacts := readPromptInjectionBusinessFacts(t, runtime)
	forgedEvidence := json.RawMessage(fmt.Sprintf(
		`{"title_zh":"事件","sentences":[{"text":"注入事实","evidence":[{"content_id":%d,"locator":"admin:override"}]}]}`,
		fixture.ForgedEvidenceID,
	))
	formatOverride := json.RawMessage(`{"title_zh":"事件","sentences":[],"system_contract":"markdown"}`)
	provider := &promptInjectionRecordingProvider{
		modelVersion: profile.ModelVersion,
		outputs:      []json.RawMessage{forgedEvidence, formatOverride, forgedEvidence, formatOverride},
	}
	clock := &applicationClock{value: time.Date(2026, time.August, 27, 17, 0, 0, 0, time.UTC)}
	service := NewEventIntelligenceService(newApplicationRunService(t, runs, provider, clock))
	input := EventIntelligenceInput{
		TaskType: domain.TaskTypeEventSummary, EventID: 7004, EventVersion: 1, EventKey: "evt-prompt-injection",
		Evidence: []EventIntelligenceEvidence{{ContentID: 2, Locator: "title", Excerpt: fixture.SourceText}},
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := service.Execute(context.Background(), input)
		if err != nil || result.Status != AnalysisStatusPending || result.ReasonCode != AnalysisReasonOutputInvalid || len(result.Result) != 0 {
			t.Fatalf("prompt-injection run %d result=%#v error=%v, want pending output_invalid without a result", attempt+1, result, err)
		}
	}

	requests := provider.recordedRequests()
	if len(requests) != 4 {
		t.Fatalf("provider requests=%d, want two runs with one repair each", len(requests))
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
	for _, pair := range [][2]int{{0, 1}, {2, 3}} {
		if requests[pair[0]].Repair != nil || requests[pair[1]].Repair == nil || string(requests[pair[1]].Repair.PreviousOutput) != string(forgedEvidence) {
			t.Fatalf("repair contract=%#v, want rejected output only on the bounded repair", requests[pair[1]].Repair)
		}
	}

	var runCount int
	var succeededRuns int
	var allFailed, allBoundedRepairs, allResultsEmpty, allErrorsSanitized bool
	var operationalAudit string
	if err := runtime.SQL.QueryRow(`
SELECT count(*), count(*) FILTER (WHERE status='succeeded'),
       bool_and(status='failed'), bool_and(attempt=2 AND repair_attempted),
       bool_and(structured_result='{}'::jsonb), bool_and(error_code=$1),
       string_agg(row_to_json(ai_runs)::text, '')
FROM ai_runs WHERE task_type='event_summary' AND target_id=7004`, domain.CodeAIOutputInvalid).
		Scan(&runCount, &succeededRuns, &allFailed, &allBoundedRepairs, &allResultsEmpty, &allErrorsSanitized, &operationalAudit); err != nil {
		t.Fatal(err)
	}
	if runCount != 2 || succeededRuns != 0 || !allFailed || !allBoundedRepairs || !allResultsEmpty || !allErrorsSanitized {
		t.Fatalf("operational audits = runs:%d succeeded:%d failed:%v repairs:%v empty:%v safe:%v", runCount, succeededRuns, allFailed, allBoundedRepairs, allResultsEmpty, allErrorsSanitized)
	}
	for _, forbidden := range []string{fixture.AttemptedCommand, fixture.FormatOverride, "attacker-output-v9", "admin:override", "伪造事实"} {
		if strings.Contains(operationalAudit, forbidden) {
			t.Fatalf("sanitized AI run audit leaked prompt-injection input %q", forbidden)
		}
	}
	if afterFacts := readPromptInjectionBusinessFacts(t, runtime); afterFacts != beforeFacts {
		t.Fatalf("prompt injection changed business facts: before=%#v after=%#v", beforeFacts, afterFacts)
	}
}

type promptInjectionBusinessFacts struct {
	Claims, ClaimEvidence, Summaries, SummarySentences, ReportItems, ReportSentences int64
}

func readPromptInjectionBusinessFacts(t *testing.T, runtime *database.Runtime) promptInjectionBusinessFacts {
	t.Helper()
	var facts promptInjectionBusinessFacts
	if err := runtime.SQL.QueryRow(`
SELECT
  (SELECT count(*) FROM claims),
  (SELECT count(*) FROM claim_evidence_versions),
  (SELECT count(*) FROM micro_event_summaries),
  (SELECT count(*) FROM micro_event_summary_sentences),
  (SELECT count(*) FROM report_items),
  (SELECT count(*) FROM report_item_sentences)`).Scan(
		&facts.Claims, &facts.ClaimEvidence, &facts.Summaries, &facts.SummarySentences, &facts.ReportItems, &facts.ReportSentences,
	); err != nil {
		t.Fatalf("read prompt-injection business facts: %v", err)
	}
	return facts
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
