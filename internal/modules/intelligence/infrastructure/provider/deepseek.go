package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/tmc/langchaingo/llms"
	langchainopenai "github.com/tmc/langchaingo/llms/openai"
)

const deepSeekProductionBaseURL = "https://api.deepseek.com"

type DeepSeekProvider struct {
	apiKey  string
	baseURL string
	client  safeStatusDoer
}

func NewDeepSeekProvider(ai config.AIConfig) (*DeepSeekProvider, error) {
	return newDeepSeekProvider(ai.DeepSeekAPIKey, deepSeekProductionBaseURL, nil)
}

func newDeepSeekProvider(apiKey, baseURL string, client *http.Client) (*DeepSeekProvider, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	return &DeepSeekProvider{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), client: safeLangChainDoer(client)}, nil
}

func (provider *DeepSeekProvider) Embed(context.Context, intelligencedomain.EmbeddingRequest) (intelligencedomain.EmbeddingResponse, error) {
	return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
}

func (provider *DeepSeekProvider) GenerateStructured(ctx context.Context, request intelligencedomain.StructuredRequest) (intelligencedomain.StructuredResponse, error) {
	if provider == nil {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	if err := request.Validate(); err != nil || request.TaskType == intelligencedomain.TaskTypeEmbedding {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	input, err := structuredInput(request)
	if err != nil {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	model, err := langchainopenai.New(
		langchainopenai.WithToken(provider.apiKey),
		langchainopenai.WithBaseURL(provider.baseURL),
		langchainopenai.WithModel(request.ModelName),
		langchainopenai.WithHTTPClient(provider.client),
	)
	if err != nil {
		return intelligencedomain.StructuredResponse{}, mapLangChainError(err)
	}
	response, err := model.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, request.Instruction),
		llms.TextParts(llms.ChatMessageTypeHuman, string(input)),
	}, llms.WithJSONMode())
	if err != nil {
		if ctx.Err() != nil {
			return intelligencedomain.StructuredResponse{}, mapLangChainError(ctx.Err())
		}
		return intelligencedomain.StructuredResponse{}, mapLangChainError(err)
	}
	return structuredLangChainResponse(response, request.ModelVersion)
}

func structuredLangChainResponse(response *llms.ContentResponse, modelVersion string) (intelligencedomain.StructuredResponse, error) {
	if response == nil || len(response.Choices) != 1 || response.Choices[0] == nil {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	output := json.RawMessage(response.Choices[0].Content)
	if !json.Valid(output) {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	usage, err := langChainUsage(response.Choices[0].GenerationInfo)
	if err != nil {
		return intelligencedomain.StructuredResponse{}, err
	}
	return intelligencedomain.StructuredResponse{ModelVersion: modelVersion, JSON: output, Usage: usage}, nil
}

func langChainUsage(info map[string]any) (intelligencedomain.Usage, error) {
	input, inputOK := integerGenerationValue(info["PromptTokens"])
	output, outputOK := integerGenerationValue(info["CompletionTokens"])
	total, totalOK := integerGenerationValue(info["TotalTokens"])
	usage := intelligencedomain.Usage{InputTokens: input, OutputTokens: output}
	calculated, err := usage.TotalTokens()
	if !inputOK || !outputOK || !totalOK || err != nil || calculated != total {
		return intelligencedomain.Usage{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	return usage, nil
}

func integerGenerationValue(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), typed >= 0
	case int64:
		return typed, typed >= 0
	case float64:
		converted := int64(typed)
		return converted, typed >= 0 && float64(converted) == typed
	default:
		return 0, false
	}
}

var _ intelligencedomain.Provider = (*DeepSeekProvider)(nil)
