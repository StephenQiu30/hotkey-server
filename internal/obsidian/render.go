package obsidian

import (
	"fmt"
	"path/filepath"
	"strings"
)

// TopicNoteInput holds all data needed to render an Obsidian topic note.
type TopicNoteInput struct {
	Date      string  // CST date, e.g. "2026-06-14"
	Monitor   string  // monitor display name
	MonitorID int64
	TopicID   int64
	TopicKey  string
	Title     string
	Heat      float64
	Trend     string // "rising" / "falling" / "stable"
	PostCount int
	Summary   string // LLM-generated summary
	Posts     []PostExcerpt
}

// PostExcerpt represents a representative post for the note body.
type PostExcerpt struct {
	Author  string
	Excerpt string
	URL     string
}

// RenderTopicNote generates an Obsidian-compatible Markdown note with YAML frontmatter.
func RenderTopicNote(in TopicNoteInput) string {
	var b strings.Builder

	// Frontmatter
	b.WriteString("---\n")
	fmt.Fprintf(&b, "type: hotkey-topic\n")
	fmt.Fprintf(&b, "date: %s\n", in.Date)
	fmt.Fprintf(&b, "monitor: %s\n", in.Monitor)
	fmt.Fprintf(&b, "monitor_id: %d\n", in.MonitorID)
	fmt.Fprintf(&b, "topic_id: %d\n", in.TopicID)
	fmt.Fprintf(&b, "topic_key: %q\n", in.TopicKey)
	fmt.Fprintf(&b, "heat: %g\n", in.Heat)
	fmt.Fprintf(&b, "trend: %s\n", in.Trend)
	fmt.Fprintf(&b, "post_count: %d\n", in.PostCount)

	// Tags
	monitorSlug := Slugify(in.Monitor)
	b.WriteString("tags:\n")
	b.WriteString("  - hotkey\n")
	b.WriteString("  - topic\n")
	fmt.Fprintf(&b, "  - monitor/%s\n", monitorSlug)

	b.WriteString("---\n\n")

	// Body: summary
	if in.Summary != "" {
		b.WriteString(in.Summary)
		b.WriteString("\n\n")
	}

	// Body: representative posts
	if len(in.Posts) > 0 {
		b.WriteString("## 代表帖\n\n")
		for i, p := range in.Posts {
			fmt.Fprintf(&b, "%d. **%s** — %s\n", i+1, p.Author, p.Excerpt)
			fmt.Fprintf(&b, "   [链接](%s)\n\n", p.URL)
		}
	}

	// Body: data footnote
	b.WriteString("---\n")
	fmt.Fprintf(&b, "> 热度: %g | 趋势: %s | 帖子数: %d\n", in.Heat, in.Trend, in.PostCount)

	return b.String()
}

// BuildPath constructs the full file path for a topic note inside the vault.
// Format: {vaultRoot}/HotKey/topics/{monitorSlug}/{date}-topic-{id}-{titleSlug}.md
func BuildPath(vaultRoot, monitorSlug, date, id, titleSlug string) string {
	filename := fmt.Sprintf("%s-topic-%s-%s.md", date, id, titleSlug)
	return filepath.Join(vaultRoot, "HotKey", "topics", monitorSlug, filename)
}
