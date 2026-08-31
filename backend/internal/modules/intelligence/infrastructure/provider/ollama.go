package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
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
	payload, err := json.Marshal(struct {
		Model      string   `json:"model"`
		Input      []string `json:"input"`
		Dimensions int      `json:"dimensions"`
	}{Model: request.ModelName, Input: request.Inputs, Dimensions: request.Dimensions})
	if err != nil {
		return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.baseURL.JoinPath("api", "embed").String(), bytes.NewReader(payload))
	if err != nil {
		return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpResponse, err := provider.client.Do(httpRequest)
	if err != nil {
		return intelligencedomain.EmbeddingResponse{}, mapLangChainError(err)
	}
	defer func() { _ = httpResponse.Body.Close() }()
	var response struct {
		Vectors         [][]float32 `json:"embeddings"`
		PromptEvalCount int64       `json:"prompt_eval_count"`
	}
	decoder := json.NewDecoder(io.LimitReader(httpResponse.Body, 32<<20))
	if err := decoder.Decode(&response); err != nil || response.PromptEvalCount < 0 {
		return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIEmbeddingInvalid)
	}
	if err := validateOllamaVectors(response.Vectors, len(request.Inputs)); err != nil {
		return intelligencedomain.EmbeddingResponse{}, err
	}
	return intelligencedomain.EmbeddingResponse{
		ModelVersion: request.ModelVersion,
		Vectors:      response.Vectors,
		Usage:        intelligencedomain.Usage{InputTokens: response.PromptEvalCount},
	}, nil
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
	payload, err := json.Marshal(ollamaStructuredRequestRecord{
		Model: request.ModelName,
		Messages: []ollamaChatMessageRecord{
			{Role: "system", Content: request.Instruction + "\nReturn exactly one JSON value matching this schema:\n" + string(request.Schema)},
			{Role: "user", Content: string(input)},
		},
		Format:  append(json.RawMessage(nil), request.Schema...),
		Stream:  false,
		Think:   false,
		Options: ollamaStructuredOptionsRecord{Temperature: 0},
	})
	if err != nil {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.baseURL.JoinPath("api", "chat").String(), bytes.NewReader(payload))
	if err != nil {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpResponse, err := provider.client.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return intelligencedomain.StructuredResponse{}, mapLangChainError(ctx.Err())
		}
		return intelligencedomain.StructuredResponse{}, mapLangChainError(err)
	}
	defer func() { _ = httpResponse.Body.Close() }()
	var response ollamaStructuredResponseRecord
	decoder := json.NewDecoder(io.LimitReader(httpResponse.Body, 32<<20))
	if err := decoder.Decode(&response); err != nil || !response.Done || !json.Valid([]byte(response.Message.Content)) || response.PromptEvalCount < 0 || response.EvalCount < 0 {
		return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIOutputInvalid)
	}
	usage := intelligencedomain.Usage{InputTokens: response.PromptEvalCount, OutputTokens: response.EvalCount}
	if _, err := usage.TotalTokens(); err != nil {
		return intelligencedomain.StructuredResponse{}, err
	}
	return intelligencedomain.StructuredResponse{
		ModelVersion: request.ModelVersion,
		JSON:         append(json.RawMessage(nil), response.Message.Content...),
		Usage:        usage,
	}, nil
}

type ollamaChatMessageRecord struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaStructuredOptionsRecord struct {
	Temperature float64 `json:"temperature"`
}

type ollamaStructuredRequestRecord struct {
	Model    string                        `json:"model"`
	Messages []ollamaChatMessageRecord     `json:"messages"`
	Format   json.RawMessage               `json:"format"`
	Stream   bool                          `json:"stream"`
	Think    bool                          `json:"think"`
	Options  ollamaStructuredOptionsRecord `json:"options"`
}

type ollamaStructuredResponseRecord struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done            bool  `json:"done"`
	PromptEvalCount int64 `json:"prompt_eval_count"`
	EvalCount       int64 `json:"eval_count"`
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
	defer func() { _ = response.Body.Close() }()
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
