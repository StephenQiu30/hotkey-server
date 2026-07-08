package obsidian_test

import (
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
)

func TestRenderMarkdownDailyDigest(t *testing.T) {
	got, err := obsidian.RenderMarkdown(obsidian.MarkdownInput{
		Kind:        dto.ExportDailyDigest,
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		ReportID:    123,
		MonitorID:   10,
		MonitorName: "AI Regulation",
		Title:       "AI Regulation 日报 2026-07-08",
		Content:     "## 今日概览\n\n今日热点。",
	})
	if err != nil {
		t.Fatalf("RenderMarkdown returned error: %v", err)
	}
	for _, want := range []string{
		"type: hotkey-digest",
		"report_id: 123",
		"monitor_id: 10",
		"- daily",
		"# AI Regulation 日报 2026-07-08",
		"## 今日概览",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("markdown missing %q:\n%s", want, got)
		}
	}
}

func TestRenderMarkdownPublishDraft(t *testing.T) {
	got, err := obsidian.RenderMarkdown(obsidian.MarkdownInput{
		Kind:        dto.ExportPublishDraft,
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		ReportID:    123,
		MonitorID:   10,
		MonitorName: "AI Regulation",
		Title:       "今日 AI 行业热点",
		Content:     "## 导语\n\n这是一篇草稿。",
	})
	if err != nil {
		t.Fatalf("RenderMarkdown returned error: %v", err)
	}
	for _, want := range []string{
		"type: hotkey-publish-draft",
		"publish_status: draft",
		"- wechat",
		"- zhihu",
		"- website",
		"# 今日 AI 行业热点",
		"## 导语",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("markdown missing %q:\n%s", want, got)
		}
	}
}
