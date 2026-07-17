// Package application coordinates intelligence domain values without SDK,
// database, or HTTP transport types.
package application

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligenceschemas "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type SchemaRegistry struct {
	contracts map[schemaKey]schemaContract
}

type schemaKey struct {
	taskType domain.TaskType
	version  string
}

type schemaContract struct {
	schemaName   string
	input        []byte
	output       []byte
	instruction  string
	inputSchema  *jsonschema.Schema
	outputSchema *jsonschema.Schema
}

// StructuredContract contains only compiled static contract material. It has
// no provider error, prompt, credential, endpoint, or object-store detail.
type StructuredContract struct {
	SchemaName, SchemaVersion string
	InputSchema, OutputSchema []byte
	Instruction               string
}

var (
	registryOnce sync.Once
	registry     *SchemaRegistry
	registryErr  error
)

// NewSchemaRegistry compiles each embedded schema exactly once per process.
func NewSchemaRegistry() (*SchemaRegistry, error) {
	registryOnce.Do(func() { registry, registryErr = buildSchemaRegistry() })
	return registry, registryErr
}

func buildSchemaRegistry() (*SchemaRegistry, error) {
	contracts := make(map[schemaKey]schemaContract, 5)
	for _, specification := range []struct {
		taskType        domain.TaskType
		inputName       string
		outputName      string
		schemaName      string
		instructionName string
	}{
		{domain.TaskTypeEmbedding, "embedding-input.schema.json", "embedding-output.schema.json", "embedding-output-v1", ""},
		{domain.TaskTypeTermExpansion, "term-expansion-input.schema.json", "term-expansion-output.schema.json", "term-expansion-output-v1", "term-expansion-instruction-v1.md"},
		{domain.TaskTypeRelevanceReview, "relevance-review-input.schema.json", "relevance-review-output.schema.json", "relevance-review-output-v1", "relevance-review-instruction-v1.md"},
		{domain.TaskTypeEventSummary, "event-summary-input.schema.json", "event-summary-output.schema.json", "event-summary-output-v1", "event-summary-instruction-v1.md"},
		{domain.TaskTypeEntityClaimExtraction, "entity-claim-input.schema.json", "entity-claim-output.schema.json", "entity-claim-output-v1", "entity-claim-instruction-v1.md"},
	} {
		input, err := readSchemaAsset(specification.inputName)
		if err != nil {
			return nil, err
		}
		output, err := readSchemaAsset(specification.outputName)
		if err != nil {
			return nil, err
		}
		inputSchema, err := compileSchema(specification.inputName, input)
		if err != nil {
			return nil, err
		}
		outputSchema, err := compileSchema(specification.outputName, output)
		if err != nil {
			return nil, err
		}
		instruction := ""
		if specification.instructionName != "" {
			asset, err := readSchemaAsset(specification.instructionName)
			if err != nil {
				return nil, err
			}
			instruction = strings.TrimSpace(string(asset))
			if instruction == "" {
				return nil, fmt.Errorf("embedded %s instruction is empty", specification.taskType)
			}
		}
		contracts[schemaKey{taskType: specification.taskType, version: "v1"}] = schemaContract{
			schemaName: specification.schemaName, input: input, output: output, instruction: instruction,
			inputSchema: inputSchema, outputSchema: outputSchema,
		}
	}
	return &SchemaRegistry{contracts: contracts}, nil
}

func readSchemaAsset(name string) ([]byte, error) {
	asset, err := fs.ReadFile(intelligenceschemas.Files, "v1/"+name)
	if err != nil {
		return nil, fmt.Errorf("read embedded AI schema %s: %w", name, err)
	}
	return asset, nil
}

func compileSchema(name string, source []byte) (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("parse embedded schema %s: %w", name, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(name, document); err != nil {
		return nil, fmt.Errorf("register embedded schema %s: %w", name, err)
	}
	schema, err := compiler.Compile(name)
	if err != nil {
		return nil, fmt.Errorf("compile embedded schema %s: %w", name, err)
	}
	return schema, nil
}

func (registry *SchemaRegistry) Structured(taskType domain.TaskType, version string) (StructuredContract, error) {
	contract, err := registry.contract(taskType, version)
	if err != nil {
		return StructuredContract{}, err
	}
	if contract.instruction == "" {
		return StructuredContract{}, fmt.Errorf("%s does not use a structured generation instruction", taskType)
	}
	return StructuredContract{
		SchemaName: contract.schemaName, SchemaVersion: version,
		InputSchema: cloneJSON(contract.input), OutputSchema: cloneJSON(contract.output), Instruction: contract.instruction,
	}, nil
}

func (registry *SchemaRegistry) ValidateInput(taskType domain.TaskType, version string, payload []byte) error {
	contract, err := registry.contract(taskType, version)
	if err != nil {
		return err
	}
	_, err = validateJSON(contract.inputSchema, payload)
	return err
}

func (registry *SchemaRegistry) ValidateOutput(taskType domain.TaskType, version string, payload []byte) error {
	contract, err := registry.contract(taskType, version)
	if err != nil {
		return err
	}
	_, err = validateJSON(contract.outputSchema, payload)
	return err
}

// RepairForInvalidOutput returns only the first invalid JSON output and a
// bounded path/keyword list. A second invalid response is terminal and never
// carries parser, provider, prompt, or schema-engine text onward.
func (registry *SchemaRegistry) RepairForInvalidOutput(taskType domain.TaskType, version string, payload []byte, repairAttempted bool) (*domain.RepairInput, error) {
	contract, err := registry.contract(taskType, version)
	if err != nil {
		return nil, err
	}
	violations, err := validateJSON(contract.outputSchema, payload)
	if err == nil {
		return nil, nil
	}
	if repairAttempted {
		return nil, domain.NewError(domain.CodeAIOutputInvalid)
	}
	if len(violations) == 0 {
		violations = []domain.SchemaViolation{{InstancePath: "/", Keyword: "json"}}
	}
	return &domain.RepairInput{PreviousOutput: cloneJSON(payload), Violations: violations}, nil
}

func (registry *SchemaRegistry) contract(taskType domain.TaskType, version string) (schemaContract, error) {
	if registry == nil {
		return schemaContract{}, fmt.Errorf("AI schema registry is nil")
	}
	contract, ok := registry.contracts[schemaKey{taskType: taskType, version: strings.TrimSpace(version)}]
	if !ok {
		return schemaContract{}, fmt.Errorf("AI schema contract is not registered")
	}
	return contract, nil
}

func validateJSON(schema *jsonschema.Schema, payload []byte) ([]domain.SchemaViolation, error) {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("AI schema payload is invalid")
	}
	if err := schema.Validate(instance); err != nil {
		return schemaViolations(err), fmt.Errorf("AI schema payload is invalid")
	}
	return nil, nil
}

func schemaViolations(err error) []domain.SchemaViolation {
	var validationError *jsonschema.ValidationError
	if !errors.As(err, &validationError) || validationError == nil {
		return nil
	}
	violations := make([]domain.SchemaViolation, 0, 16)
	collectSchemaViolations(validationError, &violations)
	return violations
}

func collectSchemaViolations(validationError *jsonschema.ValidationError, violations *[]domain.SchemaViolation) {
	if validationError == nil || len(*violations) == 16 {
		return
	}
	if len(validationError.Causes) == 0 {
		path := "/" + strings.Join(validationError.InstanceLocation, "/")
		if path == "/" && len(validationError.InstanceLocation) == 0 {
			path = "/"
		}
		keywordPath := validationError.ErrorKind.KeywordPath()
		keyword := "schema"
		if len(keywordPath) > 0 {
			keyword = keywordPath[len(keywordPath)-1]
		}
		*violations = append(*violations, domain.SchemaViolation{InstancePath: path, Keyword: keyword})
		return
	}
	for _, cause := range validationError.Causes {
		collectSchemaViolations(cause, violations)
		if len(*violations) == 16 {
			return
		}
	}
}

func cloneJSON(source []byte) []byte { return append([]byte(nil), source...) }
