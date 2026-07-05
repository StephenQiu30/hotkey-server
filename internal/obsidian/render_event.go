package obsidian

import (
	"fmt"
	"strings"
)

// EventNoteInput holds all data needed to render an Obsidian event note.
type EventNoteInput struct {
	EventID  int64
	EventKey string
	Title    string
	Date     string
	Summary  string
	TopicIDs []int64
}

// RenderEventNote generates an Obsidian-compatible Markdown note for an event.
func RenderEventNote(in EventNoteInput) string {
	var b strings.Builder

	b.WriteString("---\n")
	fmt.Fprintf(&b, "type: hotkey-event\n")
	fmt.Fprintf(&b, "event_id: %d\n", in.EventID)
	fmt.Fprintf(&b, "event_key: %q\n", in.EventKey)
	fmt.Fprintf(&b, "date: %s\n", in.Date)
	if len(in.TopicIDs) > 0 {
		fmt.Fprintf(&b, "topic_ids: [")
		for i, id := range in.TopicIDs {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%d", id)
		}
		b.WriteString("]\n")
	}
	b.WriteString("tags:\n")
	b.WriteString("  - hotkey\n")
	b.WriteString("  - event\n")
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n\n", in.Title)
	if in.Summary != "" {
		b.WriteString(in.Summary)
		b.WriteString("\n")
	}

	return b.String()
}
