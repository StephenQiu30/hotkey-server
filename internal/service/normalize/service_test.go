package normalize

import (
	"context"
	"testing"
	"time"
	"unicode/utf8"
)

func TestNormalizeCleansHTMLAndWhitespace(t *testing.T) {
	svc := NewService(DefaultConfig())
	result, err := svc.Normalize(context.Background(), Input{
		SourceID:   "src-1",
		Title:      "  <b>AI</b>  新闻  ",
		Snippet:    "<p>正文\n\n片段</p>",
		RawContent: "<div>完整正文内容</div>",
		URL:        "https://example.com/a",
		Platform:   "rss",
		Language:   "zh",
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if result.Item.Title == "" || result.Item.Title == "  <b>AI</b>  新闻  " {
		t.Fatalf("expected cleaned title, got %q", result.Item.Title)
	}
	if result.Item.Snippet == "" || result.Item.Snippet == "<p>正文\n\n片段</p>" {
		t.Fatalf("expected cleaned snippet, got %q", result.Item.Snippet)
	}
}

func TestNormalizeDetectsLanguage(t *testing.T) {
	svc := NewService(DefaultConfig())
	tests := []struct {
		name     string
		title    string
		snippet  string
		inputLang string
		wantLang string
	}{
		{name: "chinese content", title: "人工智能最新进展", snippet: "深度学习技术突破", inputLang: "", wantLang: "zh"},
		{name: "english content", title: "Latest AI breakthroughs", snippet: "Deep learning advances", inputLang: "", wantLang: "en"},
		{name: "explicit language preserved", title: "AI News", snippet: "Some content", inputLang: "ja", wantLang: "ja"},
		{name: "unknown fallback", title: "123", snippet: "456", inputLang: "", wantLang: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.Normalize(context.Background(), Input{
				SourceID: "src-1",
				Title:    tt.title,
				Snippet:  tt.snippet,
				URL:      "https://example.com/a",
				Platform: "rss",
				Language: tt.inputLang,
			})
			if err != nil {
				t.Fatalf("normalize failed: %v", err)
			}
			if result.Item.Language != tt.wantLang {
				t.Fatalf("expected language %q, got %q", tt.wantLang, result.Item.Language)
			}
		})
	}
}

func TestNormalizeTruncatesToConfigLimits(t *testing.T) {
	cfg := Config{
		MaxTitleRunes:   10,
		MaxSnippetRunes: 10,
		MaxContentRunes: 20,
		DefaultLanguage: "unknown",
	}
	svc := NewService(cfg)
	longTitle := "这是一段超过十个字符的标题内容"
	longSnippet := "这是一段超过十个字符的摘要内容"
	longContent := "这是一段超过二十个字符的完整正文内容用于测试截断"

	result, err := svc.Normalize(context.Background(), Input{
		SourceID:   "src-1",
		Title:      longTitle,
		Snippet:    longSnippet,
		RawContent: longContent,
		URL:        "https://example.com/a",
		Platform:   "web",
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if utf8.RuneCountInString(result.Item.Title) > 10 {
		t.Fatalf("title not truncated: %d runes", utf8.RuneCountInString(result.Item.Title))
	}
	if utf8.RuneCountInString(result.Item.Snippet) > 10 {
		t.Fatalf("snippet not truncated: %d runes", utf8.RuneCountInString(result.Item.Snippet))
	}
}

func TestNormalizeRejectsEmptyAfterClean(t *testing.T) {
	svc := NewService(DefaultConfig())
	_, err := svc.Normalize(context.Background(), Input{
		SourceID: "src-1",
		Title:    "",
		Snippet:  "",
		URL:      "https://example.com/a",
		Platform: "rss",
	})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestNormalizeRejectsInvalidURL(t *testing.T) {
	svc := NewService(DefaultConfig())
	_, err := svc.Normalize(context.Background(), Input{
		SourceID: "src-1",
		Title:    "AI 新闻",
		Snippet:  "正文片段",
		URL:      "not a url",
		Platform: "rss",
	})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestNormalizeSetsTimestamps(t *testing.T) {
	fixed := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	svc := NewService(DefaultConfig())
	svc.now = func() time.Time { return fixed }

	result, err := svc.Normalize(context.Background(), Input{
		SourceID: "src-1",
		Title:    "AI 新闻",
		Snippet:  "正文片段",
		URL:      "https://example.com/a",
		Platform: "rss",
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if !result.Item.CreatedAt.Equal(fixed) {
		t.Fatalf("expected created at %v, got %v", fixed, result.Item.CreatedAt)
	}
	if !result.Item.UpdatedAt.Equal(fixed) {
		t.Fatalf("expected updated at %v, got %v", fixed, result.Item.UpdatedAt)
	}
}

func TestNormalizePreservesPublishedAt(t *testing.T) {
	published := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	svc := NewService(DefaultConfig())
	result, err := svc.Normalize(context.Background(), Input{
		SourceID:    "src-1",
		Title:       "AI 新闻",
		Snippet:     "正文片段",
		URL:         "https://example.com/a",
		Platform:    "rss",
		PublishedAt: &published,
	})
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if result.Item.PublishedAt == nil || !result.Item.PublishedAt.Equal(published) {
		t.Fatalf("expected published at %v, got %v", published, result.Item.PublishedAt)
	}
}

func TestCleanTextStripsHTMLAndNormalizesWhitespace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "<b>hello</b> world", want: "hello world"},
		{input: "  multiple   spaces  ", want: "multiple spaces"},
		{"line1\n\n\nline2", "line1 line2"},
		{"<p>&amp; &lt;test&gt;</p>", "& <test>"},
		{"<script>alert('xss')</script>safe", "safe"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanText(tt.input)
			if got != tt.want {
				t.Fatalf("cleanText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectLanguageIdentifiesChinese(t *testing.T) {
	if lang := detectLanguage("人工智能最新进展"); lang != "zh" {
		t.Fatalf("expected zh, got %q", lang)
	}
}

func TestDetectLanguageIdentifiesEnglish(t *testing.T) {
	if lang := detectLanguage("Latest AI breakthroughs in machine learning"); lang != "en" {
		t.Fatalf("expected en, got %q", lang)
	}
}

func TestDetectLanguageReturnsUnknownForNumbers(t *testing.T) {
	if lang := detectLanguage("123 456 789"); lang != "unknown" {
		t.Fatalf("expected unknown, got %q", lang)
	}
}
