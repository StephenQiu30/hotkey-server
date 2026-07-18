package provider

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"strings"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/tmc/langchaingo/llms"
	langchainollama "github.com/tmc/langchaingo/llms/ollama"
)

type OllamaProvider struct {
	baseURL *url.URL
	client  *http.Client
}

func NewOllamaProvider(ai config.AIConfig) (*OllamaProvider, error) {
	if !ai.OllamaEnabled {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	return newOllamaProvider(ai.OllamaBaseURL, nil)
}

func newOllamaProvider(rawURL string, client *http.Client) (*OllamaProvider, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	parsed.Path = ""
	return &OllamaProvider{baseURL: parsed, client: safeLangChainHTTPClient(client)}, nil
}

func (provider *OllamaProvider) Embed(ctx context.Context, request intelligencedomain.EmbeddingRequest) (intelligencedomain.EmbeddingResponse, error) {
	if provider == nil {
		return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	if err := request.Validate(); err != nil || request.ModelName != intelligencedomain.OllamaQwenEmbeddingModel {
		return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	if err := provider.verifyModelDigest(ctx, request.ModelName, request.ModelVersion); err != nil {
		return intelligencedomain.EmbeddingResponse{}, err
	}
	model, err := langchainollama.New(langchainollama.WithServerURL(provider.baseURL.String()), langchainollama.WithHTTPClient(provider.client), langchainollama.WithModel(request.ModelName))
	if err != nil {
		return intelligencedomain.EmbeddingResponse{}, mapLangChainError(err)
	}
	vectors, err := model.CreateEmbedding(ctx, request.Inputs)
	if err != nil {
		if ctx.Err() != nil {
			return intelligencedomain.EmbeddingResponse{}, mapLangChainError(ctx.Err())
		}
		return intelligencedomain.EmbeddingResponse{}, mapLangChainError(err)
	}
	if err := validateOllamaVectors(vectors, len(request.Inputs)); err != nil {
		return intelligencedomain.EmbeddingResponse{}, err
	}
	return intelligencedomain.EmbeddingResponse{ModelVersion: request.ModelVersion, Vectors: vectors}, nil
}

func validateOllamaVectors(vectors [][]float32, expected int) error {
	if len(vectors) != expected {
		return intelligencedomain.NewError(intelligencedomain.CodeAIEmbeddingInvalid)
	}
	for _, vector := range vectors {
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return intelligencedomain.NewError(intelligencedomain.CodeAIEmbeddingInvalid)
			}
		}
		if err := intelligencedomain.ValidateEmbedding(vector); err != nil {
			return err
		}
	}
	return nil
}

func (provider *OllamaProvider) GenerateStructured(ctx context.Context, request intelligencedomain.StructuredRequest) (intelligencedomain.StructuredResponse, error) {
	if provider == nil {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	if err := request.Validate(); err != nil {
		return intelligencedomain.StructuredResponse{}, err
	}
	if err := provider.verifyModelDigest(ctx, request.ModelName, request.ModelVersion); err != nil {
		return intelligencedomain.StructuredResponse{}, err
	}
	input, err := structuredInput(request)
	if err != nil {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	model, err := langchainollama.New(langchainollama.WithServerURL(provider.baseURL.String()), langchainollama.WithHTTPClient(provider.client), langchainollama.WithModel(request.ModelName))
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

func (provider *OllamaProvider) verifyModelDigest(ctx context.Context, modelName, modelVersion string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.baseURL.JoinPath("api", "tags").String(), nil)
	if err != nil {
		return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	response, err := provider.client.Do(request)
	if err != nil {
		return mapLangChainError(err)
	}
	defer response.Body.Close()
	var payload struct {
		Models []struct {
			Name   string `json:"name"`
			Model  string `json:"model"`
			Digest string `json:"digest"`
		} `json:"models"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
	}
	matches := 0
	for _, candidate := range payload.Models {
		if candidate.Name == modelName || candidate.Model == modelName {
			matches++
			digest := strings.TrimPrefix(candidate.Digest, "sha256:")
			if !ollamaDigestValueValid(digest) || digest != modelVersion {
				return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
			}
		}
	}
	if matches != 1 {
		return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	return nil
}

func ollamaDigestValueValid(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

var _ intelligencedomain.Provider = (*OllamaProvider)(nil)
