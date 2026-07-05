package obsidian

import (
	"fmt"
	"strings"
)

// ThemeNoteInput holds all data needed to render an Obsidian theme note.
type ThemeNoteInput struct {
	ThemeID       int64
	Title         string
	Summary       string
	RelatedTopics []string
	EventCount    int
}

// RenderThemeNote generates an Obsidian-compatible Markdown note for a theme.
func RenderThemeNote(in ThemeNoteInput) string {
	var b strings.Builder

	b.WriteString("---\n")
	fmt.Fprintf(&b, "type: hotkey-theme\n")
	fmt.Fprintf(&b, "theme_id: %d\n", in.ThemeID)
	if in.Title != "" {
		fmt.Fprintf(&b, "title: %s\n", in.Title)
	}
	if len(in.RelatedTopics) > 0 {
		b.WriteString("related_topics:\n")
		for _, t := range in.RelatedTopics {
			fmt.Fprintf(&b, "  - %s\n", t)
		}
	}
	fmt.Fprintf(&b, "event_count: %d\n", in.EventCount)
	b.WriteString("tags:\n")
	b.WriteString("  - hotkey\n")
	b.WriteString("  - theme\n")
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n\n", in.Title)
	if in.Summary != "" {
		b.WriteString(in.Summary)
		b.WriteString("\n\n")
	}

	if len(in.RelatedTopics) > 0 {
		b.WriteString("## 相关主题\n\n")
		for _, t := range in.RelatedTopics {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n")
	fmt.Fprintf(&b, "> 事件数: %d\n", in.EventCount)

	return b.String()
}
