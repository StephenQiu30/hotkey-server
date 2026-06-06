package filter

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
)

func TestFilterPassesContentWithMatchingKeyword(t *testing.T) {
	svc := NewService(Config{
		Keywords:       []string{"AI", "人工智能", "机器学习"},
		MinTitleRunes:  1,
		MinSnippetRunes: 1,
	})
	result, err := svc.Filter(context.Background(), content.SourceItem{
		Title:   "AI 新闻：最新进展",
		Snippet: "正文片段",
	})
	if err != nil {
		t.Fatalf("filter failed: %v", err)
	}
	if !result.Accepted || result.Reason != ReasonPassed {
		t.Fatalf("expected accepted with pass, got %+v", result)
	}
}

func TestFilterRejectsContentWithoutKeyword(t *testing.T) {
	svc := NewService(Config{
		Keywords:       []string{"AI", "人工智能"},
		MinTitleRunes:  1,
		MinSnippetRunes: 1,
	})
	result, err := svc.Filter(context.Background(), content.SourceItem{
		Title:   "体育新闻",
		Snippet: "足球比赛结果",
	})
	if err != nil {
		t.Fatalf("filter failed: %v", err)
	}
	if result.Accepted || result.Reason != ReasonNoKeywords {
		t.Fatalf("expected rejected with no_keyword_match, got %+v", result)
	}
}

func TestFilterRejectsContentWithExclusionWord(t *testing.T) {
	svc := NewService(Config{
		Keywords:       []string{"AI"},
		ExcludeWords:   []string{"广告", "推广"},
		MinTitleRunes:  1,
		MinSnippetRunes: 1,
	})
	result, err := svc.Filter(context.Background(), content.SourceItem{
		Title:   "AI 广告推广",
		Snippet: "正文片段",
	})
	if err != nil {
		t.Fatalf("filter failed: %v", err)
	}
	if result.Accepted || result.Reason != ReasonExcluded {
		t.Fatalf("expected rejected with exclusion_word_match, got %+v", result)
	}
}

func TestFilterRejectsTooShortContent(t *testing.T) {
	svc := NewService(Config{
		Keywords:       []string{"AI"},
		MinTitleRunes:  5,
		MinSnippetRunes: 10,
	})
	result, err := svc.Filter(context.Background(), content.SourceItem{
		Title:   "AI",
		Snippet: "短",
	})
	if err != nil {
		t.Fatalf("filter failed: %v", err)
	}
	if result.Accepted || result.Reason != ReasonShortContent {
		t.Fatalf("expected rejected with content_too_short, got %+v", result)
	}
}

func TestFilterPassesWhenNoKeywordsConfigured(t *testing.T) {
	svc := NewService(Config{
		Keywords:       nil,
		MinTitleRunes:  1,
		MinSnippetRunes: 1,
	})
	result, err := svc.Filter(context.Background(), content.SourceItem{
		Title:   "任意标题",
		Snippet: "任意正文",
	})
	if err != nil {
		t.Fatalf("filter failed: %v", err)
	}
	if !result.Accepted || result.Reason != ReasonPassed {
		t.Fatalf("expected accepted when no keywords configured, got %+v", result)
	}
}

func TestFilterExclusionTakesPrecedenceOverKeyword(t *testing.T) {
	svc := NewService(Config{
		Keywords:       []string{"AI"},
		ExcludeWords:   []string{"spam"},
		MinTitleRunes:  1,
		MinSnippetRunes: 1,
	})
	result, err := svc.Filter(context.Background(), content.SourceItem{
		Title:   "AI spam content",
		Snippet: "正文片段",
	})
	if err != nil {
		t.Fatalf("filter failed: %v", err)
	}
	if result.Accepted || result.Reason != ReasonExcluded {
		t.Fatalf("expected exclusion to take precedence, got %+v", result)
	}
}

func TestFilterKeywordMatchIsCaseInsensitive(t *testing.T) {
	svc := NewService(Config{
		Keywords:       []string{"ai"},
		MinTitleRunes:  1,
		MinSnippetRunes: 1,
	})
	result, err := svc.Filter(context.Background(), content.SourceItem{
		Title:   "AI 新闻",
		Snippet: "正文片段",
	})
	if err != nil {
		t.Fatalf("filter failed: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("expected case-insensitive match to pass, got %+v", result)
	}
}
