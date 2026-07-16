package domain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderRequestsKeepOnlyProviderNeutralContract(t *testing.T) {
	embedding := EmbeddingRequest{
		ModelName:    "text-embedding-3-large",
		ModelVersion: "2026-07",
		Dimensions:   EmbeddingDimensions,
		Inputs:       []string{"hotkey"},
	}
	if err := embedding.Validate(); err != nil {
		t.Fatalf("EmbeddingRequest.Validate() error = %v", err)
	}

	structured := StructuredRequest{
		ModelName:     "gpt-5.6sol",
		ModelVersion:  "2026-07",
		TaskType:      TaskTypeTermExpansion,
		SchemaName:    "term-expansion-output-v1",
		SchemaVersion: "v1",
		Instruction:   "Return only JSON.",
		Schema:        json.RawMessage(`{"type":"object"}`),
		Input:         json.RawMessage(`{"intent":"AI workflow","terms":[],"language":"en"}`),
	}
	if err := structured.Validate(); err != nil {
		t.Fatalf("StructuredRequest.Validate() error = %v", err)
	}

	var _ Provider = providerFake{}
	if _, err := (providerFake{}).Embed(context.Background(), embedding); err != nil {
		t.Fatalf("Provider.Embed() error = %v", err)
	}
	if _, err := (providerFake{}).GenerateStructured(context.Background(), structured); err != nil {
		t.Fatalf("Provider.GenerateStructured() error = %v", err)
	}

	structured.Repair = &RepairInput{
		PreviousOutput: json.RawMessage(`{"terms":[]}`),
		Violations:     makeViolations(17),
	}
	if err := structured.Validate(); err == nil {
		t.Fatal("StructuredRequest.Validate() with 17 repair violations error = nil, want rejection")
	}
	structured.Repair.Violations = makeViolations(16)
	if err := structured.Validate(); err != nil {
		t.Fatalf("StructuredRequest.Validate() with bounded repair error = %v", err)
	}

	embedding.Dimensions = EmbeddingDimensions - 1
	if err := embedding.Validate(); err == nil {
		t.Fatal("EmbeddingRequest.Validate() with non-1024 dimension error = nil, want rejection")
	}
}

type providerFake struct{}

func (providerFake) Embed(_ context.Context, request EmbeddingRequest) (EmbeddingResponse, error) {
	return EmbeddingResponse{ModelVersion: request.ModelVersion, Vectors: [][]float32{make([]float32, EmbeddingDimensions)}}, nil
}

func (providerFake) GenerateStructured(_ context.Context, request StructuredRequest) (StructuredResponse, error) {
	return StructuredResponse{ModelVersion: request.ModelVersion, JSON: json.RawMessage(`{"terms":[]}`)}, nil
}

func makeViolations(count int) []SchemaViolation {
	violations := make([]SchemaViolation, count)
	for index := range violations {
		violations[index] = SchemaViolation{InstancePath: "/terms/" + strings.TrimSpace(string(rune('0'+index%10))), Keyword: "maxLength"}
	}
	return violations
}
