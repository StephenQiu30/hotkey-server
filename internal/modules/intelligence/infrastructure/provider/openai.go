// Package provider contains infrastructure-only AI provider adapters.
package provider

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"math"
	"net"
	"net/http"
	"strings"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const openAIProductionBaseURL = "https://api.openai.com/v1"

// OpenAIProvider is the only location where official OpenAI SDK types are
// allowed. Its public methods use the provider-neutral domain port.
type OpenAIProvider struct{ client openai.Client }

// NewOpenAIProvider builds a production client from the explicitly resolved
// application configuration. Its endpoint is deliberately not configurable:
// a profile credential must never be sent to an arbitrary host.
func NewOpenAIProvider(ai config.AIConfig) (*OpenAIProvider, error) {
	apiKey := strings.TrimSpace(ai.OpenAIAPIKey)
	if apiKey == "" {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}

	return &OpenAIProvider{client: openai.NewClient(
		option.WithBaseURL(openAIProductionBaseURL),
		option.WithAPIKey(apiKey),
		// Application owns retry and lease state. SDK retries would make a
		// single claimed run issue multiple untracked external calls.
		option.WithMaxRetries(0),
	)}, nil
}

func (provider *OpenAIProvider) Embed(ctx context.Context, request intelligencedomain.EmbeddingRequest) (intelligencedomain.EmbeddingResponse, error) {
	if provider == nil {
		return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	if err := request.Validate(); err != nil {
		return intelligencedomain.EmbeddingResponse{}, err
	}

	response, err := provider.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model:          openai.EmbeddingModel(request.ModelName),
		Dimensions:     openai.Int(int64(request.Dimensions)),
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
		Input:          openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: request.Inputs},
	})
	if err != nil {
		return intelligencedomain.EmbeddingResponse{}, mapOpenAIError(err)
	}
	if response == nil || response.Model != request.ModelName || len(response.Data) != len(request.Inputs) {
		return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}

	vectors := make([][]float32, len(request.Inputs))
	for _, embedding := range response.Data {
		if embedding.Index < 0 || embedding.Index >= int64(len(vectors)) || vectors[embedding.Index] != nil {
			return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIEmbeddingInvalid)
		}
		vector := make([]float32, len(embedding.Embedding))
		for index, value := range embedding.Embedding {
			if math.IsNaN(value) || math.IsInf(value, 0) || value > math.MaxFloat32 || value < -math.MaxFloat32 {
				return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIEmbeddingInvalid)
			}
			vector[index] = float32(value)
		}
		if err := intelligencedomain.ValidateEmbedding(vector); err != nil {
			return intelligencedomain.EmbeddingResponse{}, err
		}
		vectors[embedding.Index] = vector
	}
	for _, vector := range vectors {
		if vector == nil {
			return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIEmbeddingInvalid)
		}
	}

	return intelligencedomain.EmbeddingResponse{ModelVersion: request.ModelVersion, Vectors: vectors}, nil
}

func (provider *OpenAIProvider) GenerateStructured(ctx context.Context, request intelligencedomain.StructuredRequest) (intelligencedomain.StructuredResponse, error) {
	if provider == nil {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	if err := request.Validate(); err != nil {
		return intelligencedomain.StructuredResponse{}, err
	}

	var schema map[string]any
	if err := json.Unmarshal(request.Schema, &schema); err != nil || schema == nil {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	input, err := structuredInput(request)
	if err != nil {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	format := responses.ResponseFormatTextConfigParamOfJSONSchema(request.SchemaName, schema)
	format.OfJSONSchema.Strict = openai.Bool(true)

	response, err := provider.client.Responses.New(ctx, responses.ResponseNewParams{
		Model:        shared.ResponsesModel(request.ModelName),
		Instructions: openai.String(request.Instruction),
		Input:        responses.ResponseNewParamsInputUnion{OfString: openai.String(string(input))},
		Text:         responses.ResponseTextConfigParam{Format: format},
		Store:        openai.Bool(false),
	})
	if err != nil {
		return intelligencedomain.StructuredResponse{}, mapOpenAIError(err)
	}
	if response == nil || string(response.Model) != request.ModelName {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	output := json.RawMessage(response.OutputText())
	if !json.Valid(output) {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	return intelligencedomain.StructuredResponse{ModelVersion: request.ModelVersion, JSON: output}, nil
}

func structuredInput(request intelligencedomain.StructuredRequest) ([]byte, error) {
	if request.Repair == nil {
		return append([]byte(nil), request.Input...), nil
	}
	type repairViolation struct {
		InstancePath string `json:"instance_path"`
		Keyword      string `json:"keyword"`
	}
	violations := make([]repairViolation, len(request.Repair.Violations))
	for index, violation := range request.Repair.Violations {
		violations[index] = repairViolation{InstancePath: violation.InstancePath, Keyword: violation.Keyword}
	}
	return json.Marshal(struct {
		Input  json.RawMessage `json:"input"`
		Repair struct {
			PreviousOutput json.RawMessage   `json:"previous_output"`
			Violations     []repairViolation `json:"violations"`
		} `json:"repair"`
	}{
		Input: request.Input,
		Repair: struct {
			PreviousOutput json.RawMessage   `json:"previous_output"`
			Violations     []repairViolation `json:"violations"`
		}{
			PreviousOutput: request.Repair.PreviousOutput,
			Violations:     violations,
		},
	})
}

// mapOpenAIError deliberately drops every SDK/body string. The domain error
// carries the complete, stable outcome required by Application and transport.
func mapOpenAIError(err error) error {
	if err == nil {
		return nil
	}
	if stdErrors.Is(err, context.DeadlineExceeded) {
		return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
	}
	var networkError net.Error
	if stdErrors.As(err, &networkError) && networkError.Timeout() {
		return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
	}
	var apiError *openai.Error
	if stdErrors.As(err, &apiError) && apiError != nil {
		switch {
		case apiError.StatusCode == http.StatusRequestTimeout:
			return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
		case apiError.StatusCode == http.StatusTooManyRequests:
			return intelligencedomain.NewError(intelligencedomain.CodeAIProviderRateLimited)
		case apiError.StatusCode >= http.StatusInternalServerError:
			return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
		default:
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
	}
	return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
}

var _ intelligencedomain.Provider = (*OpenAIProvider)(nil)
