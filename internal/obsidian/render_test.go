package obsidian

import (
	"strings"
	"testing"
)

func TestRenderTopicNote_FrontmatterFields(t *testing.T) {
	in := TopicNoteInput{
		Date:      "2026-06-14",
		Monitor:   "AI监管",
		MonitorID: 1,
		TopicID:   42,
		TopicKey:  "ai:监管:政策",
		Title:     "AI 监管政策动态",
		Heat:      85.4,
		Trend:     "rising",
		PostCount: 12,
		Summary:   "这是摘要。",
	}
	got := RenderTopicNote(in)

	required := []string{
		"type: hotkey-topic",
		"date: 2026-06-14",
		"monitor: AI监管",
		"monitor_id: 1",
		"topic_id: 42",
		"topic_key: \"ai:监管:政策\"",
		"heat: 85.4",
		"trend: rising",
		"post_count: 12",
	}
	for _, field := range required {
		if !strings.Contains(got, field) {
			t.Errorf("frontmatter missing %q\ngot:\n%s", field, got)
		}
	}

	// tags must contain hotkey, topic, monitor/<slug>
	if !strings.Contains(got, "tags:") {
		t.Error("frontmatter missing tags:")
	}
	if !strings.Contains(got, "- hotkey") {
		t.Error("tags missing - hotkey")
	}
	if !strings.Contains(got, "- topic") {
		t.Error("tags missing - topic")
	}
}

func TestRenderTopicNote_FrontmatterDelimiters(t *testing.T) {
	in := TopicNoteInput{
		Date:      "2026-06-14",
		Monitor:   "test",
		MonitorID: 1,
		TopicID:   1,
		TopicKey:  "test",
		Title:     "Test",
	}
	got := RenderTopicNote(in)
	if !strings.HasPrefix(got, "---\n") {
		t.Error("note must start with ---\\n")
	}
	if !strings.Contains(got, "\n---\n") {
		t.Error("note must contain closing --- delimiter")
	}
}

func TestRenderTopicNote_BodyContainsSummary(t *testing.T) {
	in := TopicNoteInput{
		Date:      "2026-06-14",
		Monitor:   "test",
		MonitorID: 1,
		TopicID:   1,
		TopicKey:  "test",
		Title:     "Test",
		Summary:   "这是一段摘要内容。",
	}
	got := RenderTopicNote(in)
	if !strings.Contains(got, "这是一段摘要内容。") {
		t.Error("body must contain summary text")
	}
}

func TestRenderTopicNote_BodyContainsPosts(t *testing.T) {
	in := TopicNoteInput{
		Date:      "2026-06-14",
		Monitor:   "test",
		MonitorID: 1,
		TopicID:   1,
		TopicKey:  "test",
		Title:     "Test",
		Posts: []PostExcerpt{
			{Author: "user1", Excerpt: "帖子内容", URL: "https://x.com/user1/status/1"},
		},
	}
	got := RenderTopicNote(in)
	if !strings.Contains(got, "user1") {
		t.Error("body must contain post author")
	}
	if !strings.Contains(got, "帖子内容") {
		t.Error("body must contain post excerpt")
	}
	if !strings.Contains(got, "https://x.com/user1/status/1") {
		t.Error("body must contain post URL")
	}
}

func TestRenderTopicNote_DataFootnote(t *testing.T) {
	in := TopicNoteInput{
		Date:      "2026-06-14",
		Monitor:   "test",
		MonitorID: 1,
		TopicID:   1,
		TopicKey:  "test",
		Title:     "Test",
		Heat:      90.0,
		Trend:     "rising",
		PostCount: 5,
	}
	got := RenderTopicNote(in)
	if !strings.Contains(got, "90") {
		t.Error("footnote must contain heat value")
	}
	if !strings.Contains(got, "rising") {
		t.Error("footnote must contain trend")
	}
}

func TestBuildPath_Format(t *testing.T) {
	got := BuildPath("/vault", "ai-监管", "2026-06-14", "42", "ai-监管-政策")
	want := "/vault/HotKey/topics/ai-监管/2026-06-14-topic-42-ai-监管-政策.md"
	if got != want {
		t.Fatalf("BuildPath = %q, want %q", got, want)
	}
}
