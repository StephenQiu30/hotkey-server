// tests/unit/llm/service_test.go
package llm_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/llm"
)

// mockProvider implements llm.Provider for testing.
type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) Chat(_ context.Context, prompt string, opts ...llm.Option) (string, error) {
	return m.response, m.err
}

func TestSummarize_ReturnsSummary(t *testing.T) {
	svc := llm.NewService(&mockProvider{response: "这是一个测试摘要。"})
	result, err := svc.Summarize(context.Background(), "测试内容")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestSummarize_EmptyInput_ReturnsError(t *testing.T) {
	svc := llm.NewService(&mockProvider{response: ""})
	_, err := svc.Summarize(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestLabelTopics_ReturnsLabels(t *testing.T) {
	svc := llm.NewService(&mockProvider{response: "AI, 科技, 创新"})
	labels, err := svc.LabelTopics(context.Background(), "AI technology content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) == 0 {
		t.Fatal("expected non-empty labels")
	}
}

func TestLabelTopics_EmptyContent_ReturnsEmpty(t *testing.T) {
	svc := llm.NewService(&mockProvider{response: ""})
	labels, err := svc.LabelTopics(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 0 {
		t.Fatalf("expected 0 labels, got %d", len(labels))
	}
}

func TestProviderError_Propagated(t *testing.T) {
	svc := llm.NewService(&mockProvider{err: llm.ErrProviderError})
	_, err := svc.Summarize(context.Background(), "test")
	if err != llm.ErrProviderError {
		t.Fatalf("expected ErrProviderError, got %v", err)
	}
}

func TestChainBuildDailyDigest_CallsSummarizeAndLabel(t *testing.T) {
	svc := llm.NewService(&mockProvider{response: "test summary"})
	chain := llm.NewChain(svc)

	output, err := chain.BuildDailyDigest(context.Background(), llm.DigestInput{
		Title: "Test Digest",
		Posts: []llm.PostItem{
			{ID: 1, Title: "Post 1", Content: "Content 1", Platform: "x"},
			{ID: 2, Title: "Post 2", Content: "Content 2", Platform: "weibo"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Title != "Test Digest" {
		t.Fatalf("expected 'Test Digest', got '%s'", output.Title)
	}
	if len(output.Sections) == 0 {
		t.Fatal("expected at least one section")
	}
}

func TestChainBuildDailyDigest_EmptyPosts(t *testing.T) {
	svc := llm.NewService(&mockProvider{response: "empty digest"})
	chain := llm.NewChain(svc)

	_, err := chain.BuildDailyDigest(context.Background(), llm.DigestInput{
		Title: "Empty Digest",
		Posts: []llm.PostItem{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
