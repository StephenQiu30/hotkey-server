package obsidian

import (
	"fmt"
	"strings"
)

// DigestNoteInput holds all data needed to render an Obsidian daily digest note.
type DigestNoteInput struct {
	DigestID   int64
	Date       string
	Monitor    string
	MonitorID  int64
	TopicCount int
	EventCount int
	Topics     []DigestTopicItem
	Events     []DigestEventItem
	Summary    string
}

// DigestTopicItem represents a topic within a daily digest.
type DigestTopicItem struct {
	TopicID   int64
	Title     string
	Summary   string
	HeatScore float64
}

// DigestEventItem represents an event within a daily digest.
type DigestEventItem struct {
	EventID int64
	Title   string
	Summary string
}

// RenderDigestNote generates an Obsidian-compatible Markdown note for a daily digest.
func RenderDigestNote(in DigestNoteInput) string {
	var b strings.Builder

	b.WriteString("---\n")
	fmt.Fprintf(&b, "type: hotkey-digest\n")
	if in.DigestID > 0 {
		fmt.Fprintf(&b, "digest_id: %d\n", in.DigestID)
	}
	fmt.Fprintf(&b, "date: %s\n", in.Date)
	if in.Monitor != "" {
		fmt.Fprintf(&b, "monitor: %s\n", in.Monitor)
	}
	if in.MonitorID > 0 {
		fmt.Fprintf(&b, "monitor_id: %d\n", in.MonitorID)
	}
	fmt.Fprintf(&b, "topic_count: %d\n", in.TopicCount)
	fmt.Fprintf(&b, "event_count: %d\n", in.EventCount)
	b.WriteString("tags:\n")
	b.WriteString("  - hotkey\n")
	b.WriteString("  - digest\n")
	b.WriteString("  - daily\n")
	b.WriteString("---\n\n")

	if in.Summary != "" {
		b.WriteString(in.Summary)
		b.WriteString("\n\n")
	}

	if len(in.Topics) > 0 {
		b.WriteString("## 热点主题\n\n")
		for _, t := range in.Topics {
			if t.Title != "" {
				fmt.Fprintf(&b, "- **%s**", t.Title)
				if t.HeatScore > 0 {
					fmt.Fprintf(&b, " (热度: %.1f)", t.HeatScore)
				}
				b.WriteString("\n")
			}
			if t.Summary != "" {
				fmt.Fprintf(&b, "  - %s\n", t.Summary)
			}
		}
		b.WriteString("\n")
	}

	if len(in.Events) > 0 {
		b.WriteString("## 重要事件\n\n")
		for _, e := range in.Events {
			if e.Title != "" {
				fmt.Fprintf(&b, "- **%s**", e.Title)
				b.WriteString("\n")
			}
			if e.Summary != "" {
				fmt.Fprintf(&b, "  - %s\n", e.Summary)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n")
	fmt.Fprintf(&b, "> 热点主题: %d | 重要事件: %d\n", in.TopicCount, in.EventCount)

	return b.String()
}
