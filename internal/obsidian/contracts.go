package obsidian

import (
	"crypto/sha256"
	"fmt"
)

// Contract represents a knowledge object contract with frontmatter.
type Contract struct {
	Frontmatter map[string]any
}

// EventContractInput holds parameters for building an event contract.
type EventContractInput struct {
	EventID  int64
	EventKey string
	Title    string
	TopicIDs []int64
	Date     string
}

// BuildEventContract constructs a Contract for an Event knowledge object.
func BuildEventContract(in EventContractInput) Contract {
	return Contract{
		Frontmatter: map[string]any{
			"type":     "hotkey-event",
			"event_id": in.EventID,
			"event_key": in.EventKey,
			"topic_ids": in.TopicIDs,
			"date":     in.Date,
		},
	}
}

// BuildRevision generates a content-based revision string using SHA-256.
// Format: {objectType}:{objectID}:{hex(sha256(content)[:8])}
func BuildRevision(objectType string, objectID int64, content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%s:%d:%x", objectType, objectID, sum[:8])
}
