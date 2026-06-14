package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/llm"
)

// ---------------------------------------------------------------------------
// MockClient tests
// ---------------------------------------------------------------------------

func TestMockClient_ReturnsFixedSummary(t *testing.T) {
	mock := &llm.MockClient{Summary: "测试摘要"}
	got, err := mock.SummarizeTopic(context.Background(), llm.TopicSummaryInput{
		TopicTitle: "AI 监管",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "测试摘要" {
		t.Fatalf("got %q, want %q", got, "测试摘要")
	}
}

func TestMockClient_ReturnsError(t *testing.T) {
	mock := &llm.MockClient{Err: context.DeadlineExceeded}
	_, err := mock.SummarizeTopic(context.Background(), llm.TopicSummaryInput{})
	if err != context.DeadlineExceeded {
		t.Fatalf("got %v, want DeadlineExceeded", err)
	}
}

func TestMockClient_RecordsInput(t *testing.T) {
	mock := &llm.MockClient{Summary: "ok"}
	in := llm.TopicSummaryInput{TopicTitle: "测试主题", Heat: 99.0}
	_, _ = mock.SummarizeTopic(context.Background(), in)
	if mock.LastInput.TopicTitle != "测试主题" {
		t.Fatalf("LastInput.TopicTitle = %q, want %q", mock.LastInput.TopicTitle, "测试主题")
	}
	if mock.LastInput.Heat != 99.0 {
		t.Fatalf("LastInput.Heat = %f, want 99.0", mock.LastInput.Heat)
	}
}

// ---------------------------------------------------------------------------
// Prompt truncation tests
// ---------------------------------------------------------------------------

func TestBuildPrompt_TruncatesLongPost(t *testing.T) {
	longContent := strings.Repeat("这是一段很长的内容。", 100) // 600 chars
	in := llm.TopicSummaryInput{
		MonitorName: "AI 监管",
		TopicTitle:  "AI 新规出台",
		Posts: []llm.PostInput{
			{Author: "user1", Content: longContent, URL: "https://example.com/1"},
		},
	}
	prompt := llm.BuildPrompt(in)
	if strings.Contains(prompt, longContent) {
		t.Fatal("expected long content to be truncated, but full content found in prompt")
	}
	// Truncated content should end with "...(截断)"
	if !strings.Contains(prompt, "...(截断)") {
		t.Fatal("expected truncation marker '...(截断)' in prompt")
	}
}

func TestBuildPrompt_NoTruncationForShortPost(t *testing.T) {
	shortContent := "这是一段短内容。"
	in := llm.TopicSummaryInput{
		MonitorName: "AI 监管",
		TopicTitle:  "AI 新规出台",
		Posts: []llm.PostInput{
			{Author: "user1", Content: shortContent, URL: "https://example.com/1"},
		},
	}
	prompt := llm.BuildPrompt(in)
	if !strings.Contains(prompt, shortContent) {
		t.Fatal("expected short content to appear in prompt without truncation")
	}
	if strings.Contains(prompt, "...(截断)") {
		t.Fatal("short content should not be truncated")
	}
}

func TestBuildPrompt_ContainsTopicInfo(t *testing.T) {
	in := llm.TopicSummaryInput{
		MonitorName: "AI 监管",
		TopicTitle:  "AI 新规出台",
		TopicKey:    "ai:监管:政策",
		Heat:        85.4,
		Trend:       "rising",
		PostCount:   12,
		Posts: []llm.PostInput{
			{Author: "user1", Content: "内容", URL: "https://example.com/1"},
		},
	}
	prompt := llm.BuildPrompt(in)
	for _, want := range []string{"AI 监管", "AI 新规出台", "85.4", "rising", "12"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// OpenAI client tests (httptest)
// ---------------------------------------------------------------------------

func TestOpenAIClient_SummarizeTopic_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Verify request body structure
		var reqBody struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if reqBody.Model != "gpt-4o-mini" {
			t.Errorf("model = %q, want %q", reqBody.Model, "gpt-4o-mini")
		}
		if len(reqBody.Messages) == 0 {
			t.Fatal("expected at least one message")
		}

		// Return mock OpenAI response
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": "这是一段AI生成的摘要。",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := llm.NewOpenAIClient(llm.OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o-mini",
	})

	got, err := client.SummarizeTopic(context.Background(), llm.TopicSummaryInput{
		MonitorName: "AI 监管",
		TopicTitle:  "AI 新规出台",
		Posts:       []llm.PostInput{{Author: "u1", Content: "内容", URL: "https://x.com/1"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "这是一段AI生成的摘要。" {
		t.Fatalf("got %q, want %q", got, "这是一段AI生成的摘要。")
	}
}

func TestOpenAIClient_SummarizeTopic_NoAPIKey(t *testing.T) {
	client := llm.NewOpenAIClient(llm.OpenAIConfig{
		APIKey:  "",
		BaseURL: "http://unused",
		Model:   "gpt-4o-mini",
	})
	_, err := client.SummarizeTopic(context.Background(), llm.TopicSummaryInput{
		TopicTitle: "test",
	})
	if err == nil {
		t.Fatal("expected error when API key is empty")
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Fatalf("error should mention API key, got: %v", err)
	}
}

func TestOpenAIClient_SummarizeTopic_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer srv.Close()

	client := llm.NewOpenAIClient(llm.OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o-mini",
	})
	_, err := client.SummarizeTopic(context.Background(), llm.TopicSummaryInput{
		TopicTitle: "test",
	})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}
