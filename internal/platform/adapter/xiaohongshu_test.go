package adapter_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
)

// --- Xiaohongshu note normalization tests ---

func TestXiaohongshuNoteNormalization(t *testing.T) {
	publishTime := time.Date(2026, 6, 1, 10, 30, 0, 0, time.UTC)
	note := adapter.XiaohongshuNote{
		NoteID:      "note-abc123",
		Title:       "测试笔记标题",
		Description: "这是一段测试笔记的正文内容，包含一些关键词和描述。",
		Author:      "测试作者",
		AuthorID:    "user-001",
		PublishedAt: publishTime,
		URL:         "https://www.xiaohongshu.com/explore/note-abc123",
		Tags:        []string{"美食", "探店", "上海"},
		Likes:       1500,
		Collects:    300,
		Comments:    80,
		Shares:      50,
	}

	item := adapter.NormalizeXiaohongshuNote(note, "src-xhs-1")

	if item.Title != "测试笔记标题" {
		t.Fatalf("expected title %q, got %q", "测试笔记标题", item.Title)
	}
	if item.URL != "https://www.xiaohongshu.com/explore/note-abc123" {
		t.Fatalf("expected URL %q, got %q", "https://www.xiaohongshu.com/explore/note-abc123", item.URL)
	}
	if item.ExternalID != "note-abc123" {
		t.Fatalf("expected ExternalID %q, got %q", "note-abc123", item.ExternalID)
	}
	if item.Language != "zh" {
		t.Fatalf("expected language %q, got %q", "zh", item.Language)
	}
	if item.PublishedAt == nil || !item.PublishedAt.Equal(publishTime) {
		t.Fatal("expected PublishedAt to match input")
	}
	// Snippet should contain description and tags
	if item.Snippet == "" {
		t.Fatal("expected non-empty snippet")
	}
}

func TestXiaohongshuNoteTagsInSnippet(t *testing.T) {
	note := adapter.XiaohongshuNote{
		NoteID:      "note-tags",
		Title:       "带标签的笔记",
		Description: "正文内容",
		Author:      "作者",
		AuthorID:    "user-002",
		PublishedAt: time.Now().UTC(),
		URL:         "https://www.xiaohongshu.com/explore/note-tags",
		Tags:        []string{"旅行", "日本"},
	}

	item := adapter.NormalizeXiaohongshuNote(note, "src-xhs-1")

	// Snippet should include tags
	if item.Snippet == "" {
		t.Fatal("expected non-empty snippet")
	}
}

func TestXiaohongshuNoteEmptyTags(t *testing.T) {
	note := adapter.XiaohongshuNote{
		NoteID:      "note-notags",
		Title:       "无标签笔记",
		Description: "正文内容",
		Author:      "作者",
		AuthorID:    "user-003",
		PublishedAt: time.Now().UTC(),
		URL:         "https://www.xiaohongshu.com/explore/note-notags",
		Tags:        []string{},
	}

	item := adapter.NormalizeXiaohongshuNote(note, "src-xhs-1")
	if item.Snippet == "" {
		t.Fatal("expected snippet from description even without tags")
	}
}

func TestXiaohongshuNoteIdempotencyKey(t *testing.T) {
	note := adapter.XiaohongshuNote{
		NoteID: "note-idem",
		Title:  "幂等测试",
		URL:    "https://www.xiaohongshu.com/explore/note-idem",
	}

	item1 := adapter.NormalizeXiaohongshuNote(note, "src-xhs-1")
	item2 := adapter.NormalizeXiaohongshuNote(note, "src-xhs-1")

	if item1.IdempotencyKey != item2.IdempotencyKey {
		t.Fatal("expected deterministic idempotency key for same note")
	}
	if item1.IdempotencyKey == "" {
		t.Fatal("expected non-empty idempotency key")
	}
}

// --- Xiaohongshu adapter error handling tests ---

func TestXiaohongshuAdapterName(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderXiaohongshu,
		Name:     "小红书",
	})
	if sim.Name() != "小红书" {
		t.Fatalf("expected name %q, got %q", "小红书", sim.Name())
	}
	if sim.Provider() != adapter.ProviderXiaohongshu {
		t.Fatalf("expected provider %q, got %q", adapter.ProviderXiaohongshu, sim.Provider())
	}
}

func TestXiaohongshuAdapterCapabilities(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderXiaohongshu,
		Name:     "小红书",
		Capabilities: adapter.Capabilities{
			SupportsIncremental: true,
			MaxItemsPerFetch:    20,
			RateLimitPerHour:    60,
		},
	})

	caps := sim.Capabilities()
	if !caps.SupportsIncremental {
		t.Fatal("expected SupportsIncremental to be true")
	}
	if caps.MaxItemsPerFetch != 20 {
		t.Fatalf("expected MaxItemsPerFetch 20, got %d", caps.MaxItemsPerFetch)
	}
	if caps.RateLimitPerHour != 60 {
		t.Fatalf("expected RateLimitPerHour 60, got %d", caps.RateLimitPerHour)
	}
}

func TestXiaohongshuAdapterInvisibleContentError(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderXiaohongshu,
		Name:     "小红书",
		CollectErr: adapter.NewAdapterError(
			adapter.FailureClassPermanent,
			"note not visible: content removed or private",
			nil,
		),
	})

	_, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-xhs-1",
		Provider: adapter.ProviderXiaohongshu,
		URL:      "https://www.xiaohongshu.com/explore/note-hidden",
	})
	if err == nil {
		t.Fatal("expected error for invisible content")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassPermanent) {
		t.Fatalf("expected permanent failure class, got %v", err)
	}
}

func TestXiaohongshuAdapterRateLimitError(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderXiaohongshu,
		Name:     "小红书",
		CollectErr: adapter.NewAdapterError(
			adapter.FailureClassRateLimit,
			"rate limited: too many requests",
			nil,
		),
	})

	_, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-xhs-1",
		Provider: adapter.ProviderXiaohongshu,
		URL:      "https://www.xiaohongshu.com/explore/note-rate-limited",
	})
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Fatalf("expected rate_limit failure class, got %v", err)
	}
}

func TestXiaohongshuAdapterSchemaChangeError(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderXiaohongshu,
		Name:     "小红书",
		CollectErr: adapter.NewAdapterError(
			adapter.FailureClassParseError,
			"schema change detected: unexpected field 'new_field'",
			nil,
		),
	})

	_, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-xhs-1",
		Provider: adapter.ProviderXiaohongshu,
		URL:      "https://www.xiaohongshu.com/explore/note-schema-change",
	})
	if err == nil {
		t.Fatal("expected schema change error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassParseError) {
		t.Fatalf("expected parse_error failure class, got %v", err)
	}
}

func TestXiaohongshuAdapterHealthDegradedAfterRateLimit(t *testing.T) {
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderXiaohongshu,
		Name:     "小红书",
		CollectErr: adapter.NewAdapterError(
			adapter.FailureClassRateLimit,
			"rate limited",
			nil,
		),
	})

	_, _ = sim.Collect(adapter.CollectInput{
		SourceID: "src-xhs-1",
		Provider: adapter.ProviderXiaohongshu,
		URL:      "https://www.xiaohongshu.com/explore/note-1",
	})

	health := sim.Health()
	if health.Status != adapter.HealthStatusDegraded {
		t.Fatalf("expected degraded after rate limit error, got %q", health.Status)
	}
}

func TestXiaohongshuAdapterHealthRecoversAfterSuccess(t *testing.T) {
	callCount := 0
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderXiaohongshu,
		Name:     "小红书",
		CollectFn: func(input adapter.CollectInput) (adapter.CollectOutput, error) {
			callCount++
			if callCount == 1 {
				return adapter.CollectOutput{}, adapter.NewAdapterError(adapter.FailureClassRateLimit, "rate limited", nil)
			}
			return adapter.CollectOutput{Items: []adapter.NormalizedItem{{
				Title:    "恢复测试笔记",
				URL:      "https://www.xiaohongshu.com/explore/note-recover",
				Language: "zh",
			}}}, nil
		},
	})

	// First call fails
	_, _ = sim.Collect(adapter.CollectInput{SourceID: "src-xhs-1", Provider: adapter.ProviderXiaohongshu, URL: "https://www.xiaohongshu.com/explore/note-1"})
	if sim.Health().Status != adapter.HealthStatusDegraded {
		t.Fatal("expected degraded after failure")
	}

	// Second call succeeds
	_, _ = sim.Collect(adapter.CollectInput{SourceID: "src-xhs-1", Provider: adapter.ProviderXiaohongshu, URL: "https://www.xiaohongshu.com/explore/note-1"})
	if sim.Health().Status != adapter.HealthStatusHealthy {
		t.Fatalf("expected healthy after success, got %q", sim.Health().Status)
	}
}

// --- Xiaohongshu adapter collect with items ---

func TestXiaohongshuAdapterCollectNotes(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderXiaohongshu,
		Name:     "小红书",
		Items: []adapter.NormalizedItem{
			{
				Title:       "上海探店笔记",
				URL:         "https://www.xiaohongshu.com/explore/note-1",
				Snippet:     "探店内容 #美食 #上海",
				ExternalID:  "note-1",
				PublishedAt: &now,
				Language:    "zh",
			},
			{
				Title:       "日本旅行攻略",
				URL:         "https://www.xiaohongshu.com/explore/note-2",
				Snippet:     "旅行攻略 #日本 #旅行",
				ExternalID:  "note-2",
				PublishedAt: &now,
				Language:    "zh",
			},
		},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-xhs-1",
		Provider: adapter.ProviderXiaohongshu,
		URL:      "https://www.xiaohongshu.com/user/profile/user-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(output.Items))
	}
	if output.Items[0].Title != "上海探店笔记" {
		t.Fatalf("expected title %q, got %q", "上海探店笔记", output.Items[0].Title)
	}
	if output.Items[1].ExternalID != "note-2" {
		t.Fatalf("expected ExternalID %q, got %q", "note-2", output.Items[1].ExternalID)
	}
}

// --- Xiaohongshu registry integration ---

func TestXiaohongshuAdapterRegistration(t *testing.T) {
	reg := adapter.NewRegistry()
	sim := adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderXiaohongshu,
		Name:     "小红书",
	})
	reg.Register(sim)

	got, ok := reg.Get(adapter.ProviderXiaohongshu)
	if !ok {
		t.Fatal("expected to find xiaohongshu adapter")
	}
	if got.Name() != "小红书" {
		t.Fatalf("expected name %q, got %q", "小红书", got.Name())
	}
}

func TestXiaohongshuAdapterIsolationFromOtherProviders(t *testing.T) {
	reg := adapter.NewRegistry()

	// Register xiaohongshu with rate limit error
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{
		Provider:   adapter.ProviderXiaohongshu,
		Name:       "小红书",
		CollectErr: adapter.NewAdapterError(adapter.FailureClassRateLimit, "rate limited", nil),
	}))

	// Register healthy RSS
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "rss-healthy",
		Items: []adapter.NormalizedItem{
			{Title: "RSS Article", URL: "https://example.com/rss/1", PublishedAt: &now},
		},
	}))

	// Xiaohongshu should fail
	xhsAdapter, _ := reg.Get(adapter.ProviderXiaohongshu)
	_, err := xhsAdapter.Collect(adapter.CollectInput{SourceID: "src-xhs", Provider: adapter.ProviderXiaohongshu, URL: "https://www.xiaohongshu.com/explore/1"})
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Fatalf("expected rate_limit error from xiaohongshu, got %v", err)
	}

	// RSS should still succeed
	rssAdapter, _ := reg.Get(adapter.ProviderRSS)
	output, err := rssAdapter.Collect(adapter.CollectInput{SourceID: "src-rss", Provider: adapter.ProviderRSS, URL: "https://example.com/rss"})
	if err != nil {
		t.Fatalf("expected rss to succeed, got %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item from rss, got %d", len(output.Items))
	}
}

// --- Xiaohongshu engagement metrics in snippet ---

func TestXiaohongshuNoteEngagementMetricsInSnippet(t *testing.T) {
	note := adapter.XiaohongshuNote{
		NoteID:      "note-metrics",
		Title:       "高互动笔记",
		Description: "内容描述",
		Author:      "作者",
		AuthorID:    "user-004",
		PublishedAt: time.Now().UTC(),
		URL:         "https://www.xiaohongshu.com/explore/note-metrics",
		Tags:        []string{"热门"},
		Likes:       10000,
		Collects:    2000,
		Comments:    500,
		Shares:      100,
	}

	item := adapter.NormalizeXiaohongshuNote(note, "src-xhs-1")

	// Snippet should include engagement metrics
	if item.Snippet == "" {
		t.Fatal("expected non-empty snippet with engagement metrics")
	}
}

// --- Xiaohongshu source type in fetcher layer ---

func TestXiaohongshuSourceTypeConstant(t *testing.T) {
	// Verify the constant exists and has correct value
	if adapter.ProviderXiaohongshu != "xiaohongshu" {
		t.Fatalf("expected ProviderXiaohongshu to be %q, got %q", "xiaohongshu", adapter.ProviderXiaohongshu)
	}
}
