package export

import (
	"strings"
	"testing"
)

func TestRenderPeriodicReport_Frontmatter(t *testing.T) {
	bundle := ExportBundle{Kind: "weekly"}
	got := RenderPeriodicReport(bundle, PeriodicReportInput{
		Title:       "AI监管周报",
		PeriodLabel: "2026年第27周",
	})
	if !strings.Contains(got, "type: hotkey-export") {
		t.Error("missing export type")
	}
	if !strings.Contains(got, "export_kind: weekly") {
		t.Error("missing export_kind")
	}
	if !strings.Contains(got, "title: AI监管周报") {
		t.Error("missing title")
	}
}

func TestRenderPeriodicReport_WeeklyOverview(t *testing.T) {
	bundle := ExportBundle{Kind: "weekly", DateRange: DateRange{Start: "2026-06-24", End: "2026-06-30"}}
	got := RenderPeriodicReport(bundle, PeriodicReportInput{
		Title:       "AI监管周报",
		PeriodLabel: "2026年第27周",
		TopicCount:  10,
		EventCount:  3,
		Summary:     "本周热点集中在 AI 监管政策方向。",
	})
	if !strings.Contains(got, "## 本周概览") {
		t.Error("missing overview section")
	}
	if !strings.Contains(got, "热点主题数: 10") {
		t.Error("missing topic count")
	}
	if !strings.Contains(got, "重要事件数: 3") {
		t.Error("missing event count")
	}
	if !strings.Contains(got, "本周热点集中在 AI 监管政策方向。") {
		t.Error("missing summary text")
	}
}

func TestRenderPeriodicReport_IncludesTopics(t *testing.T) {
	bundle := ExportBundle{Kind: "weekly"}
	got := RenderPeriodicReport(bundle, PeriodicReportInput{
		Title:      "周报",
		Topics: []ReportTopicItem{
			{Rank: 1, Title: "政策动态", HeatScore: 95.0, Trend: "rising"},
			{Rank: 2, Title: "技术突破", HeatScore: 80.5, Trend: "stable"},
		},
	})
	if !strings.Contains(got, "## 热点主题排行") {
		t.Error("missing topic ranking section")
	}
	if !strings.Contains(got, "政策动态") {
		t.Error("missing topic title")
	}
	if !strings.Contains(got, "95.0") {
		t.Error("missing heat score")
	}
	if !strings.Contains(got, "rising") {
		t.Error("missing trend")
	}
}

func TestRenderPeriodicReport_DailyFrontmatter(t *testing.T) {
	bundle := ExportBundle{Kind: "daily"}
	got := RenderPeriodicReport(bundle, PeriodicReportInput{
		Title:       "AI监管日报",
		PeriodLabel: "2026-07-01",
	})
	if !strings.Contains(got, "export_kind: daily") {
		t.Error("missing daily export_kind")
	}
}

func TestRenderPeriodicReport_MonthlyFrontmatter(t *testing.T) {
	bundle := ExportBundle{Kind: "monthly"}
	got := RenderPeriodicReport(bundle, PeriodicReportInput{
		Title:       "月报",
		PeriodLabel: "2026年6月",
	})
	if !strings.Contains(got, "export_kind: monthly") {
		t.Error("missing monthly export_kind")
	}
}

func TestRenderThematicReport_Frontmatter(t *testing.T) {
	got := RenderThematicReport(ExportBundle{Kind: "thematic"}, ThematicReportInput{
		Title: "AI安全态势专题",
	})
	if !strings.Contains(got, "type: hotkey-export") {
		t.Error("missing export type")
	}
	if !strings.Contains(got, "export_kind: thematic") {
		t.Error("missing export_kind")
	}
	if !strings.Contains(got, "title: AI安全态势专题") {
		t.Error("missing title")
	}
}

func TestRenderThematicReport_Body(t *testing.T) {
	got := RenderThematicReport(ExportBundle{Kind: "thematic"}, ThematicReportInput{
		Title:   "AI安全态势专题",
		Summary: "本专题分析 AI 安全领域的监管动态和技术趋势。",
	})
	if !strings.Contains(got, "# AI安全态势专题") {
		t.Error("missing title heading")
	}
	if !strings.Contains(got, "AI 安全领域的监管动态和技术趋势") {
		t.Error("missing summary")
	}
}

func TestRenderMaterialList_Frontmatter(t *testing.T) {
	got := RenderMaterialList(MaterialListInput{
		ThemeTitle: "AI监管",
		Items: []MaterialItem{
			{Fact: "新规发布", SourceURL: "https://x.com/1"},
		},
	})
	if !strings.Contains(got, "type: hotkey-export") {
		t.Error("missing export type")
	}
	if !strings.Contains(got, "export_kind: material") {
		t.Error("missing export_kind")
	}
	if !strings.Contains(got, "theme_title: AI监管") {
		t.Error("missing theme_title")
	}
}

func TestRenderMaterialList_Items(t *testing.T) {
	got := RenderMaterialList(MaterialListInput{
		ThemeTitle: "AI监管",
		Items: []MaterialItem{
			{Fact: "新规发布", SourceURL: "https://x.com/1"},
			{Fact: "行业报告", SourceURL: "https://x.com/2"},
		},
	})
	if !strings.Contains(got, "新规发布") {
		t.Error("missing item fact")
	}
	if !strings.Contains(got, "SourceURL") {
		t.Error("expected material source")
	}
	if !strings.Contains(got, "https://x.com/1") {
		t.Error("missing source URL")
	}
}
