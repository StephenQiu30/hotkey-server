package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

func TestSchemaRegistryEmbedsAndCompilesVersionedContracts(t *testing.T) {
	registry, err := NewSchemaRegistry()
	if err != nil {
		t.Fatalf("NewSchemaRegistry() error = %v", err)
	}
	contract, err := registry.Structured(domain.TaskTypeTermExpansion, "v1")
	if err != nil {
		t.Fatalf("Structured() error = %v", err)
	}
	if contract.SchemaName != "term-expansion-output-v1" || len(contract.OutputSchema) == 0 || strings.TrimSpace(contract.Instruction) == "" {
		t.Fatalf("Structured() = %#v, want embedded term-expansion schema and instruction", contract)
	}
	if err := registry.ValidateInput(domain.TaskTypeTermExpansion, "v1", []byte(`{"intent":"AI workflow","terms":["automation"],"language":"en"}`)); err != nil {
		t.Fatalf("ValidateInput() error = %v", err)
	}
	if err := registry.ValidateOutput(domain.TaskTypeTermExpansion, "v1", []byte(`{"terms":[{"term":"workflow","language":"en"}]}`)); err != nil {
		t.Fatalf("ValidateOutput() error = %v", err)
	}
	if err := registry.ValidateInput(domain.TaskTypeEmbedding, "v1", []byte(`{"inputs":["HotKey"],"language":"und"}`)); err != nil {
		t.Fatalf("ValidateInput() embedding error = %v", err)
	}
	vector := strings.Repeat("0,", domain.EmbeddingDimensions-1) + "0"
	if err := registry.ValidateOutput(domain.TaskTypeEmbedding, "v1", []byte(`{"model_version":"2026-07","vectors":[[`+vector+`]]}`)); err != nil {
		t.Fatalf("ValidateOutput() embedding error = %v", err)
	}
}

func TestRelevanceReviewSchemaRegistryContract(t *testing.T) {
	registry, err := NewSchemaRegistry()
	if err != nil {
		t.Fatalf("NewSchemaRegistry() error = %v", err)
	}
	contract, err := registry.Structured(domain.TaskTypeRelevanceReview, "v1")
	if err != nil {
		t.Fatalf("Structured(relevance_review) error = %v", err)
	}
	if contract.SchemaName != "relevance-review-output-v1" || len(contract.OutputSchema) == 0 || strings.TrimSpace(contract.Instruction) == "" {
		t.Fatalf("Structured(relevance_review) = %#v, want embedded contract", contract)
	}
	input := relevanceFixture(t, "input.json")
	if err := registry.ValidateInput(domain.TaskTypeRelevanceReview, "v1", input); err != nil {
		t.Fatalf("ValidateInput(relevance_review) error = %v", err)
	}
	if err := registry.ValidateOutput(domain.TaskTypeRelevanceReview, "v1", relevanceFixture(t, "output.json")); err != nil {
		t.Fatalf("ValidateOutput(relevance_review) error = %v", err)
	}
	if err := registry.ValidateOutput(domain.TaskTypeRelevanceReview, "v1", []byte(`{"decision":"review","score":70,"reason":"free text"}`)); err == nil {
		t.Fatal("ValidateOutput(relevance_review) with free text error = nil, want rejection")
	}
	if err := registry.ValidateInput(domain.TaskTypeRelevanceReview, "v1", []byte(`{"content_excerpt":"safe","content_language":"en","monitor_intent":"intent","scoring_version":"relevance-v1","scores":{"semantic":70,"lexical":80,"entity":60,"title":70,"preference":50},"recall_paths":["lexical"],"reason_codes":[],"evidence_terms":[],"provider_url":"forbidden"}`)); err == nil {
		t.Fatal("ValidateInput(relevance_review) with unknown field error = nil, want rejection")
	}
	overlong := `{"content_excerpt":"` + strings.Repeat("a", 1201) + `","content_language":"en","monitor_intent":"intent","scoring_version":"relevance-v1","scores":{"semantic":70,"lexical":80,"entity":60,"title":70,"preference":50},"recall_paths":["lexical"],"reason_codes":[],"evidence_terms":[]}`
	if err := registry.ValidateInput(domain.TaskTypeRelevanceReview, "v1", []byte(overlong)); err == nil {
		t.Fatal("ValidateInput(relevance_review) with overlong excerpt error = nil, want rejection")
	}
}

func TestEventIntelligenceSchemaRegistryContracts(t *testing.T) {
	registry, err := NewSchemaRegistry()
	if err != nil {
		t.Fatalf("NewSchemaRegistry() error = %v", err)
	}
	for _, fixture := range []struct {
		taskType domain.TaskType
		input    string
		output   string
	}{
		{
			taskType: domain.TaskTypeEventSummary,
			input:    `{"event_id":7,"event_key":"evt_7","evidence":[{"content_id":11,"locator":"title","excerpt":"verified report"}]}`,
			output:   `{"title_zh":"事件","title_en":"Event","sentences":[{"text":"事实","evidence":[{"content_id":11,"locator":"title","excerpt":"verified report"}]}]}`,
		},
		{
			taskType: domain.TaskTypeEntityClaimExtraction,
			input:    `{"event_id":7,"event_key":"evt_7","evidence":[{"content_id":11,"locator":"title","excerpt":"verified report"}]}`,
			output:   `{"entities":[{"entity_key":"acme","entity_type":"organization","canonical_name":"Acme"}],"claims":[{"claim":"Acme announced a release.","evidence":[{"content_id":11,"locator":"title","excerpt":"verified report"}]}]}`,
		},
	} {
		t.Run(string(fixture.taskType), func(t *testing.T) {
			contract, err := registry.Structured(fixture.taskType, "v1")
			if err != nil || contract.SchemaVersion != "v1" || strings.TrimSpace(contract.Instruction) == "" {
				t.Fatalf("Structured(%s) = %#v / %v, want embedded contract", fixture.taskType, contract, err)
			}
			if err := registry.ValidateInput(fixture.taskType, "v1", []byte(fixture.input)); err != nil {
				t.Fatalf("ValidateInput(%s) error = %v", fixture.taskType, err)
			}
			if err := registry.ValidateInput(fixture.taskType, "v1", []byte(fixture.input[:len(fixture.input)-1]+`,"raw_content":"forbidden"}`)); err == nil {
				t.Fatalf("ValidateInput(%s) with unknown field error = nil, want rejection", fixture.taskType)
			}
			if err := registry.ValidateOutput(fixture.taskType, "v1", []byte(fixture.output)); err != nil {
				t.Fatalf("ValidateOutput(%s) error = %v", fixture.taskType, err)
			}
			if err := registry.ValidateOutput(fixture.taskType, "v1", []byte(fixture.output[:len(fixture.output)-1]+`,"raw_response":"forbidden"}`)); err == nil {
				t.Fatalf("ValidateOutput(%s) with unknown field error = nil, want rejection", fixture.taskType)
			}
		})
	}
}

func relevanceFixture(t *testing.T, name string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("testdata", "relevance", name))
	if err != nil {
		t.Fatalf("read relevance fixture %s: %v", name, err)
	}
	return payload
}

func TestSchemaRegistryRejectsUnknownOversizedAndSecondInvalidRepair(t *testing.T) {
	registry, err := NewSchemaRegistry()
	if err != nil {
		t.Fatalf("NewSchemaRegistry() error = %v", err)
	}
	if err := registry.ValidateOutput(domain.TaskTypeTermExpansion, "v1", []byte(`{"terms":[],"raw_response":"forbidden"}`)); err == nil {
		t.Fatal("ValidateOutput() with unknown field error = nil, want rejection")
	}
	oversized := `{"terms":[{"term":"` + strings.Repeat("a", 121) + `","language":"en"}]}`
	if err := registry.ValidateOutput(domain.TaskTypeTermExpansion, "v1", []byte(oversized)); err == nil {
		t.Fatal("ValidateOutput() with overlong term error = nil, want rejection")
	}
	if err := registry.ValidateOutput(domain.TaskTypeEmbedding, "v1", []byte(`{"model_version":"2026-07","vectors":[[0]]}`)); err == nil {
		t.Fatal("ValidateOutput() with 1-dimensional embedding error = nil, want rejection")
	}

	invalid := []byte(`{"terms":[{"term":"workflow","language":"fr"}]}`)
	repair, err := registry.RepairForInvalidOutput(domain.TaskTypeTermExpansion, "v1", invalid, false)
	if err != nil {
		t.Fatalf("RepairForInvalidOutput() first error = %v", err)
	}
	if repair == nil || len(repair.Violations) == 0 || len(repair.Violations) > 16 || string(repair.PreviousOutput) != string(invalid) {
		t.Fatalf("RepairForInvalidOutput() = %#v, want bounded safe repair input", repair)
	}
	if _, err := registry.RepairForInvalidOutput(domain.TaskTypeTermExpansion, "v1", invalid, true); err == nil {
		t.Fatal("RepairForInvalidOutput() second invalid output error = nil, want rejection")
	} else if code, ok := domain.CodeOf(err); !ok || code != domain.CodeAIOutputInvalid {
		t.Fatalf("RepairForInvalidOutput() code = %d/%t, want %d", code, ok, domain.CodeAIOutputInvalid)
	}
}
