// Package obsidian provides Obsidian vault note writing.
// This is a stub implementation; full logic will be added in STE-306.
package obsidian

import (
	"context"
	"fmt"
)

// NoteInput holds the data needed to write an Obsidian topic note.
type NoteInput struct {
	VaultPath   string
	MonitorID   int64
	MonitorName string
	MonitorSlug string
	TopicID     int64
	TopicKey    string
	Title       string
	Date        string
	HeatScore   float64
	Trend       string
	PostCount   int
	Summary     string
	Posts       []Post
}

// Post represents a representative post in a note.
type Post struct {
	AuthorName string
	Text       string
	URL        string
}

// Writer writes notes to an Obsidian vault.
type Writer struct{}

// NewWriter creates an Obsidian writer.
func NewWriter() *Writer {
	return &Writer{}
}

// WriteTopicNote writes a topic note to the vault and returns the file path.
// TODO(STE-306): implement real Obsidian write.
func (w *Writer) WriteTopicNote(_ context.Context, _ NoteInput) (string, error) {
	return "", fmt.Errorf("obsidian: not implemented (stub from STE-306)")
}
