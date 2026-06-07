package adapter_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
)

// --- Fixtures ---

func biliVideoItem() adapter.NormalizedItem {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return adapter.NormalizedItem{
		Title:       "【科技】2026年最值得关注的AI技术趋势",
		URL:         "https://www.bilibili.com/video/BV1xx411c7mD",
		Snippet:     "本期视频为大家盘点2026年最值得关注的AI技术趋势，包括多模态大模型、Agent框架等。",
		ExternalID:  "BV1xx411c7mD",
		PublishedAt: &now,
		Language:    "zh",
		Metadata: map[string]string{
			"content_type": "video",
			"author":       "科技观察者",
			"view_count":   "125000",
			"reply_count":  "3200",
			"like_count":   "8900",
			"coin_count":   "2100",
			"share_count":  "450",
		},
	}
}

func biliDynamicItem() adapter.NormalizedItem {
	now := time.Date(2026, 6, 2, 8, 30, 0, 0, time.UTC)
	return adapter.NormalizedItem{
		Title:       "刚看完最新一期的AI论文解读，分享几个关键观点…",
		URL:         "https://www.bilibili.com/dynamic/12345678901",
		Snippet:     "刚看完最新一期的AI论文解读，分享几个关键观点：1. 多模态融合是趋势 2. Agent能力持续增强 3. 小模型也有大作为",
		ExternalID:  "dyn_12345678901",
		PublishedAt: &now,
		Language:    "zh",
		Metadata: map[string]string{
			"content_type": "dynamic",
			"author":       "AI前沿速递",
		},
	}
}

func biliSubtitleMissingItem() adapter.NormalizedItem {
	now := time.Date(2026, 5, 28, 15, 0, 0, 0, time.UTC)
	return adapter.NormalizedItem{
		Title:       "实测：用AI写代码到底靠不靠谱？",
		URL:         "https://www.bilibili.com/video/BV1yy411c7mE",
		Snippet:     "本视频无字幕，简介替代：实际测试了5个主流AI编程助手，从代码质量、效率提升、bug率三个维度进行了全面对比。",
		ExternalID:  "BV1yy411c7mE",
		PublishedAt: &now,
		Language:    "zh",
		Metadata: map[string]string{
			"content_type":    "video",
			"author":          "程序员小明",
			"view_count":      "89000",
			"reply_count":     "2100",
			"subtitle_status": "missing",
		},
	}
}

// --- BiliBiliSimulator contract tests ---

func TestBiliBiliSimulatorReturnsVideo(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Items: []adapter.NormalizedItem{biliVideoItem()},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=123456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}

	item := output.Items[0]
	if item.Title != "【科技】2026年最值得关注的AI技术趋势" {
		t.Errorf("title = %q, want %q", item.Title, "【科技】2026年最值得关注的AI技术趋势")
	}
	if item.ExternalID != "BV1xx411c7mD" {
		t.Errorf("externalID = %q, want %q", item.ExternalID, "BV1xx411c7mD")
	}
	if item.Metadata["content_type"] != "video" {
		t.Errorf("content_type = %q, want %q", item.Metadata["content_type"], "video")
	}
	if item.Metadata["view_count"] != "125000" {
		t.Errorf("view_count = %q, want %q", item.Metadata["view_count"], "125000")
	}
}

func TestBiliBiliSimulatorReturnsDynamic(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Items: []adapter.NormalizedItem{biliDynamicItem()},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/polymer/web-dynamic/v1/feed/space?host_mid=123456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}

	item := output.Items[0]
	if item.Metadata["content_type"] != "dynamic" {
		t.Errorf("content_type = %q, want %q", item.Metadata["content_type"], "dynamic")
	}
	if item.ExternalID != "dyn_12345678901" {
		t.Errorf("externalID = %q, want %q", item.ExternalID, "dyn_12345678901")
	}
}

func TestBiliBiliSimulatorHandlesSubtitleMissing(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Items: []adapter.NormalizedItem{biliSubtitleMissingItem()},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=789",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}

	item := output.Items[0]
	if item.Metadata["subtitle_status"] != "missing" {
		t.Errorf("subtitle_status = %q, want %q", item.Metadata["subtitle_status"], "missing")
	}
	// Snippet should fall back to description when subtitle is missing
	if item.Snippet == "" {
		t.Error("expected non-empty snippet from description fallback")
	}
}

func TestBiliBiliSimulatorHandlesTakedown(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		CollectErr: adapter.NewAdapterError(adapter.FailureClassPermanent, "bilibili content unavailable (-404)", nil),
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=999",
	})
	// Takedown is permanent failure
	if err == nil {
		t.Fatal("expected error for takedown content")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassPermanent) {
		t.Fatalf("expected permanent failure class, got %v", err)
	}
	_ = output
}

func TestBiliBiliSimulatorHandlesRateLimit(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		CollectErr: adapter.NewAdapterError(adapter.FailureClassRateLimit, "bilibili rate limit (-412)", nil),
	})

	_, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=123456",
	})
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !adapter.IsAdapterError(err, adapter.FailureClassRateLimit) {
		t.Fatalf("expected rate_limit failure class, got %v", err)
	}
}

// --- BVID deduplication ---

func TestBiliBiliSimulatorDeduplicatesByExternalID(t *testing.T) {
	item := biliVideoItem()
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Items: []adapter.NormalizedItem{item, item}, // same BVID twice
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=123456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 deduplicated item, got %d", len(output.Items))
	}
}

// --- Empty title filtering ---

func TestBiliBiliSimulatorFiltersEmptyTitle(t *testing.T) {
	emptyTitle := biliVideoItem()
	emptyTitle.Title = ""
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Items: []adapter.NormalizedItem{emptyTitle, biliVideoItem()},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=123456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item (empty title filtered), got %d", len(output.Items))
	}
}

// --- Mixed content types ---

func TestBiliBiliSimulatorReturnsMixedContentTypes(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Items: []adapter.NormalizedItem{
			biliVideoItem(),
			biliDynamicItem(),
			biliSubtitleMissingItem(),
		},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=123456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(output.Items))
	}

	types := make(map[string]bool)
	for _, item := range output.Items {
		types[item.Metadata["content_type"]] = true
	}
	if !types["video"] {
		t.Error("missing content type 'video'")
	}
	if !types["dynamic"] {
		t.Error("missing content type 'dynamic'")
	}
}

// --- Adapter interface compliance ---

func TestBiliBiliSimulatorImplementsAdapterInterface(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{})
	var _ adapter.Adapter = sim // compile-time check
}

func TestBiliBiliSimulatorName(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Name: "B站采集器",
	})
	if sim.Name() != "B站采集器" {
		t.Errorf("name = %q, want %q", sim.Name(), "B站采集器")
	}
}

func TestBiliBiliSimulatorProvider(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{})
	if sim.Provider() != adapter.ProviderBiliBili {
		t.Errorf("provider = %q, want %q", sim.Provider(), adapter.ProviderBiliBili)
	}
}

func TestBiliBiliSimulatorCapabilities(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Capabilities: adapter.Capabilities{
			SupportsIncremental: true,
			MaxItemsPerFetch:    30,
			RateLimitPerHour:    120,
		},
	})
	caps := sim.Capabilities()
	if !caps.SupportsIncremental {
		t.Error("expected SupportsIncremental to be true")
	}
	if caps.MaxItemsPerFetch != 30 {
		t.Errorf("MaxItemsPerFetch = %d, want 30", caps.MaxItemsPerFetch)
	}
	if caps.RateLimitPerHour != 120 {
		t.Errorf("RateLimitPerHour = %d, want 120", caps.RateLimitPerHour)
	}
}

// --- Health tracking ---

func TestBiliBiliSimulatorHealthStartsHealthy(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{})
	health := sim.Health()
	if health.Status != adapter.HealthStatusHealthy {
		t.Errorf("initial health = %q, want %q", health.Status, adapter.HealthStatusHealthy)
	}
}

func TestBiliBiliSimulatorHealthDegradesAfterRateLimit(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		CollectErr: adapter.NewAdapterError(adapter.FailureClassRateLimit, "bilibili rate limit (-412)", nil),
	})
	_, _ = sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=123456",
	})
	if sim.Health().Status != adapter.HealthStatusDegraded {
		t.Errorf("health after rate limit = %q, want %q", sim.Health().Status, adapter.HealthStatusDegraded)
	}
}

func TestBiliBiliSimulatorHealthRecoversAfterSuccess(t *testing.T) {
	callCount := 0
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		CollectFn: func(input adapter.CollectInput) (adapter.CollectOutput, error) {
			callCount++
			if callCount == 1 {
				return adapter.CollectOutput{}, adapter.NewAdapterError(adapter.FailureClassRateLimit, "rate limited", nil)
			}
			return adapter.CollectOutput{Items: []adapter.NormalizedItem{biliVideoItem()}}, nil
		},
	})

	// First call: rate limited
	_, _ = sim.Collect(adapter.CollectInput{SourceID: "src-bilibili", Provider: adapter.ProviderBiliBili, URL: "https://api.bilibili.com/x/space/wbi/arc/search?mid=123"})
	if sim.Health().Status != adapter.HealthStatusDegraded {
		t.Fatal("expected degraded after rate limit")
	}

	// Second call: succeeds
	_, _ = sim.Collect(adapter.CollectInput{SourceID: "src-bilibili", Provider: adapter.ProviderBiliBili, URL: "https://api.bilibili.com/x/space/wbi/arc/search?mid=123"})
	if sim.Health().Status != adapter.HealthStatusHealthy {
		t.Errorf("health after success = %q, want %q", sim.Health().Status, adapter.HealthStatusHealthy)
	}
}

// --- Idempotency ---

func TestBiliBiliSimulatorGeneratesIdempotencyKey(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Items: []adapter.NormalizedItem{biliVideoItem()},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=123456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Items[0].IdempotencyKey == "" {
		t.Error("expected IdempotencyKey to be populated")
	}
}

func TestBiliBiliSimulatorDeterministicIdempotencyKey(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Items: []adapter.NormalizedItem{biliVideoItem()},
	})

	input := adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=123456",
	}

	out1, _ := sim.Collect(input)
	out2, _ := sim.Collect(input)

	if out1.Items[0].IdempotencyKey != out2.Items[0].IdempotencyKey {
		t.Errorf("idempotency keys differ: %q vs %q", out1.Items[0].IdempotencyKey, out2.Items[0].IdempotencyKey)
	}
}

// --- Metadata preservation ---

func TestBiliBiliSimulatorPreservesMetadata(t *testing.T) {
	item := biliVideoItem()
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Items: []adapter.NormalizedItem{item},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      item.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	meta := output.Items[0].Metadata
	expectedKeys := []string{"content_type", "author", "view_count", "reply_count", "like_count", "coin_count", "share_count"}
	for _, key := range expectedKeys {
		if _, ok := meta[key]; !ok {
			t.Errorf("missing metadata key %q", key)
		}
	}
}

// --- CollectFn custom logic ---

func TestBiliBiliSimulatorSupportsCollectFn(t *testing.T) {
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		CollectFn: func(input adapter.CollectInput) (adapter.CollectOutput, error) {
			return adapter.CollectOutput{
				Items: []adapter.NormalizedItem{
					{
						Title:    "动态生成的视频",
						URL:      input.URL,
						Snippet:  "通过 CollectFn 动态生成",
						Language: "zh",
						Metadata: map[string]string{"content_type": "video"},
					},
				},
			}, nil
		},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=123456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(output.Items))
	}
	if output.Items[0].Title != "动态生成的视频" {
		t.Errorf("title = %q, want %q", output.Items[0].Title, "动态生成的视频")
	}
}

// --- E2E: simulator covers video, dynamic, subtitle missing, and takedown ---

func TestBiliBiliSimulatorE2ECoversAllScenarios(t *testing.T) {
	// Video + dynamic + subtitle-missing in one collection
	sim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		Items: []adapter.NormalizedItem{
			biliVideoItem(),
			biliDynamicItem(),
			biliSubtitleMissingItem(),
		},
	})

	output, err := sim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=123456",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(output.Items))
	}

	// Verify each scenario is covered
	hasVideo, hasDynamic, hasSubtitleMissing := false, false, false
	for _, item := range output.Items {
		switch item.Metadata["content_type"] {
		case "video":
			hasVideo = true
			if item.Metadata["subtitle_status"] == "missing" {
				hasSubtitleMissing = true
			}
		case "dynamic":
			hasDynamic = true
		}
	}
	if !hasVideo {
		t.Error("missing video content type")
	}
	if !hasDynamic {
		t.Error("missing dynamic content type")
	}
	if !hasSubtitleMissing {
		t.Error("missing subtitle-missing scenario")
	}

	// Now test takedown separately (different collect behavior)
	takedownSim := adapter.NewBiliBiliSimulator(adapter.BiliBiliSimulatorConfig{
		CollectErr: adapter.NewAdapterError(adapter.FailureClassPermanent, "bilibili content unavailable (-404)", nil),
	})
	_, err = takedownSim.Collect(adapter.CollectInput{
		SourceID: "src-bilibili",
		Provider: adapter.ProviderBiliBili,
		URL:      "https://api.bilibili.com/x/space/wbi/arc/search?mid=999",
	})
	if err == nil {
		t.Error("expected error for takedown scenario")
	}
}
