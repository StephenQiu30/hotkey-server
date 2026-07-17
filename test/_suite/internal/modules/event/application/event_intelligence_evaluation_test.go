package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

type eventIntelligenceEvaluationFixture struct {
	Version         string                            `json:"version"`
	Source          eventIntelligenceEvaluationSource `json:"source"`
	SummaryCases    []eventIntelligenceEvaluationCase `json:"summary_cases"`
	ExtractionCases []eventIntelligenceEvaluationCase `json:"extraction_cases"`
}

type eventIntelligenceEvaluationSource struct {
	EventID                 int64                                 `json:"event_id"`
	EventKey                string                                `json:"event_key"`
	TitleZH                 string                                `json:"title_zh"`
	TitleEN                 string                                `json:"title_en"`
	RepresentativeContentID int64                                 `json:"representative_content_id"`
	Evidence                []eventIntelligenceEvaluationEvidence `json:"evidence"`
}

type eventIntelligenceEvaluationEvidence struct {
	ContentID int64  `json:"content_id"`
	Locator   string `json:"locator"`
	Excerpt   string `json:"excerpt"`
}

type eventIntelligenceEvaluationCase struct {
	Name       string                                `json:"name"`
	Status     string                                `json:"status"`
	ReasonCode string                                `json:"reason_code"`
	RunID      int64                                 `json:"run_id"`
	Want       string                                `json:"want"`
	Evidence   []eventIntelligenceEvaluationEvidence `json:"evidence"`
	Result     json.RawMessage                       `json:"result"`
}

func TestEventIntelligenceEvaluationV1(t *testing.T) {
	fixture := loadEventIntelligenceEvaluationFixture(t)
	if fixture.Version != "event-intelligence-evaluation-v1" {
		t.Fatalf("fixture version = %q", fixture.Version)
	}
	assertSummaryEvaluation(t, fixture)
	assertExtractionEvaluation(t, fixture)
}

func assertSummaryEvaluation(t *testing.T, fixture eventIntelligenceEvaluationFixture) {
	t.Helper()
	verified, rejected, degraded := 0, 0, 0
	for _, testCase := range fixture.SummaryCases {
		t.Run("summary/"+testCase.Name, func(t *testing.T) {
			source := fixture.eventSource(testCase.Evidence)
			runner := &eventIntelligenceRunnerStub{result: evaluationRunResult(testCase)}
			result, err := NewEventSummaryService(&eventIntelligenceReaderStub{source: source}, runner).Generate(context.Background(), source.Event.ID)
			switch testCase.Want {
			case "verified":
				verified++
				if err != nil || result.Summary.Degraded || result.RunID != testCase.RunID || len(result.Summary.Sentences) == 0 || result.Summary.Sentences[0].Evidence[0].Excerpt != source.Evidence[0].Excerpt {
					t.Fatalf("Generate() = %#v / %v", result, err)
				}
			case "rejected":
				rejected++
				if err == nil {
					t.Fatal("Generate() error = nil, want rejected fixture")
				}
			case "degraded":
				degraded++
				if err != nil || !result.Summary.Degraded || result.ReasonCode != testCase.ReasonCode || result.RunID != 0 || result.Summary.Sentences[0].Evidence[0].ContentID != *source.Event.RepresentativeContentID {
					t.Fatalf("Generate() = %#v / %v", result, err)
				}
			default:
				t.Fatalf("unsupported expected summary outcome %q", testCase.Want)
			}
		})
	}
	if verified != 1 || rejected != 3 || degraded != 1 {
		t.Fatalf("summary evaluation counts = verified:%d rejected:%d degraded:%d", verified, rejected, degraded)
	}
}

func assertExtractionEvaluation(t *testing.T, fixture eventIntelligenceEvaluationFixture) {
	t.Helper()
	verified, rejected, degraded := 0, 0, 0
	for _, testCase := range fixture.ExtractionCases {
		t.Run("extraction/"+testCase.Name, func(t *testing.T) {
			source := fixture.eventSource(testCase.Evidence)
			store := &eventIntelligenceFactStoreStub{}
			runner := &eventIntelligenceRunnerStub{result: evaluationRunResult(testCase)}
			result, err := NewEventClaimExtractionService(&eventIntelligenceReaderStub{source: source}, runner, store).Extract(context.Background(), source.Event.ID)
			switch testCase.Want {
			case "verified":
				verified++
				if err != nil || result.Status != "succeeded" || result.RunID != testCase.RunID || store.calls != 1 || len(store.facts.Claims) != 1 || store.facts.Claims[0].Evidence[0].Excerpt != source.Evidence[0].Excerpt {
					t.Fatalf("Extract() = %#v / %v facts=%#v calls=%d", result, err, store.facts, store.calls)
				}
			case "rejected":
				rejected++
				if err == nil || store.calls != 0 {
					t.Fatalf("Extract() error/calls = %v/%d, want rejected before persistence", err, store.calls)
				}
			case "degraded":
				degraded++
				if err != nil || result.Status != "degraded" || result.ReasonCode != testCase.ReasonCode || store.calls != 0 {
					t.Fatalf("Extract() = %#v / %v calls=%d", result, err, store.calls)
				}
			default:
				t.Fatalf("unsupported expected extraction outcome %q", testCase.Want)
			}
		})
	}
	if verified != 1 || rejected != 2 || degraded != 1 {
		t.Fatalf("extraction evaluation counts = verified:%d rejected:%d degraded:%d", verified, rejected, degraded)
	}
}

func (fixture eventIntelligenceEvaluationFixture) eventSource(override []eventIntelligenceEvaluationEvidence) EventIntelligenceSource {
	evidence := fixture.Source.Evidence
	if len(override) > 0 {
		evidence = override
	}
	values := make([]domain.EvidenceRef, 0, len(evidence))
	for _, item := range evidence {
		values = append(values, domain.EvidenceRef{ContentID: item.ContentID, Locator: item.Locator, Excerpt: item.Excerpt})
	}
	representative := fixture.Source.RepresentativeContentID
	return EventIntelligenceSource{Event: domain.Event{ID: fixture.Source.EventID, EventKey: fixture.Source.EventKey, TitleZH: fixture.Source.TitleZH, TitleEN: fixture.Source.TitleEN, RepresentativeContentID: &representative}, Evidence: values}
}

func evaluationRunResult(testCase eventIntelligenceEvaluationCase) intelligenceapplication.EventIntelligenceResult {
	result := intelligenceapplication.EventIntelligenceResult{Status: testCase.Status, ReasonCode: testCase.ReasonCode}
	if testCase.Status == "succeeded" {
		result.Run = intelligencedomain.Run{ID: testCase.RunID}
		result.Result = append(json.RawMessage(nil), testCase.Result...)
	}
	return result
}

func loadEventIntelligenceEvaluationFixture(t *testing.T) eventIntelligenceEvaluationFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve evaluation test file")
	}
	for directory := filepath.Dir(file); ; directory = filepath.Dir(directory) {
		candidate := filepath.Join(directory, "test", "fixtures", "event-intelligence", "v1", "acceptance.json")
		payload, err := os.ReadFile(candidate)
		if err == nil {
			var fixture eventIntelligenceEvaluationFixture
			if err := json.Unmarshal(payload, &fixture); err != nil {
				t.Fatalf("decode %s: %v", candidate, err)
			}
			return fixture
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatalf("locate evaluation fixture from %s", file)
		}
	}
}
