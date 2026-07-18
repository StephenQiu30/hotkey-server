package provider

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

const testOllamaDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestOllamaProviderBindsDigestAndEmbedsQwenInOrder(t *testing.T) {
	var tags, embeds atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/tags":
			tags.Add(1)
			writeJSON(writer, map[string]any{"models": []any{map[string]any{"name": intelligencedomain.OllamaQwenEmbeddingModel, "model": intelligencedomain.OllamaQwenEmbeddingModel, "digest": "sha256:" + testOllamaDigest}}})
		case "/api/embed":
			embeds.Add(1)
			var body struct {
				Input string `json:"input"`
			}
			_ = json.NewDecoder(request.Body).Decode(&body)
			vector := make([]float32, intelligencedomain.EmbeddingDimensions)
			if body.Input == "second" {
				vector[0] = 2
			} else {
				vector[0] = 1
			}
			writeJSON(writer, map[string]any{"embeddings": [][]float32{vector}, "prompt_eval_count": 11})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	provider, err := newOllamaProvider(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Embed(context.Background(), intelligencedomain.EmbeddingRequest{
		ModelName: intelligencedomain.OllamaQwenEmbeddingModel, ModelVersion: testOllamaDigest,
		Dimensions: intelligencedomain.EmbeddingDimensions, Inputs: []string{"first", "second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tags.Load() != 1 || embeds.Load() != 2 || len(response.Vectors) != 2 || response.Vectors[0][0] != 1 || response.Vectors[1][0] != 2 || response.Usage != (intelligencedomain.Usage{}) {
		t.Fatalf("tags=%d embeds=%d response=%#v", tags.Load(), embeds.Load(), response)
	}
}

func TestOllamaProviderGeneratesStructuredJSONWithUsage(t *testing.T) {
	var chats atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/tags":
			writeOllamaTags(writer, "qwen3:8b")
		case "/api/chat":
			chats.Add(1)
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			stream, hasStream := body["stream"]
			if body["model"] != "qwen3:8b" || body["format"] != "json" || (hasStream && stream != false) {
				t.Errorf("chat request = %#v", body)
			}
			messages, _ := body["messages"].([]any)
			if len(messages) != 2 || !strings.Contains(messages[1].(map[string]any)["content"].(string), `"previous_output"`) {
				t.Errorf("repair messages = %#v", messages)
			}
			writeJSON(writer, map[string]any{
				"model": "qwen3:8b", "message": map[string]any{"role": "assistant", "content": `{"terms":["hotkey"]}`},
				"done": true, "prompt_eval_count": 7, "eval_count": 3,
			})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	provider, err := newOllamaProvider(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	request := structuredRequest()
	request.ModelName, request.ModelVersion = "qwen3:8b", testOllamaDigest
	response, err := provider.GenerateStructured(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if string(response.JSON) != `{"terms":["hotkey"]}` || response.Usage != (intelligencedomain.Usage{InputTokens: 7, OutputTokens: 3}) || chats.Load() != 1 {
		t.Fatalf("response=%#v chats=%d", response, chats.Load())
	}
}

func TestOllamaProviderRejectsDigestDriftBeforeModelCall(t *testing.T) {
	var modelCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/tags" {
			writeJSON(writer, map[string]any{"models": []any{map[string]any{"name": intelligencedomain.OllamaQwenEmbeddingModel, "digest": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}})
			return
		}
		modelCalls.Add(1)
	}))
	t.Cleanup(server.Close)
	provider, err := newOllamaProvider(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Embed(context.Background(), intelligencedomain.EmbeddingRequest{
		ModelName: intelligencedomain.OllamaQwenEmbeddingModel, ModelVersion: testOllamaDigest,
		Dimensions: intelligencedomain.EmbeddingDimensions, Inputs: []string{"first"},
	})
	assertCode(t, err, intelligencedomain.CodeAIModelProfileInvalid)
	if modelCalls.Load() != 0 {
		t.Fatalf("model calls = %d", modelCalls.Load())
	}
}

func TestOllamaProviderRejectsWrongEmbeddingDimension(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/tags" {
			writeJSON(writer, map[string]any{"models": []any{map[string]any{"name": intelligencedomain.OllamaQwenEmbeddingModel, "digest": "sha256:" + testOllamaDigest}}})
			return
		}
		vector := make([]float32, intelligencedomain.EmbeddingDimensions-1)
		writeJSON(writer, map[string]any{"embeddings": [][]float32{vector}})
	}))
	t.Cleanup(server.Close)
	provider, err := newOllamaProvider(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Embed(context.Background(), intelligencedomain.EmbeddingRequest{ModelName: intelligencedomain.OllamaQwenEmbeddingModel, ModelVersion: testOllamaDigest, Dimensions: intelligencedomain.EmbeddingDimensions, Inputs: []string{"first"}})
	assertCode(t, err, intelligencedomain.CodeAIEmbeddingInvalid)
}

func TestOllamaProviderRejectsInvalidEmbeddingValues(t *testing.T) {
	for _, value := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		vector := make([]float32, intelligencedomain.EmbeddingDimensions)
		vector[0] = value
		assertCode(t, validateOllamaVectors([][]float32{vector}, 1), intelligencedomain.CodeAIEmbeddingInvalid)
	}
	assertCode(t, validateOllamaVectors([][]float32{make([]float32, intelligencedomain.EmbeddingDimensions+1)}, 1), intelligencedomain.CodeAIEmbeddingInvalid)
}

func TestOllamaProviderRejectsMissingModelBeforeModelCall(t *testing.T) {
	var modelCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/tags" {
			writeJSON(writer, map[string]any{"models": []any{}})
			return
		}
		modelCalls.Add(1)
	}))
	t.Cleanup(server.Close)
	provider, err := newOllamaProvider(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	request := structuredRequest()
	request.ModelName, request.ModelVersion = "qwen3:8b", testOllamaDigest
	_, err = provider.GenerateStructured(context.Background(), request)
	assertCode(t, err, intelligencedomain.CodeAIModelProfileInvalid)
	if modelCalls.Load() != 0 {
		t.Fatalf("model calls = %d", modelCalls.Load())
	}
}

func TestOllamaProviderMapsChatErrorsWithoutRetryOrBodyLeak(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		code   int
	}{
		{"rate limited", http.StatusTooManyRequests, intelligencedomain.CodeAIProviderRateLimited},
		{"server failure", http.StatusServiceUnavailable, intelligencedomain.CodeAIProviderTransient},
	} {
		t.Run(test.name, func(t *testing.T) {
			var chats atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/tags" {
					writeOllamaTags(writer, "qwen3:8b")
					return
				}
				if request.URL.Path != "/api/chat" {
					t.Fatalf("path = %s", request.URL.Path)
				}
				chats.Add(1)
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(`{"error":"private ollama diagnostic"}`))
			}))
			t.Cleanup(server.Close)
			provider, err := newOllamaProvider(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			request := structuredRequest()
			request.ModelName, request.ModelVersion = "qwen3:8b", testOllamaDigest
			_, err = provider.GenerateStructured(context.Background(), request)
			assertCode(t, err, test.code)
			if chats.Load() != 1 || strings.Contains(err.Error(), "private ollama diagnostic") {
				t.Fatalf("chats=%d err=%v", chats.Load(), err)
			}
		})
	}
}

func TestOllamaProviderMapsEmbeddingErrorsWithoutRetry(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		code   int
		delay  time.Duration
	}{
		{"rate limited", http.StatusTooManyRequests, intelligencedomain.CodeAIProviderRateLimited, 0},
		{"server failure", http.StatusServiceUnavailable, intelligencedomain.CodeAIProviderTransient, 0},
		{"deadline", http.StatusGatewayTimeout, intelligencedomain.CodeAIProviderTimeout, 50 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			var embeds atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/tags" {
					writeOllamaTags(writer, intelligencedomain.OllamaQwenEmbeddingModel)
					return
				}
				if request.URL.Path != "/api/embed" {
					t.Fatalf("path = %s", request.URL.Path)
				}
				embeds.Add(1)
				if test.delay > 0 {
					time.Sleep(test.delay)
				}
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(`{"error":"private embedding diagnostic"}`))
			}))
			t.Cleanup(server.Close)
			provider, err := newOllamaProvider(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if test.delay > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			}
			_, err = provider.Embed(ctx, intelligencedomain.EmbeddingRequest{
				ModelName: intelligencedomain.OllamaQwenEmbeddingModel, ModelVersion: testOllamaDigest,
				Dimensions: intelligencedomain.EmbeddingDimensions, Inputs: []string{"single"},
			})
			assertCode(t, err, test.code)
			if embeds.Load() != 1 || strings.Contains(err.Error(), "private embedding diagnostic") {
				t.Fatalf("embeds=%d err=%v", embeds.Load(), err)
			}
		})
	}
}

func TestOllamaProviderMapsDeadlineAndInvalidJSON(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/api/tags" {
				writeOllamaTags(writer, "qwen3:8b")
				return
			}
			time.Sleep(50 * time.Millisecond)
			writer.WriteHeader(http.StatusGatewayTimeout)
		}))
		t.Cleanup(server.Close)
		provider, err := newOllamaProvider(server.URL, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		request := structuredRequest()
		request.ModelName, request.ModelVersion = "qwen3:8b", testOllamaDigest
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err = provider.GenerateStructured(ctx, request)
		assertCode(t, err, intelligencedomain.CodeAIProviderTimeout)
	})

	t.Run("invalid JSON output", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/api/tags" {
				writeOllamaTags(writer, "qwen3:8b")
				return
			}
			writeJSON(writer, map[string]any{"model": "qwen3:8b", "message": map[string]any{"role": "assistant", "content": "not-json"}, "done": true, "prompt_eval_count": 1, "eval_count": 1})
		}))
		t.Cleanup(server.Close)
		provider, err := newOllamaProvider(server.URL, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		request := structuredRequest()
		request.ModelName, request.ModelVersion = "qwen3:8b", testOllamaDigest
		_, err = provider.GenerateStructured(context.Background(), request)
		assertCode(t, err, intelligencedomain.CodeAIOutputInvalid)
	})
}

func writeOllamaTags(writer http.ResponseWriter, model string) {
	writeJSON(writer, map[string]any{"models": []any{map[string]any{"name": model, "model": model, "digest": "sha256:" + testOllamaDigest}}})
}
