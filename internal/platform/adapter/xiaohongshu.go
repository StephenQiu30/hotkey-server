package adapter

import (
	"fmt"
	"strings"
	"time"
)

// XiaohongshuNote represents a raw Xiaohongshu (Little Red Book) note.
type XiaohongshuNote struct {
	NoteID      string    `json:"note_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Author      string    `json:"author"`
	AuthorID    string    `json:"author_id"`
	PublishedAt time.Time `json:"published_at"`
	URL         string    `json:"url"`
	Tags        []string  `json:"tags"`
	Likes       int       `json:"likes"`
	Collects    int       `json:"collects"`
	Comments    int       `json:"comments"`
	Shares      int       `json:"shares"`
	Visible     bool      `json:"visible"`
}

// NormalizeXiaohongshuNote converts a XiaohongshuNote into a NormalizedItem.
func NormalizeXiaohongshuNote(note XiaohongshuNote, sourceID string) NormalizedItem {
	snippet := buildSnippet(note)

	return NormalizedItem{
		Title:        strings.TrimSpace(note.Title),
		URL:          strings.TrimSpace(note.URL),
		Snippet:      snippet,
		ExternalID:   strings.TrimSpace(note.NoteID),
		PublishedAt:  &note.PublishedAt,
		Language:     "zh",
		IdempotencyKey: NewIdempotencyKey(sourceID, note.URL),
	}
}

// buildSnippet constructs the snippet from description, tags, and engagement metrics.
func buildSnippet(note XiaohongshuNote) string {
	var parts []string

	if desc := strings.TrimSpace(note.Description); desc != "" {
		parts = append(parts, desc)
	}

	if len(note.Tags) > 0 {
		tags := make([]string, 0, len(note.Tags))
		for _, tag := range note.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, "#"+tag)
			}
		}
		if len(tags) > 0 {
			parts = append(parts, strings.Join(tags, " "))
		}
	}

	if note.Likes > 0 || note.Collects > 0 || note.Comments > 0 {
		metrics := fmt.Sprintf("赞:%d 藏:%d 评:%d", note.Likes, note.Collects, note.Comments)
		parts = append(parts, metrics)
	}

	return strings.Join(parts, " ")
}

// ValidateXiaohongshuNote checks for conditions that should produce adapter errors.
// Returns:
//   - FailureClassPermanent if the note is not visible
//   - FailureClassParseError if required fields are missing (schema change signal)
//   - nil if the note is valid
func ValidateXiaohongshuNote(note XiaohongshuNote) error {
	if !note.Visible {
		return NewAdapterError(FailureClassPermanent, "note not visible: content removed or private", nil)
	}

	if note.NoteID == "" {
		return NewAdapterError(FailureClassParseError, "schema change detected: missing note_id", nil)
	}

	if note.Title == "" && note.Description == "" {
		return NewAdapterError(FailureClassParseError, "schema change detected: missing both title and description", nil)
	}

	return nil
}
