package application

import (
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
