package obsidian

import (
	"strings"
	"testing"
)

func TestRenderEventNote_FrontmatterType(t *testing.T) {
	got := RenderEventNote(EventNoteInput{
		EventID:   101,
		EventKey:  "evt:ai-regulation:2026-07-01",
		Title:     "AI 监管规则发布",
		Date:      "2026-07-01",
		Summary:   "监管机构发布新规。",
		TopicIDs:  []int64{42},
	})
	if !strings.Contains(got, "type: hotkey-event") {
		t.Fatal("missing event type")
	}
}

func TestRenderEventNote_FrontmatterFields(t *testing.T) {
	in := EventNoteInput{
		EventID:   101,
		EventKey:  "evt:ai",
		Title:     "AI 事件",
		Date:      "2026-07-01",
		Summary:   "摘要",
		TopicIDs:  []int64{42, 43},
	}
	got := RenderEventNote(in)

	checks := []string{
		"event_id: 101",
		"event_key: \"evt:ai\"",
		"date: 2026-07-01",
	}
	for _, c := range checks {
		if !strings.Contains(got, c) {
			t.Errorf("frontmatter missing %q", c)
		}
	}
}

func TestRenderEventNote_BodyContent(t *testing.T) {
	got := RenderEventNote(EventNoteInput{
		EventID:  101,
		EventKey: "evt:ai",
		Title:    "AI 事件",
		Date:     "2026-07-01",
		Summary:  "重要事件摘要内容",
	})
	if !strings.Contains(got, "# AI 事件") {
		t.Error("body missing title heading")
	}
	if !strings.Contains(got, "重要事件摘要内容") {
		t.Error("body missing summary")
	}
}

func TestRenderEventNote_Tags(t *testing.T) {
	got := RenderEventNote(EventNoteInput{
		EventID:   101,
		EventKey:  "evt:ai",
		Title:     "test",
		Date:      "2026-07-01",
		TopicIDs:  []int64{42},
	})
	for _, tag := range []string{"- hotkey", "- event"} {
		if !strings.Contains(got, tag) {
			t.Errorf("tags missing %q", tag)
		}
	}
}

func TestRenderEventNote_FrontmatterDelimiters(t *testing.T) {
	got := RenderEventNote(EventNoteInput{
		EventID:  1,
		EventKey: "test",
		Title:    "Test",
		Date:     "2026-07-01",
	})
	if !strings.HasPrefix(got, "---\n") {
		t.Error("note must start with ---\\n")
	}
	if !strings.Contains(got, "\n---\n") {
		t.Error("note must contain closing --- delimiter")
	}
}
