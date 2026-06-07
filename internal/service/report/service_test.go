package report

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestGenerateChannelReportWithMockQwen(t *testing.T) {
	repo := NewMemoryReportRepository()
	qwen := &mockQwen{body: "# 中文日报\n\n## 来源引用\n- [1] 来源"}
	service := newTestService(repo, qwen)

	report, err := service.GenerateChannelReport(context.Background(), GenerateReportInput{Date: "2026-05-31", ChannelID: "ai"})
	if err != nil {
		t.Fatalf("GenerateChannelReport returned error: %v", err)
	}
	if report.Status != ReportStatusSucceeded {
		t.Fatalf("expected succeeded, got %s", report.Status)
	}
	if report.PromptVersion != PromptVersion {
		t.Fatalf("expected prompt version %s, got %s", PromptVersion, report.PromptVersion)
	}
	if len(report.SourceRefs) != 2 {
		t.Fatalf("expected source refs, got %+v", report.SourceRefs)
	}
	saved, err := repo.FindReportByID(context.Background(), report.ID)
	if err != nil || saved.ID != report.ID {
		t.Fatalf("expected report saved, got %+v err=%v", saved, err)
	}
	if !strings.Contains(qwen.prompt, "只输出中文日报") || !strings.Contains(qwen.prompt, "来源引用") {
		t.Fatalf("prompt missing Chinese/source constraints: %s", qwen.prompt)
	}
}

func TestGenerateChannelReportInsufficientEvidenceDegrades(t *testing.T) {
	service := NewService(NewMemoryReportRepository(), &mockQwen{body: "不应调用"}, &mockClusters{items: []ContentItemInfo{
		{ID: "item-1", SourceID: "src-1", Title: "单一来源", URL: "https://example.test/1"},
	}}, &mockScores{}, &mockSources{sources: []SourceInfo{{ID: "src-1", ChannelIDs: []string{"ai"}}}}, nil)

	report, err := service.GenerateChannelReport(context.Background(), GenerateReportInput{Date: "2026-05-31", ChannelID: "ai"})
	if err != nil {
		t.Fatalf("GenerateChannelReport returned error: %v", err)
	}
	if report.Status != ReportStatusDegraded {
		t.Fatalf("expected degraded, got %s", report.Status)
	}
	if strings.Contains(report.Body, "编造") || !strings.Contains(report.Body, "证据不足，暂不生成影响分析") {
		t.Fatalf("expected no speculative impact analysis, got %s", report.Body)
	}
}

func TestGenerateChannelReportDashScopeNotConfigured(t *testing.T) {
	service := newTestService(NewMemoryReportRepository(), nil)
	report, err := service.GenerateChannelReport(context.Background(), GenerateReportInput{Date: "2026-05-31", ChannelID: "ai"})
	if err != nil {
		t.Fatalf("GenerateChannelReport returned error: %v", err)
	}
	if report.Status != ReportStatusFailedConfig {
		t.Fatalf("expected failed_config, got %s", report.Status)
	}
	if !strings.Contains(report.Body, "DashScope 配置缺失") {
		t.Fatalf("expected config degradation note, got %s", report.Body)
	}
}

func TestGenerateUserReportFiltersBySubscriptionsAndKeywords(t *testing.T) {
	repo := NewMemoryReportRepository()
	service := NewService(repo, &mockQwen{body: "中文日报\n来源引用：[1]"}, &mockClusters{
		clusters: []ClusterInfo{
			{ID: "cluster-ai", Title: "AI Agents", Keywords: []string{"AI"}},
			{ID: "cluster-sports", Title: "体育新闻", Keywords: []string{"sports"}},
		},
		itemsByCluster: map[string][]ContentItemInfo{
			"cluster-ai":     {{ID: "item-ai-1", SourceID: "src-ai", Title: "AI Agents 发布", Snippet: "AI tooling", URL: "https://example.test/ai1"}, {ID: "item-ai-2", SourceID: "src-ai2", Title: "AI Agent 生态", Snippet: "creator AI", URL: "https://example.test/ai2"}},
			"cluster-sports": {{ID: "item-sp-1", SourceID: "src-sp", Title: "比赛", Snippet: "sports", URL: "https://example.test/sp"}},
		},
	}, &mockScores{scores: []ScoreInfo{{ClusterID: "cluster-ai", TotalScore: 0.9}, {ClusterID: "cluster-sports", TotalScore: 0.8}}}, &mockSources{sources: []SourceInfo{
		{ID: "src-ai", ChannelIDs: []string{"tech"}},
		{ID: "src-ai2", ChannelIDs: []string{"tech"}},
		{ID: "src-sp", ChannelIDs: []string{"sports"}},
	}}, &mockPrefs{channelIDs: []string{"tech"}, keywords: []string{"AI"}})

	report, err := service.GenerateUserReport(context.Background(), GenerateReportInput{Date: "2026-05-31", UserID: "usr-1"})
	if err != nil {
		t.Fatalf("GenerateUserReport returned error: %v", err)
	}
	if len(report.InputHotspotIDs) != 1 || report.InputHotspotIDs[0] != "cluster-ai" {
		t.Fatalf("expected only subscribed keyword hotspot, got %+v", report.InputHotspotIDs)
	}
}

type mockQwen struct {
	body   string
	prompt string
}

func (m *mockQwen) GenerateReport(_ context.Context, prompt string) (string, error) {
	m.prompt = prompt
	return m.body, nil
}

func newTestService(repo ReportRepository, qwen QwenClient) *Service {
	service := NewService(repo, qwen, &mockClusters{}, &mockScores{}, &mockSources{}, nil)
	service.SetClock(func() time.Time { return time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC) })
	return service
}

type mockClusters struct {
	clusters       []ClusterInfo
	items          []ContentItemInfo
	itemsByCluster map[string][]ContentItemInfo
}

func (m *mockClusters) ListClusters(context.Context) ([]ClusterInfo, error) {
	if m.clusters != nil {
		return m.clusters, nil
	}
	return []ClusterInfo{{ID: "cluster-1", Title: "AI 工具热度上升", Keywords: []string{"AI", "创作者"}}}, nil
}

func (m *mockClusters) ListClusterItems(_ context.Context, clusterID string) ([]ContentItemInfo, error) {
	if m.itemsByCluster != nil {
		return m.itemsByCluster[clusterID], nil
	}
	if m.items != nil {
		return m.items, nil
	}
	return []ContentItemInfo{
		{ID: "item-1", SourceID: "src-1", Title: "AI 工具发布", Snippet: "新工具适合创作者", URL: "https://example.test/1"},
		{ID: "item-2", SourceID: "src-2", Title: "创作者采用 AI", Snippet: "工作流升级", URL: "https://example.test/2"},
	}, nil
}

type mockScores struct {
	scores []ScoreInfo
}

func (m *mockScores) ListScores(context.Context) ([]ScoreInfo, error) {
	if m.scores != nil {
		return m.scores, nil
	}
	return []ScoreInfo{{ClusterID: "cluster-1", TotalScore: 0.95}}, nil
}

type mockSources struct {
	sources []SourceInfo
}

func (m *mockSources) ListSources(context.Context) ([]SourceInfo, error) {
	if m.sources != nil {
		return m.sources, nil
	}
	return []SourceInfo{{ID: "src-1", Name: "来源一", ChannelIDs: []string{"ai"}}, {ID: "src-2", Name: "来源二", ChannelIDs: []string{"ai"}}}, nil
}

type mockPrefs struct {
	channelIDs []string
	keywords   []string
}

func (m *mockPrefs) ListUserChannelIDs(context.Context, string) ([]string, error) {
	return m.channelIDs, nil
}

func (m *mockPrefs) ListUserKeywords(context.Context, string) ([]string, error) {
	return m.keywords, nil
}

func TestGenerateWeeklyReportAggregatesPast7Days(t *testing.T) {
	repo := NewMemoryReportRepository()
	qwen := &mockQwen{body: "# 周报\n\n## 本周摘要\n\n周报正文。"}
	service := newTestService(repo, qwen)

	// Save 3 daily reports for the past week
	for i, date := range []string{"2026-05-25", "2026-05-27", "2026-05-29"} {
		if _, err := repo.SaveReport(context.Background(), DailyReport{
			ID:        fmt.Sprintf("daily-%d", i),
			Date:      date,
			ChannelID: "ai",
			Body:      fmt.Sprintf("日报正文 %d", i),
			Status:    ReportStatusSucceeded,
			SourceRefs: []SourceRef{
				{SourceID: "src-1", ItemID: "item-1", Title: "来源1", URL: "https://example.test/1"},
				{SourceID: "src-2", ItemID: "item-2", Title: "来源2", URL: "https://example.test/2"},
			},
			CreatedAt: time.Date(2026, 5, 25+i, 8, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 5, 25+i, 8, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("SaveReport failed: %v", err)
		}
	}

	report, err := service.GenerateWeeklyReport(context.Background(), GenerateWeeklyReportInput{
		WeekOf:    "2026-W22",
		ChannelID: "ai",
	})
	if err != nil {
		t.Fatalf("GenerateWeeklyReport returned error: %v", err)
	}
	if report.Status != ReportStatusSucceeded {
		t.Fatalf("expected succeeded, got %s", report.Status)
	}
	if report.ReportType != "weekly" {
		t.Fatalf("expected weekly report type, got %s", report.ReportType)
	}
	if len(report.DailyReportIDs) != 3 {
		t.Fatalf("expected 3 daily report IDs, got %d", len(report.DailyReportIDs))
	}
}

func TestGenerateWeeklyReportNoDailyReportsReturnsDegraded(t *testing.T) {
	repo := NewMemoryReportRepository()
	service := newTestService(repo, &mockQwen{body: "不应调用"})

	report, err := service.GenerateWeeklyReport(context.Background(), GenerateWeeklyReportInput{
		WeekOf:    "2026-W22",
		ChannelID: "ai",
	})
	if err != nil {
		t.Fatalf("GenerateWeeklyReport returned error: %v", err)
	}
	if report.Status != ReportStatusDegraded {
		t.Fatalf("expected degraded for no daily reports, got %s", report.Status)
	}
	if !strings.Contains(report.Body, "本周无日报数据") {
		t.Fatalf("expected no-data note in body, got %s", report.Body)
	}
}
