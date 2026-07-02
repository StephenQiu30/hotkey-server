package topic

import "context"

// TopicEventLinker provides an interface for linking events to topics.
type TopicEventLinker interface {
	LinkEvent(ctx context.Context, topicID, eventID int64) error
}
