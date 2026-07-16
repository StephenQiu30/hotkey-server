package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type TaskType string

const (
	TaskTypeEmbedding     TaskType = "embedding"
	TaskTypeTermExpansion TaskType = "term_expansion"
)

func (taskType TaskType) Valid() bool {
	return taskType == TaskTypeEmbedding || taskType == TaskTypeTermExpansion
}

type ProviderName string

const (
	ProviderOpenAI ProviderName = "openai"
	ProviderONNX   ProviderName = "onnx"

	OpenAICredentialReference = "env:OPENAI_API_KEY"
)

func (provider ProviderName) Valid() bool {
	return provider == ProviderOpenAI || provider == ProviderONNX
}

// Provider is intentionally independent of SDK and HTTP types.
type Provider interface {
	Embed(context.Context, EmbeddingRequest) (EmbeddingResponse, error)
	GenerateStructured(context.Context, StructuredRequest) (StructuredResponse, error)
}

type EmbeddingRequest struct {
	ModelName, ModelVersion string
	Dimensions              int
	Inputs                  []string
}

func (request EmbeddingRequest) Validate() error {
	if strings.TrimSpace(request.ModelName) == "" || strings.TrimSpace(request.ModelVersion) == "" || request.Dimensions != EmbeddingDimensions || len(request.Inputs) == 0 {
		return NewError(CodeAIModelProfileInvalid)
	}
	for _, input := range request.Inputs {
		if strings.TrimSpace(input) == "" {
			return NewError(CodeAIModelProfileInvalid)
		}
	}
	return nil
}

type EmbeddingResponse struct {
	ModelVersion string
	Vectors      [][]float32
}

type SchemaViolation struct {
	InstancePath string
	Keyword      string
}

func (violation SchemaViolation) Validate() error {
	if strings.TrimSpace(violation.InstancePath) == "" || strings.TrimSpace(violation.Keyword) == "" || len(violation.InstancePath) > 256 || len(violation.Keyword) > 64 {
		return fmt.Errorf("invalid schema violation")
	}
	return nil
}

type RepairInput struct {
	PreviousOutput json.RawMessage
	Violations     []SchemaViolation
}

func (repair RepairInput) Validate() error {
	if !json.Valid(repair.PreviousOutput) || len(repair.Violations) == 0 || len(repair.Violations) > 16 {
		return fmt.Errorf("invalid structured repair input")
	}
	for _, violation := range repair.Violations {
		if err := violation.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type StructuredRequest struct {
	ModelName, ModelVersion, SchemaName, SchemaVersion string
	TaskType                                           TaskType
	Instruction                                        string
	Schema, Input                                      json.RawMessage
	Repair                                             *RepairInput
}

func (request StructuredRequest) Validate() error {
	if strings.TrimSpace(request.ModelName) == "" || strings.TrimSpace(request.ModelVersion) == "" || !request.TaskType.Valid() ||
		strings.TrimSpace(request.SchemaName) == "" || strings.TrimSpace(request.SchemaVersion) == "" || strings.TrimSpace(request.Instruction) == "" ||
		!json.Valid(request.Schema) || !json.Valid(request.Input) {
		return NewError(CodeAIModelProfileInvalid)
	}
	if request.Repair != nil {
		if err := request.Repair.Validate(); err != nil {
			return NewError(CodeAIOutputInvalid)
		}
	}
	return nil
}

type StructuredResponse struct {
	ModelVersion string
	JSON         json.RawMessage
}
