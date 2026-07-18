package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

func TestDeepSeekProviderUsesLangChainJSONChatAndUsage(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/chat/completions" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-deepseek-key" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "deepseek-v4-pro" {
			t.Errorf("model = %#v", body["model"])
		}
		format, _ := body["response_format"].(map[string]any)
		if format["type"] != "json_object" {
			t.Errorf("response_format = %#v", body["response_format"])
		}
		messages, _ := body["messages"].([]any)
		if len(messages) != 2 {
			t.Errorf("messages = %#v", body["messages"])
		} else if human, _ := messages[1].(map[string]any); !strings.Contains(human["content"].(string), `"previous_output"`) || !strings.Contains(human["content"].(string), `"violations"`) {
			t.Errorf("repair input was not forwarded: %#v", human)
		}
		writeJSON(writer, map[string]any{
			"id": "chatcmpl-test", "object": "chat.completion", "model": "deepseek-v4-pro",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": `{"terms":["hotkey"]}`}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
		})
	}))
	t.Cleanup(server.Close)

	provider, err := newDeepSeekProvider("test-deepseek-key", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	request := structuredRequest()
	request.ModelName = "deepseek-v4-pro"
	response, err := provider.GenerateStructured(context.Background(), request)
	if err != nil {
		t.Fatalf("GenerateStructured() error = %v", err)
	}
	if string(response.JSON) != `{"terms":["hotkey"]}` || response.Usage != (intelligencedomain.Usage{InputTokens: 3, OutputTokens: 2}) || calls.Load() != 1 {
		t.Fatalf("response = %#v calls=%d", response, calls.Load())
	}
}

func TestDeepSeekProviderDoesNotRetryOrLeakErrorBody(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"message":"private provider diagnostic"}}`))
	}))
	t.Cleanup(server.Close)
	provider, err := newDeepSeekProvider("test-deepseek-key", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.GenerateStructured(context.Background(), structuredRequest())
	assertCode(t, err, intelligencedomain.CodeAIProviderRateLimited)
	if calls.Load() != 1 || err.Error() == "private provider diagnostic" {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}

func TestDeepSeekProviderMapsServerFailureWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":{"message":"private upstream failure"}}`))
	}))
	t.Cleanup(server.Close)
	provider, err := newDeepSeekProvider("test-deepseek-key", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.GenerateStructured(context.Background(), structuredRequest())
	assertCode(t, err, intelligencedomain.CodeAIProviderTransient)
	if calls.Load() != 1 || strings.Contains(err.Error(), "private upstream failure") {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}

func TestDeepSeekProviderMapsDeadlineAndTransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		writer.WriteHeader(http.StatusGatewayTimeout)
	}))
	t.Cleanup(server.Close)
	provider, err := newDeepSeekProvider("test-deepseek-key", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = provider.GenerateStructured(ctx, structuredRequest())
	assertCode(t, err, intelligencedomain.CodeAIProviderTimeout)

	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("private transport diagnostic")
	})}
	provider, err = newDeepSeekProvider("test-deepseek-key", "https://api.deepseek.invalid", client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.GenerateStructured(context.Background(), structuredRequest())
	assertCode(t, err, intelligencedomain.CodeAIProviderTransient)
	if strings.Contains(err.Error(), "private transport diagnostic") {
		t.Fatalf("transport detail leaked: %v", err)
	}
}

func TestDeepSeekProviderRejectsInvalidJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, map[string]any{
			"id": "chatcmpl-test", "object": "chat.completion", "model": "deepseek-v4-pro",
			"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "not-json"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
		})
	}))
	t.Cleanup(server.Close)
	provider, err := newDeepSeekProvider("test-deepseek-key", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.GenerateStructured(context.Background(), structuredRequest())
	assertCode(t, err, intelligencedomain.CodeAIOutputInvalid)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
