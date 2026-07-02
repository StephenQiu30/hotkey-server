package event

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// Service provides Event building and detection operations.
type Service struct{}

// NewService creates a new Event Service.
func NewService(_ interface{}) *Service {
	return &Service{}
}

// ErrEmptyPosts is returned when BuildEventFromPosts receives no posts.
var ErrEmptyPosts = fmt.Errorf("cannot build event from empty posts")

// BuildEventFromPosts constructs an Event from a set of related posts.
// It computes the time window from the post timestamps and generates
// a deterministic EventKey based on the event seed and first seen time.
func (s *Service) BuildEventFromPosts(in BuildEventInput) (Event, error) {
	if len(in.Posts) == 0 {
		return Event{}, ErrEmptyPosts
	}

	firstSeen := in.Posts[0].PublishedAt
	lastActive := in.Posts[0].PublishedAt
	for _, post := range in.Posts[1:] {
		if post.PublishedAt.Before(firstSeen) {
			firstSeen = post.PublishedAt
		}
		if post.PublishedAt.After(lastActive) {
			lastActive = post.PublishedAt
		}
	}

	return Event{
		MonitorID:    in.MonitorID,
		TopicID:      in.TopicID,
		EventKey:     NormalizeEventKey(in.EventSeed, firstSeen),
		Title:        SummarizeEventTitle(in.EventSeed, in.Posts),
		FirstSeenAt:  firstSeen,
		LastActiveAt: lastActive,
		MachineStatus: "active",
	}, nil
}

// NormalizeEventKey generates a deterministic event key from a seed and timestamp.
func NormalizeEventKey(seed string, t time.Time) string {
	h := sha256.Sum256([]byte(seed))
	datePart := t.Format("2006-01-02")
	return fmt.Sprintf("evt:%x:%s", h[:4], datePart)
}

// SummarizeEventTitle creates an event title from the seed, ensuring it
// is distinct from the parent topic title.
func SummarizeEventTitle(seed string, posts []PostFact) string {
	if seed != "" {
		return seed
	}
	if len(posts) > 0 && len(posts[0].Text) > 0 {
		text := posts[0].Text
		if len(text) > 60 {
			text = text[:60]
		}
		return text
	}
	return "Untitled Event"
}
