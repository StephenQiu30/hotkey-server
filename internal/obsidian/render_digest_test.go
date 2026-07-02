package obsidian

import (
	"strings"
	"testing"
)

func TestRenderDigestNote_FrontmatterType(t *testing.T) {
	got := RenderDigestNote(DigestNoteInput{
		Date:    "2026-07-01",
		Monitor: "AI监管",
	})
	if !strings.Contains(got, "type: hotkey-digest") {
		t.Fatal("missing digest type")
	}
}

func TestRenderDigestNote_FrontmatterFields(t *testing.T) {
	in := DigestNoteInput{
		DigestID:   1001,
		Date:       "2026-07-01",
		Monitor:    "AI监管",
		MonitorID:  1,
		TopicCount: 5,
		EventCount: 3,
		Summary:    "今日热点汇总",
	}
	got := RenderDigestNote(in)

	checks := []string{
		"digest_id: 1001",
		"date: 2026-07-01",
		"monitor: AI监管",
		"monitor_id: 1",
		"topic_count: 5",
		"event_count: 3",
	}
	for _, c := range checks {
		if !strings.Contains(got, c) {
			t.Errorf("frontmatter missing %q", c)
		}
	}
}

func TestRenderDigestNote_BodyContent(t *testing.T) {
	got := RenderDigestNote(DigestNoteInput{
		Date:       "2026-07-01",
		Monitor:    "AI监管",
		TopicCount: 5,
		EventCount: 3,
		Topics: []DigestTopicItem{
			{Title: "政策动态", Summary: "摘要内容"},
		},
		Summary: "今日热点汇总",
	})
	if !strings.Contains(got, "## 热点主题") {
		t.Error("body missing topic section heading")
	}
	if !strings.Contains(got, "政策动态") {
		t.Error("body missing topic title")
	}
}

func TestRenderDigestNote_EventsSection(t *testing.T) {
	got := RenderDigestNote(DigestNoteInput{
		Date:    "2026-07-01",
		Monitor: "AI监管",
		Events: []DigestEventItem{
			{Title: "重要事件", Summary: "事件描述"},
		},
	})
	if !strings.Contains(got, "## 重要事件") {
		t.Error("body missing events section heading")
	}
	if !strings.Contains(got, "事件描述") {
		t.Error("body missing event summary")
	}
}

func TestRenderDigestNote_Tags(t *testing.T) {
	got := RenderDigestNote(DigestNoteInput{
		Date:    "2026-07-01",
		Monitor: "AI监管",
	})
	for _, tag := range []string{"- hotkey", "- digest", "- daily"} {
		if !strings.Contains(got, tag) {
			t.Errorf("tags missing %q", tag)
		}
	}
}

func TestRenderDigestNote_FrontmatterDelimiters(t *testing.T) {
	got := RenderDigestNote(DigestNoteInput{
		Date:    "2026-07-01",
		Monitor: "test",
	})
	if !strings.HasPrefix(got, "---\n") {
		t.Error("note must start with ---\\n")
	}
	if !strings.Contains(got, "\n---\n") {
		t.Error("note must contain closing --- delimiter")
	}
}
