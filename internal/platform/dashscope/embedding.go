package dashscope

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	serviceembedding "github.com/StephenQiu30/hotkey-server/internal/service/embedding"
)

const defaultEndpoint = "https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding"

type Client struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

type Option func(*Client)

func WithEndpoint(endpoint string) Option {
	return func(c *Client) {
		if endpoint != "" {
			c.endpoint = endpoint
		}
	}
}

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func New(apiKey string, opts ...Option) *Client {
	client := &Client{
		apiKey:   strings.TrimSpace(apiKey),
		endpoint: defaultEndpoint,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

func (c *Client) Embed(ctx context.Context, input serviceembedding.Request) (serviceembedding.Response, error) {
	if c.apiKey == "" {
		return serviceembedding.Response{}, serviceembedding.ErrFailedConfig
	}
	body, err := json.Marshal(requestBody{
		Model: input.Model,
		Input: requestInput{Texts: []string{input.Text}},
	})
	if err != nil {
		return serviceembedding.Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return serviceembedding.Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return serviceembedding.Response{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return serviceembedding.Response{}, fmt.Errorf("dashscope embedding failed: status %d", resp.StatusCode)
	}
	var decoded responseBody
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return serviceembedding.Response{}, err
	}
	if len(decoded.Output.Embeddings) == 0 {
		return serviceembedding.Response{}, errors.New("dashscope embedding response missing embeddings")
	}
	if len(decoded.Output.Embeddings[0].Embedding) == 0 {
		return serviceembedding.Response{}, serviceembedding.ErrEmptyVector
	}
	return serviceembedding.Response{
		Vector: decoded.Output.Embeddings[0].Embedding,
		Model:  decoded.Output.Model,
	}, nil
}

type requestBody struct {
	Model string       `json:"model"`
	Input requestInput `json:"input"`
}

type requestInput struct {
	Texts []string `json:"texts"`
}

type responseBody struct {
	Output responseOutput `json:"output"`
}

type responseOutput struct {
	Model      string              `json:"model"`
	Embeddings []responseEmbedding `json:"embeddings"`
}

type responseEmbedding struct {
	Embedding []float64 `json:"embedding"`
}
