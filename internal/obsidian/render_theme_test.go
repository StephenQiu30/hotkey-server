package obsidian

import (
	"strings"
	"testing"
)

func TestRenderThemeNote_FrontmatterType(t *testing.T) {
	got := RenderThemeNote(ThemeNoteInput{
		ThemeID: 42,
		Title:   "AI安全监管",
	})
	if !strings.Contains(got, "type: hotkey-theme") {
		t.Fatal("missing theme type")
	}
}

func TestRenderThemeNote_FrontmatterFields(t *testing.T) {
	in := ThemeNoteInput{
		ThemeID:       42,
		Title:         "AI安全监管",
		Summary:       "AI安全相关的综合监管专题",
		RelatedTopics: []string{"AI政策", "数据安全"},
		EventCount:    5,
	}
	got := RenderThemeNote(in)

	checks := []string{
		"theme_id: 42",
		"title: AI安全监管",
	}
	for _, c := range checks {
		if !strings.Contains(got, c) {
			t.Errorf("frontmatter missing %q", c)
		}
	}
}

func TestRenderThemeNote_BodyContent(t *testing.T) {
	got := RenderThemeNote(ThemeNoteInput{
		ThemeID:       42,
		Title:         "AI安全监管",
		Summary:       "AI安全相关的综合监管专题",
		RelatedTopics: []string{"AI政策", "数据安全"},
		EventCount:    5,
	})
	if !strings.Contains(got, "# AI安全监管") {
		t.Error("body missing title heading")
	}
	if !strings.Contains(got, "AI安全相关的综合监管专题") {
		t.Error("body missing summary")
	}
	if !strings.Contains(got, "AI政策") {
		t.Error("body missing related topic")
	}
	if !strings.Contains(got, "事件数: 5") {
		t.Error("body missing event count")
	}
}

func TestRenderThemeNote_Tags(t *testing.T) {
	got := RenderThemeNote(ThemeNoteInput{
		ThemeID: 42,
		Title:   "test",
	})
	for _, tag := range []string{"- hotkey", "- theme"} {
		if !strings.Contains(got, tag) {
			t.Errorf("tags missing %q", tag)
		}
	}
}

func TestRenderThemeNote_FrontmatterDelimiters(t *testing.T) {
	got := RenderThemeNote(ThemeNoteInput{
		ThemeID: 1,
		Title:   "Test",
	})
	if !strings.HasPrefix(got, "---\n") {
		t.Error("note must start with ---\\n")
	}
	if !strings.Contains(got, "\n---\n") {
		t.Error("note must contain closing --- delimiter")
	}
}
