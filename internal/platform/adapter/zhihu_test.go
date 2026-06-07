package adapter_test

import (
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
)

// --- Fixtures ---

func zhihuAnswerItem() adapter.NormalizedItem {
	now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	return adapter.NormalizedItem{
		Title:       "如何评价 Go 语言 2026 年的发展？",
		URL:         "https://www.zhihu.com/question/123456789/answer/987654321",
		Snippet:     "Go 语言在云原生领域表现优异，2026 年泛型支持更加成熟...",
		ExternalID:  "answer-987654321",
		PublishedAt: &now,
		Language:    "zh",
		Metadata: map[string]string{
			"content_type":  "answer",
			"author":        "张三",
			"author_link":   "https://www.zhihu.com/people/zhangsan",
			"voteup_count":  "1256",
			"comment_count": "89",
			"question_id":   "123456789",
			"question_title": "如何评价 Go 语言 2026 年的发展？",
		},
	}
}

func zhihuArticleItem() adapter.NormalizedItem {
	now := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)
	return adapter.NormalizedItem{
		Title:       "深入理解 pgvector：向量数据库实战指南",
		URL:         "https://zhuanlan.zhihu.com/p/678901234",
		Snippet:     "本文介绍了 pgvector 的核心概念、索引类型选择和性能优化技巧...",
		ExternalID:  "article-678901234",
		PublishedAt: &now,
		Language:    "zh",
		Metadata: map[string]string{
			"content_type":  "article",
			"author":        "李四",
			"author_link":   "https://www.zhihu.com/people/lisi",
			"voteup_count":  "890",
			"comment_count": "45",
		},
	}
}

func zhihuQuestionItem() adapter.NormalizedItem {
	now := time.Date(2026, 4, 10, 14, 0, 0, 0, time.UTC)
	return adapter.NormalizedItem{
		Title:       "2026 年最值得学习的编程语言是什么？",
		URL:         "https://www.zhihu.com/question/111222333",
		Snippet:     "在 AI 和云原生时代，选择编程语言需要考虑生态系统、就业市场和技术趋势...",
		ExternalID:  "question-111222333",
		PublishedAt: &now,
		Language:    "zh",
		Metadata: map[string]string{
			"content_type":    "question",
			"answer_count":    "156",
			"follower_count":  "2340",
		},
	}
}

func zhihuColumnItem() adapter.NormalizedItem {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	return adapter.NormalizedItem{
		Title:       "AI 技术前沿观察",
		URL:         "https://zhuanlan.zhihu.com/column/tech-observer",
		Snippet:     "关注人工智能、大模型和 AGI 的最新进展与深度分析",
		ExternalID:  "column-tech-observer",
		PublishedAt: &now,
		Language:    "zh",
		Metadata: map[string]string{
			"content_type":    "column",
			"articles_count":  "48",
			"followers_count": "5600",
		},
	}
}

// --- ZhihuSimulator contract tests ---

func TestZhihuSimulatorReturnsAnswer(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items: []adapter.NormalizedItem{zhihuAnswerItem()},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/question/123456789/answer/987654321",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}

	item := output.Items[0]
	if item.Title != "如何评价 Go 语言 2026 年的发展？" {
		t.Errorf("title = %q, want %q", item.Title, "如何评价 Go 语言 2026 年的发展？")
	}
	if item.Language != "zh" {
		t.Errorf("language = %q, want %q", item.Language, "zh")
	}
	if item.Metadata["content_type"] != "answer" {
		t.Errorf("content_type = %q, want %q", item.Metadata["content_type"], "answer")
	}
	if item.Metadata["author"] != "张三" {
		t.Errorf("author = %q, want %q", item.Metadata["author"], "张三")
	}
	if item.Metadata["voteup_count"] != "1256" {
		t.Errorf("voteup_count = %q, want %q", item.Metadata["voteup_count"], "1256")
	}
	if item.Metadata["question_id"] != "123456789" {
		t.Errorf("question_id = %q, want %q", item.Metadata["question_id"], "123456789")
	}
}

func TestZhihuSimulatorReturnsArticle(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items: []adapter.NormalizedItem{zhihuArticleItem()},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://zhuanlan.zhihu.com/p/678901234",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}

	item := output.Items[0]
	if item.Metadata["content_type"] != "article" {
		t.Errorf("content_type = %q, want %q", item.Metadata["content_type"], "article")
	}
	if item.Metadata["author"] != "李四" {
		t.Errorf("author = %q, want %q", item.Metadata["author"], "李四")
	}
}

func TestZhihuSimulatorReturnsQuestion(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items: []adapter.NormalizedItem{zhihuQuestionItem()},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/question/111222333",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}

	item := output.Items[0]
	if item.Metadata["content_type"] != "question" {
		t.Errorf("content_type = %q, want %q", item.Metadata["content_type"], "question")
	}
	if item.Metadata["answer_count"] != "156" {
		t.Errorf("answer_count = %q, want %q", item.Metadata["answer_count"], "156")
	}
}

func TestZhihuSimulatorReturnsColumn(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items: []adapter.NormalizedItem{zhihuColumnItem()},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://zhuanlan.zhihu.com/column/tech-observer",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}

	item := output.Items[0]
	if item.Metadata["content_type"] != "column" {
		t.Errorf("content_type = %q, want %q", item.Metadata["content_type"], "column")
	}
	if item.Metadata["articles_count"] != "48" {
		t.Errorf("articles_count = %q, want %q", item.Metadata["articles_count"], "48")
	}
}

// --- Long text truncation ---

func TestZhihuSimulatorTruncatesLongText(t *testing.T) {
	longContent := strings.Repeat("这是一段很长的知乎回答内容。", 1000) // ~13000 chars
	now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	item := adapter.NormalizedItem{
		Title:       "长文本测试回答",
		URL:         "https://www.zhihu.com/question/1/answer/2",
		Snippet:     longContent,
		ExternalID:  "answer-2",
		PublishedAt: &now,
		Language:    "zh",
		Metadata: map[string]string{
			"content_type": "answer",
			"author":       "长文本作者",
		},
	}

	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items:           []adapter.NormalizedItem{item},
		MaxSnippetChars: 500,
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/question/1/answer/2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}

	result := output.Items[0]
	if len(result.Snippet) > 500+20 { // allow for ellipsis
		t.Errorf("snippet too long: %d chars, want <= ~500", len(result.Snippet))
	}
	if !strings.HasSuffix(result.Snippet, "...") {
		t.Errorf("expected truncated snippet to end with '...', got suffix %q", result.Snippet[len(result.Snippet)-10:])
	}
	if result.Metadata["needs_summary"] != "true" {
		t.Errorf("needs_summary = %q, want %q", result.Metadata["needs_summary"], "true")
	}
}

func TestZhihuSimulatorDoesNotTruncateShortText(t *testing.T) {
	shortContent := "这是一段简短的回答。"
	now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	item := adapter.NormalizedItem{
		Title:       "短文本测试",
		URL:         "https://www.zhihu.com/question/1/answer/3",
		Snippet:     shortContent,
		ExternalID:  "answer-3",
		PublishedAt: &now,
		Language:    "zh",
		Metadata:    map[string]string{"content_type": "answer"},
	}

	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items:           []adapter.NormalizedItem{item},
		MaxSnippetChars: 500,
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/question/1/answer/3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := output.Items[0]
	if result.Snippet != shortContent {
		t.Errorf("snippet should not be truncated: got %q, want %q", result.Snippet, shortContent)
	}
	if result.Metadata["needs_summary"] == "true" {
		t.Error("short text should not be marked as needs_summary")
	}
}

// --- Deleted content handling ---

func TestZhihuSimulatorReturnsPermanentErrorForDeletedContent(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		CollectErr: adapter.NewAdapterError(adapter.FailureClassPermanent, "content deleted", nil),
	})

	_, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/question/1/answer/999",
	})
	if err == nil {
		t.Fatal("expected error for deleted content")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassPermanent) {
		t.Fatalf("expected permanent failure class, got %v", err)
	}
}

// --- Permission restriction handling ---

func TestZhihuSimulatorReturnsAuthErrorForPermissionRestriction(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		CollectErr: adapter.NewAdapterError(adapter.FailureClassAuth, "permission denied: account suspended", nil),
	})

	_, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/question/1/answer/888",
	})
	if err == nil {
		t.Fatal("expected error for permission restriction")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassAuth) {
		t.Fatalf("expected auth failure class, got %v", err)
	}
}

// --- Rate limiting ---

func TestZhihuSimulatorReturnsRateLimitError(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		CollectErr: adapter.NewAdapterError(adapter.FailureClassRateLimit, "429 too many requests", nil),
	})

	_, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/api/v4/questions/1/feeds",
	})
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Fatalf("expected rate_limit failure class, got %v", err)
	}
}

// --- URL deduplication ---

func TestZhihuSimulatorDeduplicatesByURL(t *testing.T) {
	item := zhihuAnswerItem()
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items: []adapter.NormalizedItem{item, item}, // same URL twice
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/question/123456789/answer/987654321",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 deduplicated item, got %d", len(output.Items))
	}
}

// --- Adapter interface compliance ---

func TestZhihuSimulatorImplementsAdapterInterface(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{})
	var _ adapter.Adapter = sim // compile-time check
}

func TestZhihuSimulatorName(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Name: "知乎采集器",
	})
	if sim.Name() != "知乎采集器" {
		t.Errorf("name = %q, want %q", sim.Name(), "知乎采集器")
	}
}

func TestZhihuSimulatorProvider(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{})
	if sim.Provider() != adapter.ProviderZhihu {
		t.Errorf("provider = %q, want %q", sim.Provider(), adapter.ProviderZhihu)
	}
}

func TestZhihuSimulatorCapabilities(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Capabilities: adapter.Capabilities{
			SupportsIncremental: true,
			MaxItemsPerFetch:    20,
			RateLimitPerHour:    60,
		},
	})
	caps := sim.Capabilities()
	if !caps.SupportsIncremental {
		t.Error("expected SupportsIncremental to be true")
	}
	if caps.MaxItemsPerFetch != 20 {
		t.Errorf("MaxItemsPerFetch = %d, want 20", caps.MaxItemsPerFetch)
	}
	if caps.RateLimitPerHour != 60 {
		t.Errorf("RateLimitPerHour = %d, want 60", caps.RateLimitPerHour)
	}
}

func TestZhihuSimulatorHealthStartsHealthy(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{})
	health := sim.Health()
	if health.Status != adapter.HealthStatusHealthy {
		t.Errorf("initial health = %q, want %q", health.Status, adapter.HealthStatusHealthy)
	}
}

func TestZhihuSimulatorHealthDegradesAfterError(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		CollectErr: adapter.NewAdapterError(adapter.FailureClassTransient, "timeout", nil),
	})
	_, _ = sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/api/v4/questions/1",
	})
	if sim.Health().Status != adapter.HealthStatusDegraded {
		t.Errorf("health after error = %q, want %q", sim.Health().Status, adapter.HealthStatusDegraded)
	}
}

func TestZhihuSimulatorHealthRecoversAfterSuccess(t *testing.T) {
	callCount := 0
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		CollectFn: func(input adapter.CollectInput) (adapter.CollectOutput, error) {
			callCount++
			if callCount == 1 {
				return adapter.CollectOutput{}, adapter.NewAdapterError(adapter.FailureClassTransient, "timeout", nil)
			}
			return adapter.CollectOutput{Items: []adapter.NormalizedItem{zhihuAnswerItem()}}, nil
		},
	})

	// First call fails
	_, _ = sim.Collect(adapter.CollectInput{SourceID: "src-zhihu", Provider: adapter.ProviderZhihu, URL: "https://www.zhihu.com/api/v4/questions/1"})
	if sim.Health().Status != adapter.HealthStatusDegraded {
		t.Fatal("expected degraded after failure")
	}

	// Second call succeeds
	_, _ = sim.Collect(adapter.CollectInput{SourceID: "src-zhihu", Provider: adapter.ProviderZhihu, URL: "https://www.zhihu.com/api/v4/questions/1"})
	if sim.Health().Status != adapter.HealthStatusHealthy {
		t.Errorf("health after success = %q, want %q", sim.Health().Status, adapter.HealthStatusHealthy)
	}
}

// --- Idempotency ---

func TestZhihuSimulatorGeneratesIdempotencyKey(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items: []adapter.NormalizedItem{zhihuAnswerItem()},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/question/123456789/answer/987654321",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Items[0].IdempotencyKey == "" {
		t.Error("expected IdempotencyKey to be populated")
	}
}

func TestZhihuSimulatorDeterministicIdempotencyKey(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items: []adapter.NormalizedItem{zhihuAnswerItem()},
	})

	input := adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/question/123456789/answer/987654321",
	}

	out1, _ := sim.Collect(input)
	out2, _ := sim.Collect(input)

	if out1.Items[0].IdempotencyKey != out2.Items[0].IdempotencyKey {
		t.Errorf("idempotency keys differ: %q vs %q", out1.Items[0].IdempotencyKey, out2.Items[0].IdempotencyKey)
	}
}

// --- Mixed content types ---

func TestZhihuSimulatorReturnsMultipleContentTypes(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items: []adapter.NormalizedItem{
			zhihuAnswerItem(),
			zhihuArticleItem(),
			zhihuQuestionItem(),
			zhihuColumnItem(),
		},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/api/v4/feeds",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(output.Items))
	}

	types := make(map[string]bool)
	for _, item := range output.Items {
		types[item.Metadata["content_type"]] = true
	}
	for _, expected := range []string{"answer", "article", "question", "column"} {
		if !types[expected] {
			t.Errorf("missing content type %q", expected)
		}
	}
}

// --- Metadata field presence ---

func TestZhihuSimulatorPreservesMetadata(t *testing.T) {
	item := zhihuAnswerItem()
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		Items: []adapter.NormalizedItem{item},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      item.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	meta := output.Items[0].Metadata
	expectedKeys := []string{"content_type", "author", "author_link", "voteup_count", "comment_count", "question_id", "question_title"}
	for _, key := range expectedKeys {
		if _, ok := meta[key]; !ok {
			t.Errorf("missing metadata key %q", key)
		}
	}
}

// --- CollectFn custom logic ---

func TestZhihuSimulatorSupportsCollectFn(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		CollectFn: func(input adapter.CollectInput) (adapter.CollectOutput, error) {
			return adapter.CollectOutput{
				Items: []adapter.NormalizedItem{
					{
						Title:    "动态生成的回答",
						URL:      input.URL,
						Snippet:  "通过 CollectFn 动态生成",
						Language: "zh",
						Metadata: map[string]string{"content_type": "answer"},
					},
				},
			}, nil
		},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/question/1/answer/2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}
	if output.Items[0].Title != "动态生成的回答" {
		t.Errorf("title = %q, want %q", output.Items[0].Title, "动态生成的回答")
	}
}

// --- E2E acceptance: simulator returns deleted item ---

func TestZhihuSimulatorE2EDeletedItemVisible(t *testing.T) {
	sim := adapter.NewZhihuSimulator(adapter.ZhihuSimulatorConfig{
		CollectFn: func(input adapter.CollectInput) (adapter.CollectOutput, error) {
			// Simulate: some items succeed, one is deleted
			items := []adapter.NormalizedItem{
				zhihuAnswerItem(),
				zhihuArticleItem(),
			}
			// The third item is deleted - return partial success with error
			return adapter.CollectOutput{Items: items},
				adapter.NewAdapterError(adapter.FailureClassPermanent, "answer-999 deleted", nil)
		},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-zhihu",
		Provider: adapter.ProviderZhihu,
		URL:      "https://www.zhihu.com/api/v4/feeds",
	})
	// Error should be returned for deleted content
	if err == nil {
		t.Fatal("expected error for deleted content")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassPermanent) {
		t.Fatalf("expected permanent error, got %v", err)
	}
	// Partial items may still be returned
	_ = output
}
